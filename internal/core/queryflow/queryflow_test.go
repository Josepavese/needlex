package queryflow

import (
	"testing"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
)

func TestFinalizeDiscoveryResultPrefersTopRankedCandidateOverFallback(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{URL: "https://example.com/docs/install", Score: 1.45, Reason: []string{"path_hint"}},
		{URL: "https://example.com", Score: 1.25, Reason: []string{"seed_fallback"}},
	}

	finalCandidates, selected := FinalizeDiscoveryResult(candidates, "https://example.com", "https://example.com", FingerprintEvidence{}, nil)
	if len(finalCandidates) != 2 {
		t.Fatalf("unexpected candidates %#v", finalCandidates)
	}
	if selected != "https://example.com/docs/install" {
		t.Fatalf("expected top-ranked candidate to win, got %q", selected)
	}
}

func TestFinalizeDiscoveryResultKeepsFallbackWhenNoCandidatesRemain(t *testing.T) {
	_, selected := FinalizeDiscoveryResult(nil, "https://example.com", "https://example.com", FingerprintEvidence{}, nil)
	if selected != "https://example.com" {
		t.Fatalf("expected fallback to survive empty candidate set, got %q", selected)
	}
}

func TestFinalizeDiscoveryResultUsesRerankedTopCandidate(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{URL: "https://seed.example", Score: 1.00, Reason: []string{"seed_fallback"}},
		{URL: "https://seed.example/docs", Score: 0.95, Reason: []string{"path_hint"}},
	}

	_, selected := FinalizeDiscoveryResult(
		candidates,
		"https://seed.example",
		"https://seed.example",
		FingerprintEvidence{TraceID: "trace_seed", Stable: 1.0},
		nil,
	)
	if selected != "https://seed.example/docs" {
		t.Fatalf("expected reranked docs candidate to win, got %q", selected)
	}
}
