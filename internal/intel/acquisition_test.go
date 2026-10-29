package intel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/josepavese/needlex/internal/config"
)

func TestAcquisitionUsesSharedValidatedRuntimeAndSnapshotBoundMemo(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = fmt.Fprint(w, `{"data":[{"id":"Julia","meta":{"n_ctx":2048}}]}`)
		case "/tokenize":
			_, _ = fmt.Fprint(w, `{"tokens":[1,2,3]}`)
		case "/v1/systemone":
			calls.Add(1)
			var request struct {
				State     string                      `json:"state"`
				Questions map[string]decisionQuestion `json:"questions"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if !strings.Contains(request.State, "Waiting for application data") || strings.Contains(request.State, "opaque-snapshot") || strings.Contains(request.State, "baseline_action") {
				t.Error("routing input lost source text or leaked opaque identity/future outcome")
			}
			if len(request.Questions[AcquisitionQuestion].Criteria) != 3 {
				t.Error("unavailable declared-source route was offered")
			}
			_, _ = fmt.Fprint(w, `{"model":"Julia","answers":{"acquisition":{"choice":"render","probabilities":{"serve_static":0,"render":1,"insufficient":0}}}}`)
		}
	}))
	defer server.Close()
	cfg := config.DecisionConfig{Mode: "observe", BaseURL: server.URL, Model: "Julia", Revision: "pinned", TimeoutMS: 1000, MaxInputBytes: 2000, MaxQuestionTokens: 256}
	observer := NewDecisionObserver(cfg, server.Client(), nil)
	input := AcquisitionInput{Objective: "Warranty conditions", URL: "https://example.com", ContentType: "text/html", SourceHash: "opaque-snapshot-one", AllowedActions: []string{AcquisitionServe, AcquisitionRender, AcquisitionInsufficient}, Evidence: []DecisionEvidence{{ID: "unit", Text: "Waiting for application data"}}}
	first, err := observer.ObserveAcquisition(t.Context(), input)
	if err != nil || first.Status != "observed" || first.ProposedAction != AcquisitionRender || !first.ObservationOnly || first.UnassessedEvidenceCount != 0 {
		t.Fatalf("invalid observation: %#v %v", first, err)
	}
	second, err := observer.ObserveAcquisition(t.Context(), input)
	if err != nil || !second.CacheHit || calls.Load() != 1 {
		t.Fatal("same snapshot did not reuse the observer memo")
	}
	input.SourceHash = "opaque-snapshot-two"
	third, err := observer.ObserveAcquisition(t.Context(), input)
	if err != nil || third.CacheHit || calls.Load() != 2 {
		t.Fatal("changed original source reused an old routing judgment")
	}
	input.AllowedActions = []string{AcquisitionRender}
	strict, err := observer.ObserveAcquisition(t.Context(), input)
	if err != nil || strict.Status != "not_applicable" || calls.Load() != 2 || len(strict.Input.Evidence) != 0 {
		t.Fatal("strict single-route request spent inference or retained an unbounded surface")
	}
	input.AllowedActions = []string{AcquisitionServe, AcquisitionConventional}
	if _, err := observer.ObserveAcquisition(t.Context(), input); err == nil {
		t.Fatal("unmodeled conventional route entered the proposal vocabulary")
	}
}

func TestAcquisitionAdmissionAndCancellationDoNotCallRuntime(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		t.Error("inadmissible request reached runtime")
	}))
	defer server.Close()
	cfg := config.DecisionConfig{Mode: "observe", BaseURL: server.URL, Model: "Julia", Revision: "pin", TimeoutMS: 1000, MaxInputBytes: 2000, MaxQuestionTokens: 256}
	observer := NewDecisionObserver(cfg, server.Client(), nil)
	input := AcquisitionInput{Objective: "Warranty", URL: "https://example.com", AllowedActions: []string{AcquisitionServe, AcquisitionRender}, Evidence: []DecisionEvidence{{ID: "whole", Text: strings.Repeat("warranty ", 300)}}}
	result, err := observer.ObserveAcquisition(t.Context(), input)
	if err != nil || result.Status != "input_budget_exceeded" || len(result.Input.Evidence) != 0 || result.UnassessedEvidenceCount != 1 {
		t.Fatalf("whole-unit input budget was bypassed: %+v %v", result, err)
	}
	input.Evidence = []DecisionEvidence{{ID: "whole", Text: "The warranty lasts 37 months."}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := observer.ObserveAcquisition(ctx, input); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation was not honored")
	}
	input.AllowedActions = []string{AcquisitionServe, AcquisitionServe}
	if _, err := observer.ObserveAcquisition(t.Context(), input); err == nil {
		t.Fatal("duplicate choices were admitted")
	}
	if calls.Load() != 0 {
		t.Fatal("rejected observation spent native work")
	}
}
