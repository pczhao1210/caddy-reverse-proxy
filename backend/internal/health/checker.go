package health

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/aidockerfarm/gateway/internal/model"
)

const maxConcurrentChecks = 8
const checkBudget = 10 * time.Second

type Checker struct {
	cfg    model.HealthConfig
	client *http.Client
}

func NewChecker(cfg model.HealthConfig) *Checker {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Checker{cfg: cfg, client: &http.Client{Timeout: timeout}}
}

func (c *Checker) Check(ctx context.Context, routes []model.RouteConfig) []model.RouteHealthStatus {
	if c == nil || !c.cfg.Enabled {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, checkBudget)
	defer cancel()
	active := make([]model.RouteConfig, 0, len(routes))
	for _, route := range routes {
		if !route.Enabled || len(route.Upstreams) == 0 {
			continue
		}
		active = append(active, route)
	}
	statuses := make([]model.RouteHealthStatus, len(active))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for worker := 0; worker < min(maxConcurrentChecks, len(active)); worker++ {
		workers.Go(func() {
			for index := range jobs {
				route := active[index]
				status := model.RouteHealthStatus{RouteID: route.ID, Host: route.Host, Healthy: true, CheckedAt: time.Now().UTC()}
				for _, upstream := range route.Upstreams {
					if err := c.checkUpstream(ctx, upstream); err != nil {
						status.Healthy = false
						status.Error = fmt.Sprintf("%s: %v", upstream.Name, err)
						break
					}
				}
				statuses[index] = status
			}
		})
	}
	for index := range active {
		jobs <- index
	}
	close(jobs)
	workers.Wait()
	return statuses
}

func (c *Checker) checkUpstream(ctx context.Context, upstream model.UpstreamTarget) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	healthURL, err := c.healthURL(upstream)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 400 {
		return fmt.Errorf("health check returned %s", response.Status)
	}
	return nil
}

func (c *Checker) healthURL(upstream model.UpstreamTarget) (string, error) {
	parsed, err := url.Parse(upstream.URL)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("upstream URL %q must include scheme and host", upstream.URL)
	}
	healthPath := strings.TrimSpace(upstream.HealthPath)
	if healthPath == "" {
		healthPath = strings.TrimSpace(c.cfg.DefaultPath)
	}
	if healthPath == "" {
		healthPath = parsed.Path
	}
	if healthPath == "" {
		healthPath = "/"
	}
	if !strings.HasPrefix(healthPath, "/") {
		healthPath = "/" + healthPath
	}
	parsed.Path = healthPath
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
