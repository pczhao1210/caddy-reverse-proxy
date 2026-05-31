package caddy

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aidockerfarm/gateway/internal/model"
)

type Manager struct {
	cfg    model.GatewayConfig
	logger *slog.Logger
	mu     sync.Mutex
	cmd    *exec.Cmd
}

func NewManager(cfg model.GatewayConfig, logger *slog.Logger) *Manager {
	return &Manager{cfg: cfg, logger: logger}
}

func (m *Manager) Start(ctx context.Context, initialConfig []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd != nil {
		return nil
	}
	if _, err := exec.LookPath(m.cfg.CaddyBin); err != nil {
		return fmt.Errorf("caddy binary %q not found: %w", m.cfg.CaddyBin, err)
	}
	if err := os.MkdirAll(m.cfg.StateDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(m.cfg.CaddyDataDir, 0o755); err != nil {
		return err
	}

	configPath := filepath.Join(m.cfg.StateDir, "caddy.bootstrap.json")
	if err := os.WriteFile(configPath, initialConfig, 0o600); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, m.cfg.CaddyBin, "run", "--config", configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "XDG_DATA_HOME="+m.cfg.CaddyDataDir)
	if err := cmd.Start(); err != nil {
		return err
	}
	m.cmd = cmd
	m.logger.Info("caddy runtime started", "pid", cmd.Process.Pid)

	go func() {
		if err := cmd.Wait(); err != nil {
			m.logger.Warn("caddy runtime exited", "error", err)
		} else {
			m.logger.Info("caddy runtime exited")
		}
	}()
	return m.waitForAdmin(ctx)
}

func (m *Manager) waitForAdmin(ctx context.Context) error {
	endpoint := strings.TrimRight(m.cfg.CaddyAdminEndpoint, "/") + "/config/"
	client := &http.Client{Timeout: 250 * time.Millisecond}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode < 500 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("caddy admin endpoint %s was not ready", endpoint)
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd == nil || m.cmd.Process == nil {
		return
	}
	_ = m.cmd.Process.Signal(os.Interrupt)
	m.cmd = nil
}
