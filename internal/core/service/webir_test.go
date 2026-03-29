package service

import (
	"testing"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/pipeline"
)

func TestBuildWebIRBuildsSignals(t *testing.T) {
	dom := pipeline.SimplifiedDOM{
		URL:   "https://example.com",
		Title: "Needle",
		Nodes: []pipeline.SimplifiedNode{
			{
				Path:  "/article[1]/h1[1]",
				Tag:   "h1",
				Kind:  "heading",
				Text:  "Needle",
				Depth: 2,
			},
			{
				Path:  "/article[1]/p[1]",
				Tag:   "p",
				Kind:  "paragraph",
				Text:  "This node has enough content to avoid short text classification in the runtime.",
				Depth: 2,
			},
			{
				Path:  "/embedded/script[1]/text[1]",
				Tag:   "script",
				Kind:  "paragraph",
				Text:  "Embedded payload summary.",
				Depth: 3,
			},
		},
	}

	ir := buildWebIR(dom)
	if ir.Version != core.WebIRVersion {
		t.Fatalf("expected web ir version %q, got %q", core.WebIRVersion, ir.Version)
	}
	if ir.NodeCount != 3 {
		t.Fatalf("expected node count 3, got %d", ir.NodeCount)
	}
	if ir.Signals.EmbeddedNodeCount != 1 {
		t.Fatalf("expected embedded node count 1, got %d", ir.Signals.EmbeddedNodeCount)
	}
	if ir.Signals.HeadingRatio <= 0 {
		t.Fatalf("expected heading ratio > 0, got %.2f", ir.Signals.HeadingRatio)
	}
	if err := ir.Validate(); err != nil {
		t.Fatalf("expected built web ir to validate, got %v", err)
	}
}
