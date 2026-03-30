package service

import (
	"testing"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/pipeline"
)

func TestBuildWebIRBuildsSignals(t *testing.T) {
	dom := pipeline.SimplifiedDOM{
		URL:            "https://example.com",
		Title:          "Needle",
		SubstrateClass: "embedded_app_payload",
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
	if ir.Signals.SubstrateClass != "embedded_app_payload" {
		t.Fatalf("expected substrate class to be propagated, got %q", ir.Signals.SubstrateClass)
	}
	if ir.Signals.HeadingRatio <= 0 {
		t.Fatalf("expected heading ratio > 0, got %.2f", ir.Signals.HeadingRatio)
	}
	if err := ir.Validate(); err != nil {
		t.Fatalf("expected built web ir to validate, got %v", err)
	}
}

func TestEnsureMinimumDOMSynthesizesTitleNodes(t *testing.T) {
	dom := ensureMinimumDOM(pipeline.SimplifiedDOM{
		URL:   "https://example.com/forum",
		Title: "Discourse Meta",
	})
	if len(dom.Nodes) != 2 {
		t.Fatalf("expected 2 synthetic nodes, got %#v", dom.Nodes)
	}
	if dom.Nodes[0].Kind != "heading" || dom.Nodes[1].Kind != "paragraph" {
		t.Fatalf("unexpected synthetic node kinds %#v", dom.Nodes)
	}
	ir := buildWebIR(dom)
	if err := ir.Validate(); err != nil {
		t.Fatalf("expected synthetic dom to produce valid web ir, got %v", err)
	}
}
