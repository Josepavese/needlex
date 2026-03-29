package core

import "testing"

func TestWebIRValidateAcceptsCanonicalShape(t *testing.T) {
	ir := WebIR{
		Version:   WebIRVersion,
		SourceURL: "https://example.com",
		NodeCount: 2,
		Nodes: []WebIRNode{
			{
				Path:  "/article[1]/h1[1]",
				Tag:   "h1",
				Kind:  "heading",
				Text:  "Needle-X",
				Depth: 2,
			},
			{
				Path:  "/article[1]/p[1]",
				Tag:   "p",
				Kind:  "paragraph",
				Text:  "Deterministic-first extraction runtime.",
				Depth: 2,
			},
		},
		Signals: WebIRSignals{
			ShortTextRatio:    0.25,
			HeadingRatio:      0.50,
			EmbeddedNodeCount: 0,
		},
	}

	if err := ir.Validate(); err != nil {
		t.Fatalf("expected valid web_ir, got %v", err)
	}
}

func TestWebIRValidateRejectsInvalidShape(t *testing.T) {
	ir := WebIR{
		Version:   "wrong",
		SourceURL: "",
		NodeCount: 1,
		Nodes: []WebIRNode{
			{
				Path:  "",
				Tag:   "p",
				Kind:  "paragraph",
				Text:  "",
				Depth: 0,
			},
		},
		Signals: WebIRSignals{
			ShortTextRatio: -0.10,
			HeadingRatio:   2.0,
		},
	}

	if err := ir.Validate(); err == nil {
		t.Fatal("expected invalid web_ir to fail validation")
	}
}
