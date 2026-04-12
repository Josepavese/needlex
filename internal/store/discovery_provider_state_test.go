package store

import (
	"errors"
	"testing"
	"time"
)

func TestDiscoveryProviderStateStoreObserveAndLoad(t *testing.T) {
	root := t.TempDir()
	store := NewDiscoveryProviderStateStore(root)
	store.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	state, _, err := store.Observe(DiscoveryProviderObservation{
		Name:            "duckduckgo",
		Outcome:         DiscoveryProviderOutcomeBlocked,
		BlockedCooldown: 10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("observe state: %v", err)
	}
	if state.BlockedCount != 1 || state.ConsecutiveFailures != 1 {
		t.Fatalf("unexpected blocked state: %+v", state)
	}
	if state.CooldownUntil.IsZero() {
		t.Fatal("expected cooldown to be set")
	}

	loaded, err := store.Load("duckduckgo")
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if loaded.LastOutcome != DiscoveryProviderOutcomeBlocked {
		t.Fatalf("expected blocked last outcome, got %q", loaded.LastOutcome)
	}
}

func TestDiscoveryProviderStateStoreSuccessClearsCooldown(t *testing.T) {
	root := t.TempDir()
	store := NewDiscoveryProviderStateStore(root)
	now := time.Unix(1700000000, 0).UTC()
	store.now = func() time.Time { return now }

	if _, _, err := store.Observe(DiscoveryProviderObservation{
		Name:            "duckduckgo",
		Outcome:         DiscoveryProviderOutcomeTimeout,
		TimeoutCooldown: 5 * time.Minute,
	}); err != nil {
		t.Fatalf("observe timeout: %v", err)
	}
	now = now.Add(1 * time.Minute)
	state, _, err := store.Observe(DiscoveryProviderObservation{
		Name:    "duckduckgo",
		Outcome: DiscoveryProviderOutcomeSuccess,
	})
	if err != nil {
		t.Fatalf("observe success: %v", err)
	}
	if !state.CooldownUntil.IsZero() || state.ConsecutiveFailures != 0 || state.SuccessCount != 1 {
		t.Fatalf("expected success to clear cooldown/failures, got %+v", state)
	}
}

func TestDiscoveryProviderStateStoreMissing(t *testing.T) {
	store := NewDiscoveryProviderStateStore(t.TempDir())
	_, err := store.Load("missing")
	if !errors.Is(err, ErrDiscoveryProviderStateNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}
