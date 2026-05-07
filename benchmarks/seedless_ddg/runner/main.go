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
		SelectedURL          string                `json:"selected_url"`
		DiscoveryProvider    string                `json:"discovery_provider"`
		CandidateURLs        []string              `json:"candidate_urls"`
		CandidateDiagnostics []candidateDiagnostic `json:"candidate_diagnostics"`
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

type candidateDiagnostic struct {
	URL                         string   `json:"url"`
	Score                       float64  `json:"score"`
	ResourceClass               string   `json:"resource_class"`
	SemanticRole                string   `json:"semantic_role"`
	SemanticRoleConfidence      float64  `json:"semantic_role_confidence"`
	SemanticRoleIntent          float64  `json:"semantic_role_intent"`
	SemanticOriginAlignment     float64  `json:"semantic_origin_alignment"`
	SemanticDerivativeAlignment float64  `json:"semantic_derivative_alignment"`
	ClusterID                   string   `json:"cluster_id"`
	ClusterSize                 int      `json:"cluster_size"`
	Reasons                     []string `json:"reasons"`
}

type runResult struct {
	Profile          string       `json:"profile"`
	Skipped          bool         `json:"skipped,omitempty"`
	SkipReason       string       `json:"skip_reason,omitempty"`
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
	SelectedRole     string       `json:"selected_role,omitempty"`
	SelectedScore    float64      `json:"selected_score,omitempty"`
	SelectedReasons  []string     `json:"selected_reasons,omitempty"`
	ExpectedRank     int          `json:"expected_candidate_rank,omitempty"`
	ExpectedURL      string       `json:"expected_candidate_url,omitempty"`
	ExpectedRole     string       `json:"expected_candidate_role,omitempty"`
	ExpectedScore    float64      `json:"expected_candidate_score,omitempty"`
	ExpectedReasons  []string     `json:"expected_candidate_reasons,omitempty"`
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
	Attempt         int     `json:"attempt"`
	RuntimeOK       bool    `json:"runtime_ok"`
	SelectedURL     string  `json:"selected_url,omitempty"`
	SelectedDomain  string  `json:"selected_domain,omitempty"`
	SelectedPass    bool    `json:"selected_pass"`
	DiscoverySource string  `json:"discovery_provider,omitempty"`
	CandidateCount  int     `json:"candidate_count"`
	SelectedRole    string  `json:"selected_role,omitempty"`
	SelectedScore   float64 `json:"selected_score,omitempty"`
	ExpectedRank    int     `json:"expected_candidate_rank,omitempty"`
	ExpectedURL     string  `json:"expected_candidate_url,omitempty"`
	ExpectedRole    string  `json:"expected_candidate_role,omitempty"`
	ExpectedScore   float64 `json:"expected_candidate_score,omitempty"`
	DocumentFetch   string  `json:"document_fetch_mode,omitempty"`
	RetryCount      int     `json:"retry_count,omitempty"`
	RetrySleepMS    int64   `json:"retry_sleep_ms,omitempty"`
	HostPacingMS    int64   `json:"host_pacing_ms,omitempty"`
	RetryReason     string  `json:"retry_reason,omitempty"`
	ErrorKind       string  `json:"error_kind,omitempty"`
	LatencyMS       int64   `json:"latency_ms,omitempty"`
	Error           string  `json:"error,omitempty"`
}

type caseResult struct {
	ID    string      `json:"id"`
	Goal  string      `json:"goal"`
	Runs  []runResult `json:"runs"`
	Delta string      `json:"delta"`
}

