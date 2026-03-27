package core

import (
	"testing"
	"time"
)

func TestResultPackValidateAcceptsCanonicalShape(t *testing.T) {
	pack := ResultPack{
		Query:   "needle x architecture",
		Profile: ProfileStandard,
		Chunks: []Chunk{
			{
				ID:          "chk_1",
				DocID:       "doc_1",
				Text:        "High signal context.",
				HeadingPath: []string{"Overview"},
				Score:       0.92,
				Fingerprint: "fp_1",
				Confidence:  0.95,
			},
		},
		Sources: []SourceRef{
			{
				DocumentID: "doc_1",
				URL:        "https://example.com",
				ChunkIDs:   []string{"chk_1"},
			},
		},
		ProofRefs: []string{"proof_1"},
		CostReport: CostReport{
			LatencyMS: 42,
			TokenIn:   120,
			TokenOut:  40,
			LanePath:  []int{0, 1},
		},
	}

	if err := pack.Validate(); err != nil {
		t.Fatalf("expected valid result pack, got %v", err)
	}
}

func TestResultPackValidateRejectsUnknownProfile(t *testing.T) {
	pack := ResultPack{
		Query:   "needle x architecture",
		Profile: "weird",
		Chunks: []Chunk{
			{
				ID:          "chk_1",
				DocID:       "doc_1",
				Text:        "High signal context.",
				Score:       0.92,
				Fingerprint: "fp_1",
				Confidence:  0.95,
			},
		},
		Sources: []SourceRef{
			{
				DocumentID: "doc_1",
				URL:        "https://example.com",
			},
		},
		CostReport: CostReport{
			LatencyMS: 42,
			LanePath:  []int{0},
		},
	}

	if err := pack.Validate(); err == nil {
		t.Fatal("expected invalid profile to fail validation")
	}
}

func TestProofValidateRejectsInvalidLane(t *testing.T) {
	proof := Proof{
		ChunkID: "chk_1",
		SourceSpan: SourceSpan{
			Selector:  "article p",
			CharStart: 0,
			CharEnd:   12,
		},
		TransformChain: []string{"reduce:v1"},
		Lane:           9,
	}

	if err := proof.Validate(); err == nil {
		t.Fatal("expected invalid lane error")
	}
}

func TestRunContextValidateRejectsMissingIDs(t *testing.T) {
	ctx := RunContext{
		StartedAt: time.Now().UTC(),
		LaneMax:   1,
		Budget: Budget{
			MaxTokens:    1,
			MaxLatencyMS: 1,
			MaxPages:     1,
			MaxDepth:     1,
			MaxBytes:     1,
		},
	}

	if err := ctx.Validate(); err == nil {
		t.Fatal("expected invalid run context")
	}
}
