package sourceresolution

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/pipeline"
	"github.com/josepavese/needlex/internal/proof"
	"github.com/josepavese/needlex/internal/rendering"
)

type policyScorer struct {
	scores []intel.SemanticScore
	calls  int
}

func (s *policyScorer) Align(context.Context, string, []intel.SemanticCandidate) (intel.SemanticAlignment, error) {
	return intel.SemanticAlignment{}, nil
}

func (s *policyScorer) Score(context.Context, string, []intel.SemanticCandidate) ([]intel.SemanticScore, error) {
	s.calls++
	return s.scores, nil
}

func TestSemanticRenderGapAcceptsMultilingualObjectivesAndSignedScores(t *testing.T) {
	for _, objective := range []string{"Prezzi", "请查找这个产品在意大利的保修条件及退货政策"} {
		for _, score := range []float64{-0.25, 0, 0.3} {
			scorer := &policyScorer{scores: []intel.SemanticScore{{ID: "static_surface", Similarity: score}}}
			r := Resolver{Semantic: scorer}
			got, gap := r.semanticRenderGap(t.Context(), Request{Objective: objective}, pipeline.SimplifiedDOM{Nodes: []pipeline.SimplifiedNode{{Text: "A different topic"}}})
			if !gap || got != score || scorer.calls != 1 {
				t.Fatalf("objective %q score %v: got (%v,%v), calls %d", objective, score, got, gap, scorer.calls)
			}
		}
	}
}

func TestSemanticRenderGapDistinguishesUnavailableFromZero(t *testing.T) {
	for _, scores := range [][]intel.SemanticScore{nil, {{ID: "foreign", Similarity: 0}}, {{ID: "static_surface", Similarity: math.NaN()}}, {{ID: "static_surface", Similarity: math.Inf(-1)}}, {{ID: "static_surface", Similarity: 0.8}}} {
		r := Resolver{Semantic: &policyScorer{scores: scores}}
		if _, gap := r.semanticRenderGap(t.Context(), Request{Objective: "Warranty"}, pipeline.SimplifiedDOM{Nodes: []pipeline.SimplifiedNode{{Text: "Evidence"}}}); gap {
			t.Fatalf("unexpected gap for %#v", scores)
		}
	}
	scorer := &policyScorer{}
	r := Resolver{Semantic: scorer}
	r.semanticRenderGap(t.Context(), Request{}, pipeline.SimplifiedDOM{})
	if scorer.calls != 0 {
		t.Fatal("an absent objective must not request a semantic score")
	}
}

type policyRenderer struct{}

func (policyRenderer) Render(context.Context, rendering.Request) (rendering.Page, error) {
	return rendering.Page{HTML: "<title>Materialized</title><p>Evidence</p>", Browser: "test cdp", FinalURL: "https://example.com", FetchedAt: time.Now()}, nil
}

func TestRenderMetadataPreservesZeroAndNegativeSemanticScores(t *testing.T) {
	for _, score := range []float64{0, -0.25} {
		recorder := proof.NewRecorder("run-policy", "trace-policy", time.Now())
		r := Resolver{Config: config.Defaults(), Renderer: policyRenderer{}}
		_, err := r.renderPage(t.Context(), recorder, Request{Objective: "Warranty"}, pipeline.RawPage{URL: "https://example.com", FinalURL: "https://example.com"}, "auto", []string{semanticRenderEscalationReason}, score)
		if err != nil {
			t.Fatal(err)
		}
		trace := recorder.Finish(time.Now())
		if err := trace.Validate(); err != nil {
			t.Fatal(err)
		}
		metadata := trace.Stages[0].Metadata
		if metadata["semantic_gap_similarity"] != fmt.Sprintf("%.4f", score) || metadata["semantic_gap_threshold"] != "0.5000" {
			t.Fatalf("lost signed score or calibration: %#v", metadata)
		}
	}
}
