package store

import (
	"errors"
	"testing"
	"time"
)

func TestCandidateStoreObserveAndSearch(t *testing.T) {
	root := t.TempDir()
	store := NewCandidateStore(root)
	store.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	record, _, err := store.Observe(CandidateObservation{
		URL:    "https://halfpocket.net/about",
		Title:  "Halfpocket Studio",
		Source: "read",
	})
	if err != nil {
		t.Fatalf("observe candidate: %v", err)
	}
	if record.Host != "halfpocket.net" {
		t.Fatalf("expected host halfpocket.net, got %q", record.Host)
	}
	if record.SeenCount != 1 {
		t.Fatalf("expected seen count 1, got %d", record.SeenCount)
	}

	record, _, err = store.Observe(CandidateObservation{
		URL:    "https://halfpocket.net/about",
		Title:  "Halfpocket Creative Studio",
		Source: "query_discovery",
	})
	if err != nil {
		t.Fatalf("observe candidate second time: %v", err)
	}
	if record.SeenCount != 2 {
		t.Fatalf("expected seen count 2, got %d", record.SeenCount)
	}
	if len(record.Sources) != 2 {
		t.Fatalf("expected 2 unique sources, got %d", len(record.Sources))
	}
	if record.Title != "Halfpocket Creative Studio" {
		t.Fatalf("expected latest title update, got %q", record.Title)
	}

	matches, err := store.Search("halfpocket studio", 3)
	if err != nil {
		t.Fatalf("search candidates: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].URL != "https://halfpocket.net/about" {
		t.Fatalf("expected matching url, got %q", matches[0].URL)
	}
	if matches[0].Score <= 0 {
		t.Fatalf("expected positive score, got %f", matches[0].Score)
	}
}

func TestCandidateStoreSearchMissing(t *testing.T) {
	store := NewCandidateStore(t.TempDir())
	matches, err := store.Search("needle runtime", 1)
	if err != nil {
		t.Fatalf("search candidates: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no matches, got %d", len(matches))
	}
}

func TestCandidateStoreMissingLoad(t *testing.T) {
	store := NewCandidateStore(t.TempDir())
	_, err := store.loadAll()
	if !errors.Is(err, ErrCandidatesNotFound) {
		t.Fatalf("expected candidates not found, got %v", err)
	}
}

func TestCandidateStoreObserveRejectsInvalidURL(t *testing.T) {
	store := NewCandidateStore(t.TempDir())
	if _, _, err := store.Observe(CandidateObservation{URL: "not a url"}); err == nil {
		t.Fatal("expected invalid url error")
	}
}
