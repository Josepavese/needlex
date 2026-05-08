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

func TestShouldEscalateRewriteRequiresObservableSemanticGrounding(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{URL: "https://example.com/generic", Score: 1.5, Reason: []string{"candidate_intelligence"}},
		{URL: "https://example.com/other", Score: 1.0, Reason: []string{"structure_hint"}},
	}
	if !ShouldEscalateRewrite(candidates[0].URL, candidates) {
		t.Fatal("expected generic intelligence without similarity evidence to escalate")
	}
}

func TestShouldEscalateRewriteAcceptsCandidateSimilarityMetadata(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{
			URL:      "https://example.com/entity",
			Score:    1.5,
			Reason:   []string{"candidate_intelligence"},
			Metadata: map[string]string{"candidate_goal_similarity": "0.510"},
		},
		{URL: "https://example.com/other", Score: 1.0, Reason: []string{"structure_hint"}},
	}
	if ShouldEscalateRewrite(candidates[0].URL, candidates) {
		t.Fatal("expected strong candidate similarity metadata to prevent rewrite")
	}
}

func TestShouldEscalateRewriteSkipsCloseGroundedLeaderWithEvidence(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{
			URL:      "https://example.com/entity",
			Score:    1.20,
			Reason:   []string{"semantic_goal_alignment", "page_title_probe"},
			Metadata: map[string]string{"semantic_goal_similarity": "0.620"},
		},
		{URL: "https://example.com/other", Score: 1.05, Reason: []string{"structure_hint"}},
	}
	if ShouldEscalateRewrite(candidates[0].URL, candidates) {
		t.Fatal("expected evidenced semantic leader to skip rewrite even with close margin")
	}
}

func TestShouldEscalateRewriteDoesNotTrustReasonOnlyGrounding(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{URL: "https://example.com/entity", Score: 1.5, Reason: []string{"candidate_identity_alignment"}},
		{URL: "https://example.com/other", Score: 1.0, Reason: []string{"structure_hint"}},
	}
	if !ShouldEscalateRewrite(candidates[0].URL, candidates) {
		t.Fatal("expected qualitative reason without semantic score to escalate")
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
