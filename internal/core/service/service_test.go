package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/proof"
)

const testHTML = `
<html>
  <head><title>Needle Runtime</title></head>
  <body>
    <nav>ignored nav</nav>
    <article>
      <h1>Needle Runtime</h1>
      <p>Needle-X compiles noisy pages into compact context.</p>
      <h2>Details</h2>
      <p>Proof and replay are emitted for every run.</p>
      <ul><li>Local-first</li><li>Deterministic</li></ul>
    </article>
    <footer>ignored footer</footer>
  </body>
</html>
`

func TestReadRunsDeterministicPipelineEndToEnd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, testHTML)
	}))
	defer server.Close()

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

	resp, err := svc.Read(context.Background(), ReadRequest{
		URL:     server.URL,
		Profile: core.ProfileTiny,
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if resp.Document.FinalURL != server.URL {
		t.Fatalf("expected final url %q, got %q", server.URL, resp.Document.FinalURL)
	}
	if len(resp.ResultPack.Chunks) != 3 {
		t.Fatalf("expected tiny profile to keep 3 chunks, got %d", len(resp.ResultPack.Chunks))
	}
	if len(resp.ProofRecords) != len(resp.ResultPack.Chunks) {
		t.Fatalf("expected proof count to match chunks, got %d proofs and %d chunks", len(resp.ProofRecords), len(resp.ResultPack.Chunks))
	}
	if resp.Replay.StageCount != 4 {
		t.Fatalf("expected 4 stages, got %d", resp.Replay.StageCount)
	}
	if len(resp.Trace.Events) < 8 {
		t.Fatalf("expected stage start/completion events, got %d", len(resp.Trace.Events))
	}
	if resp.ResultPack.CostReport.LanePath[0] != 0 {
		t.Fatalf("expected deterministic lane path, got %#v", resp.ResultPack.CostReport.LanePath)
	}
	if resp.ResultPack.Profile != core.ProfileTiny {
		t.Fatalf("expected response profile tiny, got %q", resp.ResultPack.Profile)
	}
	if len(resp.ResultPack.Outline) == 0 {
		t.Fatal("expected outline to be populated")
	}
}

func TestReadRejectsEmptyURL(t *testing.T) {
	svc, err := New(config.Defaults(), nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = svc.Read(context.Background(), ReadRequest{})
	if err == nil {
		t.Fatal("expected empty URL to fail")
	}
}

func TestReadEscalatesLaneForAmbiguousObjective(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, testHTML)
	}))
	defer server.Close()

	cfg := config.Defaults()
	cfg.Policy.ThresholdAmbiguity = 0.20

	svc, err := New(cfg, server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

	resp, err := svc.Read(context.Background(), ReadRequest{
		URL:       server.URL,
		Profile:   core.ProfileTiny,
		Objective: "forum thread regression incident",
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(resp.ResultPack.CostReport.LanePath) != 2 || resp.ResultPack.CostReport.LanePath[1] != 1 {
		t.Fatalf("expected lane path [0 1], got %#v", resp.ResultPack.CostReport.LanePath)
	}
	if resp.ProofRecords[0].Proof.Lane != 1 {
		t.Fatalf("expected proof lane 1, got %d", resp.ProofRecords[0].Proof.Lane)
	}
	if len(resp.ProofRecords[0].Proof.RiskFlags) == 0 {
		t.Fatal("expected risk flags on escalated proof")
	}
	foundEscalation := false
	for _, event := range resp.Trace.Events {
		if event.Type == proof.EventEscalationTriggered {
			foundEscalation = true
			break
		}
	}
	if !foundEscalation {
		t.Fatal("expected escalation event in trace")
	}
}
