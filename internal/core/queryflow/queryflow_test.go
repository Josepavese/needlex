package queryflow

import (
	"testing"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
)

func TestShouldEscalateRewriteWhenLeaderLacksSemanticGrounding(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{URL: "https://example.com/other-b", Score: 1.4, Reason: []string{"structure_hint"}},
		{URL: "https://example.com/other-a", Score: 0.9, Reason: []string{"structure_hint"}},
	}
	if !ShouldEscalateRewrite(candidates[0].URL, candidates) {
		t.Fatal("expected rewrite escalation for ungrounded leader")
	}
}

func TestShouldEscalateRewriteSkipsGroundedLeaderWithClearDelta(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{URL: "https://developer.mozilla.org/en-US/docs/Web/JavaScript/Guide", Score: 1.5, Reason: []string{"semantic_goal_alignment"}, Metadata: map[string]string{"semantic_goal_similarity": "0.720"}},
		{URL: "https://javascript.info/", Score: 0.9, Reason: []string{"structure_hint"}},
	}
	if ShouldEscalateRewrite(candidates[0].URL, candidates) {
		t.Fatal("expected grounded clear leader to skip rewrite escalation")
	}
}

func TestRerankCandidatesWithFingerprintEvidencePromotesNovelCandidate(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{URL: "https://example.com/stable", Score: 1.0, Reason: []string{"semantic_goal_alignment"}},
		{URL: "https://example.com/novel", Score: 0.9, Reason: []string{"semantic_goal_alignment"}},
	}
	out := RerankCandidatesWithFingerprintEvidence(candidates, "", FingerprintEvidence{}, func(rawURL string) (FingerprintEvidence, bool) {
		if rawURL == "https://example.com/novel" {
			return FingerprintEvidence{TraceID: "trace_novel", Novelty: 0.4, Changed: true}, true
		}
		return FingerprintEvidence{TraceID: "trace_stable", Stable: 0.95}, true
	})
	if out[0].URL != "https://example.com/novel" {
		t.Fatalf("expected novel candidate first, got %#v", out)
	}
}

func TestSelectionDeltaIsAbsoluteTopGap(t *testing.T) {
	got := SelectionDelta([]discoverycore.Candidate{{Score: 0.8}, {Score: 1.1}})
	if got < 0.299 || got > 0.301 {
		t.Fatalf("unexpected delta %.3f", got)
	}
}
