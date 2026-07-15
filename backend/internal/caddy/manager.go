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
	cfg      model.GatewayConfig
	logger   *slog.Logger
	mu       sync.Mutex
	cmd      *exec.Cmd
	done     chan error
	ready    bool
	stopping bool
	lastErr  error
}

func NewManager(cfg model.GatewayConfig, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{cfg: cfg, logger: logger, done: make(chan error, 1)}
}

func (m *Manager) Start(ctx context.Context, initialConfig []byte) error {
	m.mu.Lock()
	if m.cmd != nil {
		m.mu.Unlock()
		return nil
	}
	if _, err := exec.LookPath(m.cfg.CaddyBin); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("caddy binary %q not found: %w", m.cfg.CaddyBin, err)
	}
	if err := os.MkdirAll(m.cfg.StateDir, 0o755); err != nil {
		m.mu.Unlock()
		return err
	}
	if err := os.MkdirAll(m.cfg.CaddyDataDir, 0o755); err != nil {
		m.mu.Unlock()
		return err
	}

	configPath := filepath.Join(m.cfg.StateDir, "caddy.bootstrap.json")
	if err := os.WriteFile(configPath, initialConfig, 0o600); err != nil {
		m.mu.Unlock()
		return err
	}

	cmd := exec.CommandContext(ctx, m.cfg.CaddyBin, "run", "--config", configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "XDG_DATA_HOME="+m.cfg.CaddyDataDir)
	if err := cmd.Start(); err != nil {
		m.mu.Unlock()
		return err
	}
	m.cmd = cmd
	m.ready = false
	m.stopping = false
	m.lastErr = nil
	m.logger.Info("caddy runtime started", "pid", cmd.Process.Pid)
	m.mu.Unlock()

	go m.wait(ctx, cmd)
	if err := m.waitForAdmin(ctx); err != nil {
		m.stopCommand(cmd, true)
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd != cmd {
		if m.lastErr != nil {
			return m.lastErr
		}
		return fmt.Errorf("caddy runtime exited before becoming ready")
	}
	m.ready = true
	return nil
}

func (m *Manager) wait(ctx context.Context, cmd *exec.Cmd) {
	err := cmd.Wait()

	m.mu.Lock()
	expected := m.stopping || ctx.Err() != nil
	if m.cmd == cmd {
		m.cmd = nil
	}
	m.ready = false
	if !expected {
		if err == nil {
			err = fmt.Errorf("caddy runtime exited unexpectedly")
		} else {
			err = fmt.Errorf("caddy runtime exited unexpectedly: %w", err)
		}
		m.lastErr = err
	}
	m.mu.Unlock()

	if expected {
		m.logger.Info("caddy runtime stopped")
		return
	}
	m.logger.Error("caddy runtime exited", "error", err)
	select {
	case m.done <- err:
	default:
	}
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
	m.stopCommand(nil, false)
}

func (m *Manager) stopCommand(expected *exec.Cmd, force bool) {
	m.mu.Lock()
	cmd := m.cmd
	if cmd == nil || cmd.Process == nil || expected != nil && cmd != expected {
		m.mu.Unlock()
		return
	}
	m.stopping = true
	m.ready = false
	m.mu.Unlock()

	if force {
		_ = cmd.Process.Kill()
		return
	}
	_ = cmd.Process.Signal(os.Interrupt)
}

func (m *Manager) Done() <-chan error {
	return m.done
}

func (m *Manager) Ready() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ready
}

func (m *Manager) LastError() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastErr
}
