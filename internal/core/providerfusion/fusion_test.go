package providerfusion

import (
	"testing"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
)

func TestApplyBoostsOnlySemanticClusterWithProviderDiversity(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{URL: "https://a.example", Score: 1.00, Metadata: map[string]string{"cluster_id": "cluster_1", "provider_observations": "provider_a"}},
		{URL: "https://b.example", Score: 0.92, Metadata: map[string]string{"cluster_id": "cluster_1", "provider_observations": "provider_b"}},
		{URL: "https://c.example", Score: 0.91, Metadata: map[string]string{"cluster_id": "cluster_2", "provider_observations": "provider_a,provider_b"}},
	}
	got := Apply(candidates)
	if !containsReason(got[0].Reason, ReasonSemanticQuorum) {
		t.Fatalf("expected semantic quorum reason on supported cluster, got %#v", got)
	}
	for _, candidate := range got {
		if candidate.URL == "https://c.example" && containsReason(candidate.Reason, ReasonSemanticQuorum) {
			t.Fatalf("singleton cluster must not receive quorum boost: %#v", candidate)
		}
	}
}

func TestAnnotateProviderMergesThroughDiscoverySet(t *testing.T) {
	left := AnnotateProvider([]discoverycore.Candidate{{URL: "https://example.com", Score: 1}}, "provider_a")
	right := AnnotateProvider([]discoverycore.Candidate{{URL: "https://example.com", Score: 2}}, "provider_b")
	set := discoverycore.NewSet(left)
	set.Merge(right)
	got := set.Sorted()[0]
	if got.Metadata["provider_observations"] != "provider_a,provider_b" {
		t.Fatalf("expected provider observations to merge, got %#v", got.Metadata)
	}
}

func TestApplyBoostsProviderConsensusWithoutClusterID(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{URL: "https://origin.example/docs", Score: 0.90, Metadata: map[string]string{"provider_observations": "provider_a,provider_b"}},
		{URL: "https://other.example/docs", Score: 0.92, Metadata: map[string]string{"provider_observations": "provider_a"}},
	}
	got := Apply(candidates)
	if got[0].URL != "https://origin.example/docs" {
		t.Fatalf("expected provider consensus candidate to win, got %#v", got)
	}
	if !containsReason(got[0].Reason, ReasonProviderConsensus) {
		t.Fatalf("expected provider consensus reason, got %#v", got[0].Reason)
	}
}

func containsReason(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
