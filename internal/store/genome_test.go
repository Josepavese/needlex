package store

import (
	"errors"
	"testing"
	"time"
)

func TestGenomeStoreObserveAndLoad(t *testing.T) {
	root := t.TempDir()
	store := NewGenomeStore(root)
	store.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	genome, _, err := store.Observe(GenomeObservation{
		URL:               "https://example.com/docs",
		ObservedLane:      1,
		PreferredProfile:  "tiny",
		FetchProfile:      "browser_like",
		FetchRetryProfile: "hardened",
		FetchMode:         "http",
		NoiseLevel:        "medium",
	})
	if err != nil {
		t.Fatalf("observe genome: %v", err)
	}
	if genome.Domain != "example.com" {
		t.Fatalf("expected example.com, got %q", genome.Domain)
	}
	if genome.ForceLane != 1 {
		t.Fatalf("expected force lane 1, got %d", genome.ForceLane)
	}
	if genome.PruningProfile != "standard" {
		t.Fatalf("expected standard pruning profile, got %q", genome.PruningProfile)
	}

	loaded, err := store.LoadByURL("https://example.com/anything")
	if err != nil {
		t.Fatalf("load genome: %v", err)
	}
	if loaded.FetchProfile != "browser_like" || loaded.FetchRetryProfile != "hardened" {
		t.Fatalf("expected fetch profiles persisted, got %+v", loaded)
	}
}

func TestGenomeStoreDerivesAggressiveNoiseAndRenderHints(t *testing.T) {
	root := t.TempDir()
	store := NewGenomeStore(root)
	store.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	genome, _, err := store.Observe(GenomeObservation{
		URL:              "https://community.example.com/thread/1",
		ObservedLane:     2,
		PreferredProfile: "tiny",
		RenderNeeded:     true,
		NoiseLevel:       "high",
	})
	if err != nil {
		t.Fatalf("observe genome: %v", err)
	}
	if genome.PruningProfile != "aggressive" {
		t.Fatalf("expected aggressive pruning profile, got %q", genome.PruningProfile)
	}
	if !genome.RenderNeeded {
		t.Fatal("expected render hint to be persisted")
	}
}

func TestGenomeStoreMissing(t *testing.T) {
	store := NewGenomeStore(t.TempDir())
	_, err := store.Load("missing.example")
	if !errors.Is(err, ErrGenomeNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}
