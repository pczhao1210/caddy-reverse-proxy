package logs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

const maxLineBytes = 64 * 1024

type Entry struct {
	Time    time.Time      `json:"time"`
	Source  string         `json:"source"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}

type Store struct {
	mu      sync.RWMutex
	entries []Entry
	limit   int
}

func NewStore(limit int) *Store {
	if limit <= 0 {
		limit = 1000
	}
	return &Store{entries: make([]Entry, 0, limit), limit: limit}
}

func (s *Store) Add(entry Entry) {
	if entry.Time.IsZero() {
		entry.Time = time.Now().UTC()
	}
	entry.Level = strings.ToLower(strings.TrimSpace(entry.Level))
	if entry.Level == "" {
		entry.Level = "info"
	}
	entry.Source = strings.TrimSpace(entry.Source)
	entry.Message = strings.TrimSpace(entry.Message)

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.entries) == s.limit {
		copy(s.entries, s.entries[1:])
		s.entries[len(s.entries)-1] = entry
		return
	}
	s.entries = append(s.entries, entry)
}

func (s *Store) ReadLast(limit int) []Entry {
	if limit <= 0 || limit > s.limit {
		limit = min(200, s.limit)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit > len(s.entries) {
		limit = len(s.entries)
	}
	entries := make([]Entry, 0, limit)
	for index := len(s.entries) - 1; index >= len(s.entries)-limit; index-- {
		entries = append(entries, s.entries[index])
	}
	return entries
}

func (s *Store) Writer(source, level string) io.Writer {
	return &lineWriter{store: s, source: source, level: level}
}

type lineWriter struct {
	mu      sync.Mutex
	store   *Store
	source  string
	level   string
	pending []byte
}

func (w *lineWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, data...)
	for {
		newline := bytes.IndexByte(w.pending, '\n')
		if newline < 0 {
			break
		}
		w.addLine(w.pending[:newline])
		w.pending = w.pending[newline+1:]
	}
	if len(w.pending) > maxLineBytes {
		w.addLine(w.pending[:maxLineBytes])
		w.pending = w.pending[:0]
	}
	return len(data), nil
}

func (w *lineWriter) addLine(line []byte) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return
	}
	w.store.Add(parseEntry(w.source, w.level, line))
}

func parseEntry(source, defaultLevel string, line []byte) Entry {
	entry := Entry{Time: time.Now().UTC(), Source: source, Level: defaultLevel, Message: string(line)}
	var fields map[string]any
	if json.Unmarshal(line, &fields) != nil {
		return entry
	}
	if value, ok := fields["time"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			entry.Time = parsed.UTC()
		}
	}
	if value, ok := fields["ts"].(float64); ok {
		seconds := int64(value)
		nanoseconds := int64((value - float64(seconds)) * float64(time.Second))
		entry.Time = time.Unix(seconds, nanoseconds).UTC()
	}
	if value, ok := fields["level"].(string); ok && value != "" {
		entry.Level = value
	}
	if value, ok := fields["msg"].(string); ok && value != "" {
		entry.Message = value
	}
	component := stringField(fields, "component")
	if component == "" {
		component = stringField(fields, "logger")
	}
	if component != "" {
		entry.Source = strings.TrimSuffix(source, "/") + "/" + component
	}
	for _, key := range []string{"time", "ts", "level", "msg", "component", "logger"} {
		delete(fields, key)
	}
	if len(fields) > 0 {
		entry.Fields = fields
	}
	return entry
}

func stringField(fields map[string]any, key string) string {
	value, ok := fields[key]
	if !ok {
		return ""
	}
	return fmt.Sprint(value)
}
