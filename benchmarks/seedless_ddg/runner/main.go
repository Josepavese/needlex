package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/evalutil"
)

type corpus struct {
	Version string `json:"version"`
	Cases   []struct {
		ID             string `json:"id"`
		Goal           string `json:"goal"`
		ExpectedDomain string `json:"expected_domain"`
	} `json:"cases"`
}

type queryResponse struct {
	Plan struct {
		SelectedURL       string   `json:"selected_url"`
		DiscoveryProvider string   `json:"discovery_provider"`
		CandidateURLs     []string `json:"candidate_urls"`
	} `json:"plan"`
	Document struct {
		FinalURL  string `json:"final_url"`
		FetchMode string `json:"fetch_mode"`
	} `json:"document"`
	Trace struct {
		Stages []struct {
			Stage    string            `json:"stage"`
			Metadata map[string]string `json:"metadata"`
		} `json:"stages"`
	} `json:"trace"`
}

type runResult struct {
	Profile         string   `json:"profile"`
	RuntimeOK       bool     `json:"runtime_ok"`
	SelectedURL     string   `json:"selected_url,omitempty"`
	SelectedDomain  string   `json:"selected_domain,omitempty"`
	ExpectedDomain  string   `json:"expected_domain"`
	SelectedPass    bool     `json:"selected_pass"`
	DiscoverySource string   `json:"discovery_provider,omitempty"`
	CandidateCount  int      `json:"candidate_count"`
	DocumentFetch   string   `json:"document_fetch_mode,omitempty"`
	AcquireMetadata []string `json:"acquire_metadata,omitempty"`
	Error           string   `json:"error,omitempty"`
}

type caseResult struct {
	ID    string      `json:"id"`
	Goal  string      `json:"goal"`
	Runs  []runResult `json:"runs"`
	Delta string      `json:"delta"`
}

type summary struct {
	CaseCount                   int     `json:"case_count"`
	StandardPassRate            float64 `json:"standard_pass_rate"`
	StandardSemanticPassRate    float64 `json:"standard_semantic_pass_rate"`
	BrowserLikePassRate         float64 `json:"browser_like_pass_rate"`
	BrowserLikeSemanticPassRate float64 `json:"browser_like_semantic_pass_rate"`
	ImprovementRate             float64 `json:"improvement_rate"`
	BrowserLikeBeatsStandard    int     `json:"browser_like_beats_standard"`
	BestProfile                 string  `json:"best_profile"`
}

type report struct {
	GeneratedAtUTC string       `json:"generated_at_utc"`
	CorpusVersion  string       `json:"corpus_version"`
	BinaryPath     string       `json:"binary_path"`
	Summary        summary      `json:"summary"`
	Results        []caseResult `json:"results"`
}

