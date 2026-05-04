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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/josepavese/needlex/benchmarks/internal/evalutil"
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
	Profile          string       `json:"profile"`
	AttemptCount     int          `json:"attempt_count,omitempty"`
	PassCount        int          `json:"pass_count,omitempty"`
	RuntimePassCount int          `json:"runtime_pass_count,omitempty"`
	RuntimeOK        bool         `json:"runtime_ok"`
	SelectedURL      string       `json:"selected_url,omitempty"`
	SelectedDomain   string       `json:"selected_domain,omitempty"`
	ExpectedDomain   string       `json:"expected_domain"`
	SelectedPass     bool         `json:"selected_pass"`
	DiscoverySource  string       `json:"discovery_provider,omitempty"`
	CandidateCount   int          `json:"candidate_count"`
	DocumentFetch    string       `json:"document_fetch_mode,omitempty"`
	AcquireMetadata  []string     `json:"acquire_metadata,omitempty"`
	RetryCount       int          `json:"retry_count,omitempty"`
	RetrySleepMS     int64        `json:"retry_sleep_ms,omitempty"`
	HostPacingMS     int64        `json:"host_pacing_ms,omitempty"`
	RetryReason      string       `json:"retry_reason,omitempty"`
	ErrorKind        string       `json:"error_kind,omitempty"`
	LatencyMS        int64        `json:"latency_ms,omitempty"`
	Error            string       `json:"error,omitempty"`
	Attempts         []runAttempt `json:"attempts,omitempty"`
}

type runAttempt struct {
	Attempt         int    `json:"attempt"`
	RuntimeOK       bool   `json:"runtime_ok"`
	SelectedURL     string `json:"selected_url,omitempty"`
	SelectedDomain  string `json:"selected_domain,omitempty"`
	SelectedPass    bool   `json:"selected_pass"`
	DiscoverySource string `json:"discovery_provider,omitempty"`
	CandidateCount  int    `json:"candidate_count"`
	DocumentFetch   string `json:"document_fetch_mode,omitempty"`
	RetryCount      int    `json:"retry_count,omitempty"`
	RetrySleepMS    int64  `json:"retry_sleep_ms,omitempty"`
	HostPacingMS    int64  `json:"host_pacing_ms,omitempty"`
	RetryReason     string `json:"retry_reason,omitempty"`
	ErrorKind       string `json:"error_kind,omitempty"`
	LatencyMS       int64  `json:"latency_ms,omitempty"`
	Error           string `json:"error,omitempty"`
}

type caseResult struct {
	ID    string      `json:"id"`
	Goal  string      `json:"goal"`
	Runs  []runResult `json:"runs"`
	Delta string      `json:"delta"`
}

type summary struct {
	CaseCount                   int                `json:"case_count"`
	StandardPassRate            float64            `json:"standard_pass_rate"`
	StandardSemanticPassRate    float64            `json:"standard_semantic_pass_rate"`
	BrowserLikePassRate         float64            `json:"browser_like_pass_rate"`
	BrowserLikeSemanticPassRate float64            `json:"browser_like_semantic_pass_rate"`
	ImprovementRate             float64            `json:"improvement_rate"`
	BrowserLikeBeatsStandard    int                `json:"browser_like_beats_standard"`
	BestProfile                 string             `json:"best_profile"`
	ProfilePassRates            map[string]float64 `json:"profile_pass_rates,omitempty"`
	ProfileRuntimeRates         map[string]float64 `json:"profile_runtime_rates,omitempty"`
	RetryRateByProfile          map[string]float64 `json:"retry_rate_by_profile,omitempty"`
	AvgRetryCountByProfile      map[string]float64 `json:"avg_retry_count_by_profile,omitempty"`
	AvgRetrySleepMSByProfile    map[string]float64 `json:"avg_retry_sleep_ms_by_profile,omitempty"`
	AvgHostPacingMSByProfile    map[string]float64 `json:"avg_host_pacing_ms_by_profile,omitempty"`
	RetryReasons                map[string]int     `json:"retry_reasons,omitempty"`
	ErrorKinds                  map[string]int     `json:"error_kinds,omitempty"`
	RunnerRuns                  int                `json:"runner_runs,omitempty"`
	RunnerTimeoutMS             int64              `json:"runner_timeout_ms,omitempty"`
	RunnerProfiles              []string           `json:"runner_profiles,omitempty"`
	RunnerProviderChains        []string           `json:"runner_provider_chains,omitempty"`
}

