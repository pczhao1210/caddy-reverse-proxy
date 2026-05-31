package audit

import (
	"context"
	"path/filepath"
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
