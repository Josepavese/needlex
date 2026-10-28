package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/intel"
)

type auditObserver struct {
	input intel.DecisionInput
	fail  bool
	calls int
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
	request := ReadRequest{URL: server.URL, Objective: "Warranty and returns", Profile: core.ProfileDeep, RenderMode: "off"}
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
		if observer.calls != 1 || observer.input.Objective != request.Objective || len(observer.input.Evidence) != len(response.ResultPack.Chunks) {
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
