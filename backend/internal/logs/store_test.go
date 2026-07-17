package logs

import (
	"fmt"
	"testing"
)

func TestStoreParsesStructuredLinesAndKeepsNewestEntries(t *testing.T) {
	store := NewStore(2)
	writer := store.Writer("gateway", "info")
	_, _ = writer.Write([]byte("{\"level\":\"WARN\",\"component\":\"reconcile\",\"msg\":\"first\",\"routes\":2}\n"))
	_, _ = writer.Write([]byte("second\n"))
	_, _ = writer.Write([]byte("third\n"))

	entries := store.ReadLast(10)
	if len(entries) != 2 || entries[0].Message != "third" || entries[1].Message != "second" {
		t.Fatalf("entries = %#v", entries)
	}

	structured := NewStore(2)
	_, _ = structured.Writer("gateway", "info").Write([]byte("{\"level\":\"WARN\",\"component\":\"reconcile\",\"msg\":\"failed\",\"routes\":2}\n"))
	entry := structured.ReadLast(1)[0]
	if entry.Source != "gateway/reconcile" || entry.Level != "warn" || entry.Message != "failed" || fmt.Sprint(entry.Fields["routes"]) != "2" {
		t.Fatalf("structured entry = %#v", entry)
	}
}
