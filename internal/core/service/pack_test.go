package service

import (
	"slices"
	"testing"

	"github.com/josepavese/needlex/internal/config"
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
	svc, err := New(config.Defaults(), nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ranked := svc.rankSegments("doc_1", "authentication tokens api keys", core.WebIR{
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
	svc, err := New(config.Defaults(), nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ranked := svc.rankSegments("doc_1", "company profile", core.WebIR{
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

func TestApplyBoilerplatePenaltyDemotesHeadinglessMenuChunks(t *testing.T) {
	ranked := []rankedSegment{
		{
			segment: pipeline.Segment{
				Kind:        "paragraph",
				HeadingPath: []string{"SQLite Home Page"},
				Text:        "Home Menu About Documentation Download License Support Purchase Search",
				NodePaths:   []string{"/nav/a[1]", "/nav/a[2]", "/nav/a[3]", "/nav/a[4]", "/nav/a[5]", "/nav/a[6]"},
			},
			chunk: core.Chunk{
				ID:          "chk_menu",
				DocID:       "doc",
				Text:        "Home Menu About Documentation Download License Support Purchase Search",
				Score:       0.92,
				Fingerprint: "fp_menu",
				Confidence:  0.90,
			},
		},
		{
			segment: pipeline.Segment{
				Kind:      "paragraph",
				Text:      "SQLite is a C-language library that implements a small, fast SQL database engine.",
				NodePaths: []string{"/article/p[1]"},
			},
			chunk: core.Chunk{
				ID:          "chk_core",
				DocID:       "doc",
				Text:        "SQLite is a C-language library that implements a small, fast SQL database engine.",
				Score:       0.88,
				Fingerprint: "fp_core",
				Confidence:  0.88,
			},
		},
	}

	penalized := applyBoilerplatePenalty(ranked)
	if penalized[0].chunk.Fingerprint != "fp_core" {
		t.Fatalf("expected content chunk first after boilerplate penalty, got %#v", penalized[0].chunk)
	}
}

func TestApplySubordinateFragmentDemotionDemotesShortDeepTail(t *testing.T) {
	ranked := []rankedSegment{
		{
			segment: pipeline.Segment{
				Kind:        "paragraph",
				HeadingPath: []string{"SQLite Home Page"},
				Text:        "SQLite is a C-language library that implements a small, fast SQL database engine used across mobile phones and countless applications every day.",
				NodePaths:   []string{"/article/p[1]"},
			},
			chunk: core.Chunk{
				ID:          "chk_anchor",
				DocID:       "doc",
				Text:        "SQLite is a C-language library that implements a small, fast SQL database engine used across mobile phones and countless applications every day.",
				Score:       0.92,
				Fingerprint: "fp_anchor",
				Confidence:  0.92,
			},
			ir: segmentIREvidence{headingBacked: true},
		},
		{
			segment: pipeline.Segment{
				Kind:        "paragraph",
				HeadingPath: []string{"Latest Release", "Common Links", "Sponsors"},
				Text:        "SQLite is made possible in part by sponsors and consortium members. This page was last updated on 2026-03-16 20:07:10Z.",
				NodePaths:   []string{"/aside/p[1]"},
			},
			chunk: core.Chunk{
				ID:          "chk_tail",
				DocID:       "doc",
				Text:        "SQLite is made possible in part by sponsors and consortium members. This page was last updated on 2026-03-16 20:07:10Z.",
				Score:       0.89,
				Fingerprint: "fp_tail",
				Confidence:  0.88,
			},
		},
	}
	originalTailScore := ranked[1].chunk.Score

	penalized := applySubordinateFragmentDemotion(ranked)
	if penalized[0].chunk.Fingerprint != "fp_anchor" {
		t.Fatalf("expected explanatory anchor to stay first, got %#v", penalized)
	}
	if penalized[1].chunk.Score >= originalTailScore {
		t.Fatalf("expected subordinate fragment score to drop, got %.2f", penalized[1].chunk.Score)
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

func TestSortRankedSegmentsOrdersByFinalScore(t *testing.T) {
	selected := []rankedSegment{
		{index: 0, chunk: core.Chunk{Fingerprint: "fp_low", Score: 0.64}},
		{index: 1, chunk: core.Chunk{Fingerprint: "fp_high", Score: 0.90}},
	}
	sortRankedSegments(selected)
	if selected[0].chunk.Fingerprint != "fp_high" {
		t.Fatalf("expected highest-score chunk first, got %#v", selected)
	}
}

func TestApplyIndexLikeDemotionDemotesDocsIndexWhenAnchorExists(t *testing.T) {
	ranked := []rankedSegment{
		{
			segment: pipeline.Segment{
				Kind:        "paragraph",
				HeadingPath: []string{"SQLite Home Page"},
				Text:        "SQLite is a C-language library that implements a small, fast SQL database engine. It is widely deployed in mobile devices and applications.",
				NodePaths:   []string{"/article/p[1]", "/article/p[2]"},
			},
			chunk: core.Chunk{
				ID:          "chk_anchor",
				DocID:       "doc",
				Text:        "SQLite is a C-language library that implements a small, fast SQL database engine. It is widely deployed in mobile devices and applications.",
				Score:       0.95,
				Fingerprint: "fp_anchor",
				Confidence:  0.92,
			},
		},
		{
			segment: pipeline.Segment{
				Kind:        "list_item",
				HeadingPath: []string{"Latest Release", "Common Links"},
				Text:        "Features\nWhen to use SQLite\nGetting Started\nTry it live!\nSQL Syntax\nPragmas\nJSON functions\nFrequently Asked Questions\nNews",
				NodePaths:   []string{"/ul/li[1]", "/ul/li[2]", "/ul/li[3]", "/ul/li[4]", "/ul/li[5]", "/ul/li[6]", "/ul/li[7]", "/ul/li[8]", "/ul/li[9]"},
			},
			chunk: core.Chunk{
				ID:          "chk_index",
				DocID:       "doc",
				Text:        "Features\nWhen to use SQLite\nGetting Started\nTry it live!\nSQL Syntax\nPragmas\nJSON functions\nFrequently Asked Questions\nNews",
				Score:       0.90,
				Fingerprint: "fp_index",
				Confidence:  0.90,
			},
		},
		{
			segment: pipeline.Segment{
				Kind:        "paragraph",
				HeadingPath: []string{"Latest Release", "Sponsors"},
				Text:        "SQLite is made possible in part by sponsors and consortium members.",
				NodePaths:   []string{"/p[3]"},
			},
			chunk: core.Chunk{
				ID:          "chk_sponsor",
				DocID:       "doc",
				Text:        "SQLite is made possible in part by sponsors and consortium members.",
				Score:       0.84,
				Fingerprint: "fp_sponsor",
				Confidence:  0.86,
			},
		},
	}

	penalized := applyIndexLikeDemotion(ranked)
	if penalized[0].chunk.Fingerprint != "fp_anchor" {
		t.Fatalf("expected explanatory anchor to remain first, got %#v", penalized)
	}
	if penalized[2].chunk.Fingerprint != "fp_index" {
		t.Fatalf("expected index-like chunk to be demoted below sponsor chunk, got %#v", penalized)
	}
}

func TestApplyCodeLikeDemotionDemotesIdentifierHeavyChunksWhenAnchorExists(t *testing.T) {
	ranked := []rankedSegment{
		{
			segment: pipeline.Segment{
				Kind:        "paragraph",
				HeadingPath: []string{"Usage", "Advantages"},
				Text:        "Access to high-intelligence coding models and support for multiple coding tools. The plan is designed for practical development workflows and complex task execution.",
				NodePaths:   []string{"/article/p[1]", "/article/p[2]"},
			},
			chunk: core.Chunk{
				ID:          "chk_anchor",
				DocID:       "doc",
				Text:        "Access to high-intelligence coding models and support for multiple coding tools. The plan is designed for practical development workflows and complex task execution.",
				Score:       0.92,
				Fingerprint: "fp_anchor",
				Confidence:  0.92,
			},
		},
		{
			segment: pipeline.Segment{
				Kind:        "paragraph",
				HeadingPath: []string{"Usage", "How to Switch Models"},
				Text:        "ANTHROPIC_DEFAULT_OPUS_MODEL : GLM-4.7\nANTHROPIC_DEFAULT_SONNET_MODEL : GLM-4.7\nANTHROPIC_DEFAULT_HAIKU_MODEL : GLM-4.5-Air",
				NodePaths:   []string{"/ul/li[1]", "/ul/li[2]", "/ul/li[3]"},
			},
			chunk: core.Chunk{
				ID:          "chk_code",
				DocID:       "doc",
				Text:        "ANTHROPIC_DEFAULT_OPUS_MODEL : GLM-4.7\nANTHROPIC_DEFAULT_SONNET_MODEL : GLM-4.7\nANTHROPIC_DEFAULT_HAIKU_MODEL : GLM-4.5-Air",
				Score:       0.95,
				Fingerprint: "fp_code",
				Confidence:  0.92,
			},
		},
	}

	penalized := applyCodeLikeDemotion(ranked)
	if penalized[0].chunk.Fingerprint != "fp_anchor" {
		t.Fatalf("expected explanatory anchor to outrank code-like chunk, got %#v", penalized)
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
