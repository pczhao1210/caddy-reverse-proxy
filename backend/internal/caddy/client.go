package caddy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	endpoint string
	http     *http.Client
}

func NewClient(endpoint string) *Client {
	return &Client{endpoint: strings.TrimRight(endpoint, "/"), http: &http.Client{Timeout: 10 * time.Second}}
}

func (c *Client) Config(ctx context.Context) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/config/", nil)
	if err != nil {
		return nil, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("read active caddy config: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("read active caddy config returned %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 8*1024*1024 {
		return nil, fmt.Errorf("active caddy config exceeds size limit")
	}
	return data, nil
}

func (c *Client) Load(ctx context.Context, config []byte) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/load", bytes.NewReader(config))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("load caddy config: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("load caddy config returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	return nil
}
