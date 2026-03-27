package service

import (
	"strings"
	"testing"
)

func TestCompactTinyTextPreservesObjectiveTerms(t *testing.T) {
	compacted, changed := compactTinyText(
		"The runtime reduces HTML into a stable intermediate representation before ranking and packing.",
		"stable ranking packing",
	)
	if !changed {
		t.Fatal("expected tiny compaction to change the text")
	}
	for _, token := range []string{"stable", "ranking", "packing"} {
		if !strings.Contains(strings.ToLower(compacted), token) {
			t.Fatalf("expected compacted text to keep %q, got %q", token, compacted)
		}
	}
}

func TestCompactTinyTextFallsBackForShortContent(t *testing.T) {
	compacted, changed := compactTinyText("Proof replay deterministic context.", "proof replay")
	if changed {
		t.Fatalf("expected short content to remain unchanged, got %q", compacted)
	}
	if compacted != "Proof replay deterministic context." {
		t.Fatalf("unexpected fallback text %q", compacted)
	}
}
