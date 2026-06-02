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
		_, _ = fmt.Fprint(w, `<html><head><title>App Shell</title></head><body><app-root></app-root><script>window._a2s={"configuration":{"blog":[{"title":"Needle Runtime","description":"<p>Needle-X compiles noisy pages into compact proof-carrying context for agents.</p>"}]}}</script></body></html>`)
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
		_, _ = fmt.Fprint(w, `<html><head><title>App Shell</title></head><body><app-root></app-root></body></html>`)
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

func TestReadRequiredRenderUsesRenderedDOM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>App Shell</title></head><body><app-root></app-root></body></html>`)
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
