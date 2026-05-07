package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/josepavese/needlex/benchmarks/internal/evalutil"
)

type corpus struct {
	Version string `json:"version"`
	Cases   []struct {
		ID          string `json:"id"`
		Goal        string `json:"goal"`
		ExpectedURL string `json:"expected_url"`
	} `json:"cases"`
}

type runResult struct {
	Profile         string   `json:"profile"`
	Skipped         bool     `json:"skipped,omitempty"`
	SkipReason      string   `json:"skip_reason,omitempty"`
	Pass            bool     `json:"pass"`
	SelectedURL     string   `json:"selected_url,omitempty"`
	Provider        string   `json:"provider,omitempty"`
	Candidates      []string `json:"candidates,omitempty"`
	CandidateCount  int      `json:"candidate_count,omitempty"`
	LatencyMS       int64    `json:"latency_ms,omitempty"`
	RetryCount      int      `json:"retry_count,omitempty"`
	RetrySleepMS    int64    `json:"retry_sleep_ms,omitempty"`
	HostPacingMS    int64    `json:"host_pacing_ms,omitempty"`
	RetryReason     string   `json:"retry_reason,omitempty"`
	AcquireMetadata []string `json:"acquire_metadata,omitempty"`
	ErrorKind       string   `json:"error_kind,omitempty"`
	Error           string   `json:"error,omitempty"`
}

type endpointQueryResponse struct {
	Plan struct {
		DiscoveryProvider string   `json:"discovery_provider"`
		SelectedURL       string   `json:"selected_url"`
		CandidateURLs     []string `json:"candidate_urls"`
	} `json:"plan"`
	CostReport struct {
		LatencyMS int64 `json:"latency_ms"`
	} `json:"cost_report"`
	Trace struct {
		Stages []struct {
			Stage    string            `json:"stage"`
			Metadata map[string]string `json:"metadata"`
		} `json:"stages"`
	} `json:"trace"`
}

type caseResult struct {
	ID          string      `json:"id"`
	Goal        string      `json:"goal"`
	ExpectedURL string      `json:"expected_url"`
	Runs        []runResult `json:"runs"`
}

type summary struct {
	CaseCount                int                `json:"case_count"`
	BaselinePassRate         float64            `json:"baseline_pass_rate"`
	SemanticPassRate         float64            `json:"semantic_pass_rate"`
	LLMPassRate              float64            `json:"llm_pass_rate"`
	LLMConfigured            bool               `json:"llm_configured"`
	RetryRateByProfile       map[string]float64 `json:"retry_rate_by_profile,omitempty"`
	AvgRetryCountByProfile   map[string]float64 `json:"avg_retry_count_by_profile,omitempty"`
	AvgRetrySleepMSByProfile map[string]float64 `json:"avg_retry_sleep_ms_by_profile,omitempty"`
	AvgHostPacingMSByProfile map[string]float64 `json:"avg_host_pacing_ms_by_profile,omitempty"`
	RetryReasons             map[string]int     `json:"retry_reasons,omitempty"`
}

type report struct {
	GeneratedAtUTC string       `json:"generated_at_utc"`
	CorpusVersion  string       `json:"corpus_version"`
	BinaryPath     string       `json:"binary_path"`
	Summary        summary      `json:"summary"`
	Results        []caseResult `json:"results"`
}

type benchmarkConfigs struct {
	baseline           string
	semantic           string
	semanticConfigured bool
	semanticSkipReason string
	llm                string
	llmConfigured      bool
	llmSkipReason      string
}

