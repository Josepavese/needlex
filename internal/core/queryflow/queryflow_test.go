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
