package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
	coreservice "github.com/josepavese/needlex/internal/core/service"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/store"
)

type discoveryCorpus struct {
	Version string          `json:"version"`
	Cases   []discoveryCase `json:"cases"`
}
type discoveryCase struct {
	Name                 string   `json:"name"`
	Mode                 string   `json:"mode"`
	Goal                 string   `json:"goal"`
	SeedHTML             string   `json:"seed_html"`
	CanonicalEntity      string   `json:"canonical_entity,omitempty"`
	RewriteQueries       []string `json:"rewrite_queries,omitempty"`
	ExpectedRewrite      bool     `json:"expected_rewrite,omitempty"`
	ExpectedSelectedURL  string   `json:"expected_selected_url,omitempty"`
	ExpectedSelectedPath string   `json:"expected_selected_suffix,omitempty"`
	ExpectLocalProvider  bool     `json:"expect_local_provider"`
	ExpectBootstrap      bool     `json:"expect_bootstrap"`
	RewriteScenario      string   `json:"rewrite_scenario,omitempty"`
}
type discoveryRow struct {
	Name            string `json:"name"`
	Mode            string `json:"mode"`
	SelectedURL     string `json:"selected_url"`
	Provider        string `json:"provider,omitempty"`
	CandidateCount  int    `json:"candidate_count"`
	BootstrapHits   int    `json:"bootstrap_hits"`
	Pass            bool   `json:"pass"`
	Failure         string `json:"failure,omitempty"`
	LocalProvider   bool   `json:"local_provider"`
	BootstrapActive bool   `json:"bootstrap_active"`
	RewriteApplied  bool   `json:"rewrite_applied"`
	EntityPreserved bool   `json:"entity_preserved"`
	RewriteQueries  int    `json:"rewrite_queries"`
	FallbackUsed    bool   `json:"fallback_used"`
}
type discoveryReport struct {
	GeneratedAtUTC string         `json:"generated_at_utc"`
	CorpusVersion  string         `json:"corpus_version"`
	Summary        discoveryStats `json:"summary"`
	Rows           []discoveryRow `json:"rows"`
	Regressions    []string       `json:"regressions,omitempty"`
}

type discoveryStats struct {
	CaseCount              int     `json:"case_count"`
	PassRate               float64 `json:"pass_rate"`
	ExpectedRewriteCases   int     `json:"expected_rewrite_cases"`
	RewriteActivationRate  float64 `json:"rewrite_activation_rate"`
	RewritePrecision       float64 `json:"rewrite_precision"`
	EntityPreservationRate float64 `json:"entity_preservation_rate"`
	FallbackRate           float64 `json:"fallback_rate"`
}

func TestExportDiscoveryEval(t *testing.T) {
	outPath := getenv("NEEDLEX_DISCOVERY_EVAL_OUT", "improvements/discovery-eval-latest.json")
	baselinePath := getenv("NEEDLEX_DISCOVERY_EVAL_BASELINE", "improvements/discovery-eval-baseline.json")
	corpusPath := getenv("NEEDLEX_DISCOVERY_EVAL_CORPUS", "benchmarks/corpora/discovery-corpus-v1.json")
	updateBaseline := strings.EqualFold(strings.TrimSpace(os.Getenv("NEEDLEX_DISCOVERY_EVAL_UPDATE_BASELINE")), "1")
	withRepoRoot(t)

	data, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	var corpus discoveryCorpus
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatalf("decode corpus: %v", err)
	}
	rep := discoveryReport{GeneratedAtUTC: time.Now().UTC().Format(time.RFC3339), CorpusVersion: corpus.Version, Rows: make([]discoveryRow, 0, len(corpus.Cases))}
	for _, item := range corpus.Cases {
		row, err := runDiscoveryCase(t, item)
		if err != nil {
			t.Fatalf("%s: %v", item.Name, err)
		}
		rep.Rows = append(rep.Rows, row)
		if !row.Pass {
			rep.Regressions = append(rep.Regressions, fmt.Sprintf("%s: %s", row.Name, row.Failure))
		}
	}
	rep.Summary = summarizeDiscoveryReport(corpus.Cases, rep.Rows)
	if prior, err := loadDiscoveryReport(baselinePath); err == nil {
		rep.Regressions = append(rep.Regressions, compareDiscoveryReports(prior, rep)...)
	}
	if err := writeDiscoveryReport(outPath, rep); err != nil {
		t.Fatalf("write report: %v", err)
	}
	if updateBaseline {
		if err := writeDiscoveryReport(baselinePath, rep); err != nil {
			t.Fatalf("write baseline: %v", err)
		}
	}
	for _, row := range rep.Rows {
		t.Logf("%s mode=%s selected=%s provider=%s bootstrap_hits=%d rewrite=%t entity=%t fallback=%t pass=%v", row.Name, row.Mode, row.SelectedURL, row.Provider, row.BootstrapHits, row.RewriteApplied, row.EntityPreserved, row.FallbackUsed, row.Pass)
	}
	if len(rep.Regressions) > 0 {
		for _, issue := range rep.Regressions {
			t.Logf("regression: %s", issue)
		}
		t.Fatalf("discovery eval regressions detected: %d", len(rep.Regressions))
	}
}

