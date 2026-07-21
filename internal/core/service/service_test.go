package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/core/agentreadable"
	"github.com/josepavese/needlex/internal/core/sourceresolution"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/pipeline"
	"github.com/josepavese/needlex/internal/proof"
	"github.com/josepavese/needlex/internal/rendering"
	"github.com/josepavese/needlex/internal/store"
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

func TestNewWithStateRootUsesProvidedPALRoot(t *testing.T) {
	root := t.TempDir()
	svc, err := NewWithStateRoot(testConfig(), nil, root)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if svc.storeRoot != root {
		t.Fatalf("expected service store root %q, got %q", root, svc.storeRoot)
	}
	if _, _, err := svc.discoveryProviders.Observe(store.DiscoveryProviderObservation{
		Name:    "example",
		Outcome: store.DiscoveryProviderOutcomeSuccess,
	}); err != nil {
		t.Fatalf("observe provider state: %v", err)
	}
	if _, err := svc.discoveryProviders.Load("example"); err != nil {
		t.Fatalf("expected provider state under service root: %v", err)
	}
}

func TestReadRunsDeterministicPipelineEndToEnd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, testHTML)
	}))
	defer server.Close()

	svc, err := New(testConfig(), server.Client())
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
	if resp.WebIR.Version != core.WebIRVersion {
		t.Fatalf("expected web_ir version %q, got %q", core.WebIRVersion, resp.WebIR.Version)
	}
	if resp.WebIR.NodeCount == 0 {
		t.Fatal("expected web_ir nodes to be populated")
	}
	if len(resp.ProofRecords) != len(resp.ResultPack.Chunks) {
		t.Fatalf("expected proof count to match chunks, got %d proofs and %d chunks", len(resp.ProofRecords), len(resp.ResultPack.Chunks))
	}
	if resp.Replay.StageCount != 5 {
		t.Fatalf("expected 5 stages, got %d", resp.Replay.StageCount)
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
	svc, err := New(testConfig(), nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = svc.Read(context.Background(), ReadRequest{})
	if err == nil {
		t.Fatal("expected empty URL to fail")
	}
}

func TestReadSynthesizesMinimalDOMWhenReducerYieldsNoTextNodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Discourse Meta</title></head><body><nav><a href="/latest">Latest</a></nav><script>window.__APP__={}</script></body></html>`)
	}))
	defer server.Close()

	svc, err := New(testConfig(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

	resp, err := svc.Read(context.Background(), ReadRequest{URL: server.URL, Profile: core.ProfileStandard})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if resp.WebIR.NodeCount == 0 {
		t.Fatal("expected fallback web_ir nodes to be synthesized")
	}
	if len(resp.ResultPack.Chunks) == 0 {
		t.Fatal("expected fallback pack chunk")
	}
}

func TestReadEscalatesLaneForAmbiguousObjective(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, testHTML)
	}))
	defer server.Close()

	cfg := testConfig()
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

	cfg := testConfig()
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

	svc, err := New(testConfig(), server.Client())
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

func TestReadTraceSkipsModelInterventionWhenCoverageGateSuppressesRoute(t *testing.T) {
	pageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Needle Runtime</title></head><body><article><h1>Needle Runtime</h1><h2>Overview</h2><p>Operator incident notes for runtime failures.</p><h2>Details</h2><p>Remediation workflow for operators handling incidents.</p></article></body></html>`)
	}))
	defer pageServer.Close()

	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type candidate struct {
			ChunkID string `json:"chunk_id"`
		}
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode model request: %v", err)
		}
		rawInput := payload.Messages[len(payload.Messages)-1].Content
		_, after, ok := strings.Cut(rawInput, "input=")
		if !ok {
			t.Fatalf("missing input payload in %q", rawInput)
		}
		var input struct {
			Candidates []candidate `json:"candidates"`
		}
		if err := json.Unmarshal([]byte(after), &input); err != nil {
			t.Fatalf("decode input payload: %v", err)
		}
		if len(input.Candidates) < 2 {
			t.Fatalf("expected 2 candidates, got %#v", input.Candidates)
		}
		content, err := json.Marshal(map[string]any{
			"selected_chunk_ids": []string{input.Candidates[1].ChunkID},
			"rejected_chunk_ids": []string{input.Candidates[0].ChunkID},
			"decision_reason":    "second candidate is more grounded",
			"confidence":         0.91,
		})
		if err != nil {
			t.Fatalf("marshal content: %v", err)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"finish_reason": "stop",
					"message": map[string]any{
						"content": string(content),
					},
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     36,
				"completion_tokens": 11,
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer modelServer.Close()

	cfg := testConfig()
	cfg.Models.Backend = "openai-compatible"
	cfg.Models.BaseURL = modelServer.URL
	cfg.Models.Router = "qwen-ambiguity"

	svc, err := New(cfg, pageServer.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.semantic = fakeSemanticAligner{suppressed: true, reason: "semantic_dominance"}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

	resp, err := svc.Read(context.Background(), ReadRequest{
		URL:       pageServer.URL,
		Profile:   core.ProfileStandard,
		Objective: "operator incident remediation",
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	for _, event := range resp.Trace.Events {
		if event.Type == proof.EventModelIntervention {
			t.Fatalf("expected no model intervention trace when coverage gate suppresses route, got %#v", resp.Trace.Events)
		}
	}
	for _, record := range resp.ProofRecords {
		for _, invocation := range record.Proof.ModelInvocations {
			if invocation.Task == intel.TaskResolveAmbiguity {
				t.Fatalf("expected no resolve_ambiguity invocation in proof records, got %#v", resp.ProofRecords)
			}
		}
	}
}

func TestReadPackTraceIncludesFingerprintStability(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, testHTML)
	}))
	defer server.Close()

	svc, err := New(testConfig(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	seed := svc.rankSegments(context.Background(), "doc_seed", "", core.WebIR{}, []pipeline.Segment{{Text: "Needle-X compiles noisy pages into compact context.", HeadingPath: []string{"Needle Runtime"}}})
	resp, err := svc.Read(context.Background(), ReadRequest{
		URL:                server.URL,
		Profile:            core.ProfileTiny,
		StableFingerprints: []string{seed[0].chunk.Fingerprint},
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	found := false
	for _, stage := range resp.Trace.Stages {
		if stage.Stage == "pack" &&
			stage.Metadata["stable_fp_hits"] != "" &&
			stage.Metadata["novel_fp_hits"] != "" &&
			stage.Metadata["delta_class"] != "" &&
			stage.Metadata["reuse_mode"] == "delta_aware" &&
			stage.Metadata["reuse_eligible"] != "" &&
			stage.Metadata["reuse_applied"] != "" &&
			stage.Metadata["selected_ir_embedded_hits"] != "" &&
			stage.Metadata["selected_ir_heading_hits"] != "" &&
			stage.Metadata["selected_ir_shallow_hits"] != "" &&
			stage.Metadata["intel_task_route_count"] != "" &&
			stage.Metadata["web_ir_policy_embedded_required"] != "" &&
			stage.Metadata["web_ir_policy_heading_required"] != "" &&
			stage.Metadata["web_ir_policy_noise_swap"] != "" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected fingerprint stability metadata in pack trace, got %#v", resp.Trace.Stages)
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

	svc, err := New(testConfig(), server.Client())
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

	svc, err := New(testConfig(), server.Client())
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

func TestReadDoesNotRecoverFromCustomAppShellEmbeddedPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>App Shell</title><script src="/runtime.js"></script><script src="/bundle.js"></script></head><body><main></main><script type="application/json">{"items":[{"title":"Needle Runtime","description":"Needle-X compiles noisy pages into compact proof-carrying context for agents."}]}</script></body></html>`)
	}))
	defer server.Close()

	svc, err := New(testConfig(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

	resp, err := svc.Read(context.Background(), ReadRequest{
		URL:     server.URL,
		Profile: core.ProfileStandard,
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(resp.ResultPack.Chunks) == 0 {
		t.Fatal("expected synthetic shell chunks")
	}
	for _, chunk := range resp.ResultPack.Chunks {
		if strings.Contains(chunk.Text, "Needle-X compiles noisy pages") {
			t.Fatalf("custom embedded payload should not be extracted, got %q", chunk.Text)
		}
	}
	if resp.WebIR.Signals.SubstrateClass != "client_rendered_app" {
		t.Fatalf("expected render-required substrate, got %q", resp.WebIR.Signals.SubstrateClass)
	}
}

func TestReadPrefersMarkdownVariantForAgentReadableDocs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/docs/cli-reference":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, `<html><head><title>CLI reference</title></head><body><header>Search Ask AI Help Center Status Sign In Guides API Reference Changelog</header><main></main></body></html>`)
		case "/docs/cli-reference.md":
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			_, _ = fmt.Fprint(w, "# CLI reference\n\nThe Brevo CLI can authenticate, create OAuth apps, scaffold starter code, and run a local test server.\n\n## Commands\n\n- brevo login\n- brevo app create\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc, err := New(testConfig(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	resp, err := svc.Read(context.Background(), ReadRequest{
		URL:       server.URL + "/docs/cli-reference",
		Objective: "Analyze CLI commands",
		Profile:   core.ProfileStandard,
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if resp.Document.FinalURL != server.URL+"/docs/cli-reference.md" {
		t.Fatalf("expected markdown variant final url, got %q", resp.Document.FinalURL)
	}
	if !containsChunkText(resp.ResultPack.Chunks, "brevo app create") {
		t.Fatalf("expected markdown CLI command, got %#v", resp.ResultPack.Chunks)
	}
	if resp.WebIR.Signals.SubstrateClass != "agent_markdown" {
		t.Fatalf("expected agent_markdown substrate, got %q", resp.WebIR.Signals.SubstrateClass)
	}
}

func TestReadDiscoversMarkdownFromRobotsSitemap(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/docs/cli-reference":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, `<html><head><title>CLI reference</title><script src="/runtime.js"></script><script src="/bundle.js"></script></head><body><main></main></body></html>`)
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = fmt.Fprintf(w, "User-agent: *\nAllow: /\nSitemap: %s/sitemap.xml\n", serverURL)
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml; charset=utf-8")
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><urlset><url><loc>%s/agent/docs/cli-reference.md</loc></url></urlset>`, serverURL)
		case "/agent/docs/cli-reference.md":
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			_, _ = fmt.Fprint(w, "# CLI reference\n\nThe CLI supports semantic retrieval, rendering escalation, and agent-readable source discovery.\n\n## Commands\n\n- needlex read\n- needlex query\n")
		default:
			http.NotFound(w, r)
		}
	}))
	serverURL = server.URL
	defer server.Close()

	svc, err := New(testConfig(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	resp, err := svc.Read(context.Background(), ReadRequest{
		URL:       server.URL + "/docs/cli-reference",
		Objective: "Analyze CLI commands",
		Profile:   core.ProfileStandard,
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if resp.Document.FinalURL != server.URL+"/agent/docs/cli-reference.md" {
		t.Fatalf("expected sitemap markdown final url, got %q", resp.Document.FinalURL)
	}
	if !containsChunkText(resp.ResultPack.Chunks, "needlex query") {
		t.Fatalf("expected markdown command from sitemap candidate, got %#v", resp.ResultPack.Chunks)
	}
}

func TestReadAgentReadableDeclaredModeSkipsConventionalSitemap(t *testing.T) {
	var sitemapFetches int
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/docs/cli-reference":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, `<html><head><title>CLI reference</title><script src="/runtime.js"></script><script src="/bundle.js"></script></head><body><main></main></body></html>`)
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = fmt.Fprintf(w, "User-agent: *\nAllow: /\nSitemap: %s/sitemap.xml\n", serverURL)
		case "/sitemap.xml":
			sitemapFetches++
			w.Header().Set("Content-Type", "application/xml; charset=utf-8")
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><urlset><url><loc>%s/agent/docs/cli-reference.md</loc></url></urlset>`, serverURL)
		case "/agent/docs/cli-reference.md":
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			_, _ = fmt.Fprint(w, "# CLI reference\n\nThe CLI supports agent-readable source discovery.\n")
		default:
			http.NotFound(w, r)
		}
	}))
	serverURL = server.URL
	defer server.Close()

	cfg := testConfig()
	cfg.Render.Enabled = false
	svc := newTestService(t, cfg, server.Client())
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	resp, err := svc.Read(context.Background(), ReadRequest{
		URL:               server.URL + "/docs/cli-reference",
		Objective:         "Analyze CLI commands",
		Profile:           core.ProfileStandard,
		AgentReadableMode: "declared",
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if sitemapFetches != 0 {
		t.Fatalf("expected declared mode to skip conventional sitemap, got %d fetches", sitemapFetches)
	}
	if resp.Document.FinalURL != server.URL+"/docs/cli-reference" {
		t.Fatalf("expected original HTML page, got %q", resp.Document.FinalURL)
	}
}

func TestReadUsesOpenAPIFromAPICatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/developers":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Link", `</.well-known/api-catalog>; rel="api-catalog"`)
			_, _ = fmt.Fprint(w, `<html><head><title>Developers</title></head><body><main></main></body></html>`)
		case "/.well-known/api-catalog":
			w.Header().Set("Content-Type", "application/linkset+json; charset=utf-8")
			_, _ = fmt.Fprint(w, `{"linkset":[{"anchor":"/","service-desc":[{"href":"/openapi.json","type":"application/openapi+json"}]}]}`)
		case "/openapi.json":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = fmt.Fprint(w, `{"openapi":"3.1.0","info":{"title":"Example API","version":"1.0.0"},"paths":{"/widgets":{"get":{"description":"List widgets for agents"}}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc, err := New(testConfig(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	resp, err := svc.Read(context.Background(), ReadRequest{
		URL:       server.URL + "/developers",
		Objective: "Find API operations",
		Profile:   core.ProfileStandard,
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if resp.Document.FinalURL != server.URL+"/openapi.json" {
		t.Fatalf("expected OpenAPI final url, got %q", resp.Document.FinalURL)
	}
	if resp.WebIR.Signals.SourceKind != agentreadable.KindServiceDescription {
		t.Fatalf("expected service description source kind, got %q", resp.WebIR.Signals.SourceKind)
	}
	if !containsChunkText(resp.ResultPack.Chunks, "List widgets for agents") {
		t.Fatalf("expected OpenAPI content, got %#v", resp.ResultPack.Chunks)
	}
}

type agentReadableChoiceAligner struct{}

func (agentReadableChoiceAligner) Align(context.Context, string, []intel.SemanticCandidate) (intel.SemanticAlignment, error) {
	return intel.SemanticAlignment{}, nil
}

func (agentReadableChoiceAligner) Score(_ context.Context, objective string, candidates []intel.SemanticCandidate) ([]intel.SemanticScore, error) {
	objective = strings.ToLower(objective)
	out := make([]intel.SemanticScore, 0, len(candidates))
	for _, candidate := range candidates {
		text := strings.ToLower(candidate.Text)
		score := 0.10
		if strings.Contains(objective, "billing") && strings.Contains(text, "billing") {
			score = 0.95
		}
		if strings.Contains(objective, "cli") && strings.Contains(text, "cli") {
			score = 0.90
		}
		out = append(out, intel.SemanticScore{ID: candidate.ID, Similarity: score})
	}
	return out, nil
}

func TestReadSelectsAgentReadableCandidateSemanticallyAcrossPhases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/developers":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, `<html><head><title>Developers</title><script src="/runtime.js"></script><script src="/bundle.js"></script></head><body><main></main></body></html>`)
		case "/developers.md":
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			_, _ = fmt.Fprint(w, "# Developer overview\n\nGeneral developer setup, installation notes, and onboarding content for command line usage.\n\n## Start\n\n- install tools\n- authenticate locally\n")
		case "/openapi.json":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = fmt.Fprint(w, `{"openapi":"3.1.0","info":{"title":"Billing API","version":"1.0.0"},"paths":{"/billing/invoices":{"get":{"description":"List billing invoices for accounts"}}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc, err := New(testConfig(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.semantic = agentReadableChoiceAligner{}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	resp, err := svc.Read(context.Background(), ReadRequest{
		URL:       server.URL + "/developers",
		Objective: "Find billing API operations",
		Profile:   core.ProfileStandard,
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if resp.Document.FinalURL != server.URL+"/openapi.json" {
		t.Fatalf("expected semantic selection to prefer OpenAPI, got %q", resp.Document.FinalURL)
	}
}

func TestReadRespectsRobotsForConventionalAgentReadableCandidates(t *testing.T) {
	var markdownFetches int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/docs/private":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, `<html><head><title>Private</title><script src="/runtime.js"></script><script src="/bundle.js"></script></head><body><main></main></body></html>`)
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = fmt.Fprint(w, "User-agent: *\nDisallow: /docs/private.md\nAllow: /\n")
		case "/docs/private.md":
			markdownFetches++
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			_, _ = fmt.Fprint(w, "# Private\n\nThis disallowed markdown should not be fetched or selected by conventional probing.\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc, err := New(testConfig(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	resp, err := svc.Read(context.Background(), ReadRequest{
		URL:       server.URL + "/docs/private",
		Objective: "Read private markdown",
		Profile:   core.ProfileStandard,
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if markdownFetches != 0 {
		t.Fatalf("expected robots-disallowed markdown not to be fetched, got %d fetches", markdownFetches)
	}
	if resp.Document.FinalURL == server.URL+"/docs/private.md" {
		t.Fatalf("expected disallowed markdown not to be selected")
	}
}

func TestReadAgentReadableConventionalProbesStopNearDeadline(t *testing.T) {
	var markdownFetches int
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/docs/slow":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, `<html><head><title>Slow docs</title><script src="/runtime.js"></script><script src="/bundle.js"></script></head><body><main></main></body></html>`)
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = fmt.Fprintf(w, "User-agent: *\nAllow: /\nSitemap: %s/sitemap.xml\n", serverURL)
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml; charset=utf-8")
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><urlset><url><loc>%s/docs/slow-a.md</loc></url><url><loc>%s/docs/slow-b.md</loc></url><url><loc>%s/docs/slow-c.md</loc></url></urlset>`, serverURL, serverURL, serverURL)
		case "/docs/slow-a.md", "/docs/slow-b.md", "/docs/slow-c.md":
			markdownFetches++
			time.Sleep(sourceresolution.AgentReadableProbeTimeout + time.Second)
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			_, _ = fmt.Fprint(w, "# Slow markdown\n\nThis should be best-effort under deadline.\n")
		default:
			http.NotFound(w, r)
		}
	}))
	serverURL = server.URL
	defer server.Close()

	cfg := testConfig()
	cfg.Render.Enabled = false
	svc := newTestService(t, cfg, server.Client())
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	started := time.Now()
	resp, err := svc.Read(ctx, ReadRequest{
		URL:       server.URL + "/docs/slow",
		Objective: "Read slow docs",
		Profile:   core.ProfileStandard,
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("expected deadline-aware agent-readable probing, elapsed %s", elapsed)
	}
	if markdownFetches > 1 {
		t.Fatalf("expected conventional probes to stop near deadline, got %d markdown fetches", markdownFetches)
	}
	if resp.Document.FinalURL != server.URL+"/docs/slow" {
		t.Fatalf("expected fallback HTML page, got %q", resp.Document.FinalURL)
	}
}

func TestReadSelectsLLMSMarkdownLinkSemantically(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/docs/start":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, `<html><head><title>Start</title><script src="/runtime.js"></script><script src="/bundle.js"></script></head><body><main></main></body></html>`)
		case "/llms.txt":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = fmt.Fprint(w, "# Agent index\n\n- [CLI guide](/cli.md)\n- [Billing API reference](/billing.md)\n")
		case "/cli.md":
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			_, _ = fmt.Fprint(w, "# CLI guide\n\nThe CLI guide explains local commands, profiles, and installation workflows for operators.\n")
		case "/billing.md":
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			_, _ = fmt.Fprint(w, "# Billing API reference\n\nBilling API operations list invoices, account balances, payment methods, and customer subscription history.\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc, err := New(testConfig(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.semantic = agentReadableChoiceAligner{}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	resp, err := svc.Read(context.Background(), ReadRequest{
		URL:       server.URL + "/docs/start",
		Objective: "Find billing documentation",
		Profile:   core.ProfileStandard,
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if resp.Document.FinalURL != server.URL+"/billing.md" {
		t.Fatalf("expected semantic llms link selection to prefer billing doc, got %q", resp.Document.FinalURL)
	}
}

type fakeRenderer struct {
	page rendering.Page
	err  error
}

func (f fakeRenderer) Render(context.Context, rendering.Request) (rendering.Page, error) {
	return f.page, f.err
}

func TestReadAutoRenderIsEnabledByDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>App Shell</title><script src="/runtime.js"></script><script src="/bundle.js"></script></head><body><main></main></body></html>`)
	}))
	defer server.Close()

	cfg := config.Defaults()
	enableDiscoverSemantic(&cfg, "")
	svc, err := New(cfg, server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.renderer = fakeRenderer{page: rendering.Page{
		URL:       server.URL,
		FinalURL:  server.URL,
		HTML:      `<html><head><title>Rendered</title></head><body><article><h1>Rendered</h1><p>Default auto render exposes JavaScript content without a render flag.</p></article></body></html>`,
		Browser:   "fake",
		Duration:  time.Millisecond,
		FetchedAt: time.Unix(1700000000, 0).UTC(),
	}}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	resp, err := svc.Read(context.Background(), ReadRequest{
		URL:     server.URL,
		Profile: core.ProfileStandard,
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if resp.Document.FetchMode != core.FetchModeRender {
		t.Fatalf("expected default auto render fetch mode, got %q", resp.Document.FetchMode)
	}
	if !containsChunkText(resp.ResultPack.Chunks, "without a render flag") {
		t.Fatalf("expected rendered content, got %#v", resp.ResultPack.Chunks)
	}
}

func TestReadAutoRenderSkipsWhenContextDeadlineIsTooClose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>App Shell</title><script src="/runtime.js"></script><script src="/bundle.js"></script></head><body><main></main></body></html>`)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Render.Enabled = true
	svc := newTestService(t, cfg, server.Client())
	renderer := &countingRenderer{page: rendering.Page{
		URL:       server.URL,
		FinalURL:  server.URL,
		HTML:      `<html><head><title>Rendered</title></head><body><article><h1>Rendered</h1><p>Should not render near deadline.</p></article></body></html>`,
		Browser:   "fake",
		Duration:  time.Millisecond,
		FetchedAt: time.Unix(1700000000, 0).UTC(),
	}}
	svc.renderer = renderer

	ctx, cancel := context.WithTimeout(context.Background(), sourceresolution.AutoRenderDeadlineMinRemaining-time.Second)
	defer cancel()
	resp, err := svc.Read(ctx, ReadRequest{
		URL:     server.URL,
		Profile: core.ProfileStandard,
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if renderer.calls != 0 {
		t.Fatalf("expected auto render to skip near deadline, got %d calls", renderer.calls)
	}
	if resp.Document.FetchMode == core.FetchModeRender {
		t.Fatalf("did not expect render fetch mode near deadline")
	}
}

func TestReadAutoRenderUsesDeadlineBoundedTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>App Shell</title><script src="/runtime.js"></script><script src="/bundle.js"></script></head><body><main></main></body></html>`)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Render.Enabled = true
	cfg.Render.TimeoutMS = 30000
	svc := newTestService(t, cfg, server.Client())
	renderer := &countingRenderer{page: rendering.Page{
		URL:       server.URL,
		FinalURL:  server.URL,
		HTML:      `<html><head><title>Rendered</title></head><body><article><h1>Rendered</h1><p>Bounded render timeout.</p></article></body></html>`,
		Browser:   "fake",
		Duration:  time.Millisecond,
		FetchedAt: time.Unix(1700000000, 0).UTC(),
	}}
	svc.renderer = renderer

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	resp, err := svc.Read(ctx, ReadRequest{
		URL:     server.URL,
		Profile: core.ProfileStandard,
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if resp.Document.FetchMode != core.FetchModeRender {
		t.Fatalf("expected render fetch mode, got %q", resp.Document.FetchMode)
	}
	if renderer.lastRequest.Timeout > sourceresolution.AutoRenderDeadlineTimeout {
		t.Fatalf("expected bounded render timeout <= %s, got %s", sourceresolution.AutoRenderDeadlineTimeout, renderer.lastRequest.Timeout)
	}
}

func TestReadRequiredRenderUsesRenderedDOM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>App Shell</title><script src="/runtime.js"></script><script src="/bundle.js"></script></head><body><main></main></body></html>`)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Render.Enabled = true
	svc, err := New(cfg, server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.renderer = fakeRenderer{page: rendering.Page{
		URL:       server.URL,
		FinalURL:  server.URL,
		HTML:      `<html><head><title>Rendered</title></head><body><article><h1>Rendered</h1><p>Rendered JavaScript content exposes the final application data for agents.</p></article></body></html>`,
		Browser:   "fake",
		Duration:  time.Millisecond,
		FetchedAt: time.Unix(1700000000, 0).UTC(),
	}}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	resp, err := svc.Read(context.Background(), ReadRequest{
		URL:        server.URL,
		Profile:    core.ProfileStandard,
		RenderMode: "required",
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if resp.Document.FetchMode != core.FetchModeRender {
		t.Fatalf("expected render fetch mode, got %q", resp.Document.FetchMode)
	}
	if !containsChunkText(resp.ResultPack.Chunks, "Rendered JavaScript content") {
		t.Fatalf("expected rendered content, got %#v", resp.ResultPack.Chunks)
	}
}

func TestReadRequiredRenderIncludesApplicationNetworkEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>App Shell</title><script src="/runtime.js"></script><script src="/bundle.js"></script></head><body><main></main></body></html>`)
	}))
	defer server.Close()

	cfg := testConfig()
	cfg.Render.Enabled = true
	svc, err := New(cfg, server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.renderer = fakeRenderer{page: rendering.Page{
		URL:       server.URL,
		FinalURL:  server.URL,
		HTML:      `<html><head><title>Rendered Shell</title></head><body><main>Application shell</main></body></html>`,
		Browser:   "fake",
		Duration:  time.Millisecond,
		FetchedAt: time.Unix(1700000000, 0).UTC(),
		NetworkResources: []rendering.NetworkResource{{
			URL:          server.URL + "/stream",
			Type:         "EventSource",
			ContentType:  "text/event-stream",
			Source:       "event_source",
			Body:         "data: {\"name\":\"Villa Semantica\",\"city\":\"Siena\",\"description\":\"Rendered network evidence feeds agent chunks.\"}\n\ndata: end\n\n",
			BodyBytes:    118,
			MessageCount: 2,
			Finished:     true,
		}},
		NetworkStats: rendering.NetworkStats{
			ResourceCount:       1,
			EventSourceMessages: 2,
			BodyBytes:           118,
			IdleReason:          "network_idle",
		},
	}}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	resp, err := svc.Read(context.Background(), ReadRequest{
		URL:        server.URL,
		Profile:    core.ProfileStandard,
		RenderMode: "required",
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if resp.Document.FetchMode != core.FetchModeRender {
		t.Fatalf("expected render fetch mode, got %q", resp.Document.FetchMode)
	}
	if !containsChunkText(resp.ResultPack.Chunks, "Villa Semantica") || !containsChunkText(resp.ResultPack.Chunks, "Rendered network evidence") {
		t.Fatalf("expected network evidence chunks, got %#v", resp.ResultPack.Chunks)
	}
}

func TestRenderNetworkEvidenceTextOrdersJSONFieldsDeterministically(t *testing.T) {
	resource := rendering.NetworkResource{
		URL:         "https://example.com/api/data.json",
		Type:        "Fetch",
		ContentType: "application/json",
		Body:        `{"zeta":"last","alpha":"first","middle":"center"}`,
	}
	first := rendering.EvidenceText([]rendering.NetworkResource{resource})
	for i := 0; i < 20; i++ {
		if got := rendering.EvidenceText([]rendering.NetworkResource{resource}); got != first {
			t.Fatalf("expected deterministic network evidence text, first=%q got=%q", first, got)
		}
	}
	alpha := strings.Index(first, "alpha: first")
	middle := strings.Index(first, "middle: center")
	zeta := strings.Index(first, "zeta: last")
	if alpha < 0 || middle < 0 || zeta < 0 || alpha > middle || middle > zeta {
		t.Fatalf("expected sorted JSON fields, got %q", first)
	}
}

func TestRenderNetworkEvidenceTextPreservesUsefulURLsAndSanitizesSensitiveQueries(t *testing.T) {
	text := rendering.EvidenceText([]rendering.NetworkResource{{
		URL:         "https://example.com/api/data.json",
		Type:        "Fetch",
		ContentType: "application/json",
		Body:        `{"endpoint":"https://example.com/api/pricing?token=secret-value#frag","image":"https://example.com/assets/photo.jpg?sig=abc","description":"pricing endpoint"}`,
	}})
	if !strings.Contains(text, "https://example.com/api/pricing") {
		t.Fatalf("expected useful endpoint URL to be preserved, got %q", text)
	}
	if strings.Contains(text, "token=") || strings.Contains(text, "secret-value") || strings.Contains(text, "#frag") {
		t.Fatalf("expected sensitive URL query and fragment to be removed, got %q", text)
	}
	if strings.Contains(text, "photo.jpg") {
		t.Fatalf("expected media asset URL to be filtered, got %q", text)
	}
}