func main() {
	var outPath, casesPath string
	flag.StringVar(&outPath, "out", "improvements/seedless-ddg-benchmark-latest.json", "output report path")
	flag.StringVar(&casesPath, "cases", "benchmarks/corpora/seedless-ddg-corpus-v1.json", "seedless ddg corpus path")
	flag.Parse()

	c, err := loadCorpus(casesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load corpus: %v\n", err)
		os.Exit(1)
	}
	binaryPath, cleanup, err := buildNeedleBinary()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build binary: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	tempDir, err := os.MkdirTemp("", "needlex-seedless-ddg-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tempDir)

	providerChain := "https://lite.duckduckgo.com/lite/,https://html.duckduckgo.com/html/"
	standardCfg, err := writeConfig(tempDir, "standard.json", map[string]any{
		"fetch":     map[string]any{"profile": "standard", "retry_profile": "standard"},
		"discovery": map[string]any{"provider_chain": providerChain},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "write standard config: %v\n", err)
		os.Exit(1)
	}
	browserCfg, err := writeConfig(tempDir, "browser.json", map[string]any{
		"fetch":     map[string]any{"profile": "browser_like", "retry_profile": "hardened"},
		"discovery": map[string]any{"provider_chain": providerChain},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "write browser config: %v\n", err)
		os.Exit(1)
	}
	semanticBaseURL, stopSemantic, err := startSemanticServer(tempDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start semantic server: %v\n", err)
		os.Exit(1)
	}
	defer stopSemantic()
	standardSemanticCfg, err := writeConfig(tempDir, "standard-semantic.json", map[string]any{
		"fetch":     map[string]any{"profile": "standard", "retry_profile": "standard"},
		"discovery": map[string]any{"provider_chain": providerChain},
		"semantic": map[string]any{
			"enabled":  true,
			"backend":  "openai-embeddings",
			"base_url": semanticBaseURL,
			"model":    "intfloat/multilingual-e5-small",
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "write standard semantic config: %v\n", err)
		os.Exit(1)
	}
	browserSemanticCfg, err := writeConfig(tempDir, "browser-semantic.json", map[string]any{
		"fetch":     map[string]any{"profile": "browser_like", "retry_profile": "hardened"},
		"discovery": map[string]any{"provider_chain": providerChain},
		"semantic": map[string]any{
			"enabled":  true,
			"backend":  "openai-embeddings",
			"base_url": semanticBaseURL,
			"model":    "intfloat/multilingual-e5-small",
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "write browser semantic config: %v\n", err)
		os.Exit(1)
	}

	results := make([]caseResult, 0, len(c.Cases))
	for i, item := range c.Cases {
		fmt.Printf("[seedless-ddg] %s case %d/%d start id=%s\n", time.Now().Format("15:04:05"), i+1, len(c.Cases), item.ID)
		standard := runCase(binaryPath, standardCfg, "standard", item.Goal, item.ExpectedDomain)
		standardSemantic := runCase(binaryPath, standardSemanticCfg, "standard_semantic", item.Goal, item.ExpectedDomain)
		browser := runCase(binaryPath, browserCfg, "browser_like", item.Goal, item.ExpectedDomain)
		browserSemantic := runCase(binaryPath, browserSemanticCfg, "browser_like_semantic", item.Goal, item.ExpectedDomain)
		row := caseResult{
			ID:    item.ID,
			Goal:  item.Goal,
			Runs:  []runResult{standard, standardSemantic, browser, browserSemantic},
			Delta: compareAllRuns(standard, standardSemantic, browser, browserSemantic),
		}
		results = append(results, row)
		fmt.Printf("[seedless-ddg] %s case %d/%d done id=%s std=%t std_sem=%t browser=%t browser_sem=%t delta=%s\n", time.Now().Format("15:04:05"), i+1, len(c.Cases), item.ID, standard.SelectedPass, standardSemantic.SelectedPass, browser.SelectedPass, browserSemantic.SelectedPass, row.Delta)
	}

	rep := report{
		GeneratedAtUTC: time.Now().UTC().Format(time.RFC3339),
		CorpusVersion:  c.Version,
		BinaryPath:     binaryPath,
		Summary:        summarize(results),
		Results:        results,
	}
	if err := evalutil.WriteJSON(outPath, rep); err != nil {
		fmt.Fprintf(os.Stderr, "write report: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Seedless DDG benchmark written to %s\n", outPath)
}

func loadCorpus(path string) (corpus, error) {
	var c corpus
	data, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(data, &c)
	return c, err
}

func buildNeedleBinary() (string, func(), error) {
	tempDir, err := os.MkdirTemp("", "needlex-seedless-ddg-bin-*")
	if err != nil {
		return "", nil, err
	}
	binaryPath := filepath.Join(tempDir, "needle")
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/needle")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", nil, err
	}
	return binaryPath, func() { _ = os.RemoveAll(tempDir) }, nil
}

func writeConfig(dir, name string, payload map[string]any) (string, error) {
	path := filepath.Join(dir, name)
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, raw, 0o644)
}

func runCase(binaryPath, configPath, profile, goal, expectedDomain string) runResult {
	result := runResult{Profile: profile, ExpectedDomain: expectedDomain}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binaryPath, "query", "--goal", goal, "--json", "--json-mode", "full", "--config", configPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.Error = "timeout"
			return result
		}
		result.Error = strings.TrimSpace(string(out))
		return result
	}
	var resp queryResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		result.Error = err.Error()
		return result
	}
	result.RuntimeOK = true
	result.SelectedURL = strings.TrimSpace(resp.Plan.SelectedURL)
	result.SelectedDomain = canonicalHost(result.SelectedURL)
	result.SelectedPass = domainMatches(result.SelectedDomain, expectedDomain)
	result.DiscoverySource = strings.TrimSpace(resp.Plan.DiscoveryProvider)
	result.CandidateCount = len(resp.Plan.CandidateURLs)
	result.DocumentFetch = strings.TrimSpace(resp.Document.FetchMode)
	for _, stage := range resp.Trace.Stages {
		if stage.Stage == "acquire" {
			if mode := strings.TrimSpace(stage.Metadata["fetch_mode"]); mode != "" {
				result.AcquireMetadata = append(result.AcquireMetadata, "fetch_mode="+mode)
			}
			if prof := strings.TrimSpace(stage.Metadata["fetch_profile"]); prof != "" {
				result.AcquireMetadata = append(result.AcquireMetadata, "fetch_profile="+prof)
			}
			if finalURL := strings.TrimSpace(stage.Metadata["final_url"]); finalURL != "" {
				result.AcquireMetadata = append(result.AcquireMetadata, "final_url="+finalURL)
			}
		}
	}
	return result
}