func main() {
	var outPath, casesPath string
	flag.StringVar(&outPath, "out", "improvements/native-api-endpoint-benchmark-latest.json", "output report path")
	flag.StringVar(&casesPath, "cases", "benchmarks/corpora/native-api-endpoint-corpus-v1.json", "endpoint corpus path")
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

	tempDir, err := os.MkdirTemp("", "needlex-native-endpoint-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "temp dir: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	cfgs, stopSemantic, err := prepareBenchmarkConfigs(tempDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare configs: %v\n", err)
		os.Exit(1)
	}
	defer stopSemantic()

	results := runEndpointCases(binaryPath, c.Cases, cfgs)
	rep := report{
		GeneratedAtUTC: time.Now().UTC().Format(time.RFC3339),
		CorpusVersion:  c.Version,
		BinaryPath:     binaryPath,
		Summary:        summarize(results, cfgs.llmConfigured),
		Results:        results,
	}
	if err := evalutil.WriteJSON(outPath, rep); err != nil {
		fmt.Fprintf(os.Stderr, "write report: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Native API endpoint benchmark written to %s\n", outPath)
}

func prepareBenchmarkConfigs(tempDir string) (benchmarkConfigs, func(), error) {
	providerChain := "https://lite.duckduckgo.com/lite/,https://html.duckduckgo.com/html/"
	baselineCfg, err := writeEndpointBaselineConfig(tempDir, providerChain)
	if err != nil {
		return benchmarkConfigs{}, nil, err
	}
	stopSemantic := func() {}
	semanticCfg, err := writeEndpointSemanticConfig(tempDir, providerChain)
	if err != nil {
		stopSemantic()
		return benchmarkConfigs{}, nil, err
	}
	llmCfg, llmConfigured, llmSkipReason, err := maybeWriteLLMConfig(tempDir, providerChain)
	if err != nil {
		stopSemantic()
		return benchmarkConfigs{}, nil, err
	}
	return benchmarkConfigs{baseline: baselineCfg, semantic: semanticCfg, semanticConfigured: true, semanticSkipReason: "", llm: llmCfg, llmConfigured: llmConfigured, llmSkipReason: llmSkipReason}, stopSemantic, nil
}

func writeEndpointBaselineConfig(tempDir, providerChain string) (string, error) {
	return writeConfig(tempDir, "baseline.json", map[string]any{
		"fetch":     map[string]any{"profile": "browser_like", "retry_profile": "hardened"},
		"discovery": map[string]any{"provider_chain": providerChain},
		"models":    map[string]any{"backend": "noop"},
	})
}

func writeEndpointSemanticConfig(tempDir, providerChain string) (string, error) {
	return writeConfig(tempDir, "semantic.json", map[string]any{
		"fetch":     map[string]any{"profile": "browser_like", "retry_profile": "hardened"},
		"discovery": map[string]any{"provider_chain": providerChain},
		"models":    map[string]any{"backend": "noop"},
		"semantic":  endpointSemanticPayload(),
	})
}

func runEndpointCases(binaryPath string, cases []struct {
	ID          string `json:"id"`
	Goal        string `json:"goal"`
	ExpectedURL string `json:"expected_url"`
}, cfgs benchmarkConfigs,
) []caseResult {
	results := make([]caseResult, 0, len(cases))
	for i, item := range cases {
		fmt.Printf("[native-endpoint] %s case %d/%d start id=%s\n", time.Now().Format("15:04:05"), i+1, len(cases), item.ID)
		row := runEndpointCase(binaryPath, item.ID, item.Goal, item.ExpectedURL, cfgs)
		results = append(results, row)
		fmt.Printf("[native-endpoint] %s case %d/%d done id=%s baseline=%t semantic=%t llm=%t\n",
			time.Now().Format("15:04:05"), i+1, len(cases), item.ID,
			row.Runs[0].Pass, row.Runs[1].Pass, row.Runs[2].Pass)
		time.Sleep(2 * time.Second)
	}
	return results
}

func runEndpointCase(binaryPath, id, goal, expectedURL string, cfgs benchmarkConfigs) caseResult {
	row := caseResult{
		ID:          id,
		Goal:        goal,
		ExpectedURL: expectedURL,
		Runs: []runResult{
			runCase(binaryPath, cfgs.baseline, "baseline", goal, expectedURL),
		},
	}
	if cfgs.semanticConfigured {
		row.Runs = append(row.Runs, runCase(binaryPath, cfgs.semantic, "semantic", goal, expectedURL))
	} else {
		row.Runs = append(row.Runs, runResult{Profile: "semantic", Skipped: true, SkipReason: cfgs.semanticSkipReason})
	}
	if cfgs.llmConfigured {
		row.Runs = append(row.Runs, runCase(binaryPath, cfgs.llm, "llm_enabled", goal, expectedURL))
	} else {
		row.Runs = append(row.Runs, runResult{Profile: "llm_enabled", Skipped: true, SkipReason: cfgs.llmSkipReason})
	}
	return row
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
	tempDir, err := os.MkdirTemp("", "needlex-native-endpoint-bin-*")
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

func maybeWriteLLMConfig(dir, providerChain string) (string, bool, string, error) {
	backend := strings.TrimSpace(os.Getenv("NEEDLEX_MODELS_BACKEND"))
	baseURL := strings.TrimSpace(os.Getenv("NEEDLEX_MODELS_BASE_URL"))
	router := strings.TrimSpace(os.Getenv("NEEDLEX_MODELS_ROUTER"))
	if backend != "openai-compatible" || baseURL == "" || router == "" {
		return "", false, "set NEEDLEX_MODELS_BACKEND=openai-compatible, NEEDLEX_MODELS_BASE_URL and NEEDLEX_MODELS_ROUTER", nil
	}
	payload := map[string]any{
		"fetch":     map[string]any{"profile": "browser_like", "retry_profile": "hardened"},
		"discovery": map[string]any{"provider_chain": providerChain},
		"semantic":  endpointSemanticPayload(),
		"models": map[string]any{
			"backend":   backend,
			"base_url":  baseURL,
			"router":    router,
			"judge":     firstNonEmpty(strings.TrimSpace(os.Getenv("NEEDLEX_MODELS_JUDGE")), router),
			"extractor": firstNonEmpty(strings.TrimSpace(os.Getenv("NEEDLEX_MODELS_EXTRACTOR")), router),
			"formatter": firstNonEmpty(strings.TrimSpace(os.Getenv("NEEDLEX_MODELS_FORMATTER")), router),
			"api_key":   strings.TrimSpace(os.Getenv("NEEDLEX_MODELS_API_KEY")),
		},
	}
	configPath, err := writeConfig(dir, "llm.json", payload)
	return configPath, true, "", err
}

func endpointSemanticPayload() map[string]any {
	payload := map[string]any{}
	if endpoint := strings.TrimSpace(os.Getenv("NEEDLEX_SEMANTIC_EMBEDDING_URL")); endpoint != "" {
		payload["embedding_url"] = endpoint
	}
	if providerModel := strings.TrimSpace(os.Getenv("NEEDLEX_SEMANTIC_PROVIDER_MODEL")); providerModel != "" {
		payload["provider_model"] = providerModel
	}
	if vectorSpace := strings.TrimSpace(os.Getenv("NEEDLEX_SEMANTIC_VECTOR_SPACE")); vectorSpace != "" {
		payload["vector_space"] = vectorSpace
	}
	return payload
}

func runCase(binaryPath, configPath, profile, goal, expectedURL string) runResult {
	result := runResult{Profile: profile}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	started := time.Now()
	cmd := exec.CommandContext(ctx, binaryPath, "query", "--goal", goal, "--json", "--json-mode", "full", "--config", configPath)
	out, err := cmd.CombinedOutput()
	result.LatencyMS = time.Since(started).Milliseconds()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.ErrorKind = "benchmark_timeout"
			result.Error = "timeout"
			return result
		}
		result.Error = strings.TrimSpace(string(out))
		result.ErrorKind = classifyRunError(result.Error)
		return result
	}
	var resp endpointQueryResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		result.ErrorKind = "invalid_json"
		result.Error = err.Error()
		return result
	}
	return finalizeRunResult(result, resp, expectedURL)
}

