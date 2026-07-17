package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aidockerfarm/gateway/internal/api"
	"github.com/aidockerfarm/gateway/internal/audit"
	gatewayazure "github.com/aidockerfarm/gateway/internal/azure"
	"github.com/aidockerfarm/gateway/internal/caddy"
	"github.com/aidockerfarm/gateway/internal/config"
	"github.com/aidockerfarm/gateway/internal/docker"
	"github.com/aidockerfarm/gateway/internal/health"
	"github.com/aidockerfarm/gateway/internal/reconcile"
	"github.com/aidockerfarm/gateway/internal/routes"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	store := routes.NewStore(cfg.RoutesFile)
	if err := store.Load(); err != nil {
		logger.Error("failed to load routes", "error", err)
		os.Exit(1)
	}
	auditLogger := audit.NewLogger(cfg.Audit, logger.With("component", "audit"))

	renderer := caddy.NewRenderer(cfg)
	initialConfig, err := renderer.Render(store.List())
	if err != nil {
		logger.Error("failed to render initial caddy config", "error", err)
		os.Exit(1)
	}

	manager := caddy.NewManager(cfg.Gateway, logger.With("component", "caddy"))
	if err := manager.Start(ctx, initialConfig); err != nil {
		logger.Error("caddy runtime did not start", "error", err)
		os.Exit(1)
	}
	defer manager.Stop()

	var dockerDiscoverer *docker.Discoverer
	if cfg.Docker.Enabled {
		dockerDiscoverer = docker.NewDiscoverer(cfg.Docker, logger.With("component", "docker"))
	}
	var reconcileDiscoverer reconcile.Discoverer
	var apiDiscoverer api.Discoverer
	if dockerDiscoverer != nil {
		reconcileDiscoverer = dockerDiscoverer
		apiDiscoverer = dockerDiscoverer
	}

	var azureManager reconcile.AzureManager
	if cfg.Azure.Enabled {
		manager, err := gatewayazure.NewManager(cfg, logger.With("component", "azure"))
		if err != nil {
			logger.Warn("azure manager did not start; Azure reconciliation will be disabled", "error", err)
		} else {
			azureManager = manager
		}
	}
	var azurePermissionChecker api.AzurePermissionChecker
	permissionChecker, err := gatewayazure.NewPermissionChecker()
	if err != nil {
		logger.Warn("Azure permission checker did not start", "error", err)
	} else {
		azurePermissionChecker = permissionChecker
	}

	caddyClient := caddy.NewClient(cfg.Gateway.CaddyAdminEndpoint)
	var healthChecker reconcile.HealthChecker
	if cfg.Health.Enabled {
		healthChecker = health.NewChecker(cfg.Health)
	}
	reconciler := reconcile.New(reconcile.Options{
		Config:        cfg,
		Store:         store,
		Discoverer:    reconcileDiscoverer,
		AzureManager:  azureManager,
		HealthChecker: healthChecker,
		AuditLogger:   auditLogger,
		Renderer:      renderer,
		Loader:        caddyClient,
		Logger:        logger.With("component", "reconcile"),
	})
	go reconciler.Run(ctx)

	apiServer := api.NewServer(api.Options{
		Config:                 cfg,
		Store:                  store,
		Discoverer:             apiDiscoverer,
		Reconciler:             reconciler,
		Runtime:                manager,
		AuditLog:               auditLogger,
		CertificateStore:       config.NewCertificateStore(cfg.Gateway.CertificateFile),
		SettingsStore:          config.NewSettingsStore(config.SettingsPath(cfg)),
		AzurePermissionChecker: azurePermissionChecker,
		Logger:                 logger.With("component", "api"),
	})

	server := &http.Server{
		Addr:              cfg.Control.Listen,
		Handler:           apiServer.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("management server listening", "addr", cfg.Control.Listen, "profile", cfg.Profile)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("management server failed", "error", err)
			stop()
		}
	}()

	var runtimeErr error
	select {
	case <-ctx.Done():
	case runtimeErr = <-manager.Done():
		logger.Error("required caddy runtime exited; shutting down", "error", runtimeErr)
		stop()
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Warn("management server shutdown failed", "error", err)
	}
	if runtimeErr != nil {
		manager.Stop()
		os.Exit(1)
	}
}
