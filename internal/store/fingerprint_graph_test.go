package store

import (
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/core"
)

func TestFingerprintGraphStoreObserveAndDelta(t *testing.T) {
	root := t.TempDir()
	store := NewFingerprintGraphStore(root)
	store.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	first, _, err := store.Observe("https://example.com/docs", "trace_1", []core.Chunk{
		fakeChunk(),
	})
	if err != nil {
		t.Fatalf("observe first: %v", err)
	}
	if len(first.Added) != 1 || first.Added[0] != "fp_1" {
		t.Fatalf("expected fp_1 added on first observe, got %#v", first)
	}

	store.now = func() time.Time { return time.Unix(1700000001, 0).UTC() }
	second, _, err := store.Observe("https://example.com/docs", "trace_2", []core.Chunk{
		fakeChunk(),
		{
			ID:          "chk_2",
			DocID:       "doc_1",
			Text:        "New chunk.",
			HeadingPath: []string{"Needle Runtime", "Delta"},
			Score:       0.8,
			Fingerprint: "fp_2",
			Confidence:  0.9,
		},
	})
	if err != nil {
		t.Fatalf("observe second: %v", err)
	}
	if len(second.Retained) != 1 || second.Retained[0].Fingerprint != "fp_1" {
		t.Fatalf("expected fp_1 retained, got %#v", second.Retained)
	}
	if len(second.Added) != 1 || second.Added[0] != "fp_2" {
		t.Fatalf("expected fp_2 added, got %#v", second.Added)
	}
	if len(second.Removed) != 0 {
		t.Fatalf("expected no removals, got %#v", second.Removed)
	}

	graph, err := store.Load("https://example.com/docs")
	if err != nil {
		t.Fatalf("load graph: %v", err)
	}
	if graph.LatestTraceID != "trace_2" {
		t.Fatalf("expected latest trace trace_2, got %q", graph.LatestTraceID)
	}
	if len(graph.History) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(graph.History))
	}
}

func TestFingerprintGraphStoreObserveDetectsRemoved(t *testing.T) {
	root := t.TempDir()
	store := NewFingerprintGraphStore(root)
	store.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	_, _, err := store.Observe("https://example.com/docs", "trace_1", []core.Chunk{
		fakeChunk(),
		{
			ID:          "chk_2",
			DocID:       "doc_1",
			Text:        "Old chunk.",
			HeadingPath: []string{"Needle Runtime", "Old"},
			Score:       0.7,
			Fingerprint: "fp_old",
			Confidence:  0.88,
		},
	})
	if err != nil {
		t.Fatalf("observe first: %v", err)
	}

	store.now = func() time.Time { return time.Unix(1700000001, 0).UTC() }
	delta, _, err := store.Observe("https://example.com/docs", "trace_2", []core.Chunk{fakeChunk()})
	if err != nil {
		t.Fatalf("observe second: %v", err)
	}
	if len(delta.Removed) != 1 || delta.Removed[0] != "fp_old" {
		t.Fatalf("expected fp_old removed, got %#v", delta.Removed)
	}
}