func finalizeRunResult(result runResult, resp endpointQueryResponse, expectedURL string) runResult {
	result.SelectedURL = strings.TrimSpace(resp.Plan.SelectedURL)
	result.Provider = strings.TrimSpace(resp.Plan.DiscoveryProvider)
	result.Candidates = append(result.Candidates, resp.Plan.CandidateURLs...)
	result.CandidateCount = len(resp.Plan.CandidateURLs)
	if resp.CostReport.LatencyMS > 0 {
		result.LatencyMS = resp.CostReport.LatencyMS
	}
	for _, stage := range resp.Trace.Stages {
		if stage.Stage != "acquire" {
			continue
		}
		applyAcquireStage(&result, stage.Metadata)
	}
	result.Pass = sameCanonicalURL(result.SelectedURL, expectedURL)
	if !result.Pass {
		if strings.TrimSpace(result.SelectedURL) == "" {
			result.ErrorKind = "empty_selection"
		} else {
			result.ErrorKind = "ranking_miss"
		}
	}
	return result
}

func applyAcquireStage(result *runResult, metadata map[string]string) {
	for _, key := range []string{"fetch_mode", "fetch_profile", "final_url"} {
		if value := strings.TrimSpace(metadata[key]); value != "" {
			result.AcquireMetadata = append(result.AcquireMetadata, key+"="+value)
		}
	}
	result.RetryCount = parseInt(metadata["retry_count"])
	result.RetrySleepMS = int64(parseInt(metadata["retry_sleep_ms"]))
	result.HostPacingMS = int64(parseInt(metadata["host_pacing_ms"]))
	result.RetryReason = strings.TrimSpace(metadata["retry_reason"])
	appendNumericAcquireMetadata(result)
}