func runDiscoveryCase(t *testing.T, item discoveryCase) (discoveryRow, error) {
	t.Helper()
	var seedURL string
	seed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if strings.HasSuffix(r.URL.Path, "/docs/replay") {
			_, _ = fmt.Fprint(w, `<html><head><title>Replay Guide</title></head><body><article><h1>Replay Guide</h1><p>Proof replay deterministic context.</p></article></body></html>`)
			return
		}
		_, _ = fmt.Fprint(w, strings.ReplaceAll(item.SeedHTML, "SEED_URL", seedURL))
	}))
	defer seed.Close()
	seedURL = seed.URL

	cfg := config.Defaults()
	cfg.Semantic.VectorSpace = intel.DenseSemanticVectorSpace
	cfg.Semantic.EmbeddingURL = newDiscoveryEvalEmbeddingServer(t).URL
	cfg.Semantic.TimeoutMS = 5000
	cfg.Memory.Enabled = false
	svc, err := coreservice.New(cfg, seed.Client())
	if err != nil {
		return discoveryRow{}, err
	}

	row := discoveryRow{Name: item.Name, Mode: item.Mode}
	switch item.Mode {
	case "same_site":
		resp, err := svc.Discover(context.Background(), coreservice.DiscoverRequest{Goal: item.Goal, SeedURL: seedURL, SameDomain: true, MaxCandidates: 5})
		if err != nil {
			return row, err
		}
		row.SelectedURL, row.CandidateCount = resp.SelectedURL, len(resp.Candidates)
		row.Provider = "local_same_site"
	case "seedless_local":
		root, err := os.MkdirTemp("", "needlex-discovery-eval-*")
		if err != nil {
			return row, err
		}
		defer func() { _ = os.RemoveAll(root) }()
		_, _, _ = store.NewCandidateStore(root).Observe(store.CandidateObservation{URL: seedURL, Title: "Proof Replay Deterministic Guide", Source: "seedless_eval"})
		req := coreservice.PrepareQueryRequestWithLocalState(root, coreservice.QueryRequest{Goal: item.Goal, DiscoveryMode: coreservice.QueryDiscoverySameSite}, cfg, intel.NewSemanticAligner(cfg, seed.Client()))
		resp, err := svc.Query(context.Background(), req)
		if err != nil {
			return row, err
		}
		row.SelectedURL, row.Provider, row.CandidateCount = resp.Plan.SelectedURL, resp.Plan.DiscoveryProvider, len(resp.Plan.CandidateURLs)
		if row.Provider == "" {
			row.Provider = "local_same_site"
		}
	case "seedless_web_rewrite":
		searchHits := []string{}
		originalGoalHits := 0
		pageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			switch r.URL.Path {
			case item.ExpectedSelectedPath:
				title := item.CanonicalEntity
				if strings.TrimSpace(title) == "" {
					title = "Target Entity"
				}
				_, _ = fmt.Fprintf(w, `<html><head><title>%s</title></head><body><article><h1>%s</h1><p>%s is a dance school in Cassine with classes, events, and community activities.</p></article></body></html>`, title, title, title)
			default:
				_, _ = fmt.Fprint(w, `<html><head><title>Other Dance School</title></head><body><article><h1>Other Dance School</h1><p>Generic directory entry.</p></article></body></html>`)
			}
		}))
		defer pageServer.Close()

		searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			query := r.URL.Query().Get("q")
			searchHits = append(searchHits, query)
			if strings.EqualFold(strings.TrimSpace(query), strings.TrimSpace(item.Goal)) {
				originalGoalHits++
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			switch item.RewriteScenario {
			case "bootstrap_clear":
				_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s%s">%s</a><a class="result__a" href="%s/other-a">Other Dance School</a></body></html>`, pageServer.URL, item.ExpectedSelectedPath, item.CanonicalEntity, pageServer.URL)
				return
			case "rewrite_invalid":
				if strings.EqualFold(strings.TrimSpace(query), strings.TrimSpace(item.Goal)) {
					_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s%s">%s</a><a class="result__a" href="%s/other-b">Cassine Events</a></body></html>`, pageServer.URL, item.ExpectedSelectedPath, item.CanonicalEntity, pageServer.URL)
					return
				}
				_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s/wrong-target">Generic School</a></body></html>`, pageServer.URL)
				return
			case "rewrite_fallback_empty":
				if strings.EqualFold(strings.TrimSpace(query), strings.TrimSpace(item.Goal)) {
					_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s%s">%s</a><a class="result__a" href="%s/other-a">Other Dance School</a></body></html>`, pageServer.URL, item.ExpectedSelectedPath, item.CanonicalEntity, pageServer.URL)
					return
				}
				_, _ = fmt.Fprint(w, `<html><body>No results</body></html>`)
				return
			}
			for _, candidate := range item.RewriteQueries {
				if strings.EqualFold(strings.TrimSpace(query), strings.TrimSpace(candidate)) {
					_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s%s">%s</a></body></html>`, pageServer.URL, item.ExpectedSelectedPath, item.CanonicalEntity)
					return
				}
			}
			_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s/other-a">Other Dance School</a><a class="result__a" href="%s/other-b">Cassine Events</a></body></html>`, pageServer.URL, pageServer.URL)
		}))
		defer searchServer.Close()

		modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			rewriteQueries := item.RewriteQueries
			canonicalEntity := item.CanonicalEntity
			switch item.RewriteScenario {
			case "rewrite_invalid":
				rewriteQueries = []string{"dance school near Alessandria", "adult dance classes Alessandria"}
			}
			payload := map[string]any{
				"search_queries":   rewriteQueries,
				"canonical_entity": canonicalEntity,
				"locality_hints":   []string{},
				"category_hints":   []string{},
				"confidence":       0.92,
			}
			raw, _ := json.Marshal(payload)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"finish_reason": "stop",
					"message":       map[string]any{"content": string(raw)},
				}},
				"usage": map[string]any{"prompt_tokens": 32, "completion_tokens": 24},
			})
		}))
		defer modelServer.Close()

		cfg.Models.Backend = "openai-compatible"
		cfg.Models.BaseURL = modelServer.URL
		cfg.Models.Router = "discovery-eval-rewriter"
		svc, err = coreservice.New(cfg, pageServer.Client())
		if err != nil {
			return discoveryRow{}, err
		}
		svc.SetWebDiscoverBaseURL(searchServer.URL)
		resp, err := svc.Query(context.Background(), coreservice.QueryRequest{Goal: item.Goal, DiscoveryMode: coreservice.QueryDiscoveryWeb})
		if err != nil {
			return row, err
		}
		row.SelectedURL, row.Provider, row.CandidateCount = resp.Plan.SelectedURL, resp.Plan.DiscoveryProvider, len(resp.Plan.CandidateURLs)
		row.BootstrapHits = len(searchHits)
		for _, decision := range resp.Plan.Compiler.Decisions {
			if decision.ReasonCode == "NX_PLAN_QUERY_REWRITE" {
				row.RewriteApplied = true
				if queries := strings.TrimSpace(decision.Metadata["queries"]); queries != "" {
					row.RewriteQueries = len(strings.Split(queries, " | "))
					row.EntityPreserved = strings.TrimSpace(decision.Metadata["canonical_entity"]) != ""
				}
				break
			}
		}
		row.FallbackUsed = row.RewriteApplied == false && originalGoalHits > 0 && item.RewriteScenario == "rewrite_fallback_empty"
	default:
		return row, fmt.Errorf("unsupported mode %q", item.Mode)
	}
	row.LocalProvider = strings.HasPrefix(strings.TrimSpace(row.Provider), "local_")
	row.BootstrapActive = row.Provider != "" && !row.LocalProvider
	row.Pass, row.Failure = evaluateDiscoveryCase(item, row)
	return row, nil
}

func newDiscoveryEvalEmbeddingServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode embedding request: %v", err)
		}
		vectors := make([][]float32, 0, len(req.Input))
		for _, input := range req.Input {
			vectors = append(vectors, discoveryEvalEmbeddingVector(input))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": vectors})
	}))
	t.Cleanup(server.Close)
	return server
}

func discoveryEvalEmbeddingVector(text string) []float32 {
	vector := make([]float32, 10)
	if axis, ok := discoveryEvalExactAxes[normalizeDiscoveryFixtureText(text)]; ok {
		vector[axis] = 1
		return vector
	}
	for _, rawURL := range extractDiscoveryFixtureURLs(text) {
		if axis, ok := discoveryEvalURLAxis(rawURL); ok {
			vector[axis] = 1
			return vector
		}
	}
	if axis, ok := discoveryEvalIdentityAxis(text); ok {
		vector[axis] = 1
		return vector
	}
	return vector
}

var discoveryEvalExactAxes = map[string]int{
	"proof replay deterministic":                               0,
	"proof replay deterministic guide":                         0,
	"replay guide":                                             0,
	"replay guide html_like":                                   0,
	"replay guide replay guide":                                0,
	"asd charly brown dance school alessandria":                2,
	"asd charly brown html_like":                               2,
	"asd charly brown asd charly brown html_like":              2,
	"understand what asd charly brown does in alessandria":     2,
	"voglio capire cosa fa asd charly brown a cassine":         2,
	"understand what charly brown dance school does":           2,
	"half pocket matrice":                                      3,
	"half pocket html_like":                                    3,
	"half pocket half pocket html_like":                        3,
	"capire cosa fa half pocket in italia":                     3,
	"playwright installation":                                  4,
	"find the official site for playwright browser automation": 4,
	"playwright html_like":                                     4,
	"playwright playwright html_like":                          4,
	"sqlite download":                                          5,
	"find the official site for sqlite database engine":        5,
	"sqlite html_like":                                         5,
	"sqlite sqlite html_like":                                  5,
}