type report struct {
	GeneratedAtUTC string       `json:"generated_at_utc"`
	CorpusVersion  string       `json:"corpus_version"`
	BinaryPath     string       `json:"binary_path"`
	Summary        summary      `json:"summary"`
	Results        []caseResult `json:"results"`
}

type seedlessOptions struct {
	outPath        string
	casesPath      string
	profiles       string
	providerChains string
	runs           int
	timeoutMS      int64
}

type seedlessConfigs struct {
	profiles       []seedlessProfileConfig
	providerChains []string
}

type seedlessProfileConfig struct {
	name string
	path string
}

type seedlessProfileDefinition struct {
	name         string
	fetchProfile string
	retryProfile string
	semantic     bool
}

type seedlessProviderChain struct {
	name          string
	providerChain string
}

type seedlessSummaryAgg struct {
	stdPass, stdSemPass, browserPass, browserSemPass int
	profileWins                                      map[string]int
	profilePass                                      map[string]int
	profileRuntimePass                               map[string]int
	retryRuns                                        map[string]int
	totalRuns                                        map[string]int
	retryCountSum                                    map[string]int
	retrySleepSum                                    map[string]int64
	hostPacingSum                                    map[string]int64
	retryReasons                                     map[string]int
	errorKinds                                       map[string]int
}

func main() {
	opts := parseSeedlessOptions()
	c, err := loadCorpus(opts.casesPath)
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
	defer func() { _ = os.RemoveAll(tempDir) }()

	cfgs, stopSemantic, err := prepareSeedlessConfigs(tempDir, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare configs: %v\n", err)
		os.Exit(1)
	}
	defer stopSemantic()

	results := runSeedlessCases(binaryPath, c.Cases, cfgs, opts)
	rep := report{
		GeneratedAtUTC: time.Now().UTC().Format(time.RFC3339),
		CorpusVersion:  c.Version,
		BinaryPath:     binaryPath,
		Summary:        summarize(results, opts.runs, opts.timeoutMS, cfgs),
		Results:        results,
	}
	if err := evalutil.WriteJSON(opts.outPath, rep); err != nil {
		fmt.Fprintf(os.Stderr, "write report: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Seedless DDG benchmark written to %s\n", opts.outPath)
}

func parseSeedlessOptions() seedlessOptions {
	var opts seedlessOptions
	flag.StringVar(&opts.outPath, "out", "improvements/seedless-ddg-benchmark-latest.json", "output report path")
	flag.StringVar(&opts.casesPath, "cases", "benchmarks/corpora/seedless-ddg-corpus-v1.json", "seedless ddg corpus path")
	flag.StringVar(&opts.profiles, "profiles", "standard,standard_semantic,browser_like,browser_like_semantic", "comma-separated profiles: standard, standard_semantic, browser_like, browser_like_semantic")
	flag.StringVar(&opts.providerChains, "provider-chains", "ddg_bing=https://lite.duckduckgo.com/lite/,https://html.duckduckgo.com/html/,https://www.bing.com/search", "semicolon-separated provider chains, optionally name=url1,url2")
	flag.IntVar(&opts.runs, "runs", 3, "number of attempts per case/profile")
	flag.Int64Var(&opts.timeoutMS, "timeout-ms", 25000, "per-run timeout in milliseconds")
	flag.Parse()
	if opts.runs <= 0 {
		opts.runs = 1
	}
	if opts.timeoutMS <= 0 {
		opts.timeoutMS = 25000
	}
	return opts
}

func parseSeedlessProfiles(raw string) ([]seedlessProfileDefinition, error) {
	parts := strings.Split(raw, ",")
	out := make([]seedlessProfileDefinition, 0, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		profile, ok := seedlessProfileByName(name)
		if !ok {
			return nil, fmt.Errorf("unsupported seedless profile %q", name)
		}
		out = append(out, profile)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one seedless profile is required")
	}
	return out, nil
}

func seedlessProfileByName(name string) (seedlessProfileDefinition, bool) {
	switch strings.TrimSpace(name) {
	case "standard":
		return seedlessProfileDefinition{name: "standard", fetchProfile: "standard", retryProfile: "standard"}, true
	case "standard_semantic":
		return seedlessProfileDefinition{name: "standard_semantic", fetchProfile: "standard", retryProfile: "standard", semantic: true}, true
	case "browser_like":
		return seedlessProfileDefinition{name: "browser_like", fetchProfile: "browser_like", retryProfile: "hardened"}, true
	case "browser_like_semantic":
		return seedlessProfileDefinition{name: "browser_like_semantic", fetchProfile: "browser_like", retryProfile: "hardened", semantic: true}, true
	default:
		return seedlessProfileDefinition{}, false
	}
}

func seedlessProfilesNeedSemantic(profiles []seedlessProfileDefinition) bool {
	for _, profile := range profiles {
		if profile.semantic {
			return true
		}
	}
	return false
}

func parseProviderChains(raw string) ([]seedlessProviderChain, error) {
	entries := strings.Split(raw, ";")
	out := make([]seedlessProviderChain, 0, len(entries))
	for i, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		name := fmt.Sprintf("chain_%d", i+1)
		providerChain := entry
		if before, after, ok := strings.Cut(entry, "="); ok {
			name = safeStateComponent(before)
			providerChain = strings.TrimSpace(after)
		}
		if strings.TrimSpace(providerChain) == "" {
			return nil, fmt.Errorf("provider chain %q is empty", entry)
		}
		out = append(out, seedlessProviderChain{name: name, providerChain: providerChain})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one provider chain is required")
	}
	return out, nil
}

