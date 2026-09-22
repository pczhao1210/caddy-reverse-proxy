package audit

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidockerfarm/gateway/internal/model"
)

func TestRecordAndReadLast(t *testing.T) {
	logger := NewLogger(model.AuditConfig{Enabled: true, File: filepath.Join(t.TempDir(), "audit.jsonl")}, nil)
	if err := logger.Record(context.Background(), "route.create", map[string]any{"routeId": "one"}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if err := logger.Record(context.Background(), "reconcile.complete", map[string]any{"appliedRoutes": 1}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	events, err := logger.ReadLast(1)
	if err != nil {
		t.Fatalf("ReadLast() error = %v", err)
	}
	if len(events) != 1 || events[0].Event != "reconcile.complete" {
		t.Fatalf("events = %#v", events)
	}
}

type countedReader struct {
	io.ReaderAt
	bytesRead int
}

func (reader *countedReader) ReadAt(data []byte, offset int64) (int, error) {
	count, err := reader.ReaderAt.ReadAt(data, offset)
	reader.bytesRead += count
	return count, err
}

func TestTailReadsOnlyRecentBlocks(t *testing.T) {
	data := strings.Repeat("{\"event\":\"old\"}\n", 100000) + "{\"event\":\"recent\"}\n{\"event\":"
	reader := &countedReader{ReaderAt: strings.NewReader(data)}
	logger := NewLogger(model.AuditConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	events, err := logger.readLast(reader, int64(len(data)), 1)
	if err != nil || len(events) != 1 || events[0].Event != "recent" {
		t.Fatalf("events=%v err=%v", events, err)
	}
	if reader.bytesRead != 64*1024 {
		t.Fatalf("read %d bytes for a tail fitting in one block", reader.bytesRead)
	}
}

func TestTailPreservesLongLinesAndChronologicalOrder(t *testing.T) {
	longLine, err := json.Marshal(Event{Event: "long", Fields: map[string]any{"payload": strings.Repeat("x", 160000)}})
	if err != nil {
		t.Fatal(err)
	}
	for _, ending := range []string{"", "\n"} {
		path := filepath.Join(t.TempDir(), "audit.jsonl")
		data := "{\"event\":\"first\"}\n" + string(longLine) + "\ninvalid\n{\"event\":\"last\"}" + ending
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		logger := NewLogger(model.AuditConfig{Enabled: true, File: path}, nil)
		events, err := logger.ReadLast(3)
		if err != nil || len(events) != 3 || events[0].Event != "first" || events[1].Event != "long" || events[2].Event != "last" {
			t.Fatalf("events=%d err=%v", len(events), err)
		}
	}
}

func BenchmarkReadLastLargeLog(b *testing.B) {
	path := filepath.Join(b.TempDir(), "audit.jsonl")
	if err := os.WriteFile(path, []byte(strings.Repeat("{\"event\":\"historical\"}\n", 1000000)), 0o600); err != nil {
		b.Fatal(err)
	}
	logger := NewLogger(model.AuditConfig{Enabled: true, File: path}, nil)
	b.ResetTimer()
	b.ReportAllocs()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := logger.ReadLast(100); err != nil {
			b.Fatal(err)
		}
	}
}
