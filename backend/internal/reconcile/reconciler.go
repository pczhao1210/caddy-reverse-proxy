package reconcile

import (
	"context"
	"fmt"
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
	azureMu        sync.Mutex
	lastDiscovered []model.RouteConfig
	mu             sync.RWMutex
	generation     uint64
	cancelChecks   context.CancelFunc
	last           model.ReconcileResult
}

func New(options Options) *Reconciler {
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	reconciler := &Reconciler{cfg: options.Config, store: options.Store, discoverer: options.Discoverer, azureManager: options.AzureManager, healthChecker: options.HealthChecker, auditLogger: options.AuditLogger, renderer: options.Renderer, loader: options.Loader, logger: logger}
	return reconciler
}

func (r *Reconciler) Run(ctx context.Context) {
	_ = r.SyncWithoutPendingRoutingChanges(ctx)
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
			_ = r.SyncWithoutPendingRoutingChanges(ctx)
		}
	}
}

func (r *Reconciler) RoutingChangesPending() bool {
	return r.store.Pending()
}

func (r *Reconciler) WithRuntimeConfig(ctx context.Context, inspect func([]byte) error) error {
	r.syncMu.Lock()
	defer r.syncMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	reader, ok := r.loader.(interface {
		Config(context.Context) ([]byte, error)
	})
	if !ok {
		return fmt.Errorf("active Caddy configuration is unavailable")
	}
	data, err := reader.Config(ctx)
	if err != nil {
		return err
	}
	return inspect(data)
}

func (r *Reconciler) Sync(ctx context.Context) model.ReconcileResult {
	return r.sync(ctx, false, nil, nil)
}

func (r *Reconciler) SyncWithoutPendingRoutingChanges(ctx context.Context) model.ReconcileResult {
	return r.sync(ctx, true, nil, nil)
}

func (r *Reconciler) SyncWithCommit(ctx context.Context, cfg *model.AppConfig, commit func(routes.Snapshot) error) model.ReconcileResult {
	return r.sync(ctx, false, cfg, commit)
}

func (r *Reconciler) sync(ctx context.Context, preservePendingRoutes bool, candidate *model.AppConfig, commit func(routes.Snapshot) error) model.ReconcileResult {
	r.syncMu.Lock()
	locked := true
	defer func() {
		if locked {
			r.syncMu.Unlock()
		}
	}()
	r.mu.Lock()
	if r.cancelChecks != nil {
		r.cancelChecks()
	}
	checksContext, cancel := context.WithCancel(ctx)
	defer cancel()
	r.cancelChecks = cancel
	r.generation++
	generation := r.generation
	r.mu.Unlock()

	started := time.Now().UTC()
	previousConfig := r.configSnapshot()
	previous := r.store.AppliedSnapshot()
	previousDiscovered := append([]model.RouteConfig(nil), r.lastDiscovered...)
	if candidate != nil {
		r.UpdateConfig(*candidate)
	}
	cfg := r.configSnapshot()
	snapshot := r.store.DesiredSnapshot()
	if preservePendingRoutes {
		snapshot = previous
	}
	explicitRoutes := snapshot.Routes
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
		if candidate != nil {
			r.UpdateConfig(previousConfig)
		}
		result.Error = err.Error()
		return r.finish(result, generation)
	}
	if err := r.loader.Load(ctx, rendered); err != nil {
		if candidate != nil {
			r.UpdateConfig(previousConfig)
		}
		result.Error = err.Error()
		return r.finish(result, generation)
	}
	result.AppliedRoutes = len(allRoutes)
	result.CaddyLoaded = true
	if !preservePendingRoutes {
		if commit == nil {
			commit = r.store.MarkApplied
		}
		if err := commit(snapshot); err != nil {
			if candidate != nil {
				r.UpdateConfig(previousConfig)
			}
			rollbackRoutes := append(previous.Routes, previousDiscovered...)
			rollback, rollbackErr := r.renderer.Render(rollbackRoutes)
			if rollbackErr == nil {
				rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
				rollbackErr = r.loader.Load(rollbackCtx, rollback)
				cancel()
			}
			result.Error = fmt.Sprintf("persist applied configuration: %v", err)
			if rollbackErr != nil {
				result.Error += fmt.Sprintf("; restore previous runtime configuration: %v", rollbackErr)
			} else {
				result.CaddyLoaded = false
				result.AppliedRoutes = len(rollbackRoutes)
			}
			return r.finish(result, generation)
		}
	}
	locked = false
	r.syncMu.Unlock()
	if r.healthChecker != nil {
		result.RouteHealth = r.healthChecker.Check(checksContext, allRoutes)
		result.HealthChecks = len(result.RouteHealth)
		for _, status := range result.RouteHealth {
			if !status.Healthy {
				result.UnhealthyRoutes++
			}
		}
	}
	if r.azureManager != nil {
		r.azureMu.Lock()
		if checksContext.Err() == nil {
			result.Azure = r.azureRoutes(checksContext, cfg, allRoutes)
		}
		r.azureMu.Unlock()
		if result.Azure.Error != "" {
			result.Error = result.Azure.Error
			return r.finish(result, generation)
		}
	}
	return r.finish(result, generation)
}

func (r *Reconciler) azureRoutes(ctx context.Context, cfg model.AppConfig, routes []model.RouteConfig) model.AzureResult {
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

func (r *Reconciler) finish(result model.ReconcileResult, generation uint64) model.ReconcileResult {
	result.FinishedAt = time.Now().UTC()
	result.Duration = result.FinishedAt.Sub(result.StartedAt)
	r.mu.Lock()
	if r.generation == generation {
		if result.CaddyLoaded && result.RouteHealth != nil {
			r.store.SetRuntimeStatus(result.RouteHealth)
		}
		r.last = result
	}
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