func discoveryEvalURLAxis(raw string) (int, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}
	host := strings.ToLower(parsed.Hostname())
	path := strings.ToLower(parsed.EscapedPath())
	switch host {
	case "playwright.dev":
		return 4, true
	case "sqlite.org", "www.sqlite.org":
		return 5, true
	}
	switch {
	case path == "/docs/replay":
		return 0, true
	case path == "/asd-charly-brown":
		return 2, true
	case path == "/half-pocket":
		return 3, true
	case path == "/playwright":
		return 4, true
	case path == "/sqlite":
		return 5, true
	case strings.HasSuffix(host, "charlybrown.it"):
		return 2, true
	case strings.HasSuffix(host, "halfpocket.it"):
		return 3, true
	}
	return 0, false
}

func discoveryEvalIdentityAxis(text string) (int, bool) {
	fields := make([]string, 0)
	for _, field := range strings.Fields(strings.ToLower(text)) {
		field = strings.Trim(field, `"'()[]{}<>,.;`)
		if field == "" {
			continue
		}
		fields = append(fields, field)
		switch field {
		case "/docs/replay":
			return 0, true
		case "/asd-charly-brown":
			return 2, true
		case "/half-pocket":
			return 3, true
		case "/playwright":
			return 4, true
		case "/sqlite":
			return 5, true
		}
	}
	if hasExactFieldWindow(fields, []string{"proof", "replay", "deterministic", "guide"}) {
		return 0, true
	}
	return 0, false
}

