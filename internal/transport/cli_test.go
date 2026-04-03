package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
	coreservice "github.com/josepavese/needlex/internal/core/service"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/memory"
	"github.com/josepavese/needlex/internal/proof"
	"github.com/josepavese/needlex/internal/store"
)

type stubSemanticAligner struct {
	scores map[string]float64
}

func (s stubSemanticAligner) Align(context.Context, string, []intel.SemanticCandidate) (intel.SemanticAlignment, error) {
	return intel.SemanticAlignment{}, nil
}

func (s stubSemanticAligner) Score(_ context.Context, _ string, candidates []intel.SemanticCandidate) ([]intel.SemanticScore, error) {
	out := make([]intel.SemanticScore, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, intel.SemanticScore{ID: candidate.ID, Similarity: s.scores[candidate.ID]})
	}
	return out, nil
}

func TestRunnerReadJSON(t *testing.T) {
	var captured coreservice.ReadRequest
	root := t.TempDir()
	runner := Runner{
		loadConfig: func(path string) (config.Config, error) {
			return config.Defaults(), nil
		},
		read: func(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error) {
			captured = req
			return fakeResponse(), nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runner.Run([]string{"read", "https://example.com", "--json", "--profile", "tiny"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d with stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"summary"`) || !strings.Contains(stdout.String(), `"uncertainty"`) || !strings.Contains(stdout.String(), `"chunks"`) || strings.Contains(stdout.String(), `"proof_records"`) {
		t.Fatalf("expected compact json payload, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"kind": "page_read"`) {
		t.Fatalf("expected page_read kind in compact payload, got %q", stdout.String())
	}
	if captured.Profile != "tiny" {
		t.Fatalf("expected profile to be forwarded, got %q", captured.Profile)
	}
}

func TestRunnerReadJSONFull(t *testing.T) {
	root := t.TempDir()
	runner := Runner{
		loadConfig: func(path string) (config.Config, error) {
			return config.Defaults(), nil
		},
		read: func(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error) {
			return fakeResponse(), nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runner.Run([]string{"read", "https://example.com", "--json", "--json-mode", "full"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d with stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"document"`) || !strings.Contains(stdout.String(), `"proof_records"`) {
		t.Fatalf("expected full json payload, got %q", stdout.String())
	}
}

func TestRunnerQueryJSON(t *testing.T) {
	root := t.TempDir()
	var captured coreservice.QueryRequest
	runner := Runner{
		loadConfig: func(path string) (config.Config, error) {
			return config.Defaults(), nil
		},
		query: func(ctx context.Context, cfg config.Config, req coreservice.QueryRequest) (coreservice.QueryResponse, error) {
			captured = req
			return fakeQueryResponse(req), nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runner.Run([]string{"query", "https://example.com", "--goal", "proof replay deterministic", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d with stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"selected_url"`) || !strings.Contains(stdout.String(), `"selection_why"`) || !strings.Contains(stdout.String(), `"uncertainty"`) || strings.Contains(stdout.String(), `"result_pack"`) {
		t.Fatalf("expected compact query json payload, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"kind": "goal_query"`) {
		t.Fatalf("expected goal_query kind in compact payload, got %q", stdout.String())
	}
	if captured.Goal != "proof replay deterministic" {
		t.Fatalf("expected goal to be forwarded, got %q", captured.Goal)
	}
	if captured.DiscoveryMode != "" {
		t.Fatalf("expected default discovery mode passthrough to remain empty, got %q", captured.DiscoveryMode)
	}
}

func TestRunnerQueryJSONFull(t *testing.T) {
	root := t.TempDir()
	runner := Runner{
		loadConfig: func(path string) (config.Config, error) {
			return config.Defaults(), nil
		},
		query: func(ctx context.Context, cfg config.Config, req coreservice.QueryRequest) (coreservice.QueryResponse, error) {
			return fakeQueryResponse(req), nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runner.Run([]string{"query", "https://example.com", "--goal", "proof replay deterministic", "--json", "--json-mode", "full"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d with stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"result_pack"`) || !strings.Contains(stdout.String(), `"proof_records"`) {
		t.Fatalf("expected full query json payload, got %q", stdout.String())
	}
}

func TestRunnerQueryTextIncludesWebIRSignals(t *testing.T) {
	root := t.TempDir()
	runner := Runner{
		loadConfig: func(path string) (config.Config, error) {
			return config.Defaults(), nil
		},
		query: func(ctx context.Context, cfg config.Config, req coreservice.QueryRequest) (coreservice.QueryResponse, error) {
			return fakeQueryResponse(req), nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runner.Run([]string{"query", "https://example.com", "--goal", "proof replay deterministic"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d with stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Web IR Signals: heading=0.50 short_text=0.50 embedded=0") {
		t.Fatalf("expected query output to include web ir signals, got %q", stdout.String())
	}
}

func TestRunnerQueryDiscoveryFlag(t *testing.T) {
	root := t.TempDir()
	var captured coreservice.QueryRequest
	runner := Runner{
		loadConfig: func(path string) (config.Config, error) {
			return config.Defaults(), nil
		},
		query: func(ctx context.Context, cfg config.Config, req coreservice.QueryRequest) (coreservice.QueryResponse, error) {
			captured = req
			return fakeQueryResponse(req), nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runner.Run([]string{"query", "https://example.com", "--goal", "proof replay deterministic", "--discovery", "off", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d with stderr %q", code, stderr.String())
	}
	if captured.DiscoveryMode != "off" {
		t.Fatalf("expected discovery mode to be forwarded, got %q", captured.DiscoveryMode)
	}
}

func TestRunnerQueryWithoutSeedURL(t *testing.T) {
	root := t.TempDir()
	var captured coreservice.QueryRequest
	runner := Runner{
		loadConfig: func(path string) (config.Config, error) {
			return config.Defaults(), nil
		},
		query: func(ctx context.Context, cfg config.Config, req coreservice.QueryRequest) (coreservice.QueryResponse, error) {
			captured = req
			return fakeQueryResponse(req), nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runner.Run([]string{"query", "--goal", "proof replay deterministic", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d with stderr %q", code, stderr.String())
	}
	if captured.SeedURL != "" {
		t.Fatalf("expected empty seed url for query-only mode, got %q", captured.SeedURL)
	}
}

func TestRunnerQueryAutoSeedsFromCandidateMemory(t *testing.T) {
	root := t.TempDir()
	candidateStore := store.NewCandidateStore(root)
	if _, _, err := candidateStore.Observe(store.CandidateObservation{
		URL:    "https://halfpocket.net/about",
		Title:  "Halfpocket Studio",
		Source: "read",
	}); err != nil {
		t.Fatalf("seed candidate store: %v", err)
	}

	var captured coreservice.QueryRequest
	semantic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[1,0]},{"object":"embedding","index":1,"embedding":[0.95,0.05]}],"model":"cli-semantic"}`)
	}))
	defer semantic.Close()
	runner := Runner{
		loadConfig: func(path string) (config.Config, error) {
			cfg := config.Defaults()
			cfg.Semantic.Enabled = true
			cfg.Semantic.Backend = "openai-embeddings"
			cfg.Semantic.BaseURL = semantic.URL
			cfg.Semantic.Model = "cli-semantic"
			return cfg, nil
		},
		query: func(ctx context.Context, cfg config.Config, req coreservice.QueryRequest) (coreservice.QueryResponse, error) {
			captured = req
			return fakeQueryResponse(req), nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runner.Run([]string{"query", "--goal", "halfpocket studio profile", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d with stderr %q", code, stderr.String())
	}
	if captured.SeedURL != "https://halfpocket.net/about" {
		t.Fatalf("expected auto-seeded url from candidate memory, got %q", captured.SeedURL)
	}
	if len(captured.DomainHints) == 0 || captured.DomainHints[0] != "halfpocket.net" {
		t.Fatalf("expected domain hint halfpocket.net, got %#v", captured.DomainHints)
	}
}

func TestRunnerQueryDoesNotAutoSeedWhenDiscoveryOff(t *testing.T) {
	root := t.TempDir()
	candidateStore := store.NewCandidateStore(root)
	if _, _, err := candidateStore.Observe(store.CandidateObservation{
		URL:    "https://halfpocket.net/about",
		Title:  "Halfpocket Studio",
		Source: "read",
	}); err != nil {
		t.Fatalf("seed candidate store: %v", err)
	}

	var captured coreservice.QueryRequest
	semantic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[1,0]},{"object":"embedding","index":1,"embedding":[0.95,0.05]}],"model":"cli-semantic"}`)
	}))
	defer semantic.Close()
	runner := Runner{
		loadConfig: func(path string) (config.Config, error) {
			cfg := config.Defaults()
			cfg.Semantic.Enabled = true
			cfg.Semantic.Backend = "openai-embeddings"
			cfg.Semantic.BaseURL = semantic.URL
			cfg.Semantic.Model = "cli-semantic"
			return cfg, nil
		},
		query: func(ctx context.Context, cfg config.Config, req coreservice.QueryRequest) (coreservice.QueryResponse, error) {
			captured = req
			return fakeQueryResponse(req), nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runner.Run([]string{"query", "--goal", "halfpocket studio profile", "--discovery", "off", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d with stderr %q", code, stderr.String())
	}
	if captured.SeedURL != "" {
		t.Fatalf("expected no auto-seeding when discovery=off, got %q", captured.SeedURL)
	}
	if len(captured.DomainHints) != 0 {
		t.Fatalf("expected no domain hints when discovery=off and no seed, got %#v", captured.DomainHints)
	}
}

func TestRunnerQueryExpandsDomainHintsFromDomainGraph(t *testing.T) {
	root := t.TempDir()
	domainGraphStore := store.NewDomainGraphStore(root)
	if _, _, err := domainGraphStore.Observe("https://seed.example/root", "https://expansion.example/docs", "query_discovery"); err != nil {
		t.Fatalf("seed domain graph: %v", err)
	}
	if _, _, err := domainGraphStore.Observe("https://seed.example/root", "https://expansion.example/docs-2", "query_discovery"); err != nil {
		t.Fatalf("seed domain graph second edge: %v", err)
	}

	var captured coreservice.QueryRequest
	runner := Runner{
		loadConfig: func(path string) (config.Config, error) {
			return config.Defaults(), nil
		},
		query: func(ctx context.Context, cfg config.Config, req coreservice.QueryRequest) (coreservice.QueryResponse, error) {
			captured = req
			return fakeQueryResponse(req), nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runner.Run([]string{"query", "https://seed.example/root", "--goal", "proof replay deterministic", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d with stderr %q", code, stderr.String())
	}

	foundSeed := false
	foundExpansion := false
	for _, hint := range captured.DomainHints {
		if hint == "seed.example" {
			foundSeed = true
		}
		if hint == "expansion.example" {
			foundExpansion = true
		}
	}
	if !foundSeed {
		t.Fatalf("expected seed domain hint, got %#v", captured.DomainHints)
	}
	if !foundExpansion {
		t.Fatalf("expected expanded domain hint, got %#v", captured.DomainHints)
	}
}

func TestRunnerCrawlJSON(t *testing.T) {
	root := t.TempDir()
	runner := Runner{
		loadConfig: func(path string) (config.Config, error) {
			return config.Defaults(), nil
		},
		crawl: func(ctx context.Context, cfg config.Config, req coreservice.CrawlRequest) (coreservice.CrawlResponse, error) {
			return fakeCrawlResponse(), nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runner.Run([]string{"crawl", "https://example.com", "--json", "--max-pages", "2", "--same-domain"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d with stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"summary"`) || !strings.Contains(stdout.String(), `"kind": "bounded_crawl"`) || strings.Contains(stdout.String(), `"pages"`) {
		t.Fatalf("expected compact crawl json payload, got %q", stdout.String())
	}
}

func TestRunnerReadText(t *testing.T) {
	root := t.TempDir()
	runner := Runner{
		loadConfig: func(path string) (config.Config, error) {
			return config.Defaults(), nil
		},
		read: func(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error) {
			return fakeResponse(), nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runner.Run([]string{"read", "https://example.com"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d with stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Title: Needle Runtime") {
		t.Fatalf("expected text output, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Web IR Signals: heading=0.50 short_text=0.50 embedded=0") {
		t.Fatalf("expected web ir signal line, got %q", stdout.String())
	}
}

func TestRunnerReadStoresCandidateMemory(t *testing.T) {
	root := t.TempDir()
	runner := Runner{
		loadConfig: func(path string) (config.Config, error) {
			return config.Defaults(), nil
		},
		read: func(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error) {
			return fakeResponse(), nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := runner.Run([]string{"read", "https://example.com", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("expected exit 0, got %d with stderr %q", code, stderr.String())
	}

	matches, err := store.NewCandidateStore(root).Search(context.Background(), "needlex runtime", 1, stubSemanticAligner{
		scores: map[string]float64{"https://example.com": 0.92},
	})
	if err != nil {
		t.Fatalf("search candidate store: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 candidate match, got %d", len(matches))
	}
	if matches[0].URL != "https://example.com" {
		t.Fatalf("expected candidate url https://example.com, got %q", matches[0].URL)
	}
}

func TestRunnerUnknownCommand(t *testing.T) {
	runner := NewRunner()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runner.Run([]string{"unknown"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected usage exit 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("expected unknown command message, got %q", stderr.String())
	}
}

func TestRunnerHelpListsDayOneCommands(t *testing.T) {
	runner := NewRunner()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runner.Run([]string{"help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	for _, command := range []string{"needlex crawl", "needlex query", "needlex read", "needlex mcp", "needlex version"} {
		if !strings.Contains(stdout.String(), command) {
			t.Fatalf("expected help to include %q, got %q", command, stdout.String())
		}
	}
}

func TestRunnerVersion(t *testing.T) {
	runner := NewRunner()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runner.Run([]string{"version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", code, stderr.String())
	}
	if strings.TrimSpace(stdout.String()) == "" {
		t.Fatal("expected non-empty version output")
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr, got %q", stderr.String())
	}
}

func TestRunnerReplayAndDiff(t *testing.T) {
	root := t.TempDir()
	runner := Runner{
		loadConfig: config.Load,
		read: func(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error) {
			resp := fakeResponse()
			resp.Trace.TraceID = req.URL
			resp.Trace.RunID = req.URL
			resp.Trace.Stages = []proof.StageSnapshot{
				{
					Stage:       "acquire",
					StartedAt:   time.Unix(1700000000, 0).UTC(),
					CompletedAt: time.Unix(1700000001, 0).UTC(),
					OutputHash:  req.URL,
				},
			}
			return resp, nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if code := runner.Run([]string{"read", "trace_a"}, &stdout, &stderr); code != 0 {
		t.Fatalf("seed trace a failed: %d %q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"read", "trace_b"}, &stdout, &stderr); code != 0 {
		t.Fatalf("seed trace b failed: %d %q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()

	if code := runner.Run([]string{"replay", "trace_a", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("replay failed: %d %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"trace_id": "trace_a"`) {
		t.Fatalf("expected replay json, got %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"diff", "trace_a", "trace_b"}, &stdout, &stderr); code != 0 {
		t.Fatalf("diff failed: %d %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Changed Stages: 1") {
		t.Fatalf("expected diff output, got %q", stdout.String())
	}
}

func TestRunnerProofByTraceAndChunk(t *testing.T) {
	root := t.TempDir()
	runner := Runner{
		loadConfig: config.Load,
		read: func(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error) {
			resp := fakeResponse()
			resp.Trace.TraceID = "trace_for_proof"
			return resp, nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if code := runner.Run([]string{"read", "https://example.com"}, &stdout, &stderr); code != 0 {
		t.Fatalf("seed proof records failed: %d %q", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"proof", "trace_for_proof", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("proof by trace failed: %d %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"trace_id": "trace_for_proof"`) {
		t.Fatalf("expected trace id in output, got %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"proof", "chk_1"}, &stdout, &stderr); code != 0 {
		t.Fatalf("proof by chunk failed: %d %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Chunk ID: chk_1") {
		t.Fatalf("expected chunk proof output, got %q", stdout.String())
	}
}

func TestRunnerPruneAll(t *testing.T) {
	root := t.TempDir()
	runner := Runner{
		loadConfig: config.Load,
		read: func(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error) {
			resp := fakeResponse()
			resp.Trace.TraceID = "trace_for_prune"
			return resp, nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if code := runner.Run([]string{"read", "https://example.com"}, &stdout, &stderr); code != 0 {
		t.Fatalf("seed state failed: %d %q", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"prune", "--all", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("prune failed: %d %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"removed_files": 4`) {
		t.Fatalf("expected 4 removed files, got %q", stdout.String())
	}
}

func TestRunnerMemoryStatsAndSearch(t *testing.T) {
	root := t.TempDir()
	semantic := newMemoryEmbeddingServer()
	defer semantic.Close()

	cfg := config.Defaults()
	cfg.Memory.Enabled = true
	cfg.Semantic.Enabled = true
	cfg.Semantic.Backend = "openai-embeddings"
	cfg.Semantic.BaseURL = semantic.URL
	cfg.Semantic.Model = "memory-test-embed"
	cfg.Memory.EmbeddingBackend = cfg.Semantic.Backend
	cfg.Memory.EmbeddingModel = cfg.Semantic.Model

	seedMemoryDocument(t, root, cfg, semantic.Client(), "https://playwright.dev/docs/intro", "Installation | Playwright", "Install Playwright and run the installation command to download browser binaries.")

	runner := Runner{
		loadConfig: func(path string) (config.Config, error) {
			return cfg, nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := runner.Run([]string{"memory", "stats", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("memory stats failed: %d %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"document_count": 1`) || !strings.Contains(stdout.String(), `"embedding_count": 1`) {
		t.Fatalf("expected memory stats json, got %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"memory", "search", "playwright installation", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("memory search failed: %d %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"https://playwright.dev/docs/intro"`) || !strings.Contains(stdout.String(), `"proof_ref": "proof_install"`) {
		t.Fatalf("expected memory candidate in search output, got %q", stdout.String())
	}
}

func TestRunnerMemoryPrune(t *testing.T) {
	root := t.TempDir()
	semantic := newMemoryEmbeddingServer()
	defer semantic.Close()

	cfg := config.Defaults()
	cfg.Memory.Enabled = true
	cfg.Memory.MaxDocuments = 1
	cfg.Memory.MaxEmbeddings = 1
	cfg.Memory.MaxEdges = 1
	cfg.Semantic.Enabled = true
	cfg.Semantic.Backend = "openai-embeddings"
	cfg.Semantic.BaseURL = semantic.URL
	cfg.Semantic.Model = "memory-test-embed"
	cfg.Memory.EmbeddingBackend = cfg.Semantic.Backend
	cfg.Memory.EmbeddingModel = cfg.Semantic.Model

	seedMemoryDocument(t, root, cfg, semantic.Client(), "https://playwright.dev/docs/intro", "Installation | Playwright", "Install Playwright and run the installation command to download browser binaries.")
	seedMemoryDocument(t, root, cfg, semantic.Client(), "https://playwright.dev/docs/test-runners", "Test Runners | Playwright", "Integrate Playwright with common test runners and tooling.")

	runner := Runner{
		loadConfig: func(path string) (config.Config, error) {
			return cfg, nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := runner.Run([]string{"memory", "prune", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("memory prune failed: %d %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"documents": 1`) || !strings.Contains(stdout.String(), `"embeddings": 1`) {
		t.Fatalf("expected removed counts in prune output, got %q", stdout.String())
	}
}

func TestRunnerReadUsesGenomeForceLane(t *testing.T) {
	root := t.TempDir()
	genomeStore := store.NewGenomeStore(root)
	if _, _, err := genomeStore.Observe(store.GenomeObservation{
		URL:              "https://example.com/docs",
		ObservedLane:     1,
		PreferredProfile: "tiny",
		PruningProfile:   "aggressive",
		RenderNeeded:     true,
	}); err != nil {
		t.Fatalf("seed genome: %v", err)
	}

	var captured coreservice.ReadRequest
	runner := Runner{
		loadConfig: func(path string) (config.Config, error) {
			return config.Defaults(), nil
		},
		read: func(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error) {
			captured = req
			resp := fakeResponse()
			resp.Document.FinalURL = "https://example.com/docs"
			resp.ResultPack.CostReport.LanePath = []int{0, 1}
			return resp, nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := runner.Run([]string{"read", "https://example.com/docs"}, &stdout, &stderr); code != 0 {
		t.Fatalf("read failed: %d %q", code, stderr.String())
	}
	if captured.ForceLane != 1 {
		t.Fatalf("expected force lane 1 from genome, got %d", captured.ForceLane)
	}
	if captured.Profile != "tiny" {
		t.Fatalf("expected profile from genome, got %q", captured.Profile)
	}
	if captured.PruningProfile != "aggressive" {
		t.Fatalf("expected pruning profile from genome, got %q", captured.PruningProfile)
	}
	if !captured.RenderHint {
		t.Fatal("expected render hint from genome")
	}
}

func fakeResponse() coreservice.ReadResponse {
	return coreservice.ReadResponse{
		Document: core.Document{
			ID:        "doc_1",
			URL:       "https://example.com",
			FinalURL:  "https://example.com",
			Title:     "Needle Runtime",
			FetchedAt: time.Unix(1700000000, 0).UTC(),
			FetchMode: core.FetchModeHTTP,
			RawHash:   "sha256_abc",
		},
		WebIR: core.WebIR{
			Version:   core.WebIRVersion,
			SourceURL: "https://example.com",
			NodeCount: 2,
			Nodes: []core.WebIRNode{
				{
					Path:  "/article[1]/h1[1]",
					Tag:   "h1",
					Kind:  "heading",
					Text:  "Needle Runtime",
					Depth: 2,
				},
				{
					Path:  "/article[1]/p[1]",
					Tag:   "p",
					Kind:  "paragraph",
					Text:  "Compact context.",
					Depth: 2,
				},
			},
			Signals: core.WebIRSignals{
				ShortTextRatio:    0.5,
				HeadingRatio:      0.5,
				EmbeddedNodeCount: 0,
			},
		},
		ResultPack: core.ResultPack{
			Objective: "read",
			Profile:   core.ProfileStandard,
			Chunks: []core.Chunk{
				{
					ID:          "chk_1",
					DocID:       "doc_1",
					Text:        "Compact context.",
					HeadingPath: []string{"Needle Runtime"},
					Score:       0.9,
					Fingerprint: "fp_1",
					Confidence:  0.95,
				},
			},
			Sources: []core.SourceRef{
				{
					DocumentID: "doc_1",
					URL:        "https://example.com",
					ChunkIDs:   []string{"chk_1"},
				},
			},
			ProofRefs: []string{"proof_1"},
			CostReport: core.CostReport{
				LatencyMS: 10,
				TokenIn:   0,
				TokenOut:  0,
				LanePath:  []int{0},
			},
		},
		AgentContext: coreservice.AgentContext{
			URL:   "https://example.com",
			Title: "Needle Runtime",
			Chunks: []coreservice.AgentChunk{
				{
					ID:          "chk_1",
					Text:        "Compact context.",
					HeadingPath: []string{"Needle Runtime"},
					SourceURL:   "https://example.com",
					ProofRef:    "proof_1",
				},
			},
		},
		ProofRecords: []proof.ProofRecord{
			{
				ID: "proof_1",
				Proof: core.Proof{
					ChunkID:        "chk_1",
					SourceSpan:     core.SourceSpan{Selector: "/article/p[1]", CharStart: 0, CharEnd: 16},
					TransformChain: []string{"reduce:v1", "segment:v1", "pack:v1"},
					Lane:           0,
				},
			},
		},
		Trace: proof.RunTrace{
			RunID:      "run_1",
			TraceID:    "trace_1",
			StartedAt:  time.Unix(1700000000, 0).UTC(),
			FinishedAt: time.Unix(1700000001, 0).UTC(),
			Stages: []proof.StageSnapshot{
				{Stage: "acquire", StartedAt: time.Unix(1700000000, 0).UTC(), CompletedAt: time.Unix(1700000000, 0).UTC(), OutputHash: "hash"},
			},
			Events: []proof.TraceEvent{
				{Type: proof.EventStageStarted, Stage: "acquire", Timestamp: time.Unix(1700000000, 0).UTC()},
			},
		},
		Replay: proof.ReplayReport{
			RunID:      "run_1",
			TraceID:    "trace_1",
			StageCount: 1,
			EventCount: 1,
		},
	}
}

func newMemoryEmbeddingServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			Input any `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		inputs := memoryInputsForTest(payload.Input)
		data := make([]map[string]any, 0, len(inputs))
		for i, input := range inputs {
			data = append(data, map[string]any{
				"object":    "embedding",
				"index":     i,
				"embedding": memoryVectorForTest(input),
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data":   data,
			"model":  "memory-test-embed",
		})
	}))
}

func memoryInputsForTest(raw any) []string {
	switch typed := raw.(type) {
	case string:
		return []string{typed}
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if value, ok := item.(string); ok {
				out = append(out, value)
			}
		}
		return out
	default:
		return nil
	}
}

func memoryVectorForTest(text string) []float64 {
	const dims = 64
	vector := make([]float64, dims)
	for _, token := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	}) {
		if token == "" {
			continue
		}
		h := fnv.New32a()
		_, _ = h.Write([]byte(token))
		idx := int(h.Sum32() % dims)
		vector[idx] += 1
	}
	return vector
}

func seedMemoryDocument(t *testing.T, root string, cfg config.Config, client *http.Client, pageURL, title, text string) {
	t.Helper()
	store := memory.NewSQLiteStore(root, cfg.Memory.Path)
	service := memory.NewService(cfg.Memory, store, intel.NewTextEmbedder(cfg, client))
	err := service.Observe(context.Background(), memory.Observation{
		Document: core.Document{
			URL:       pageURL,
			FinalURL:  pageURL,
			Title:     title,
			FetchMode: core.FetchModeHTTP,
		},
		ResultPack: core.ResultPack{
			Profile: core.ProfileStandard,
			Chunks: []core.Chunk{
				{
					ID:          "chk_" + title,
					DocID:       "doc_" + title,
					Text:        text,
					HeadingPath: []string{title},
					Score:       0.91,
					Confidence:  0.95,
				},
			},
			ProofRefs: []string{"proof_install"},
			Links:     []string{pageURL},
		},
		ProofRecords: []proof.ProofRecord{{ID: "proof_install"}},
		TraceID:      "trace_" + title,
		SourceKind:   "read",
	})
	if err != nil {
		t.Fatalf("seed memory document: %v", err)
	}
}

func fakeQueryResponse(req coreservice.QueryRequest) coreservice.QueryResponse {
	read := fakeResponse()
	read.Trace.TraceID = "trace_query"
	read.ResultPack.Query = req.Goal
	discoveryMode := req.DiscoveryMode
	if discoveryMode == "" {
		discoveryMode = coreservice.QueryDiscoverySameSite
	}
	return coreservice.QueryResponse{
		Plan: coreservice.QueryPlan{
			Goal:          req.Goal,
			SeedURL:       req.SeedURL,
			Profile:       core.ProfileStandard,
			DiscoveryMode: discoveryMode,
			SelectedURL:   firstNonEmpty(req.SeedURL, read.Document.FinalURL),
			CandidateURLs: []string{firstNonEmpty(req.SeedURL, read.Document.FinalURL)},
			Budget: core.Budget{
				MaxTokens:    8000,
				MaxLatencyMS: 1800,
				MaxPages:     20,
				MaxDepth:     2,
				MaxBytes:     4_000_000,
			},
			LaneMax: 3,
		},
		Document:   read.Document,
		WebIR:      read.WebIR,
		ResultPack: read.ResultPack,
		AgentContext: coreservice.AgentContext{
			URL:   read.Document.FinalURL,
			Title: read.Document.Title,
			Candidates: []coreservice.AgentCandidate{
				{URL: firstNonEmpty(req.SeedURL, read.Document.FinalURL), Label: "Example", Reason: []string{"seed_fallback"}},
			},
			Chunks: read.AgentContext.Chunks,
		},
		ProofRefs:    read.ResultPack.ProofRefs,
		ProofRecords: read.ProofRecords,
		Trace:        read.Trace,
		TraceID:      read.Trace.TraceID,
		CostReport:   read.ResultPack.CostReport,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func fakeCrawlResponse() coreservice.CrawlResponse {
	read := fakeResponse()
	return coreservice.CrawlResponse{
		Documents: []core.Document{read.Document},
		Summary: coreservice.CrawlSummary{
			SeedURL:         "https://example.com",
			PagesVisited:    1,
			MaxDepthReached: 0,
			SameDomain:      true,
			ChunkCount:      len(read.ResultPack.Chunks),
		},
		Pages: []coreservice.ReadResponse{read},
	}
}
