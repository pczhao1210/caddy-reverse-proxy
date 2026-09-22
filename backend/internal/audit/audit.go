package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
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

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	return l.readLast(file, info.Size(), limit)
}

func (l *Logger) readLast(reader io.ReaderAt, size int64, limit int) ([]Event, error) {
	const blockSize = 64 * 1024
	const maxLineSize = 1024 * 1024
	events := make([]Event, 0, limit)
	parse := func(line []byte) error {
		if len(line) == 0 {
			return nil
		}
		if len(line) > maxLineSize {
			return fmt.Errorf("read audit log: line exceeds %d bytes", maxLineSize)
		}
		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			l.logger.Warn("skipping invalid audit log line", "error", err)
			return nil
		}
		events = append(events, event)
		return nil
	}
	var remainder []byte
	for size > 0 && len(events) < limit {
		count := min(int64(blockSize), size)
		size -= count
		data := make([]byte, int(count)+len(remainder))
		if _, err := reader.ReadAt(data[:count], size); err != nil {
			return nil, fmt.Errorf("read audit log: %w", err)
		}
		copy(data[count:], remainder)
		for len(events) < limit {
			separator := bytes.LastIndexByte(data, '\n')
			if separator < 0 {
				break
			}
			if err := parse(data[separator+1:]); err != nil {
				return nil, err
			}
			data = data[:separator]
		}
		if len(events) == limit {
			break
		}
		if len(data) > maxLineSize {
			return nil, fmt.Errorf("read audit log: line exceeds %d bytes", maxLineSize)
		}
		remainder = data
		if size == 0 {
			if err := parse(remainder); err != nil {
				return nil, err
			}
		}
	}
	slices.Reverse(events)
	return events, nil
}