func hasExactFieldWindow(fields, window []string) bool {
	if len(window) == 0 || len(fields) < len(window) {
		return false
	}
	for i := 0; i <= len(fields)-len(window); i++ {
		ok := true
		for j := range window {
			if fields[i+j] != window[j] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func extractDiscoveryFixtureURLs(text string) []string {
	fields := strings.Fields(text)
	out := make([]string, 0, 2)
	for _, field := range fields {
		field = strings.Trim(field, `"'()[]{}<>,.;`)
		if strings.HasPrefix(field, "http://") || strings.HasPrefix(field, "https://") {
			out = append(out, field)
		}
	}
	return out
}

func normalizeDiscoveryFixtureText(text string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(text))), " ")
}

func evaluateDiscoveryCase(item discoveryCase, row discoveryRow) (bool, string) {
	if item.ExpectedSelectedURL != "" && row.SelectedURL != item.ExpectedSelectedURL {
		return false, fmt.Sprintf("selected_url mismatch %q != %q", row.SelectedURL, item.ExpectedSelectedURL)
	}
	if item.ExpectedSelectedPath != "" && !strings.HasSuffix(row.SelectedURL, item.ExpectedSelectedPath) {
		return false, fmt.Sprintf("selected_url suffix mismatch %q !~ %q", row.SelectedURL, item.ExpectedSelectedPath)
	}
	if row.LocalProvider != item.ExpectLocalProvider {
		return false, fmt.Sprintf("local_provider mismatch %t != %t", row.LocalProvider, item.ExpectLocalProvider)
	}
	if row.BootstrapActive != item.ExpectBootstrap {
		return false, fmt.Sprintf("bootstrap_active mismatch %t != %t", row.BootstrapActive, item.ExpectBootstrap)
	}
	if row.RewriteApplied != item.ExpectedRewrite {
		return false, fmt.Sprintf("rewrite_applied mismatch %t != %t", row.RewriteApplied, item.ExpectedRewrite)
	}
	return true, ""
}

func summarizeDiscoveryReport(cases []discoveryCase, rows []discoveryRow) discoveryStats {
	stats := discoveryStats{CaseCount: len(rows)}
	if len(rows) == 0 {
		return stats
	}
	passes := 0
	expectedRewrite := 0
	rewriteApplied := 0
	rewriteCorrect := 0
	entityPreserved := 0
	fallbacks := 0
	caseByName := map[string]discoveryCase{}
	for _, item := range cases {
		caseByName[item.Name] = item
		if item.ExpectedRewrite {
			expectedRewrite++
		}
	}
	for _, row := range rows {
		if row.Pass {
			passes++
		}
		if row.RewriteApplied {
			rewriteApplied++
			if row.EntityPreserved {
				entityPreserved++
			}
			if item, ok := caseByName[row.Name]; ok && item.ExpectedRewrite {
				rewriteCorrect++
			}
		}
		if row.FallbackUsed {
			fallbacks++
		}
	}
	stats.PassRate = float64(passes) / float64(len(rows))
	stats.ExpectedRewriteCases = expectedRewrite
	stats.RewriteActivationRate = float64(rewriteApplied) / float64(len(rows))
	if rewriteApplied > 0 {
		stats.RewritePrecision = float64(rewriteCorrect) / float64(rewriteApplied)
		stats.EntityPreservationRate = float64(entityPreserved) / float64(rewriteApplied)
	}
	stats.FallbackRate = float64(fallbacks) / float64(len(rows))
	return stats
}

func compareDiscoveryReports(previous, current discoveryReport) []string {
	prev := map[string]discoveryRow{}
	for _, row := range previous.Rows {
		prev[row.Name] = row
	}
	regressions := []string{}
	for _, row := range current.Rows {
		if prior, ok := prev[row.Name]; ok && prior.Pass && !row.Pass {
			regressions = append(regressions, fmt.Sprintf("%s regressed: %s", row.Name, row.Failure))
		}
	}
	return regressions
}

func writeDiscoveryReport(path string, rep discoveryReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func loadDiscoveryReport(path string) (discoveryReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return discoveryReport{}, err
	}
	var rep discoveryReport
	if err := json.Unmarshal(data, &rep); err != nil {
		return discoveryReport{}, err
	}
	return rep, nil
}

func withRepoRoot(t *testing.T) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(filepath.Join("..", "..", "..")); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