func appendNumericAcquireMetadata(result *runResult) {
	if result.RetryCount > 0 {
		result.AcquireMetadata = append(result.AcquireMetadata, fmt.Sprintf("retry_count=%d", result.RetryCount))
	}
	if result.RetrySleepMS > 0 {
		result.AcquireMetadata = append(result.AcquireMetadata, fmt.Sprintf("retry_sleep_ms=%d", result.RetrySleepMS))
	}
	if result.HostPacingMS > 0 {
		result.AcquireMetadata = append(result.AcquireMetadata, fmt.Sprintf("host_pacing_ms=%d", result.HostPacingMS))
	}
	if result.RetryReason != "" {
		result.AcquireMetadata = append(result.AcquireMetadata, "retry_reason="+result.RetryReason)
	}
}

func parseInt(raw string) int {
	value, _ := strconv.Atoi(strings.TrimSpace(raw))
	return value
}

func classifyRunError(raw string) string {
	text := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case text == "":
		return "unknown"
	case strings.Contains(text, "duckduckgo provider blocked by anti-bot challenge"):
		return "provider_blocked"
	case strings.Contains(text, "unsupported content type"):
		return "unsupported_content_type"
	case strings.Contains(text, "timeout"):
		return "timeout"
	case strings.Contains(text, "no segments produced"):
		return "empty_segments"
	case strings.Contains(text, "returned no candidates"):
		return "no_candidates"
	case strings.Contains(text, "returned no choices"):
		return "model_empty_choices"
	case strings.Contains(text, "returned empty content"):
		return "model_empty_content"
	default:
		return "runtime_error"
	}
}

func sameCanonicalURL(left, right string) bool {
	return canonicalizeURL(left) == canonicalizeURL(right)
}

func canonicalizeURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	host = strings.TrimPrefix(host, "www.")
	path := strings.TrimSuffix(parsed.EscapedPath(), "/")
	if path == "" {
		path = "/"
	}
	return strings.ToLower(parsed.Scheme) + "://" + host + path
}

func summarize(results []caseResult, llmConfigured bool) summary {
	count := len(results)
	if count == 0 {
		return summary{}
	}
	var baselinePass, semanticPass, llmPass int
	totalRuns := map[string]int{}
	retryRuns := map[string]int{}
	retryCountSum := map[string]int{}
	retrySleepSum := map[string]int64{}
	hostPacingSum := map[string]int64{}
	retryReasons := map[string]int{}
	for _, row := range results {
		for _, run := range row.Runs {
			totalRuns[run.Profile]++
			if run.RetryCount > 0 {
				retryRuns[run.Profile]++
				retryCountSum[run.Profile] += run.RetryCount
			}
			retrySleepSum[run.Profile] += run.RetrySleepMS
			hostPacingSum[run.Profile] += run.HostPacingMS
			if run.RetryReason != "" {
				retryReasons[run.RetryReason]++
			}
			switch run.Profile {
			case "baseline":
				if run.Pass {
					baselinePass++
				}
			case "semantic":
				if run.Pass {
					semanticPass++
				}
			case "llm_enabled":
				if run.Pass {
					llmPass++
				}
			}
		}
	}
	retryRateByProfile := map[string]float64{}
	avgRetryCountByProfile := map[string]float64{}
	avgRetrySleepMSByProfile := map[string]float64{}
	avgHostPacingMSByProfile := map[string]float64{}
	for _, name := range []string{"baseline", "semantic", "llm_enabled"} {
		if totalRuns[name] == 0 {
			continue
		}
		retryRateByProfile[name] = float64(retryRuns[name]) / float64(totalRuns[name])
		avgRetryCountByProfile[name] = float64(retryCountSum[name]) / float64(totalRuns[name])
		avgRetrySleepMSByProfile[name] = float64(retrySleepSum[name]) / float64(totalRuns[name])
		avgHostPacingMSByProfile[name] = float64(hostPacingSum[name]) / float64(totalRuns[name])
	}
	return summary{
		CaseCount:                count,
		BaselinePassRate:         float64(baselinePass) / float64(count),
		SemanticPassRate:         float64(semanticPass) / float64(count),
		LLMPassRate:              float64(llmPass) / float64(count),
		LLMConfigured:            llmConfigured,
		RetryRateByProfile:       retryRateByProfile,
		AvgRetryCountByProfile:   avgRetryCountByProfile,
		AvgRetrySleepMSByProfile: avgRetrySleepMSByProfile,
		AvgHostPacingMSByProfile: avgHostPacingMSByProfile,
		RetryReasons:             retryReasons,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
