package service

import (
	"testing"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/pipeline"
)

func TestResolveProfileDefaultsToStandard(t *testing.T) {
	profile, err := resolveProfile("")
	if err != nil {
		t.Fatalf("resolve profile: %v", err)
	}
	if profile != core.ProfileStandard {
		t.Fatalf("expected standard profile, got %q", profile)
	}
}

func TestRankSegmentsBoostsObjectiveMatch(t *testing.T) {
	ranked := rankSegments("doc_1", "api auth", []pipeline.Segment{
		{
			Kind:        "paragraph",
			HeadingPath: []string{"Introduction"},
			Text:        "General overview text.",
			NodePaths:   []string{"/a"},
		},
		{
			Kind:        "paragraph",
			HeadingPath: []string{"API Auth"},
			Text:        "Authentication tokens and API keys.",
			NodePaths:   []string{"/b"},
		},
	})

	if len(ranked) != 2 {
		t.Fatalf("expected two ranked segments, got %d", len(ranked))
	}
	if ranked[0].chunk.HeadingPath[0] != "API Auth" {
		t.Fatalf("expected objective-matching segment first, got %#v", ranked[0].chunk.HeadingPath)
	}
}

func TestSelectProfileLimitsChunkCount(t *testing.T) {
	ranked := make([]rankedSegment, 7)
	for i := range ranked {
		ranked[i] = rankedSegment{
			index: i,
			chunk: core.Chunk{
				ID:          "chk",
				DocID:       "doc",
				Text:        "text",
				Score:       0.9,
				Fingerprint: "fp",
				Confidence:  0.9,
			},
		}
	}

	if got := len(selectProfile(ranked, core.ProfileTiny)); got != 2 {
		t.Fatalf("expected tiny profile to keep 2 chunks, got %d", got)
	}
	if got := len(selectProfile(ranked, core.ProfileStandard)); got != 6 {
		t.Fatalf("expected standard profile to keep 6 chunks, got %d", got)
	}
	if got := len(selectProfile(ranked, core.ProfileDeep)); got != 7 {
		t.Fatalf("expected deep profile to keep all chunks, got %d", got)
	}
}

func TestApplyContaminationPenaltyDemotesSpam(t *testing.T) {
	ranked := []rankedSegment{
		{
			index: 0,
			chunk: core.Chunk{
				ID:          "chk_1",
				DocID:       "doc",
				Text:        "casino bonus 22bet piattaforma aams",
				Score:       0.95,
				Fingerprint: "fp_1",
				Confidence:  0.90,
			},
		},
		{
			index: 1,
			chunk: core.Chunk{
				ID:          "chk_2",
				DocID:       "doc",
				Text:        "Intralogistica e gestione magazzino per aziende.",
				Score:       0.88,
				Fingerprint: "fp_2",
				Confidence:  0.90,
			},
		},
	}

	penalized := applyContaminationPenalty(ranked, "intralogistica")
	if penalized[0].chunk.Fingerprint != "fp_2" {
		t.Fatalf("expected clean segment first after contamination penalty, got %#v", penalized[0].chunk)
	}
}

func TestContaminationRiskFlagsDisabledForGamblingObjective(t *testing.T) {
	flags := contaminationRiskFlags("casino bonus 22bet payout", "casino migliori payout")
	if len(flags) != 0 {
		t.Fatalf("expected no contamination flags for gambling objective, got %#v", flags)
	}
}
