package store

import (
	"errors"
	"testing"
	"time"
)

func TestDomainGraphObserveAndExpand(t *testing.T) {
	root := t.TempDir()
	store := NewDomainGraphStore(root)
	store.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	if _, _, err := store.Observe("https://halfpocket.net", "https://docs.halfpocket.net", "query_discovery"); err != nil {
		t.Fatalf("observe edge 1: %v", err)
	}
	if _, _, err := store.Observe("https://halfpocket.net", "https://docs.halfpocket.net", "query_discovery"); err != nil {
		t.Fatalf("observe edge 2: %v", err)
	}
	if _, _, err := store.Observe("https://halfpocket.net", "https://blog.halfpocket.net", "query_discovery"); err != nil {
		t.Fatalf("observe edge 3: %v", err)
	}

	matches, err := store.Expand([]string{"halfpocket.net"}, 5)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if len(matches) < 2 {
		t.Fatalf("expected at least 2 matches, got %d", len(matches))
	}
	if matches[0].Domain != "docs.halfpocket.net" {
		t.Fatalf("expected docs domain first, got %q", matches[0].Domain)
	}
	if matches[0].Score <= matches[1].Score {
		t.Fatalf("expected top score > second score, got %f <= %f", matches[0].Score, matches[1].Score)
	}
}

func TestDomainGraphExpandMissing(t *testing.T) {
	store := NewDomainGraphStore(t.TempDir())
	matches, err := store.Expand([]string{"example.com"}, 3)
	if err != nil {
		t.Fatalf("expand missing graph: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}
}

func TestDomainGraphMissingLoad(t *testing.T) {
	store := NewDomainGraphStore(t.TempDir())
	_, err := store.loadAll()
	if !errors.Is(err, ErrDomainGraphNotFound) {
		t.Fatalf("expected domain graph not found, got %v", err)
	}
}
