package service

import (
	"slices"
	"testing"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/intel"
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
	ranked := rankSegments("doc_1", "authentication tokens api keys", core.WebIR{
		Version:   core.WebIRVersion,
		SourceURL: "https://example.com",
		NodeCount: 2,
		Nodes: []core.WebIRNode{
			{Path: "/a", Tag: "div", Kind: "container", Text: "General overview text.", Depth: 6},
			{Path: "/b", Tag: "p", Kind: "paragraph", Text: "Authentication tokens and API keys.", Depth: 2},
		},
	}, []pipeline.Segment{
		{
			Kind:        "paragraph",
			HeadingPath: nil,
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

func TestRankSegmentsUsesWebIREmbeddedEvidence(t *testing.T) {
	ranked := rankSegments("doc_1", "company profile", core.WebIR{
		Version:   core.WebIRVersion,
		SourceURL: "https://example.com",
		NodeCount: 2,
		Nodes: []core.WebIRNode{
			{Path: "/article/p[1]", Tag: "p", Kind: "paragraph", Text: "Standard paragraph.", Depth: 5},
			{Path: "/embedded/state[1]", Tag: "script", Kind: "paragraph", Text: "Embedded company profile.", Depth: 2},
		},
	}, []pipeline.Segment{
		{
			Kind:      "paragraph",
			Text:      "Standard paragraph.",
			NodePaths: []string{"/article/p[1]"},
		},
		{
			Kind:      "paragraph",
			Text:      "Embedded company profile.",
			NodePaths: []string{"/embedded/state[1]"},
		},
	})

	if len(ranked) != 2 {
		t.Fatalf("expected two ranked segments, got %d", len(ranked))
	}
	if ranked[0].chunk.Text != "Embedded company profile." {
		t.Fatalf("expected embedded IR-backed segment first, got %q", ranked[0].chunk.Text)
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

func TestApplyFingerprintNoveltyBiasPromotesNovelSegments(t *testing.T) {
	ranked := []rankedSegment{
		{index: 0, chunk: core.Chunk{Fingerprint: "fp_stable", Score: 0.90}},
		{index: 1, chunk: core.Chunk{Fingerprint: "fp_novel", Score: 0.87}},
	}
	biased := applyFingerprintNoveltyBias(ranked, []string{"fp_stable"})
	if biased[0].chunk.Fingerprint != "fp_novel" {
		t.Fatalf("expected novel chunk first, got %#v", biased)
	}
}

func TestApplyStableReadGatePreservesStableAnchor(t *testing.T) {
	ranked := []rankedSegment{
		{index: 0, chunk: core.Chunk{Fingerprint: "fp_novel", Score: 0.95}},
		{index: 1, chunk: core.Chunk{Fingerprint: "fp_stable", Score: 0.80}},
	}
	selected := applyStableReadGate(ranked, []rankedSegment{{index: 0, chunk: ranked[0].chunk}}, []string{"fp_stable"})
	if selected[0].chunk.Fingerprint != "fp_stable" {
		t.Fatalf("expected stable anchor to be preserved, got %#v", selected)
	}
}

func TestDeltaClassFromHits(t *testing.T) {
	if got := deltaClassFromHits(2, 0); got != "stable" {
		t.Fatalf("expected stable, got %q", got)
	}
	if got := deltaClassFromHits(0, 2); got != "changed" {
		t.Fatalf("expected changed, got %q", got)
	}
	if got := deltaClassFromHits(1, 1); got != "mixed" {
		t.Fatalf("expected mixed, got %q", got)
	}
}

func TestReuseMode(t *testing.T) {
	if got := reuseMode(nil); got != "fresh" {
		t.Fatalf("expected fresh, got %q", got)
	}
	if got := reuseMode([]string{"fp_1"}); got != "delta_aware" {
		t.Fatalf("expected delta_aware, got %q", got)
	}
}

func TestContaminationRiskFlagsDisabledForGamblingObjective(t *testing.T) {
	flags := contaminationRiskFlags("casino bonus 22bet payout", "casino migliori payout")
	if len(flags) != 0 {
		t.Fatalf("expected no contamination flags for gambling objective, got %#v", flags)
	}
}

func TestApplyIRSelectionPolicyInjectsEmbeddedAnchor(t *testing.T) {
	ranked := []rankedSegment{
		{
			chunk: core.Chunk{Fingerprint: "fp_top"},
			ir:    segmentIREvidence{headingBacked: true},
		},
		{
			chunk: core.Chunk{Fingerprint: "fp_embedded"},
			ir:    segmentIREvidence{embedded: true},
		},
	}
	selected := []rankedSegment{ranked[0]}
	out, report := applyIRSelectionPolicy(ranked, selected, core.WebIR{
		Signals: core.WebIRSignals{
			EmbeddedNodeCount: 2,
		},
	}, core.ProfileStandard)

	if len(out) != 1 {
		t.Fatalf("expected one selected segment, got %d", len(out))
	}
	if out[0].chunk.Fingerprint != "fp_embedded" {
		t.Fatalf("expected embedded anchor replacement, got %q", out[0].chunk.Fingerprint)
	}
	if !report.EmbeddedAnchorRequired || !report.EmbeddedAnchorApplied {
		t.Fatalf("expected embedded anchor policy to be required and applied, got %#v", report)
	}
}

func TestBuildProofRecordsIncludesWebIRProvenance(t *testing.T) {
	selected := []rankedSegment{
		{
			segment: pipeline.Segment{
				Kind:      "paragraph",
				Text:      "Needle-X runtime.",
				NodePaths: []string{"/embedded/state[1]"},
			},
			chunk: core.Chunk{
				ID:          "chk_1",
				DocID:       "doc_1",
				Text:        "Needle-X runtime.",
				Score:       0.9,
				Fingerprint: "fp_1",
				Confidence:  0.9,
			},
			ir: segmentIREvidence{
				kindMatch:        true,
				embedded:         true,
				headingBacked:    true,
				averageNodeDepth: 3,
			},
		},
	}
	records, err := buildProofRecords(selected, map[string]intel.Decision{
		"fp_1": {Lane: 0},
	})
	if err != nil {
		t.Fatalf("build proof records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one proof record, got %d", len(records))
	}

	chain := records[0].Proof.TransformChain
	for _, marker := range []string{
		"web_ir:v1",
		"web_ir:embedded:v1",
		"web_ir:heading_backed:v1",
		"web_ir:kind_match:v1",
		"web_ir:depth:shallow:v1",
		"web_ir:evidence:strong:v1",
	} {
		if !slices.Contains(chain, marker) {
			t.Fatalf("expected transform marker %q in %#v", marker, chain)
		}
	}

	flags := records[0].Proof.RiskFlags
	for _, marker := range []string{"ir_embedded", "ir_heading_backed", "ir_shallow_depth"} {
		if !slices.Contains(flags, marker) {
			t.Fatalf("expected risk flag %q in %#v", marker, flags)
		}
	}
}
