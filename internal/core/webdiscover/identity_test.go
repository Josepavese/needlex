package webdiscover

import (
	"testing"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
)

func TestIdentityBaseLinksSkipsCanonicalEquivalentSource(t *testing.T) {
	links, _ := IdentityBaseLinks("https://example.com/docs/index.html", []IdentityReferenceCandidate{
		{URL: "https://www.example.com/docs/", Label: "Example Docs", Relation: "canonical"},
		{URL: "https://example.com/reference", Label: "Example Reference", Relation: "alternate"},
	})

	if len(links) != 1 {
		t.Fatalf("expected one non-self identity link, got %#v", links)
	}
	if links[0].URL != "https://example.com/reference" {
		t.Fatalf("expected alternate link to remain, got %#v", links)
	}
}

func TestIdentityDiscoverCandidatesKeepsSameFamilyIdentitySubordinate(t *testing.T) {
	source := discoverycore.Candidate{URL: "https://example.com/docs/page", Label: "Example Docs"}
	scored := []discoverycore.Candidate{
		{URL: "https://example.com/docs/", Label: "Example Docs", Score: 1.0},
	}

	got := IdentityDiscoverCandidates(source, scored, map[string]string{
		"https://example.com/docs/": "canonical",
	}, map[string]float64{
		"https://example.com/docs/": 0.50,
	})

	if len(got) != 1 {
		t.Fatalf("expected same-family identity candidate, got %#v", got)
	}
	if hasTestReason(got[0].Reason, "external_family_recovery") {
		t.Fatalf("same-family identity must not be external recovery, got %#v", got[0].Reason)
	}
	if got[0].Metadata["identity_reference_scope"] != "same_family" {
		t.Fatalf("expected same-family scope metadata, got %#v", got[0].Metadata)
	}
	if got[0].Score >= 1.40 {
		t.Fatalf("same-family identity boost should remain subordinate, got score %.3f", got[0].Score)
	}
}

func TestIdentityDiscoverCandidatesRequiresSemanticGroundingForExternalAlternateFamily(t *testing.T) {
	source := discoverycore.Candidate{URL: "https://mirror.example/page", Label: "Mirror"}
	scored := []discoverycore.Candidate{
		{URL: "https://origin.example/page", Label: "Origin", Score: 1.0},
	}
	relations := map[string]string{"https://origin.example/page": "alternate"}

	if got := IdentityDiscoverCandidates(source, scored, relations, map[string]float64{
		"https://origin.example/page": 0.10,
	}); len(got) != 0 {
		t.Fatalf("expected ungrounded external identity to be rejected, got %#v", got)
	}

	got := IdentityDiscoverCandidates(source, scored, relations, map[string]float64{
		"https://origin.example/page": 0.42,
	})
	if len(got) != 1 {
		t.Fatalf("expected grounded external identity candidate, got %#v", got)
	}
	if !hasTestReason(got[0].Reason, "external_family_recovery") {
		t.Fatalf("expected external recovery reason, got %#v", got[0].Reason)
	}
	if got[0].Metadata["identity_reference_scope"] != "external_family" {
		t.Fatalf("expected external-family scope metadata, got %#v", got[0].Metadata)
	}
}

func hasTestReason(reasons []string, wanted string) bool {
	for _, reason := range reasons {
		if reason == wanted {
			return true
		}
	}
	return false
}
