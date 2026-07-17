package logs

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
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

func TestStoreEvictsOldestEntriesAtByteLimit(t *testing.T) {
	now := time.Now().UTC()
	sample := Entry{Time: now, Source: "test", Level: "info", Message: strings.Repeat("x", 64)}
	encoded, err := json.Marshal(sample)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	store := newStore(10, len(encoded)*2)
	for index := 0; index < 3; index++ {
		store.Add(Entry{
			Time: now, Source: "test", Level: "info",
			Message: fmt.Sprintf("%05d%s", index, strings.Repeat("x", 59)),
		})
	}

	entries := store.ReadLast(10)
	if len(entries) != 2 || !strings.HasPrefix(entries[0].Message, "00002") || !strings.HasPrefix(entries[1].Message, "00001") {
		t.Fatalf("entries = %#v", entries)
	}
	if store.storedBytes > store.byteLimit {
		t.Fatalf("stored bytes = %d, limit = %d", store.storedBytes, store.byteLimit)
	}
}

func TestWriterTruncatesOversizedLineWithoutSplittingIt(t *testing.T) {
	store := NewStore(10)
	payload := strings.Repeat("x", maxLineBytes+1024) + "\nnext\n"
	if _, err := store.Writer("test", "info").Write([]byte(payload)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	entries := store.ReadLast(10)
	if len(entries) != 2 || entries[0].Message != "next" || len(entries[1].Message) != maxLineBytes {
		t.Fatalf("entries = %#v", entries)
	}
}
