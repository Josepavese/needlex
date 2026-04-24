package observability

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoggerWritesRedactedJSONAndStats(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "needlex.jsonl")
	logger := NewLoggerWithOptions(Options{Path: path, MaxBytes: 1024, MaxFiles: 2})
	written, err := logger.Write(Event{
		Level:     LevelError,
		Component: "transport",
		Surface:   "cli",
		Operation: "read",
		Event:     "runtime.error",
		Message:   "read failed",
		Error:     "TOKEN=super-secret unexpected status code 403",
		Fields: map[string]any{
			"url": "https://example.com/?api_key=secret",
		},
	})
	if err != nil {
		t.Fatalf("write log: %v", err)
	}
	if written.ID == "" || written.TimestampUTC == "" {
		t.Fatalf("event missing id/timestamp: %+v", written)
	}
	stats, err := logger.Stats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if !stats.Exists || stats.LineCount != 1 || stats.LastEvent.ID != written.ID {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	tail, err := logger.Tail(1)
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	if len(tail) != 1 || strings.Contains(tail[0].Error, "super-secret") || strings.Contains(tail[0].Fields["url"].(string), "secret") {
		t.Fatalf("expected redacted tail event, got %+v", tail)
	}
}

func TestLoggerRotates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "needlex.jsonl")
	logger := NewLoggerWithOptions(Options{Path: path, MaxBytes: 180, MaxFiles: 2})
	for i := 0; i < 5; i++ {
		if _, err := logger.Write(Event{Level: LevelError, Event: "runtime.error", Message: strings.Repeat("x", 120)}); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	stats, err := logger.Stats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if len(stats.Rotated) == 0 {
		t.Fatalf("expected rotated logs, got %+v", stats)
	}
}
