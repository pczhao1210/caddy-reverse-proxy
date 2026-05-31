package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aidockerfarm/gateway/internal/model"
)

type Event struct {
	Time   time.Time      `json:"time"`
	Event  string         `json:"event"`
	Fields map[string]any `json:"fields,omitempty"`
}

type Logger struct {
	cfg    model.AuditConfig
	logger *slog.Logger
	mu     sync.Mutex
}

func NewLogger(cfg model.AuditConfig, logger *slog.Logger) *Logger {
	if logger == nil {
		logger = slog.Default()
	}
	return &Logger{cfg: cfg, logger: logger}
}

func (l *Logger) Record(_ context.Context, event string, fields map[string]any) error {
	if l == nil || !l.cfg.Enabled || l.cfg.File == "" {
		return nil
	}
	entry := Event{Time: time.Now().UTC(), Event: event, Fields: fields}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(l.cfg.File), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(l.cfg.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func (l *Logger) ReadLast(limit int) ([]Event, error) {
	if l == nil || !l.cfg.Enabled || l.cfg.File == "" {
		return []Event{}, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	file, err := os.Open(l.cfg.File)
	if err != nil {
		if os.IsNotExist(err) {
			return []Event{}, nil
		}
		return nil, err
	}
	defer file.Close()

	events := make([]Event, 0, limit)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			l.logger.Warn("skipping invalid audit log line", "error", err)
			continue
		}
		events = append(events, event)
		if len(events) > limit {
			events = events[1:]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read audit log: %w", err)
	}
	return events, nil
}