type summary struct {
	CaseCount                   int                       `json:"case_count"`
	StandardPassRate            float64                   `json:"standard_pass_rate"`
	StandardSemanticPassRate    float64                   `json:"standard_semantic_pass_rate"`
	BrowserLikePassRate         float64                   `json:"browser_like_pass_rate"`
	BrowserLikeSemanticPassRate float64                   `json:"browser_like_semantic_pass_rate"`
	ImprovementRate             float64                   `json:"improvement_rate"`
	BrowserLikeBeatsStandard    int                       `json:"browser_like_beats_standard"`
	BestProfile                 string                    `json:"best_profile"`
	ProfilePassRates            map[string]float64        `json:"profile_pass_rates,omitempty"`
	ProfileRuntimeRates         map[string]float64        `json:"profile_runtime_rates,omitempty"`
	RetryRateByProfile          map[string]float64        `json:"retry_rate_by_profile,omitempty"`
	AvgRetryCountByProfile      map[string]float64        `json:"avg_retry_count_by_profile,omitempty"`
	AvgRetrySleepMSByProfile    map[string]float64        `json:"avg_retry_sleep_ms_by_profile,omitempty"`
	AvgHostPacingMSByProfile    map[string]float64        `json:"avg_host_pacing_ms_by_profile,omitempty"`
	AvgLatencyMSByProfile       map[string]float64        `json:"avg_latency_ms_by_profile,omitempty"`
	P95LatencyMSByProfile       map[string]int64          `json:"p95_latency_ms_by_profile,omitempty"`
	TimeoutRateByProfile        map[string]float64        `json:"timeout_rate_by_profile,omitempty"`
	AvgCandidateCountByProfile  map[string]float64        `json:"avg_candidate_count_by_profile,omitempty"`
	AvgExpectedRankByProfile    map[string]float64        `json:"avg_expected_candidate_rank_by_profile,omitempty"`
	ErrorKindsByProfile         map[string]map[string]int `json:"error_kinds_by_profile,omitempty"`
	RetryReasons                map[string]int            `json:"retry_reasons,omitempty"`
	ErrorKinds                  map[string]int            `json:"error_kinds,omitempty"`
	RunnerRuns                  int                       `json:"runner_runs,omitempty"`
	RunnerTimeoutMS             int64                     `json:"runner_timeout_ms,omitempty"`
	RunnerProfiles              []string                  `json:"runner_profiles,omitempty"`
	RunnerProviderChains        []string                  `json:"runner_provider_chains,omitempty"`
}

type report struct {
	GeneratedAtUTC string       `json:"generated_at_utc"`
	CorpusVersion  string       `json:"corpus_version"`
	BinaryPath     string       `json:"binary_path"`
	Summary        summary      `json:"summary"`
	Results        []caseResult `json:"results"`
}

type seedlessOptions struct {
	outPath         string
	casesPath       string
	profiles        string
	providerChains  string
	runs            int
	timeoutMS       int64
	checkpointEvery int
	keepState       bool
}

type seedlessConfigs struct {
	profiles       []seedlessProfileConfig
	providerChains []string
}