func canonicalHost(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	host = strings.TrimPrefix(host, "www.")
	return host
}

func domainMatches(actual, expected string) bool {
	actual = canonicalHost("https://" + actual)
	expected = canonicalHost("https://" + expected)
	return actual == expected || strings.HasSuffix(actual, "."+expected)
}

func compareAllRuns(standard, standardSemantic, browser, browserSemantic runResult) string {
	profiles := []runResult{standard, standardSemantic, browser, browserSemantic}
	best := profiles[0]
	for _, profile := range profiles[1:] {
		if boolScore(profile) > boolScore(best) {
			best = profile
		}
	}
	return best.Profile
}

func boolScore(r runResult) int {
	score := 0
	if r.RuntimeOK {
		score++
	}
	if r.SelectedPass {
		score += 10
	}
	return score
}

func summarize(results []caseResult) summary {
	var stdPass, stdSemPass, browserPass, browserSemPass int
	profileWins := map[string]int{}
	for _, row := range results {
		for _, run := range row.Runs {
			switch run.Profile {
			case "standard":
				if run.SelectedPass {
					stdPass++
				}
			case "standard_semantic":
				if run.SelectedPass {
					stdSemPass++
				}
			case "browser_like":
				if run.SelectedPass {
					browserPass++
				}
			case "browser_like_semantic":
				if run.SelectedPass {
					browserSemPass++
				}
			}
		}
		profileWins[row.Delta]++
	}
	count := len(results)
	if count == 0 {
		return summary{}
	}
	bestProfile := ""
	bestCount := -1
	for _, name := range []string{"standard", "standard_semantic", "browser_like", "browser_like_semantic"} {
		if profileWins[name] > bestCount {
			bestCount = profileWins[name]
			bestProfile = name
		}
	}
	return summary{
		CaseCount:                   count,
		StandardPassRate:            float64(stdPass) / float64(count),
		StandardSemanticPassRate:    float64(stdSemPass) / float64(count),
		BrowserLikePassRate:         float64(browserPass) / float64(count),
		BrowserLikeSemanticPassRate: float64(browserSemPass) / float64(count),
		ImprovementRate:             float64(browserSemPass-stdPass) / float64(count),
		BrowserLikeBeatsStandard:    browserSemPass - stdPass,
		BestProfile:                 bestProfile,
	}
}

func startSemanticServer(tempDir string) (string, func(), error) {
	logPath := filepath.Join(tempDir, "semantic.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return "", nil, err
	}
	cmd := exec.Command("python3", "scripts/run_semantic_embed_upstream.py")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return "", nil, err
	}
	baseURL := "http://127.0.0.1:18180"
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/healthz")
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return baseURL, func() {
					_ = cmd.Process.Kill()
					_, _ = cmd.Process.Wait()
					_ = logFile.Close()
				}, nil
			}
		}
		time.Sleep(1 * time.Second)
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
	_ = logFile.Close()
	logRaw, _ := os.ReadFile(logPath)
	return "", nil, fmt.Errorf("semantic server not healthy: %s", strings.TrimSpace(string(logRaw)))
}
