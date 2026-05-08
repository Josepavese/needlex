package webdiscover

import (
	"math"
	"testing"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
)

func TestDampenWeakProvenanceTrapsDoesNotTreatGoalSimilarityAsFamilyEvidence(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{
			URL:   "https://related.example/",
			Score: 3.20,
			Reason: []string{
				"semantic_goal_alignment",
				"same_host_canonical_root",
				"same_family_canonical_root",
			},
		},
		{
			URL:   "https://origin.example/about",
			Score: 2.80,
			Reason: []string{
				"semantic_goal_alignment",
				"host_root_identity_probe",
			},
		},
	}

	got := DampenWeakProvenanceTraps(candidates)
	if got[0].URL != "https://origin.example/about" {
		t.Fatalf("expected provenanced family to beat semantically related root, got %#v", got)
	}
	if !hasTestReason(got[1].Reason, "weak_canonical_root_context_penalty") {
		t.Fatalf("expected weak canonical root penalty, got %#v", got[1].Reason)
	}
}

func TestCanonicalizeCandidateFamiliesCapsRepeatedRootBoost(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{URL: "https://project.example/docs", Score: 1.20},
		{URL: "https://project.example/download", Score: 1.18},
		{URL: "https://project.example/about", Score: 1.16},
		{URL: "https://project.example/", Score: 1.00},
	}

	got := CanonicalizeCandidateFamilies(candidates)
	var root discoverycore.Candidate
	for _, candidate := range got {
		if candidate.URL == "https://project.example/" {
			root = candidate
			break
		}
	}
	if root.URL == "" {
		t.Fatalf("expected root candidate in output: %#v", got)
	}
	if math.Abs(root.Score-1.28) > 0.0001 {
		t.Fatalf("expected one capped root boost, got score %.2f", root.Score)
	}
}

func TestDampenCrossFamilyMirrorRoutesPenalizesEmbeddedOriginHost(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{
			URL:    "https://mirror.example/web/origin.example/docs/guide.html",
			Score:  2.80,
			Reason: []string{"page_title_probe", "web_ir_probe"},
		},
		{
			URL:    "https://origin.example/docs/",
			Score:  2.50,
			Reason: []string{"host_root_identity_probe"},
		},
	}

	got := DampenCrossFamilyMirrorRoutes(candidates)
	if got[0].URL != "https://origin.example/docs/" {
		t.Fatalf("expected origin family to beat embedded-host mirror, got %#v", got)
	}
	if !hasTestReason(got[1].Reason, "cross_family_mirror_route_penalty") {
		t.Fatalf("expected mirror route penalty, got %#v", got[1].Reason)
	}
}

func TestDampenCrossFamilyMirrorRoutesPenalizesDescendantRouteMirror(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{
			URL:    "https://mirror.example/en-US/docs/guide",
			Score:  2.80,
			Reason: []string{"page_title_probe", "web_ir_probe"},
		},
		{
			URL:    "https://origin.example/en-US/",
			Score:  2.55,
			Reason: []string{"host_root_identity_probe"},
		},
	}

	got := DampenCrossFamilyMirrorRoutes(candidates)
	if got[0].URL != "https://origin.example/en-US/" {
		t.Fatalf("expected origin route to beat descendant mirror, got %#v", got)
	}
}

func TestDampenCrossFamilyMirrorRoutesDoesNotPenalizeDeepPageForUnrelatedRoot(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{
			URL:    "https://origin.example/en-US/docs/guide",
			Score:  2.50,
			Reason: []string{"page_title_probe", "web_ir_probe"},
		},
		{
			URL:    "https://other.example/",
			Score:  2.10,
			Reason: []string{"host_root_identity_probe"},
		},
	}

	got := DampenCrossFamilyMirrorRoutes(candidates)
	if hasTestReason(got[0].Reason, "cross_family_mirror_route_penalty") {
		t.Fatalf("expected unrelated root not to penalize deep official page, got %#v", got)
	}
}

func TestPromoteRecoveredCanonicalOriginsBoostsRecoveredRoot(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{
			URL:    "https://community.example/wiki/",
			Score:  2.70,
			Reason: []string{"host_root_identity_probe"},
		},
		{
			URL:    "https://origin.example/",
			Score:  2.55,
			Reason: []string{"external_family_recovery", "page_expand", "semantic_evidence_probe"},
		},
	}

	got := PromoteRecoveredCanonicalOrigins(candidates)
	if got[0].URL != "https://origin.example/" {
		t.Fatalf("expected recovered canonical origin to win, got %#v", got)
	}
	if !hasTestReason(got[0].Reason, "recovered_canonical_origin") {
		t.Fatalf("expected recovered canonical origin reason, got %#v", got[0].Reason)
	}
}

func TestPromoteRecoveredCanonicalOriginsRequiresSemanticGrounding(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{
			URL:    "https://unrelated.example/",
			Score:  2.70,
			Reason: []string{"external_family_recovery", "page_expand"},
		},
		{
			URL:    "https://origin.example/docs",
			Score:  2.60,
			Reason: []string{"host_root_identity_probe"},
		},
	}

	got := PromoteRecoveredCanonicalOrigins(candidates)
	if got[0].URL != "https://unrelated.example/" && hasTestReason(got[0].Reason, "recovered_canonical_origin") {
		t.Fatalf("unexpected recovered canonical origin boost without semantic grounding: %#v", got)
	}
	for _, candidate := range got {
		if candidate.URL == "https://unrelated.example/" && hasTestReason(candidate.Reason, "recovered_canonical_origin") {
			t.Fatalf("unexpected recovered canonical origin reason without semantic grounding: %#v", candidate)
		}
	}
}

func TestDampenWeakProvenanceTrapsIsIdempotent(t *testing.T) {
	candidates := []discoverycore.Candidate{
		{
			URL:    "https://weak.example/path",
			Score:  2.70,
			Reason: []string{"same_family_child_recovery", "page_expand_child_context", "weak_recovered_family_context_penalty"},
		},
		{
			URL:    "https://origin.example/docs",
			Score:  2.50,
			Reason: []string{"host_root_identity_probe"},
		},
	}

	got := DampenWeakProvenanceTraps(candidates)
	if got[0].URL != "https://weak.example/path" || got[0].Score != 2.70 {
		t.Fatalf("expected existing penalty not to be applied twice, got %#v", got)
	}
}