func providerChainNames(chains []seedlessProviderChain) []string {
	out := make([]string, 0, len(chains))
	for _, chain := range chains {
		out = append(out, chain.name)
	}
	return out
}

func prepareSeedlessConfigs(tempDir string, opts seedlessOptions) (seedlessConfigs, func(), error) {
	selectedProfiles, err := parseSeedlessProfiles(opts.profiles)
	if err != nil {
		return seedlessConfigs{}, nil, err
	}
	chains, err := parseProviderChains(opts.providerChains)
	if err != nil {
		return seedlessConfigs{}, nil, err
	}
	semanticBaseURL := ""
	stopSemantic := func() {}
	if seedlessProfilesNeedSemantic(selectedProfiles) {
		semanticBaseURL, stopSemantic, err = startSemanticServer(tempDir)
		if err != nil {
			return seedlessConfigs{}, nil, err
		}
	}
	configs := make([]seedlessProfileConfig, 0, len(selectedProfiles)*len(chains))
	for _, chain := range chains {
		for _, profile := range selectedProfiles {
			name := profile.name
			if len(chains) > 1 {
				name += "@" + chain.name
			}
			baseURL := ""
			if profile.semantic {
				baseURL = semanticBaseURL
			}
			path, err := writeSeedlessConfig(tempDir, safeStateComponent(name)+".json", chain.providerChain, profile.fetchProfile, profile.retryProfile, baseURL)
			if err != nil {
				stopSemantic()
				return seedlessConfigs{}, nil, err
			}
			configs = append(configs, seedlessProfileConfig{name: name, path: path})
		}
	}
	return seedlessConfigs{profiles: configs, providerChains: providerChainNames(chains)}, stopSemantic, nil
}

func writeSeedlessConfig(tempDir, name, providerChain, fetchProfile, retryProfile, semanticBaseURL string) (string, error) {
	payload := map[string]any{
		"fetch":     map[string]any{"profile": fetchProfile, "retry_profile": retryProfile},
		"discovery": map[string]any{"provider_chain": providerChain},
	}
	if semanticBaseURL != "" {
		payload["semantic"] = map[string]any{
			"enabled":  true,
			"backend":  "openai-embeddings",
			"base_url": semanticBaseURL,
			"model":    "intfloat/multilingual-e5-small",
		}
	}
	return writeConfig(tempDir, name, payload)
}

func runSeedlessCases(binaryPath string, cases []struct {
	ID             string `json:"id"`
	Goal           string `json:"goal"`
	ExpectedDomain string `json:"expected_domain"`
}, cfgs seedlessConfigs, opts seedlessOptions,
) []caseResult {
	results := make([]caseResult, 0, len(cases))
	for i, item := range cases {
		fmt.Printf("[seedless-ddg] %s case %d/%d start id=%s\n", time.Now().Format("15:04:05"), i+1, len(cases), item.ID)
		row := runSeedlessCase(binaryPath, item.ID, item.Goal, item.ExpectedDomain, cfgs, opts)
		results = append(results, row)
		fmt.Printf("[seedless-ddg] %s case %d/%d done id=%s best=%s profile_passes=%s\n", time.Now().Format("15:04:05"), i+1, len(cases), item.ID, row.Delta, formatProfilePasses(row.Runs))
	}
	return results
}

