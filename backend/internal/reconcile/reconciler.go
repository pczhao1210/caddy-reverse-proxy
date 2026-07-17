package reconcile

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/aidockerfarm/gateway/internal/model"
	"github.com/aidockerfarm/gateway/internal/routes"
)

type Renderer interface {
	Render([]model.RouteConfig) ([]byte, error)
}

type CertificateUpdater interface {
	UpdateCertificate(model.CertificateConfig)
}

type ConfigUpdater interface {
	UpdateConfig(model.AppConfig)
}

type Loader interface {
	Load(context.Context, []byte) error
}

type Discoverer interface {
	Discover(context.Context) ([]model.ContainerService, []model.RouteConfig, error)
}

type AzureManager interface {
	Reconcile(context.Context, []model.RouteConfig) model.AzureResult
}

type HealthChecker interface {
	Check(context.Context, []model.RouteConfig) []model.RouteHealthStatus
}

type AuditLogger interface {
	Record(context.Context, string, map[string]any) error
}

type Options struct {
	Config        model.AppConfig
	Store         *routes.Store
	Discoverer    Discoverer
	AzureManager  AzureManager
	HealthChecker HealthChecker
	AuditLogger   AuditLogger
	Renderer      Renderer
	Loader        Loader
	Logger        *slog.Logger
}

type Reconciler struct {
	cfg            model.AppConfig
	store          *routes.Store
	discoverer     Discoverer
	azureManager   AzureManager
	healthChecker  HealthChecker
	auditLogger    AuditLogger
	renderer       Renderer
	loader         Loader
	logger         *slog.Logger
	syncMu         sync.Mutex
	lastDiscovered []model.RouteConfig
	mu             sync.RWMutex
	last           model.ReconcileResult
}

func New(options Options) *Reconciler {
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Reconciler{cfg: options.Config, store: options.Store, discoverer: options.Discoverer, azureManager: options.AzureManager, healthChecker: options.HealthChecker, auditLogger: options.AuditLogger, renderer: options.Renderer, loader: options.Loader, logger: logger}
}

func (r *Reconciler) Run(ctx context.Context) {
	_ = r.Sync(ctx)
	interval := time.Duration(r.configSnapshot().ReconcileIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = r.Sync(ctx)
		}
	}
}

func (r *Reconciler) Sync(ctx context.Context) model.ReconcileResult {
	r.syncMu.Lock()
	defer r.syncMu.Unlock()

	started := time.Now().UTC()
	cfg := r.configSnapshot()
	explicitRoutes := r.store.List()
	result := model.ReconcileResult{StartedAt: started, Profile: string(cfg.Profile), ExplicitRoutes: len(explicitRoutes)}

	allRoutes := append([]model.RouteConfig{}, explicitRoutes...)
	if r.discoverer != nil {
		_, discoveredRoutes, err := r.discoverer.Discover(ctx)
		if err != nil {
			discoveredRoutes = append([]model.RouteConfig{}, r.lastDiscovered...)
			r.logger.Warn("docker discovery failed; keeping last successful routes", "error", err, "routes", len(discoveredRoutes))
		} else {
			r.lastDiscovered = append([]model.RouteConfig{}, discoveredRoutes...)
		}
		result.DiscoveredRoutes = len(discoveredRoutes)
		allRoutes = append(allRoutes, discoveredRoutes...)
	}

	rendered, err := r.renderer.Render(allRoutes)
	if err != nil {
		result.Error = err.Error()
		return r.finish(result)
	}
	if err := r.loader.Load(ctx, rendered); err != nil {
		result.Error = err.Error()
		return r.finish(result)
	}

	result.AppliedRoutes = len(allRoutes)
	result.CaddyLoaded = true
	if r.healthChecker != nil {
		result.RouteHealth = r.healthChecker.Check(ctx, allRoutes)
		result.HealthChecks = len(result.RouteHealth)
		for _, status := range result.RouteHealth {
			if !status.Healthy {
				result.UnhealthyRoutes++
			}
		}
		r.store.SetRuntimeStatus(result.RouteHealth)
	}
	if r.azureManager != nil {
		result.Azure = r.azureRoutes(ctx, allRoutes)
		if result.Azure.Error != "" {
			result.Error = result.Azure.Error
			return r.finish(result)
		}
	}
	return r.finish(result)
}

func (r *Reconciler) azureRoutes(ctx context.Context, routes []model.RouteConfig) model.AzureResult {
	cfg := r.configSnapshot()
	output := append([]model.RouteConfig{}, routes...)
	if cfg.Control.ManagementHost != "" {
		output = append(output, model.RouteConfig{
			ID:        "management-ui",
			Host:      cfg.Control.ManagementHost,
			Exposure:  "protected",
			Enabled:   true,
			Public:    true,
			HTTPS:     true,
			Protected: true,
			Source:    "management",
		})
	}
	return r.azureManager.Reconcile(ctx, output)
}

func (r *Reconciler) UpdateCertificate(cert model.CertificateConfig) {
	r.mu.Lock()
	r.cfg.Gateway.Certificate = cert
	r.mu.Unlock()
	if updater, ok := r.renderer.(CertificateUpdater); ok {
		updater.UpdateCertificate(cert)
	}
}

func (r *Reconciler) UpdateConfig(cfg model.AppConfig) {
	r.mu.Lock()
	r.cfg = cfg
	r.mu.Unlock()
	if updater, ok := r.renderer.(ConfigUpdater); ok {
		updater.UpdateConfig(cfg)
	}
}

func (r *Reconciler) configSnapshot() model.AppConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cfg
}

func (r *Reconciler) Last() model.ReconcileResult {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.last
}

func (r *Reconciler) finish(result model.ReconcileResult) model.ReconcileResult {
	result.FinishedAt = time.Now().UTC()
	result.Duration = result.FinishedAt.Sub(result.StartedAt)
	r.mu.Lock()
	r.last = result
	r.mu.Unlock()
	if r.auditLogger != nil {
		if err := r.auditLogger.Record(context.Background(), "reconcile.complete", map[string]any{
			"profile":         result.Profile,
			"appliedRoutes":   result.AppliedRoutes,
			"healthChecks":    result.HealthChecks,
			"unhealthyRoutes": result.UnhealthyRoutes,
			"caddyLoaded":     result.CaddyLoaded,
			"dnsRecords":      result.Azure.DNSRecords,
			"dnsDeleted":      result.Azure.DNSDeleted,
			"nsgRules":        result.Azure.NSGRules,
			"nsgDeleted":      result.Azure.NSGDeleted,
			"error":           result.Error,
		}); err != nil {
			r.logger.Warn("write audit event failed", "error", err)
		}
	}
	if result.Error != "" {
		r.logger.Warn("reconcile failed", "error", result.Error)
		return result
	}
	r.logger.Info("reconcile complete", "routes", result.AppliedRoutes)
	return result
}
