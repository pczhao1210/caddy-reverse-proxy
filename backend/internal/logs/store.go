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

const (
	maxLineBytes  = 64 * 1024
	maxStoreBytes = 8 * 1024 * 1024
)

type Entry struct {
	Time    time.Time      `json:"time"`
	Source  string         `json:"source"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}

type Store struct {
	mu          sync.RWMutex
	entries     []Entry
	sizes       []int
	start       int
	count       int
	storedBytes int
	limit       int
	byteLimit   int
}

func NewStore(limit int) *Store {
	if limit <= 0 {
		limit = 1000
	}
	return newStore(limit, maxStoreBytes)
}

func newStore(limit, byteLimit int) *Store {
	return &Store{
		entries:   make([]Entry, limit),
		sizes:     make([]int, limit),
		limit:     limit,
		byteLimit: byteLimit,
	}
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
	encoded, err := json.Marshal(entry)
	if err != nil || len(encoded) > s.byteLimit {
		return
	}
	entrySize := len(encoded)

	s.mu.Lock()
	defer s.mu.Unlock()
	for s.count > 0 && (s.count == s.limit || s.storedBytes+entrySize > s.byteLimit) {
		s.storedBytes -= s.sizes[s.start]
		s.entries[s.start] = Entry{}
		s.sizes[s.start] = 0
		s.start = (s.start + 1) % s.limit
		s.count--
	}
	index := (s.start + s.count) % s.limit
	s.entries[index] = entry
	s.sizes[index] = entrySize
	s.storedBytes += entrySize
	s.count++
}

func (s *Store) ReadLast(limit int) []Entry {
	if limit <= 0 || limit > s.limit {
		limit = min(200, s.limit)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit > s.count {
		limit = s.count
	}
	entries := make([]Entry, 0, limit)
	for offset := 0; offset < limit; offset++ {
		index := (s.start + s.count - 1 - offset + s.limit) % s.limit
		entries = append(entries, s.entries[index])
	}
	return entries
}

func (s *Store) Writer(source, level string) io.Writer {
	return &lineWriter{store: s, source: source, level: level}
}

type lineWriter struct {
	mu       sync.Mutex
	store    *Store
	source   string
	level    string
	pending  []byte
	dropping bool
}

func (w *lineWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	written := len(data)
	for len(data) > 0 {
		newline := bytes.IndexByte(data, '\n')
		part := data
		if newline >= 0 {
			part = data[:newline]
		}
		if !w.dropping {
			available := maxLineBytes - len(w.pending)
			if len(part) > available {
				w.pending = append(w.pending, part[:available]...)
				w.dropping = true
			} else {
				w.pending = append(w.pending, part...)
			}
		}
		if newline < 0 {
			break
		}
		w.addLine(w.pending)
		w.pending = w.pending[:0]
		w.dropping = false
		data = data[newline+1:]
	}
	return written, nil
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
