package intel

import (
	"context"
	"errors"
	"testing"
	"time"
)

type failingSemanticAligner struct {
	calls int
}

func (f *failingSemanticAligner) Align(context.Context, string, []SemanticCandidate) (SemanticAlignment, error) {
	f.calls++
	return SemanticAlignment{}, errors.New("upstream unavailable")
}

func (f *failingSemanticAligner) Score(context.Context, string, []SemanticCandidate) ([]SemanticScore, error) {
	f.calls++
	return nil, errors.New("upstream unavailable")
}

func TestResilientSemanticAlignerTripsCooldownOnFailure(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	inner := &failingSemanticAligner{}
	aligner := &resilientSemanticAligner{
		inner:    inner,
		now:      func() time.Time { return now },
		cooldown: 5 * time.Second,
	}

	scores, err := aligner.Score(context.Background(), "goal", []SemanticCandidate{{ID: "a", Text: "alpha"}})
	if err != nil {
		t.Fatalf("expected error to be swallowed, got %v", err)
	}
	if scores != nil {
		t.Fatalf("expected nil scores on failure, got %#v", scores)
	}
	if inner.calls != 1 {
		t.Fatalf("expected one upstream call, got %d", inner.calls)
	}

	_, _ = aligner.Score(context.Background(), "goal", []SemanticCandidate{{ID: "a", Text: "alpha"}})
	if inner.calls != 1 {
		t.Fatalf("expected cooldown to suppress second call, got %d", inner.calls)
	}

	now = now.Add(6 * time.Second)
	_, _ = aligner.Score(context.Background(), "goal", []SemanticCandidate{{ID: "a", Text: "alpha"}})
	if inner.calls != 2 {
		t.Fatalf("expected call after cooldown expiry, got %d", inner.calls)
	}
}
