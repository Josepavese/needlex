package service

import (
	"testing"

	"github.com/josepavese/needlex/internal/core"
)

func TestDeduplicateSelectedDropsNearDuplicateChunks(t *testing.T) {
	selected := []rankedSegment{
		{
			index: 0,
			chunk: core.Chunk{
				Text: "Needle-X compiles noisy pages into compact context for agents.",
			},
		},
		{
			index: 1,
			chunk: core.Chunk{
				Text: "Needle-X compiles noisy pages into compact context for agents. with additional sentence.",
			},
		},
		{
			index: 2,
			chunk: core.Chunk{
				Text: "Replay and diff keep extraction auditable.",
			},
		},
	}

	out := deduplicateSelected(selected, nil)
	if len(out) != 2 {
		t.Fatalf("expected dedup to keep 2 chunks, got %d", len(out))
	}
}

func TestDeduplicateSelectedPrefersNovelOverStableNearDuplicate(t *testing.T) {
	selected := []rankedSegment{
		{index: 0, chunk: core.Chunk{Text: "Needle-X compiles noisy pages into compact context for agents.", Fingerprint: "fp_stable"}},
		{index: 1, chunk: core.Chunk{Text: "Needle-X compiles noisy pages into compact context for agents. with additional sentence.", Fingerprint: "fp_novel"}},
	}
	out := deduplicateSelected(selected, []string{"fp_stable"})
	if len(out) != 1 {
		t.Fatalf("expected one chunk after dedup, got %d", len(out))
	}
	if out[0].chunk.Fingerprint != "fp_novel" {
		t.Fatalf("expected novel chunk to replace stable duplicate, got %q", out[0].chunk.Fingerprint)
	}
}

func TestNormalizeOutlineLabelCompactsBreadcrumbNoise(t *testing.T) {
	label := normalizeOutlineLabel([]string{
		"Diamo forma alle tue idee. > GR > Design emozionale",
		"Diamo forma alle tue idee. > GR > Design emozionale",
		"Innovazione e tecnologia",
	})
	if label != "GR > Design emozionale > Innovazione e tecnologia" {
		t.Fatalf("unexpected normalized outline label %q", label)
	}
}

func TestApplyPartialSelectionReusePromotesStableSubset(t *testing.T) {
	ranked := []rankedSegment{
		{index: 0, chunk: core.Chunk{Fingerprint: "fp_novel_1", Score: 0.99}},
		{index: 1, chunk: core.Chunk{Fingerprint: "fp_keep", Score: 0.98}},
		{index: 2, chunk: core.Chunk{Fingerprint: "fp_keep_2", Score: 0.97}},
	}
	selected := []rankedSegment{
		{index: 1, chunk: core.Chunk{Fingerprint: "fp_keep", Score: 0.98}},
		{index: 2, chunk: core.Chunk{Fingerprint: "fp_keep_2", Score: 0.97}},
		{index: 0, chunk: core.Chunk{Fingerprint: "fp_novel_1", Score: 0.99}},
	}
	out, eligible, reused, reusedSet := applyPartialSelectionReuse(ranked, selected, []string{"fp_keep", "fp_keep_2"})
	if eligible != 2 || reused != 1 {
		t.Fatalf("expected eligible=2 reused=1, got eligible=%d reused=%d", eligible, reused)
	}
	if _, ok := reusedSet["fp_keep"]; !ok || out[0].chunk.Fingerprint != "fp_keep" {
		t.Fatalf("expected stable fingerprint to be reused first, got %#v reused=%#v", out, reusedSet)
	}
}

func TestApplyPartialSelectionReuseSkipsMixedSelection(t *testing.T) {
	ranked := []rankedSegment{
		{index: 0, chunk: core.Chunk{Fingerprint: "fp_stable_1", Score: 0.98}},
		{index: 1, chunk: core.Chunk{Fingerprint: "fp_stable_2", Score: 0.97}},
		{index: 2, chunk: core.Chunk{Fingerprint: "fp_novel_1", Score: 0.96}},
		{index: 3, chunk: core.Chunk{Fingerprint: "fp_novel_2", Score: 0.95}},
		{index: 4, chunk: core.Chunk{Fingerprint: "fp_novel_3", Score: 0.94}},
		{index: 5, chunk: core.Chunk{Fingerprint: "fp_novel_4", Score: 0.93}},
	}
	selected := []rankedSegment{
		{index: 0, chunk: core.Chunk{Fingerprint: "fp_stable_1", Score: 0.98}},
		{index: 2, chunk: core.Chunk{Fingerprint: "fp_novel_1", Score: 0.96}},
		{index: 3, chunk: core.Chunk{Fingerprint: "fp_novel_2", Score: 0.95}},
		{index: 4, chunk: core.Chunk{Fingerprint: "fp_novel_3", Score: 0.94}},
		{index: 5, chunk: core.Chunk{Fingerprint: "fp_novel_4", Score: 0.93}},
	}
	out, eligible, reused, reusedSet := applyPartialSelectionReuse(ranked, selected, []string{"fp_stable_1", "fp_stable_2"})
	if eligible != 2 || reused != 0 || len(reusedSet) != 0 {
		t.Fatalf("expected eligible=2 reused=0 with empty reused set, got eligible=%d reused=%d set=%#v", eligible, reused, reusedSet)
	}
	if len(out) != len(selected) {
		t.Fatalf("expected selection unchanged, got %d vs %d", len(out), len(selected))
	}
}