type seedlessProfileConfig struct {
	name       string
	path       string
	skipReason string
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
	latencyValues                                    map[string][]int64
	timeoutRuns                                      map[string]int
	candidateCountSum                                map[string]int
	candidateCountSamples                            map[string]int
	expectedRankSum                                  map[string]int
	expectedRankSamples                              map[string]int
	retryReasons                                     map[string]int
	errorKinds                                       map[string]int
	errorKindsByProfile                              map[string]map[string]int
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

	results, err := runSeedlessCases(binaryPath, c.Cases, cfgs, opts, func(results []caseResult) error {
		return writeSeedlessReport(opts.outPath, c.Version, binaryPath, results, opts, cfgs)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "checkpoint report: %v\n", err)
		os.Exit(1)
	}
	if err := writeSeedlessReport(opts.outPath, c.Version, binaryPath, results, opts, cfgs); err != nil {
		fmt.Fprintf(os.Stderr, "write report: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Seedless DDG benchmark written to %s\n", opts.outPath)
}

func writeSeedlessReport(outPath, corpusVersion, binaryPath string, results []caseResult, opts seedlessOptions, cfgs seedlessConfigs) error {
	return evalutil.WriteJSON(outPath, report{
		GeneratedAtUTC: time.Now().UTC().Format(time.RFC3339),
		CorpusVersion:  corpusVersion,
		BinaryPath:     binaryPath,
		Summary:        summarize(results, opts.runs, opts.timeoutMS, cfgs),
		Results:        results,
	})
}

func parseSeedlessOptions() seedlessOptions {
	var opts seedlessOptions
	flag.StringVar(&opts.outPath, "out", "improvements/seedless-ddg-benchmark-latest.json", "output report path")
	flag.StringVar(&opts.casesPath, "cases", "benchmarks/corpora/seedless-ddg-corpus-v1.json", "seedless ddg corpus path")
	flag.StringVar(&opts.profiles, "profiles", "standard,standard_semantic,browser_like,browser_like_semantic", "comma-separated profiles: standard, standard_semantic, browser_like, browser_like_semantic")
	flag.StringVar(&opts.providerChains, "provider-chains", "ddg_bing=https://lite.duckduckgo.com/lite/,https://html.duckduckgo.com/html/,https://www.bing.com/search", "semicolon-separated provider chains, optionally name=url1,url2")
	flag.IntVar(&opts.runs, "runs", 3, "number of attempts per case/profile")
	flag.Int64Var(&opts.timeoutMS, "timeout-ms", 25000, "per-run timeout in milliseconds")
	flag.IntVar(&opts.checkpointEvery, "checkpoint-every", 1, "write a partial report every N completed cases; 0 disables checkpointing")
	flag.BoolVar(&opts.keepState, "keep-state", false, "keep per-attempt NEEDLEX_HOME state under the benchmark temp dir")
	flag.Parse()
	if opts.runs <= 0 {
		opts.runs = 1
	}
	if opts.timeoutMS <= 0 {
		opts.timeoutMS = 25000
	}
	if opts.checkpointEvery < 0 {
		opts.checkpointEvery = 0
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
	stopSemantic := func() {}
	configs := make([]seedlessProfileConfig, 0, len(selectedProfiles)*len(chains))
	for _, chain := range chains {
		for _, profile := range selectedProfiles {
			name := profile.name
			if len(chains) > 1 {
				name += "@" + chain.name
			}
			path, err := writeSeedlessConfig(tempDir, safeStateComponent(name)+".json", chain.providerChain, profile.fetchProfile, profile.retryProfile, profile.semantic)
			if err != nil {
				stopSemantic()
				return seedlessConfigs{}, nil, err
			}
			configs = append(configs, seedlessProfileConfig{name: name, path: path})
		}
	}
	return seedlessConfigs{profiles: configs, providerChains: providerChainNames(chains)}, stopSemantic, nil
}

func writeSeedlessConfig(tempDir, name, providerChain, fetchProfile, retryProfile string, semantic bool) (string, error) {
	payload := map[string]any{
		"fetch":     map[string]any{"profile": fetchProfile, "retry_profile": retryProfile},
		"discovery": map[string]any{"provider_chain": providerChain},
	}
	if semantic {
		semanticPayload := map[string]any{}
		if endpoint := strings.TrimSpace(os.Getenv("NEEDLEX_SEMANTIC_EMBEDDING_URL")); endpoint != "" {
			semanticPayload["embedding_url"] = endpoint
		}
		if providerModel := strings.TrimSpace(os.Getenv("NEEDLEX_SEMANTIC_PROVIDER_MODEL")); providerModel != "" {
			semanticPayload["provider_model"] = providerModel
		}
		if vectorSpace := strings.TrimSpace(os.Getenv("NEEDLEX_SEMANTIC_VECTOR_SPACE")); vectorSpace != "" {
			semanticPayload["vector_space"] = vectorSpace
		}
		payload["semantic"] = semanticPayload
	}
	return writeConfig(tempDir, name, payload)
}

func runSeedlessCases(binaryPath string, cases []struct {
	ID             string `json:"id"`
	Goal           string `json:"goal"`
	ExpectedDomain string `json:"expected_domain"`
}, cfgs seedlessConfigs, opts seedlessOptions,
	checkpoint func([]caseResult) error,
) ([]caseResult, error) {
	results := make([]caseResult, 0, len(cases))
	for i, item := range cases {
		fmt.Printf("[seedless-ddg] %s case %d/%d start id=%s\n", time.Now().Format("15:04:05"), i+1, len(cases), item.ID)
		row := runSeedlessCase(binaryPath, item.ID, item.Goal, item.ExpectedDomain, cfgs, opts)
		results = append(results, row)
		if checkpoint != nil && opts.checkpointEvery > 0 && len(results)%opts.checkpointEvery == 0 {
			if err := checkpoint(results); err != nil {
				return results, err
			}
		}
		fmt.Printf("[seedless-ddg] %s case %d/%d done id=%s best=%s profile_passes=%s\n", time.Now().Format("15:04:05"), i+1, len(cases), item.ID, row.Delta, formatProfilePasses(row.Runs))
	}
	return results, nil
}

func runSeedlessCase(binaryPath, id, goal, expectedDomain string, cfgs seedlessConfigs, opts seedlessOptions) caseResult {
	runs := make([]runResult, 0, len(cfgs.profiles))
	for _, profile := range cfgs.profiles {
		if profile.skipReason != "" {
			runs = append(runs, runResult{Profile: profile.name, Skipped: true, SkipReason: profile.skipReason, ExpectedDomain: expectedDomain})
			continue
		}
		runs = append(runs, runCase(binaryPath, profile.path, profile.name, id, goal, expectedDomain, opts.runs, opts.timeoutMS, opts.keepState))
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
		if run.Skipped {
			parts = append(parts, fmt.Sprintf("%s=skipped", run.Profile))
			continue
		}
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

func runCase(binaryPath, configPath, profile, caseID, goal, expectedDomain string, runs int, timeoutMS int64, keepState bool) runResult {
	if runs <= 1 {
		result := runCaseOnce(binaryPath, configPath, profile, caseID, "single", goal, expectedDomain, timeoutMS, keepState)
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
		attempt := runCaseOnce(binaryPath, configPath, profile, caseID, fmt.Sprintf("attempt-%d", i+1), goal, expectedDomain, timeoutMS, keepState)
		attempts = append(attempts, runAttempt{
			Attempt:         i + 1,
			RuntimeOK:       attempt.RuntimeOK,
			SelectedURL:     attempt.SelectedURL,
			SelectedDomain:  attempt.SelectedDomain,
			SelectedPass:    attempt.SelectedPass,
			DiscoverySource: attempt.DiscoverySource,
			CandidateCount:  attempt.CandidateCount,
			SelectedRole:    attempt.SelectedRole,
			SelectedScore:   attempt.SelectedScore,
			ExpectedRank:    attempt.ExpectedRank,
			ExpectedURL:     attempt.ExpectedURL,
			ExpectedRole:    attempt.ExpectedRole,
			ExpectedScore:   attempt.ExpectedScore,
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

func runCaseOnce(binaryPath, configPath, profile, caseID, attemptID, goal, expectedDomain string, timeoutMS int64, keepState bool) runResult {
	result := runResult{Profile: profile, ExpectedDomain: expectedDomain}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()
	started := time.Now()
	cmd := exec.CommandContext(ctx, binaryPath, "query", "--goal", goal, "--json", "--json-mode", "full", "--config", configPath)
	stateRoot := seedlessStateRoot(configPath, caseID, profile, attemptID)
	if !keepState {
		defer func() { _ = os.RemoveAll(stateRoot) }()
	}
	cmd.Env = append(os.Environ(), "NEEDLEX_HOME="+stateRoot)
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
	if selected := diagnosticForURL(resp.Plan.CandidateDiagnostics, result.SelectedURL); selected.URL != "" {
		result.SelectedRole = strings.TrimSpace(selected.SemanticRole)
		result.SelectedScore = selected.Score
		result.SelectedReasons = append([]string{}, selected.Reasons...)
	}
	if expected, rank := diagnosticForExpectedDomain(resp.Plan.CandidateDiagnostics, resp.Plan.CandidateURLs, expectedDomain); expected.URL != "" {
		result.ExpectedRank = rank
		result.ExpectedURL = strings.TrimSpace(expected.URL)
		result.ExpectedRole = strings.TrimSpace(expected.SemanticRole)
		result.ExpectedScore = expected.Score
		result.ExpectedReasons = append([]string{}, expected.Reasons...)
	}
	result.DocumentFetch = strings.TrimSpace(resp.Document.FetchMode)
	for _, stage := range resp.Trace.Stages {
		if stage.Stage != "acquire" {
			continue
		}
		applyAcquireStage(&result, stage.Metadata)
	}
	if !result.SelectedPass {
		result.ErrorKind = classifySelectionMiss(resp, expectedDomain)
	}
	return result
}

func classifySelectionMiss(resp queryResponse, expectedDomain string) string {
	selectedURL := strings.TrimSpace(resp.Plan.SelectedURL)
	if selectedURL == "" {
		return "empty_selection"
	}
	if len(resp.Plan.CandidateURLs) == 0 {
		return "empty_candidates"
	}
	selected := diagnosticForURL(resp.Plan.CandidateDiagnostics, selectedURL)
	if candidatePoolContainsDomain(resp.Plan.CandidateURLs, expectedDomain) {
		return "right_family_not_selected"
	}
	if selected.SemanticRole != "" {
		switch selected.SemanticRole {
		case "derivative_representation":
			return "derivative_surface_selected"
		case "social_context":
			return "context_surface_selected"
		case "distribution_node":
			return "distribution_surface_selected"
		}
	}
	if selected.ResourceClass != "" && selected.ResourceClass != "html_like" {
		return "non_document_surface_selected"
	}
	return "wrong_family_selected"
}

func diagnosticForURL(items []candidateDiagnostic, rawURL string) candidateDiagnostic {
	key := strings.TrimSpace(rawURL)
	for _, item := range items {
		if strings.TrimSpace(item.URL) == key {
			return item
		}
	}
	return candidateDiagnostic{}
}

func diagnosticForExpectedDomain(items []candidateDiagnostic, urls []string, expectedDomain string) (candidateDiagnostic, int) {
	for idx, rawURL := range urls {
		if !domainMatches(canonicalHost(rawURL), expectedDomain) {
			continue
		}
		item := diagnosticForURL(items, rawURL)
		if item.URL == "" {
			item.URL = strings.TrimSpace(rawURL)
		}
		return item, idx + 1
	}
	return candidateDiagnostic{}, 0
}

func candidatePoolContainsDomain(urls []string, expectedDomain string) bool {
	expected := canonicalHost("https://" + expectedDomain)
	if expected == "" {
		return false
	}
	for _, rawURL := range urls {
		if domainMatches(canonicalHost(rawURL), expected) {
			return true
		}
	}
	return false
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
	case strings.Contains(text, "class=unsupported_content_type"):
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
		if betterProfileRun(profile, best) {
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

func betterProfileRun(candidate, current runResult) bool {
	left := buildProfileRunMerit(candidate)
	right := buildProfileRunMerit(current)
	return left.betterThan(right)
}

type profileRunMerit struct {
	selectedPass     int
	passCount        int
	attemptCount     int
	runtimeOK        int
	runtimePassCount int
	expectedRank     int
	latencyMS        int64
}

func buildProfileRunMerit(r runResult) profileRunMerit {
	attempts := r.AttemptCount
	passCount := r.PassCount
	runtimePassCount := r.RuntimePassCount
	if attempts <= 0 {
		attempts = 1
		passCount = boolToInt(r.SelectedPass)
		runtimePassCount = boolToInt(r.RuntimeOK)
	}
	return profileRunMerit{
		selectedPass:     boolToInt(r.SelectedPass),
		passCount:        passCount,
		attemptCount:     attempts,
		runtimeOK:        boolToInt(r.RuntimeOK),
		runtimePassCount: runtimePassCount,
		expectedRank:     r.ExpectedRank,
		latencyMS:        r.LatencyMS,
	}
}

func (m profileRunMerit) betterThan(other profileRunMerit) bool {
	if m.selectedPass != other.selectedPass {
		return m.selectedPass > other.selectedPass
	}
	if cmp := compareFractions(m.passCount, m.attemptCount, other.passCount, other.attemptCount); cmp != 0 {
		return cmp > 0
	}
	if m.runtimeOK != other.runtimeOK {
		return m.runtimeOK > other.runtimeOK
	}
	if cmp := compareFractions(m.runtimePassCount, m.attemptCount, other.runtimePassCount, other.attemptCount); cmp != 0 {
		return cmp > 0
	}
	if m.expectedRank > 0 && other.expectedRank > 0 && m.expectedRank != other.expectedRank {
		return m.expectedRank < other.expectedRank
	}
	if m.expectedRank > 0 && other.expectedRank == 0 {
		return true
	}
	if m.expectedRank == 0 && other.expectedRank > 0 {
		return false
	}
	if m.latencyMS > 0 && other.latencyMS > 0 && m.latencyMS != other.latencyMS {
		return m.latencyMS < other.latencyMS
	}
	return false
}

func compareFractions(leftN, leftD, rightN, rightD int) int {
	if leftD <= 0 {
		leftD = 1
	}
	if rightD <= 0 {
		rightD = 1
	}
	left := leftN * rightD
	right := rightN * leftD
	switch {
	case left > right:
		return 1
	case left < right:
		return -1
	default:
		return 0
	}
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
		ImprovementRate:             float64(agg.browserPass-agg.stdPass) / float64(count),
		BrowserLikeBeatsStandard:    agg.browserPass - agg.stdPass,
		BestProfile:                 bestProfile(agg.profilePass, agg.profileRuntimePass, agg.profileWins, agg.latencyValues),
		ProfilePassRates:            profileRates(agg.profilePass, count),
		ProfileRuntimeRates:         profileRates(agg.profileRuntimePass, count),
		RetryRateByProfile:          retry.retryRateByProfile,
		AvgRetryCountByProfile:      retry.avgRetryCountByProfile,
		AvgRetrySleepMSByProfile:    retry.avgRetrySleepMSByProfile,
		AvgHostPacingMSByProfile:    retry.avgHostPacingMSByProfile,
		AvgLatencyMSByProfile:       avgLatencyMSByProfile(agg.latencyValues),
		P95LatencyMSByProfile:       p95LatencyMSByProfile(agg.latencyValues),
		TimeoutRateByProfile:        rateByProfile(agg.timeoutRuns, agg.totalRuns),
		AvgCandidateCountByProfile:  avgCandidateCountByProfile(agg.candidateCountSum, agg.candidateCountSamples),
		AvgExpectedRankByProfile:    avgCandidateCountByProfile(agg.expectedRankSum, agg.expectedRankSamples),
		ErrorKindsByProfile:         agg.errorKindsByProfile,
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
		profileWins:           map[string]int{},
		profilePass:           map[string]int{},
		profileRuntimePass:    map[string]int{},
		retryRuns:             map[string]int{},
		totalRuns:             map[string]int{},
		retryCountSum:         map[string]int{},
		retrySleepSum:         map[string]int64{},
		hostPacingSum:         map[string]int64{},
		latencyValues:         map[string][]int64{},
		timeoutRuns:           map[string]int{},
		candidateCountSum:     map[string]int{},
		candidateCountSamples: map[string]int{},
		expectedRankSum:       map[string]int{},
		expectedRankSamples:   map[string]int{},
		retryReasons:          map[string]int{},
		errorKinds:            map[string]int{},
		errorKindsByProfile:   map[string]map[string]int{},
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
		if attempt.LatencyMS > 0 {
			a.latencyValues[run.Profile] = append(a.latencyValues[run.Profile], attempt.LatencyMS)
		}
		if attempt.CandidateCount > 0 {
			a.candidateCountSum[run.Profile] += attempt.CandidateCount
			a.candidateCountSamples[run.Profile]++
		}
		if attempt.ExpectedRank > 0 {
			a.expectedRankSum[run.Profile] += attempt.ExpectedRank
			a.expectedRankSamples[run.Profile]++
		}
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
			if a.errorKindsByProfile[run.Profile] == nil {
				a.errorKindsByProfile[run.Profile] = map[string]int{}
			}
			a.errorKindsByProfile[run.Profile][attempt.ErrorKind]++
			if attempt.ErrorKind == "benchmark_timeout" || attempt.ErrorKind == "timeout" {
				a.timeoutRuns[run.Profile]++
			}
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
		SelectedRole:    run.SelectedRole,
		SelectedScore:   run.SelectedScore,
		ExpectedRank:    run.ExpectedRank,
		ExpectedURL:     run.ExpectedURL,
		ExpectedRole:    run.ExpectedRole,
		ExpectedScore:   run.ExpectedScore,
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

func bestProfile(profilePass, profileRuntimePass, profileWins map[string]int, latencyValues map[string][]int64) string {
	names := map[string]struct{}{}
	for name := range profilePass {
		names[name] = struct{}{}
	}
	for name := range profileRuntimePass {
		names[name] = struct{}{}
	}
	for name := range profileWins {
		names[name] = struct{}{}
	}
	best := ""
	for _, name := range sortedSeedlessSetKeys(names) {
		if best == "" || betterSummaryProfile(name, best, profilePass, profileRuntimePass, profileWins, latencyValues) {
			best = name
		}
	}
	return best
}

func betterSummaryProfile(candidate, current string, profilePass, profileRuntimePass, profileWins map[string]int, latencyValues map[string][]int64) bool {
	if profilePass[candidate] != profilePass[current] {
		return profilePass[candidate] > profilePass[current]
	}
	if profileRuntimePass[candidate] != profileRuntimePass[current] {
		return profileRuntimePass[candidate] > profileRuntimePass[current]
	}
	if profileWins[candidate] != profileWins[current] {
		return profileWins[candidate] > profileWins[current]
	}
	leftLatency, leftOK := avgLatencyForProfile(latencyValues[candidate])
	rightLatency, rightOK := avgLatencyForProfile(latencyValues[current])
	if leftOK && rightOK && leftLatency != rightLatency {
		return leftLatency < rightLatency
	}
	if leftOK != rightOK {
		return leftOK
	}
	return false
}

func avgLatencyForProfile(values []int64) (float64, bool) {
	if len(values) == 0 {
		return 0, false
	}
	sum := int64(0)
	for _, value := range values {
		sum += value
	}
	return float64(sum) / float64(len(values)), true
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

func rateByProfile(numerators, denominators map[string]int) map[string]float64 {
	out := map[string]float64{}
	for _, name := range sortedSeedlessMapKeys(denominators) {
		if denominators[name] == 0 {
			continue
		}
		out[name] = float64(numerators[name]) / float64(denominators[name])
	}
	return out
}

func avgLatencyMSByProfile(values map[string][]int64) map[string]float64 {
	out := map[string]float64{}
	for _, name := range sortedSeedlessMapKeys(values) {
		items := values[name]
		if len(items) == 0 {
			continue
		}
		sum := int64(0)
		for _, value := range items {
			sum += value
		}
		out[name] = float64(sum) / float64(len(items))
	}
	return out
}

func p95LatencyMSByProfile(values map[string][]int64) map[string]int64 {
	out := map[string]int64{}
	for _, name := range sortedSeedlessMapKeys(values) {
		items := append([]int64{}, values[name]...)
		if len(items) == 0 {
			continue
		}
		sort.Slice(items, func(i, j int) bool { return items[i] < items[j] })
		idx := (len(items)*95 + 99) / 100
		if idx <= 0 {
			idx = 1
		}
		out[name] = items[idx-1]
	}
	return out
}

func avgCandidateCountByProfile(sums, samples map[string]int) map[string]float64 {
	out := map[string]float64{}
	for _, name := range sortedSeedlessMapKeys(samples) {
		if samples[name] == 0 {
			continue
		}
		out[name] = float64(sums[name]) / float64(samples[name])
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

func sortedSeedlessSetKeys(items map[string]struct{}) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