func runSeedlessCase(binaryPath, id, goal, expectedDomain string, cfgs seedlessConfigs, opts seedlessOptions) caseResult {
	runs := make([]runResult, 0, len(cfgs.profiles))
	for _, profile := range cfgs.profiles {
		runs = append(runs, runCase(binaryPath, profile.path, profile.name, id, goal, expectedDomain, opts.runs, opts.timeoutMS))
	}
	return caseResult{
		ID:    id,
		Goal:  goal,
		Runs:  runs,
		Delta: compareAllRuns(runs...),
	}
}

func formatProfilePasses(runs []runResult) string {
	parts := make([]string, 0, len(runs))
	for _, run := range runs {
		parts = append(parts, fmt.Sprintf("%s=%t", run.Profile, run.SelectedPass))
	}
	return strings.Join(parts, ",")
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

func runCase(binaryPath, configPath, profile, caseID, goal, expectedDomain string, runs int, timeoutMS int64) runResult {
	if runs <= 1 {
		result := runCaseOnce(binaryPath, configPath, profile, caseID, "single", goal, expectedDomain, timeoutMS)
		result.AttemptCount = 1
		result.PassCount = boolToInt(result.SelectedPass)
		result.RuntimePassCount = boolToInt(result.RuntimeOK)
		return result
	}
	attempts := make([]runAttempt, 0, runs)
	passCount := 0
	runtimePassCount := 0
	best := runResult{Profile: profile, ExpectedDomain: expectedDomain}
	bestScore := -1
	errorKinds := map[string]int{}
	selectedURLCounts := map[string]int{}
	for i := 0; i < runs; i++ {
		attempt := runCaseOnce(binaryPath, configPath, profile, caseID, fmt.Sprintf("attempt-%d", i+1), goal, expectedDomain, timeoutMS)
		attempts = append(attempts, runAttempt{
			Attempt:         i + 1,
			RuntimeOK:       attempt.RuntimeOK,
			SelectedURL:     attempt.SelectedURL,
			SelectedDomain:  attempt.SelectedDomain,
			SelectedPass:    attempt.SelectedPass,
			DiscoverySource: attempt.DiscoverySource,
			CandidateCount:  attempt.CandidateCount,
			DocumentFetch:   attempt.DocumentFetch,
			RetryCount:      attempt.RetryCount,
			RetrySleepMS:    attempt.RetrySleepMS,
			HostPacingMS:    attempt.HostPacingMS,
			RetryReason:     attempt.RetryReason,
			ErrorKind:       attempt.ErrorKind,
			LatencyMS:       attempt.LatencyMS,
			Error:           attempt.Error,
		})
		if attempt.SelectedPass {
			passCount++
		}
		if attempt.RuntimeOK {
			runtimePassCount++
		}
		if attempt.ErrorKind != "" {
			errorKinds[attempt.ErrorKind]++
		}
		if attempt.SelectedURL != "" {
			selectedURLCounts[attempt.SelectedURL]++
		}
		if score := boolScore(attempt); score > bestScore {
			bestScore = score
			best = attempt
		}
	}
	best.AttemptCount = runs
	best.PassCount = passCount
	best.RuntimePassCount = runtimePassCount
	best.RuntimeOK = runtimePassCount*2 >= runs
	best.SelectedPass = passCount*2 >= runs
	best.Attempts = attempts
	if best.SelectedURL == "" {
		best.SelectedURL = mostCommonKey(selectedURLCounts)
		best.SelectedDomain = canonicalHost(best.SelectedURL)
	}
	if !best.SelectedPass {
		best.ErrorKind = mostCommonKey(errorKinds)
	}
	return best
}

func runCaseOnce(binaryPath, configPath, profile, caseID, attemptID, goal, expectedDomain string, timeoutMS int64) runResult {
	result := runResult{Profile: profile, ExpectedDomain: expectedDomain}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()
	started := time.Now()
	cmd := exec.CommandContext(ctx, binaryPath, "query", "--goal", goal, "--json", "--json-mode", "full", "--config", configPath)
	cmd.Env = append(os.Environ(), "NEEDLEX_HOME="+seedlessStateRoot(configPath, caseID, profile, attemptID))
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
	var resp queryResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		result.Error = err.Error()
		result.ErrorKind = "invalid_json"
		return result
	}
	return finalizeRunResult(result, resp, expectedDomain)
}

