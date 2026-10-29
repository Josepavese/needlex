package intel

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/josepavese/needlex/internal/config"
)

func TestDecisionMemoEvictsAndDoesNotShareCallerState(t *testing.T) {
	var memo decisionMemo
	first := sha256.Sum256([]byte("0"))
	for i := 0; i <= decisionMemoEntries; i++ {
		key := sha256.Sum256([]byte(fmt.Sprint(i)))
		memo.set(key, DecisionAnswer{Choice: "supported", Probabilities: map[string]float64{"supported": 1}})
	}
	if _, ok := memo.get(first); ok {
		t.Fatal("memo exceeded its lifetime bound")
	}
	last := sha256.Sum256([]byte(fmt.Sprint(decisionMemoEntries)))
	answer, ok := memo.get(last)
	if !ok {
		t.Fatal("latest entry was lost")
	}
	answer.Probabilities["supported"] = 0
	untouched, _ := memo.get(last)
	if untouched.Probabilities["supported"] != 1 || len(memo.values) != decisionMemoEntries {
		t.Fatal("memo storage leaked or was mutable")
	}
}

func TestDecisionCancellationKeepsAllEvidenceUnassessed(t *testing.T) {
	cfg := config.DecisionConfig{Mode: "observe", BaseURL: "http://127.0.0.1:1", Model: "Julia", Revision: "pin", TimeoutMS: 1000, MaxQuestionTokens: 256, MaxInputBytes: 1000}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	observation, err := NewDecisionObserver(cfg, nil, nil).Observe(ctx, DecisionInput{Assertion: "A claim", Evidence: []DecisionEvidence{{ID: "a", Text: "first"}, {ID: "b", Text: "second"}}})
	if err != context.Canceled || observation.UnassessedEvidenceCount != 2 || len(observation.Answers) != 0 {
		t.Fatalf("canceled work was reported as assessed: %#v %v", observation, err)
	}
}
