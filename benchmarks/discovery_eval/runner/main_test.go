package main

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
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
		row, err := runDiscoveryCase(item)
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

func runDiscoveryCase(item discoveryCase) (discoveryRow, error) {
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
	semantic := newDiscoveryEvalSemanticServer()
	defer semantic.Close()
	cfg.Semantic.Enabled = true
	cfg.Semantic.Backend = "openai-embeddings"
	cfg.Semantic.BaseURL = semantic.URL
	cfg.Semantic.Model = "discovery-eval-embed"
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
		defer os.RemoveAll(root)
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
				_, _ = fmt.Fprintf(w, `<html><head><title>%s</title></head><body><article><h1>%s</h1><p>Target entity page.</p></article></body></html>`, title, title)
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
				for _, candidate := range item.RewriteQueries {
					if strings.EqualFold(strings.TrimSpace(query), strings.TrimSpace(candidate)) {
						_, _ = fmt.Fprint(w, `<html><body>No results</body></html>`)
						return
					}
				}
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
					if entity := strings.TrimSpace(item.CanonicalEntity); entity != "" {
						row.EntityPreserved = true
						for _, query := range strings.Split(queries, " | ") {
							if !strings.Contains(strings.ToLower(query), strings.ToLower(entity)) {
								row.EntityPreserved = false
								break
							}
						}
					}
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

func newDiscoveryEvalSemanticServer() *httptest.Server {
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
		inputs := []string{}
		switch typed := payload.Input.(type) {
		case string:
			inputs = append(inputs, typed)
		case []any:
			for _, item := range typed {
				if value, ok := item.(string); ok {
					inputs = append(inputs, value)
				}
			}
		}
		data := make([]map[string]any, 0, len(inputs))
		for i, input := range inputs {
			data = append(data, map[string]any{
				"object":    "embedding",
				"index":     i,
				"embedding": discoveryEvalVector(input),
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data":   data,
			"model":  "discovery-eval-embed",
		})
	}))
}

func discoveryEvalVector(text string) []float64 {
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
		vector[int(h.Sum32()%dims)] += 1
	}
	return vector
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
