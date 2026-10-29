package intel

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/josepavese/needlex/internal/config"
)

type decisionScorer struct{ scores []SemanticScore }

func (s decisionScorer) Align(context.Context, string, []SemanticCandidate) (SemanticAlignment, error) {
	return SemanticAlignment{}, nil
}

func (s decisionScorer) Score(context.Context, string, []SemanticCandidate) ([]SemanticScore, error) {
	return s.scores, nil
}

func TestDecisionSelectsWholeEvidenceUsingSignedDenseScores(t *testing.T) {
	for _, score := range []float64{0, -0.25} {
		o := localDecisionObserver{cfg: config.DecisionConfig{MaxInputBytes: 100}, semantic: decisionScorer{[]SemanticScore{{ID: "a", Similarity: -0.5}, {ID: "b", Similarity: score}, {ID: "foreign", Similarity: 1}}}}
		input := DecisionInput{Assertion: "Warranty is37months", Evidence: []DecisionEvidence{{ID: "a", Text: "Unrelated information"}, {ID: "b", Text: "保修期为37个月。"}, {ID: "large", Text: strings.Repeat("x", 200)}}}
		unit, similarity, _, err := o.selectEvidence(t.Context(), input)
		if err != nil || unit == nil || unit.ID != "b" || unit.Text != input.Evidence[1].Text || similarity == nil || *similarity != score {
			t.Fatalf("selection/truncation error: %#v %v %v", unit, similarity, err)
		}
	}
}

func TestDecisionDoesNotReplaceMissingDenseSelectionWithLexicalOverlap(t *testing.T) {
	o := localDecisionObserver{cfg: config.DecisionConfig{MaxInputBytes: 100}, semantic: decisionScorer{[]SemanticScore{{ID: "a", Similarity: math.NaN()}, {ID: "foreign", Similarity: 1}}}}
	input := DecisionInput{Assertion: "Warranty", Evidence: []DecisionEvidence{{ID: "a", Text: "Warranty warranty warranty"}, {ID: "b", Text: "37 months"}}}
	unit, _, reason, err := o.selectEvidence(t.Context(), input)
	if err != nil || unit != nil || reason != "selection_unavailable" {
		t.Fatalf("missing dense scores became a fabricated ranking: %#v %s %v", unit, reason, err)
	}
	input.Evidence[1].ID = "a"
	if _, _, _, err := o.selectEvidence(t.Context(), input); err == nil {
		t.Fatal("conflicting evidence identities were accepted")
	}
}