func seedlessStateRoot(configPath, caseID, profile, attemptID string) string {
	return filepath.Join(filepath.Dir(configPath), "state", safeStateComponent(caseID), safeStateComponent(profile), safeStateComponent(attemptID))
}

func safeStateComponent(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	cleaned := strings.Trim(b.String(), "-")
	if cleaned == "" {
		return "unknown"
	}
	return cleaned
}

func finalizeRunResult(result runResult, resp queryResponse, expectedDomain string) runResult {
	result.RuntimeOK = true
	result.SelectedURL = strings.TrimSpace(resp.Plan.SelectedURL)
	result.SelectedDomain = canonicalHost(result.SelectedURL)
	result.SelectedPass = domainMatches(result.SelectedDomain, expectedDomain)
	result.DiscoverySource = strings.TrimSpace(resp.Plan.DiscoveryProvider)
	result.CandidateCount = len(resp.Plan.CandidateURLs)
	result.DocumentFetch = strings.TrimSpace(resp.Document.FetchMode)
	for _, stage := range resp.Trace.Stages {
		if stage.Stage != "acquire" {
			continue
		}
		applyAcquireStage(&result, stage.Metadata)
	}
	if !result.SelectedPass {
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

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func mostCommonKey[T comparable](items map[T]int) T {
	var zero T
	bestCount := -1
	best := zero
	for key, count := range items {
		if count > bestCount {
			bestCount = count
			best = key
		}
	}
	return best
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
	default:
		return "runtime_error"
	}
}

func parseInt(raw string) int {
	value, _ := strconv.Atoi(strings.TrimSpace(raw))
	return value
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

func compareAllRuns(profiles ...runResult) string {
	if len(profiles) == 0 {
		return ""
	}
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
	if r.RetryCount > 0 {
		score++
	}
	return score
}

func summarize(results []caseResult, runs int, timeoutMS int64, cfgs seedlessConfigs) summary {
	agg := newSeedlessSummaryAgg()
	for _, row := range results {
		agg.recordRow(row)
	}
	count := len(results)
	if count == 0 {
		return summary{}
	}
	retry := agg.retrySummaries()
	return summary{
		CaseCount:                   count,
		StandardPassRate:            float64(agg.stdPass) / float64(count),
		StandardSemanticPassRate:    float64(agg.stdSemPass) / float64(count),
		BrowserLikePassRate:         float64(agg.browserPass) / float64(count),
		BrowserLikeSemanticPassRate: float64(agg.browserSemPass) / float64(count),
		ImprovementRate:             float64(agg.browserSemPass-agg.stdPass) / float64(count),
		BrowserLikeBeatsStandard:    agg.browserSemPass - agg.stdPass,
		BestProfile:                 bestProfile(agg.profileWins),
		ProfilePassRates:            profileRates(agg.profilePass, count),
		ProfileRuntimeRates:         profileRates(agg.profileRuntimePass, count),
		RetryRateByProfile:          retry.retryRateByProfile,
		AvgRetryCountByProfile:      retry.avgRetryCountByProfile,
		AvgRetrySleepMSByProfile:    retry.avgRetrySleepMSByProfile,
		AvgHostPacingMSByProfile:    retry.avgHostPacingMSByProfile,
		RetryReasons:                agg.retryReasons,
		ErrorKinds:                  agg.errorKinds,
		RunnerRuns:                  runs,
		RunnerTimeoutMS:             timeoutMS,
		RunnerProfiles:              seedlessConfigNames(cfgs.profiles),
		RunnerProviderChains:        cfgs.providerChains,
	}
}

func newSeedlessSummaryAgg() seedlessSummaryAgg {
	return seedlessSummaryAgg{
		profileWins:        map[string]int{},
		profilePass:        map[string]int{},
		profileRuntimePass: map[string]int{},
		retryRuns:          map[string]int{},
		totalRuns:          map[string]int{},
		retryCountSum:      map[string]int{},
		retrySleepSum:      map[string]int64{},
		hostPacingSum:      map[string]int64{},
		retryReasons:       map[string]int{},
		errorKinds:         map[string]int{},
	}
}

func (a *seedlessSummaryAgg) recordRow(row caseResult) {
	for _, run := range row.Runs {
		a.recordRun(run)
	}
	a.profileWins[row.Delta]++
}

func (a *seedlessSummaryAgg) recordRun(run runResult) {
	for _, attempt := range runAttempts(run) {
		a.totalRuns[run.Profile]++
		if attempt.RetryCount > 0 {
			a.retryRuns[run.Profile]++
			a.retryCountSum[run.Profile] += attempt.RetryCount
		}
		a.retrySleepSum[run.Profile] += attempt.RetrySleepMS
		a.hostPacingSum[run.Profile] += attempt.HostPacingMS
		if attempt.RetryReason != "" {
			a.retryReasons[attempt.RetryReason]++
		}
		if attempt.ErrorKind != "" {
			a.errorKinds[attempt.ErrorKind]++
		}
	}
	a.recordProfilePass(run)
}

func (a *seedlessSummaryAgg) recordProfilePass(run runResult) {
	if run.RuntimeOK {
		a.profileRuntimePass[run.Profile]++
	}
	if run.SelectedPass {
		a.profilePass[run.Profile]++
	}
	if strings.Contains(run.Profile, "@") {
		return
	}
	switch run.Profile {
	case "standard":
		if run.SelectedPass {
			a.stdPass++
		}
	case "standard_semantic":
		if run.SelectedPass {
			a.stdSemPass++
		}
	case "browser_like":
		if run.SelectedPass {
			a.browserPass++
		}
	case "browser_like_semantic":
		if run.SelectedPass {
			a.browserSemPass++
		}
	}
}

func runAttempts(run runResult) []runAttempt {
	if len(run.Attempts) > 0 {
		return run.Attempts
	}
	return []runAttempt{{
		RuntimeOK:       run.RuntimeOK,
		SelectedURL:     run.SelectedURL,
		SelectedDomain:  run.SelectedDomain,
		SelectedPass:    run.SelectedPass,
		DiscoverySource: run.DiscoverySource,
		CandidateCount:  run.CandidateCount,
		DocumentFetch:   run.DocumentFetch,
		RetryCount:      run.RetryCount,
		RetrySleepMS:    run.RetrySleepMS,
		HostPacingMS:    run.HostPacingMS,
		RetryReason:     run.RetryReason,
		ErrorKind:       run.ErrorKind,
		LatencyMS:       run.LatencyMS,
		Error:           run.Error,
	}}
}

type retrySummaryMaps struct {
	retryRateByProfile       map[string]float64
	avgRetryCountByProfile   map[string]float64
	avgRetrySleepMSByProfile map[string]float64
	avgHostPacingMSByProfile map[string]float64
}

func (a seedlessSummaryAgg) retrySummaries() retrySummaryMaps {
	out := retrySummaryMaps{
		retryRateByProfile:       map[string]float64{},
		avgRetryCountByProfile:   map[string]float64{},
		avgRetrySleepMSByProfile: map[string]float64{},
		avgHostPacingMSByProfile: map[string]float64{},
	}
	for _, name := range sortedSeedlessMapKeys(a.totalRuns) {
		if a.totalRuns[name] == 0 {
			continue
		}
		out.retryRateByProfile[name] = float64(a.retryRuns[name]) / float64(a.totalRuns[name])
		out.avgRetryCountByProfile[name] = float64(a.retryCountSum[name]) / float64(a.totalRuns[name])
		out.avgRetrySleepMSByProfile[name] = float64(a.retrySleepSum[name]) / float64(a.totalRuns[name])
		out.avgHostPacingMSByProfile[name] = float64(a.hostPacingSum[name]) / float64(a.totalRuns[name])
	}
	return out
}

func bestProfile(profileWins map[string]int) string {
	best := ""
	bestCount := -1
	for _, name := range sortedSeedlessMapKeys(profileWins) {
		if profileWins[name] > bestCount {
			bestCount = profileWins[name]
			best = name
		}
	}
	return best
}

func profileRates(counts map[string]int, denominator int) map[string]float64 {
	if denominator <= 0 {
		return nil
	}
	out := map[string]float64{}
	for _, name := range sortedSeedlessMapKeys(counts) {
		out[name] = float64(counts[name]) / float64(denominator)
	}
	return out
}

func seedlessConfigNames(configs []seedlessProfileConfig) []string {
	out := make([]string, 0, len(configs))
	for _, cfg := range configs {
		out = append(out, cfg.name)
	}
	return out
}

func sortedSeedlessMapKeys[V any](items map[string]V) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
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
