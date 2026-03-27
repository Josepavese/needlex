package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
	if len(resp.ResultPack.Chunks) != 2 {
		t.Fatalf("expected tiny profile to keep 2 chunks, got %d", len(resp.ResultPack.Chunks))
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

func TestReadAppliesExtractorAndFormatterAtHigherLanes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Needle Runtime</title></head><body><article><h1>Needle Runtime</h1><p>Short proof. Replay deterministic context.</p></article></body></html>`)
	}))
	defer server.Close()

	cfg := config.Defaults()
	cfg.Policy.ThresholdAmbiguity = 0.10

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
		Objective: "proof replay deterministic context",
		ForceLane: 3,
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(resp.ResultPack.CostReport.LanePath) != 4 || resp.ResultPack.CostReport.LanePath[3] != 3 {
		t.Fatalf("expected lane path [0 1 2 3], got %#v", resp.ResultPack.CostReport.LanePath)
	}
	if len(resp.ProofRecords[0].Proof.ModelInvocations) < 4 {
		t.Fatalf("expected router/judge/extractor/formatter invocations, got %d", len(resp.ProofRecords[0].Proof.ModelInvocations))
	}
	if !strings.HasSuffix(resp.ResultPack.Chunks[0].Text, ".") {
		t.Fatalf("expected formatter to normalize punctuation, got %q", resp.ResultPack.Chunks[0].Text)
	}
}

func TestReadTinyCompactionIsTraced(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Needle Runtime</title></head><body><article><h1>Needle Runtime</h1><p>The runtime reduces HTML into a stable intermediate representation before ranking and packing.</p><p>Replay and diff keep every extraction auditable and locally inspectable without a backend.</p></article></body></html>`)
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
		URL:       server.URL,
		Profile:   core.ProfileTiny,
		Objective: "stable ranking packing",
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	foundCompactTrace := false
	for _, record := range resp.ProofRecords {
		for _, step := range record.Proof.TransformChain {
			if step == "pack:tiny_compact:v1" {
				foundCompactTrace = true
				break
			}
		}
	}
	if !foundCompactTrace {
		t.Fatal("expected tiny compaction to be recorded in transform chain")
	}
}

func TestReadUsesBrowserLikeUserAgentWhenRenderHintIsSet(t *testing.T) {
	var seenUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenUserAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Needle Runtime</title></head><body><article><h1>Needle Runtime</h1><p>Compact context.</p></article></body></html>`)
	}))
	defer server.Close()

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

	_, err = svc.Read(context.Background(), ReadRequest{
		URL:        server.URL,
		RenderHint: true,
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !strings.Contains(seenUserAgent, "Mozilla/5.0") {
		t.Fatalf("expected browser-like user agent, got %q", seenUserAgent)
	}
}

func TestReadAppliesAggressivePruningProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Needle Runtime</title></head><body><div class="hero-banner">Hero chrome</div><article><h1>Needle Runtime</h1><p>Useful compact context.</p></article></body></html>`)
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
		URL:            server.URL,
		Profile:        core.ProfileTiny,
		PruningProfile: "aggressive",
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	for _, chunk := range resp.ResultPack.Chunks {
		if strings.Contains(chunk.Text, "Hero chrome") {
			t.Fatalf("expected aggressive pruning to remove hero content, got %q", chunk.Text)
		}
	}
}
