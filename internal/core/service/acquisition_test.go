package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/rendering"
)

type acquisitionSpy struct {
	input intel.AcquisitionInput
	calls int
}

func (s *acquisitionSpy) ObserveAcquisition(_ context.Context, input intel.AcquisitionInput) (intel.AcquisitionObservation, error) {
	s.calls++
	s.input = input
	return intel.AcquisitionObservation{Input: input, ProposedAction: intel.AcquisitionServe, ObservationOnly: true, DecisionExecution: intel.DecisionExecution{Status: "observed", Calibration: "unqualified_in_domain", Specification: intel.AcquisitionSpecification}}, nil
}

func TestAcquisitionObservationUsesOriginalSurfaceAfterOperationalRender(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<title>Application</title><main>Waiting for application data</main>`))
	}))
	defer server.Close()
	cfg := testConfig()
	cfg.Render.Enabled = true
	cfg.Agent.ReadableEnabled = false
	svc := newTestService(t, cfg, server.Client())
	svc.renderer = fakeRenderer{page: rendering.Page{URL: server.URL, FinalURL: server.URL, HTML: `<title>Conditions</title><h1>Warranty</h1><p>The complete product warranty lasts for 37 months after purchase.</p>`, Browser: "fixture cdp", FetchedAt: time.Unix(1700000000, 0)}}
	spy := &acquisitionSpy{}
	svc.acquisitionObserver = spy
	request := ReadRequest{URL: server.URL, Objective: "Warranty conditions", Profile: "deep", RenderMode: "auto"}
	baseline, err := svc.Read(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if spy.calls != 0 || baseline.AgentContext.Acquisition != nil {
		t.Fatal("ordinary read invoked acquisition observer")
	}
	request.ObserveAcquisition = true
	observed, err := svc.Read(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if spy.calls != 1 || len(spy.input.Evidence) == 0 {
		t.Fatal("missing original decision state")
	}
	for _, evidence := range spy.input.Evidence {
		if strings.Contains(evidence.Text, "37 months") {
			t.Fatal("rendered outcome leaked into the original decision state")
		}
	}
	if observed.AgentContext.Acquisition.BaselineAction != intel.AcquisitionRender || observed.AgentContext.Acquisition.ProposedAction != intel.AcquisitionServe {
		t.Fatal("proposal changed or obscured the actual operational route")
	}
	if !reflect.DeepEqual(observed.ResultPack.Chunks, baseline.ResultPack.Chunks) || !reflect.DeepEqual(observed.ProofRecords, baseline.ProofRecords) || !reflect.DeepEqual(observed.ResultPack.Sources, baseline.ResultPack.Sources) {
		t.Fatal("observation changed source content or proofs")
	}
	if observed.acquisitionPage != nil {
		t.Fatal("original raw page retained beyond its final consumer")
	}
}

func TestQueryObservesOnlyFinalOriginalSurface(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<title>Warranty</title><main><h1>Warranty</h1><p>The warranty lasts for 37 months after purchase.</p></main>`))
	}))
	defer server.Close()
	cfg := testConfig()
	cfg.Agent.ReadableEnabled = false
	svc := newTestService(t, cfg, server.Client())
	spy := &acquisitionSpy{}
	svc.acquisitionObserver = spy
	response, err := svc.Query(t.Context(), QueryRequest{Goal: "Warranty", SeedURL: server.URL, DiscoveryMode: QueryDiscoveryOff, RenderMode: "off", ObserveAcquisition: true})
	if err != nil {
		t.Fatal(err)
	}
	if spy.calls != 1 || spy.input.URL != response.Plan.SelectedURL || response.AgentContext.Acquisition == nil {
		t.Fatalf("query audit: calls=%d input=%q selected=%q observation=%+v", spy.calls, spy.input.URL, response.Plan.SelectedURL, response.AgentContext.Acquisition)
	}
}
