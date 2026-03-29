package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
	coreservice "github.com/josepavese/needlex/internal/core/service"
	"github.com/josepavese/needlex/internal/store"
)

type discoveryCorpus struct {
	Version string          `json:"version"`
	Cases   []discoveryCase `json:"cases"`
}
type discoveryCase struct {
	Name                 string `json:"name"`
	Mode                 string `json:"mode"`
	Goal                 string `json:"goal"`
	SeedHTML             string `json:"seed_html"`
	ExpectedSelectedURL  string `json:"expected_selected_url,omitempty"`
	ExpectedSelectedPath string `json:"expected_selected_suffix,omitempty"`
	ExpectLocalProvider  bool   `json:"expect_local_provider"`
	ExpectBootstrap      bool   `json:"expect_bootstrap"`
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
}
type discoveryReport struct {
	GeneratedAtUTC string         `json:"generated_at_utc"`
	CorpusVersion  string         `json:"corpus_version"`
	Rows           []discoveryRow `json:"rows"`
	Regressions    []string       `json:"regressions,omitempty"`
}

func TestExportDiscoveryEval(t *testing.T) {
	outPath := getenv("NEEDLEX_DISCOVERY_EVAL_OUT", "improvements/discovery-eval-latest.json")
	baselinePath := getenv("NEEDLEX_DISCOVERY_EVAL_BASELINE", "improvements/discovery-eval-baseline.json")
	corpusPath := getenv("NEEDLEX_DISCOVERY_EVAL_CORPUS", "testdata/benchmark/discovery-corpus-v1.json")
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
		t.Logf("%s mode=%s selected=%s provider=%s bootstrap_hits=%d pass=%v", row.Name, row.Mode, row.SelectedURL, row.Provider, row.BootstrapHits, row.Pass)
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

	svc, err := coreservice.New(config.Defaults(), seed.Client())
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
		req := coreservice.PrepareQueryRequestWithLocalState(root, coreservice.QueryRequest{Goal: item.Goal, DiscoveryMode: coreservice.QueryDiscoverySameSite})
		resp, err := svc.Query(context.Background(), req)
		if err != nil {
			return row, err
		}
		row.SelectedURL, row.Provider, row.CandidateCount = resp.Plan.SelectedURL, resp.Plan.DiscoveryProvider, len(resp.Plan.CandidateURLs)
		if row.Provider == "" {
			row.Provider = "local_same_site"
		}
	default:
		return row, fmt.Errorf("unsupported mode %q", item.Mode)
	}
	row.LocalProvider = strings.HasPrefix(strings.TrimSpace(row.Provider), "local_")
	row.BootstrapActive = row.Provider != "" && !row.LocalProvider
	row.Pass, row.Failure = evaluateDiscoveryCase(item, row)
	return row, nil
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
	return true, ""
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
	if err := os.Chdir(filepath.Join("..", "..")); err != nil {
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
