package service

import (
	"context"
	"testing"

	"github.com/josepavese/needlex/internal/config"
)

func TestTargetKindDoesNotPromoteBroadTargetsFromNativeFallback(t *testing.T) {
	cfg := config.Defaults()
	cfg.Semantic.Enabled = false
	svc := newTestService(t, cfg, nil)
	candidates := []DiscoverCandidate{
		{URL: "https://example.com/products/cloud", Score: 10},
		{URL: "https://example.com/", Score: 1},
	}

	got := svc.applyTargetKindRerank(context.Background(), "official main home page broad identity overview", candidates)
	if got[0].URL != candidates[0].URL {
		t.Fatalf("expected native fallback to avoid broad target-kind promotion, got %#v", got)
	}
	for _, candidate := range got {
		if candidate.Metadata["target_kind"] != "" {
			t.Fatalf("expected no target_kind metadata from native fallback, got %#v", got)
		}
	}
}
