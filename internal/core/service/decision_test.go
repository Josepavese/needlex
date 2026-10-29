package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/intel"
)

type auditObserver struct {
	input intel.DecisionInput
	fail  bool
	calls int
}

func TestDecisionSelectionFailureDoesNotCoolDownLaterRetrieval(t *testing.T) {
	var attempted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !attempted.Swap(true) {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		var request struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		vectors := make([][]float32, len(request.Input))
		for i := range vectors {
			vectors[i] = []float32{1, 0, 0}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": vectors})
	}))
	defer server.Close()
	cfg := testConfig()
	cfg.Semantic.EmbeddingURL = server.URL
	cfg.Decision = config.DecisionConfig{Mode: "observe", BaseURL: server.URL, Model: "Julia", Revision: "pin", TimeoutMS: 1000, MaxQuestionTokens: 256, MaxInputBytes: 1000}
	svc := newTestService(t, cfg, server.Client())
	_, err := svc.decisionObserver.Observe(t.Context(), intel.DecisionInput{Assertion: "audit assertion", Evidence: []intel.DecisionEvidence{{ID: "a", Text: "first audit passage"}, {ID: "b", Text: "second audit passage"}}})
	if err == nil {
		t.Fatal("failed advisory semantic request was hidden")
	}
	scores, err := svc.semantic.Score(t.Context(), "independent retrieval objective", []intel.SemanticCandidate{{ID: "new", Text: "independent retrieval passage"}})
	if err != nil || len(scores) != 1 {
		t.Fatalf("advisory failure disabled later dense retrieval: %#v %v", scores, err)
	}
}

func (o *auditObserver) Observe(_ context.Context, input intel.DecisionInput) (intel.DecisionObservation, error) {
	o.input = input
	o.calls++
	result := intel.DecisionObservation{Status: "observed", Model: "Julia", Revision: "pinned", Specification: intel.DecisionSpecification}
	if o.fail {
		return result, errors.New("runtime unavailable")
	}
	return result, nil
}

func TestReadDecisionObserverCannotChangeCompiledEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><title>Conditions</title><main><h1>Warranty</h1><p>The product warranty lasts for 37 months after purchase. Every customer can request a repair throughout this period.</p><p>Returns are accepted within 29 days after delivery. Returned items must include all supplied accessories.</p></main></html>`))
	}))
	defer server.Close()
	svc := newTestService(t, testConfig(), server.Client())
	request := ReadRequest{URL: server.URL, Objective: "Warranty and returns", Assertion: "Warranty lasts37months", Profile: core.ProfileDeep, RenderMode: "off"}
	baseline, err := svc.Read(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	for _, fail := range []bool{false, true} {
		observer := &auditObserver{fail: fail}
		svc.decisionObserver = observer
		response, err := svc.Read(t.Context(), request)
		if err != nil {
			t.Fatal(err)
		}
		if observer.calls != 1 || observer.input.Assertion != request.Assertion || len(observer.input.Evidence) != len(response.ResultPack.Chunks) {
			t.Fatal("audit lost evidence or was repeated")
		}
		if fail && response.AgentContext.Decision.Status != "unavailable" {
			t.Fatal("audit failure was hidden")
		}
		if response.AgentContext.Decision == nil || !reflect.DeepEqual(response.ResultPack.Chunks, baseline.ResultPack.Chunks) || !reflect.DeepEqual(response.ResultPack.Sources, baseline.ResultPack.Sources) || !reflect.DeepEqual(response.ProofRecords, baseline.ProofRecords) {
			t.Fatal("advisory audit changed compiled evidence or proofs")
		}
		found := false
		for _, event := range response.Trace.Events {
			if event.Stage == "decision_observation" && event.Data["observation"] != "" {
				found = true
			}
		}
		if !found {
			t.Fatal("typed observation was not retained in trace")
		}
	}
}
