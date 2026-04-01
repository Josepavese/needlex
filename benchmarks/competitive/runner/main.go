package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/evalutil"
)

type corpus struct {
	Version string       `json:"version"`
	Cases   []seededCase `json:"cases"`
}

type seededCase struct {
	ID               string   `json:"id"`
	Family           string   `json:"family"`
	Language         string   `json:"language"`
	SeedURL          string   `json:"seed_url"`
	TaskType         string   `json:"task_type"`
	Goal             string   `json:"goal,omitempty"`
	ExpectedURL      string   `json:"expected_url"`
	ExpectedDomain   string   `json:"expected_domain"`
	MustContainFacts []string `json:"must_contain_facts,omitempty"`
	MustExposeProof  bool     `json:"must_expose_proof"`
	Notes            string   `json:"notes,omitempty"`
}

type compactChunk struct {
	Text     string `json:"text"`
	ProofRef string `json:"proof_ref"`
}

type compactCost struct {
	LatencyMS int64 `json:"latency_ms"`
}

type compactRead struct {
	Kind       string         `json:"kind"`
	URL        string         `json:"url"`
	Summary    string         `json:"summary"`
	Chunks     []compactChunk `json:"chunks"`
	CostReport compactCost    `json:"cost_report"`
}

type compactQuery struct {
	Kind         string         `json:"kind"`
	SelectedURL  string         `json:"selected_url"`
	Summary      string         `json:"summary"`
	SelectionWhy []string       `json:"selection_why"`
	Chunks       []compactChunk `json:"chunks"`
	CostReport   compactCost    `json:"cost_report"`
}

type proofPayload struct {
	ProofRecords []struct {
		Proof struct {
			SourceSpan struct {
				Selector string `json:"selector"`
			} `json:"source_span"`
		} `json:"proof"`
	} `json:"proof_records"`
}

type competitorResult struct {
	Competitor           string   `json:"competitor"`
	Category             string   `json:"category"`
	Configured           bool     `json:"configured"`
	Supported            bool     `json:"supported"`
	Skipped              bool     `json:"skipped"`
	SkipReason           string   `json:"skip_reason,omitempty"`
	RuntimeOK            bool     `json:"runtime_ok"`
	QualityPass          bool     `json:"quality_pass"`
	SelectedURLPass      bool     `json:"selected_url_pass"`
	ProofUsable          bool     `json:"proof_usable"`
	ActualURL            string   `json:"actual_url,omitempty"`
	SummaryPresent       bool     `json:"summary_present"`
	FactCoverage         float64  `json:"fact_coverage"`
	CoveredFacts         []string `json:"covered_facts,omitempty"`
	MissingFacts         []string `json:"missing_facts,omitempty"`
	ClaimToSourceSteps   *int     `json:"claim_to_source_steps,omitempty"`
	PostProcessingBurden int      `json:"post_processing_burden"`
	HopCountToTarget     *int     `json:"hop_count_to_target,omitempty"`
	PacketBytes          int      `json:"packet_bytes"`
	ChunkCount           int      `json:"chunk_count"`
	LatencyMS            int64    `json:"latency_ms"`
	SelectionWhy         []string `json:"selection_why,omitempty"`
	FailureClasses       []string `json:"failure_classes,omitempty"`
	Error                string   `json:"error,omitempty"`
	Cached               bool     `json:"cached,omitempty"`
}

type caseResult struct {
	ID          string             `json:"id"`
	Family      string             `json:"family"`
	Language    string             `json:"language"`
	TaskType    string             `json:"task_type"`
	SeedURL     string             `json:"seed_url"`
	Goal        string             `json:"goal,omitempty"`
	ExpectedURL string             `json:"expected_url"`
	Results     []competitorResult `json:"results"`
}

type competitorSummary struct {
	Competitor                string  `json:"competitor"`
	Category                  string  `json:"category"`
	CaseCount                 int     `json:"case_count"`
	Configured                bool    `json:"configured"`
	SupportedRate             float64 `json:"supported_rate"`
	RuntimeSuccessRate        float64 `json:"runtime_success_rate"`
	QualityPassRate           float64 `json:"quality_pass_rate"`
	SelectedURLPassRate       float64 `json:"selected_url_pass_rate"`
	ProofUsabilityRate        float64 `json:"proof_usability_rate"`
	FactCoverageRate          float64 `json:"fact_coverage_rate"`
	ClaimToSourceCoverageRate float64 `json:"claim_to_source_coverage_rate"`
	AvgClaimToSourceSteps     float64 `json:"avg_claim_to_source_steps"`
	AvgPostProcessingBurden   float64 `json:"avg_post_processing_burden"`
	CachedReuseRate           float64 `json:"cached_reuse_rate"`
	CachedReuseCount          int     `json:"cached_reuse_count"`
	AvgHopCountToTarget       float64 `json:"avg_hop_count_to_target"`
	PacketReductionVsBaseline float64 `json:"packet_reduction_vs_baseline"`
	AvgLatencyMS              int64   `json:"avg_latency_ms"`
	AvgPacketBytes            int64   `json:"avg_packet_bytes"`
	SkippedCases              int     `json:"skipped_cases"`
}

type report struct {
	GeneratedAtUTC string              `json:"generated_at_utc"`
	CorpusVersion  string              `json:"corpus_version"`
	Competitors    []competitorSummary `json:"competitors"`
	Cases          []caseResult        `json:"cases"`
}

type runCache struct {
	Entries map[string]competitorResult `json:"entries"`
}

type adapter interface {
	Name() string
	Category() string
	Availability() (configured bool, reason string)
	ProofComparable() bool
	Supports(seededCase) bool
	Run(context.Context, seededCase) competitorResult
}

func main() {
	var outPath, casesPath, competitorsArg, cachePath string
	flag.StringVar(&outPath, "out", "improvements/competitive-benchmark-latest.json", "output report path")
	flag.StringVar(&casesPath, "cases", "benchmarks/corpora/competitive-corpus-v1.json", "competitive corpus path")
	flag.StringVar(&competitorsArg, "competitors", "needlex,jina,firecrawl,tavily,exa,brave-search,vercel-browser-agent", "comma-separated competitor list")
	flag.StringVar(&cachePath, "cache", ".needlex/competitive-benchmark-cache.json", "local cache path for reusable competitor results")
	flag.Parse()

	c, err := loadCorpus(casesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load corpus: %v\n", err)
		os.Exit(1)
	}

	adapters, err := resolveAdapters(splitCSV(competitorsArg))
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve adapters: %v\n", err)
		os.Exit(1)
	}

	cache, err := loadRunCache(cachePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load cache: %v\n", err)
		os.Exit(1)
	}

	results := make([]caseResult, 0, len(c.Cases))
	for i, item := range c.Cases {
		fmt.Printf("[competitive] %s case %d/%d start id=%s family=%s task=%s\n", time.Now().Format("15:04:05"), i+1, len(c.Cases), item.ID, item.Family, item.TaskType)
		row := caseResult{ID: item.ID, Family: item.Family, Language: item.Language, TaskType: item.TaskType, SeedURL: item.SeedURL, Goal: item.Goal, ExpectedURL: item.ExpectedURL}
		for _, a := range adapters {
			configured, reason := a.Availability()
			if !configured {
				row.Results = append(row.Results, competitorResult{Competitor: a.Name(), Category: a.Category(), Configured: false, Skipped: true, SkipReason: reason})
				continue
			}
			if !a.Supports(item) {
				row.Results = append(row.Results, competitorResult{Competitor: a.Name(), Category: a.Category(), Configured: true, Supported: false, Skipped: true, SkipReason: "unsupported_task"})
				continue
			}
			if cacheableCompetitor(a.Name()) {
				cacheKey := makeCacheKey(c.Version, a.Name(), item)
				if cached, ok := cache.Entries[cacheKey]; ok {
					enrichDerivedMetrics(item, &cached)
					cached.Competitor = a.Name()
					cached.Category = a.Category()
					cached.Configured = true
					cached.Supported = true
					cached.Cached = true
					row.Results = append(row.Results, cached)
					continue
				}
				res := a.Run(context.Background(), item)
				enrichDerivedMetrics(item, &res)
				res.Competitor = a.Name()
				res.Category = a.Category()
				res.Configured = true
				res.Supported = true
				cache.Entries[cacheKey] = res
				row.Results = append(row.Results, res)
				continue
			}
			res := a.Run(context.Background(), item)
			enrichDerivedMetrics(item, &res)
			res.Competitor = a.Name()
			res.Category = a.Category()
			res.Configured = true
			res.Supported = true
			row.Results = append(row.Results, res)
		}
		results = append(results, row)
	}

	rep := report{GeneratedAtUTC: time.Now().UTC().Format(time.RFC3339), CorpusVersion: c.Version, Competitors: summarizeByCompetitor(results, adapters), Cases: results}
	if err := evalutil.WriteJSON(outPath, rep); err != nil {
		fmt.Fprintf(os.Stderr, "write report: %v\n", err)
		os.Exit(1)
	}
	if err := saveRunCache(cachePath, cache); err != nil {
		fmt.Fprintf(os.Stderr, "write cache: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Competitive benchmark written to %s\n", outPath)
}

func loadCorpus(path string) (corpus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return corpus{}, err
	}
	var c corpus
	if err := json.Unmarshal(data, &c); err != nil {
		return corpus{}, err
	}
	return c, nil
}

func loadRunCache(path string) (runCache, error) {
	if strings.TrimSpace(path) == "" {
		return runCache{Entries: map[string]competitorResult{}}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return runCache{Entries: map[string]competitorResult{}}, nil
		}
		return runCache{}, err
	}
	var cache runCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return runCache{}, err
	}
	if cache.Entries == nil {
		cache.Entries = map[string]competitorResult{}
	}
	return cache, nil
}

func saveRunCache(path string, cache runCache) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return evalutil.WriteJSON(path, cache)
}

func resolveAdapters(names []string) ([]adapter, error) {
	out := make([]adapter, 0, len(names))
	for _, name := range names {
		switch strings.TrimSpace(strings.ToLower(name)) {
		case "needlex":
			out = append(out, newNeedleXAdapter())
		case "jina":
			out = append(out, jinaAdapter{})
		case "firecrawl":
			out = append(out, firecrawlAdapter{})
		case "tavily":
			out = append(out, tavilyAdapter{})
		case "exa":
			out = append(out, exaAdapter{})
		case "brave-search":
			out = append(out, braveAdapter{})
		case "vercel-browser-agent":
			out = append(out, vercelBrowserAgentAdapter{})
		default:
			return nil, fmt.Errorf("unknown competitor %q", name)
		}
	}
	return out, nil
}

func cacheableCompetitor(name string) bool {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "firecrawl", "tavily", "exa", "brave-search", "vercel-browser-agent":
		return true
	default:
		return false
	}
}

func makeCacheKey(corpusVersion string, competitor string, item seededCase) string {
	payload, _ := json.Marshal(struct {
		CorpusVersion string     `json:"corpus_version"`
		Competitor    string     `json:"competitor"`
		Item          seededCase `json:"item"`
	}{
		CorpusVersion: corpusVersion,
		Competitor:    strings.TrimSpace(strings.ToLower(competitor)),
		Item:          item,
	})
	sum := fnv.New64a()
	_, _ = sum.Write(payload)
	return fmt.Sprintf("%x", sum.Sum64())
}

type needleXAdapter struct {
	binaryPath string
	cleanup    func()
	err        error
}

func newNeedleXAdapter() *needleXAdapter {
	path, cleanup, err := buildNeedleBinary()
	return &needleXAdapter{binaryPath: path, cleanup: cleanup, err: err}
}

func (a *needleXAdapter) Name() string          { return "needlex" }
func (a *needleXAdapter) Category() string      { return "product" }
func (a *needleXAdapter) ProofComparable() bool { return true }
func (a *needleXAdapter) Availability() (bool, string) {
	if a.err != nil {
		return false, a.err.Error()
	}
	return true, ""
}
func (a *needleXAdapter) Supports(seededCase) bool { return true }
func (a *needleXAdapter) Run(_ context.Context, item seededCase) competitorResult {
	res := competitorResult{}
	switch item.TaskType {
	case "same_site_query_routing":
		payload, raw, err := runNeedleJSON(a.binaryPath, "query", item.SeedURL, "--goal", item.Goal, "--json")
		if err != nil {
			res.Error = err.Error()
			res.FailureClasses = []string{classifyExecutionError(err.Error())}
			return res
		}
		res.RuntimeOK = true
		var out compactQuery
		if err := json.Unmarshal(payload, &out); err != nil {
			res.Error = err.Error()
			res.FailureClasses = []string{"decode_error"}
			return res
		}
		res.ActualURL = strings.TrimSpace(out.SelectedURL)
		res.SelectedURLPass = sameCanonicalURL(res.ActualURL, item.ExpectedURL)
		res.SummaryPresent = strings.TrimSpace(out.Summary) != ""
		res.ChunkCount = len(out.Chunks)
		res.PacketBytes = len(raw)
		res.LatencyMS = out.CostReport.LatencyMS
		res.SelectionWhy = append([]string{}, out.SelectionWhy...)
		res.ProofUsable = verifyNeedleProof(a.binaryPath, out.Chunks, item.MustExposeProof)
		text := buildNeedleText(out.Summary, out.Chunks)
		res.FactCoverage, res.CoveredFacts, res.MissingFacts = evaluateFactCoverage(text, item.MustContainFacts)
		res.ClaimToSourceSteps = estimateClaimToSourceSteps(res)
		res.PostProcessingBurden = estimatePostProcessingBurden(item, res)
		res.HopCountToTarget = estimateHopCountToTarget(item, res.SelectedURLPass)
	case "read_page_understanding", "read_then_answer":
		payload, raw, err := runNeedleJSON(a.binaryPath, "read", item.SeedURL, "--json")
		if err != nil {
			res.Error = err.Error()
			res.FailureClasses = []string{classifyExecutionError(err.Error())}
			return res
		}
		res.RuntimeOK = true
		var out compactRead
		if err := json.Unmarshal(payload, &out); err != nil {
			res.Error = err.Error()
			res.FailureClasses = []string{"decode_error"}
			return res
		}
		res.ActualURL = strings.TrimSpace(out.URL)
		res.SelectedURLPass = sameCanonicalURL(res.ActualURL, item.ExpectedURL)
		res.SummaryPresent = strings.TrimSpace(out.Summary) != ""
		res.ChunkCount = len(out.Chunks)
		res.PacketBytes = len(raw)
		res.LatencyMS = out.CostReport.LatencyMS
		res.ProofUsable = verifyNeedleProof(a.binaryPath, out.Chunks, item.MustExposeProof)
		text := buildNeedleText(out.Summary, out.Chunks)
		res.FactCoverage, res.CoveredFacts, res.MissingFacts = evaluateFactCoverage(text, item.MustContainFacts)
		res.ClaimToSourceSteps = estimateClaimToSourceSteps(res)
		res.PostProcessingBurden = estimatePostProcessingBurden(item, res)
		res.HopCountToTarget = estimateHopCountToTarget(item, res.SelectedURLPass)
	default:
		res.Skipped = true
		res.SkipReason = "unsupported_task"
		return res
	}
	res.FailureClasses = classifyQualityFailures(res, item, a.ProofComparable())
	res.QualityPass = len(res.FailureClasses) == 0
	return res
}

type jinaAdapter struct{}

func (j jinaAdapter) Name() string                 { return "jina" }
func (j jinaAdapter) Category() string             { return "simple_baseline" }
func (j jinaAdapter) ProofComparable() bool        { return false }
func (j jinaAdapter) Availability() (bool, string) { return true, "" }
func (j jinaAdapter) Supports(item seededCase) bool {
	return item.TaskType != "same_site_query_routing"
}
func (j jinaAdapter) Run(_ context.Context, item seededCase) competitorResult {
	res := competitorResult{ProofUsable: !item.MustExposeProof}
	resp, err := http.DefaultClient.Get("https://r.jina.ai/http://" + strings.TrimPrefix(item.SeedURL, "https://"))
	if err != nil {
		res.Error = err.Error()
		res.FailureClasses = []string{classifyExecutionError(err.Error())}
		return res
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		res.Error = fmt.Sprintf("unexpected status %d", resp.StatusCode)
		res.FailureClasses = []string{"runtime_error"}
		return res
	}
	data, err := ioReadAll(resp.Body)
	if err != nil {
		res.Error = err.Error()
		res.FailureClasses = []string{classifyExecutionError(err.Error())}
		return res
	}
	res.RuntimeOK = true
	res.ActualURL = item.SeedURL
	res.SelectedURLPass = sameCanonicalURL(item.SeedURL, item.ExpectedURL)
	res.PacketBytes = len(data)
	res.LatencyMS = 0
	text := strings.TrimSpace(string(data))
	res.SummaryPresent = text != ""
	if text != "" {
		res.ChunkCount = 1
	}
	res.FactCoverage, res.CoveredFacts, res.MissingFacts = evaluateFactCoverage(text, item.MustContainFacts)
	res.ClaimToSourceSteps = estimateClaimToSourceSteps(res)
	res.PostProcessingBurden = estimatePostProcessingBurden(item, res)
	res.HopCountToTarget = estimateHopCountToTarget(item, res.SelectedURLPass)
	res.FailureClasses = classifyQualityFailures(res, item, j.ProofComparable())
	res.QualityPass = len(res.FailureClasses) == 0
	return res
}

type firecrawlAdapter struct{}

func (a firecrawlAdapter) Name() string          { return "firecrawl" }
func (a firecrawlAdapter) Category() string      { return "market_reference" }
func (a firecrawlAdapter) ProofComparable() bool { return false }
func (a firecrawlAdapter) Availability() (bool, string) {
	if strings.TrimSpace(os.Getenv("FIRECRAWL_API_KEY")) == "" {
		return false, "missing_env:FIRECRAWL_API_KEY"
	}
	return true, ""
}
func (a firecrawlAdapter) Supports(item seededCase) bool {
	switch item.TaskType {
	case "same_site_query_routing", "read_page_understanding", "read_then_answer":
		return true
	default:
		return false
	}
}
func (a firecrawlAdapter) Run(ctx context.Context, item seededCase) competitorResult {
	switch item.TaskType {
	case "same_site_query_routing":
		return a.runSearch(ctx, item)
	case "read_page_understanding", "read_then_answer":
		return a.runScrape(ctx, item)
	default:
		return competitorResult{Skipped: true, SkipReason: "unsupported_task"}
	}
}

type tavilyAdapter struct{}

func (a tavilyAdapter) Name() string          { return "tavily" }
func (a tavilyAdapter) Category() string      { return "market_reference" }
func (a tavilyAdapter) ProofComparable() bool { return false }
func (a tavilyAdapter) Availability() (bool, string) {
	if strings.TrimSpace(os.Getenv("TAVILY_API_KEY")) == "" {
		return false, "missing_env:TAVILY_API_KEY"
	}
	return true, ""
}
func (a tavilyAdapter) Supports(item seededCase) bool {
	switch item.TaskType {
	case "same_site_query_routing", "read_page_understanding", "read_then_answer":
		return true
	default:
		return false
	}
}
func (a tavilyAdapter) Run(ctx context.Context, item seededCase) competitorResult {
	switch item.TaskType {
	case "same_site_query_routing":
		return a.runSearch(ctx, item)
	case "read_page_understanding", "read_then_answer":
		return a.runExtract(ctx, item)
	default:
		return competitorResult{Skipped: true, SkipReason: "unsupported_task"}
	}
}

type exaAdapter struct{}

func (a exaAdapter) Name() string          { return "exa" }
func (a exaAdapter) Category() string      { return "market_reference" }
func (a exaAdapter) ProofComparable() bool { return false }
func (a exaAdapter) Availability() (bool, string) {
	if strings.TrimSpace(os.Getenv("EXA_API_KEY")) == "" {
		return false, "missing_env:EXA_API_KEY"
	}
	return true, ""
}
func (a exaAdapter) Supports(item seededCase) bool {
	switch item.TaskType {
	case "same_site_query_routing", "read_page_understanding", "read_then_answer":
		return true
	default:
		return false
	}
}
func (a exaAdapter) Run(ctx context.Context, item seededCase) competitorResult {
	switch item.TaskType {
	case "same_site_query_routing":
		return a.runSearch(ctx, item)
	case "read_page_understanding", "read_then_answer":
		return a.runContents(ctx, item)
	default:
		return competitorResult{Skipped: true, SkipReason: "unsupported_task"}
	}
}

type braveAdapter struct{}

func (a braveAdapter) Name() string          { return "brave-search" }
func (a braveAdapter) Category() string      { return "market_reference" }
func (a braveAdapter) ProofComparable() bool { return false }
func (a braveAdapter) Availability() (bool, string) {
	if strings.TrimSpace(os.Getenv("BRAVE_SEARCH_API_KEY")) == "" {
		return false, "missing_env:BRAVE_SEARCH_API_KEY"
	}
	return true, ""
}
func (a braveAdapter) Supports(item seededCase) bool {
	return item.TaskType == "same_site_query_routing"
}
func (a braveAdapter) Run(ctx context.Context, item seededCase) competitorResult {
	return a.runSearch(ctx, item)
}

type vercelBrowserAgentAdapter struct{}

func (a vercelBrowserAgentAdapter) Name() string          { return "vercel-browser-agent" }
func (a vercelBrowserAgentAdapter) Category() string      { return "adjacent_browser_agent" }
func (a vercelBrowserAgentAdapter) ProofComparable() bool { return false }
func (a vercelBrowserAgentAdapter) Availability() (bool, string) {
	if strings.TrimSpace(os.Getenv("VERCEL_BROWSER_AGENT_ENDPOINT")) == "" {
		return false, "missing_env:VERCEL_BROWSER_AGENT_ENDPOINT"
	}
	return true, ""
}
func (a vercelBrowserAgentAdapter) Supports(item seededCase) bool {
	switch item.TaskType {
	case "same_site_query_routing", "read_page_understanding", "read_then_answer":
		return true
	default:
		return false
	}
}
func (a vercelBrowserAgentAdapter) Run(ctx context.Context, item seededCase) competitorResult {
	body := map[string]any{
		"id":                 item.ID,
		"family":             item.Family,
		"language":           item.Language,
		"seed_url":           item.SeedURL,
		"task_type":          item.TaskType,
		"goal":               item.Goal,
		"expected_domain":    item.ExpectedDomain,
		"must_contain_facts": item.MustContainFacts,
	}
	var out struct {
		URL         string `json:"url"`
		SelectedURL string `json:"selected_url"`
		Summary     string `json:"summary"`
		Text        string `json:"text"`
		LatencyMS   int64  `json:"latency_ms"`
		Chunks      []struct {
			Text string `json:"text"`
		} `json:"chunks"`
	}
	headers := map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}
	if token := strings.TrimSpace(os.Getenv("VERCEL_BROWSER_AGENT_TOKEN")); token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	raw, err := doJSONRequestWithHeaders(ctx, http.MethodPost, strings.TrimSpace(os.Getenv("VERCEL_BROWSER_AGENT_ENDPOINT")), headers, body, &out)
	if err != nil {
		return competitorResult{Error: err.Error(), FailureClasses: []string{classifyExecutionError(err.Error())}}
	}
	actualURL := firstNonEmpty(out.SelectedURL, out.URL, item.SeedURL)
	text := firstNonEmpty(out.Summary, out.Text, joinChunkTexts(out.Chunks))
	res := competitorResult{
		RuntimeOK:       true,
		ActualURL:       strings.TrimSpace(actualURL),
		SelectedURLPass: sameCanonicalURL(actualURL, item.ExpectedURL),
		SummaryPresent:  strings.TrimSpace(firstNonEmpty(out.Summary, text)) != "",
		PacketBytes:     len(raw),
		ChunkCount:      max(1, len(out.Chunks)),
		LatencyMS:       out.LatencyMS,
		ProofUsable:     false,
	}
	res.FactCoverage, res.CoveredFacts, res.MissingFacts = evaluateFactCoverage(text, item.MustContainFacts)
	res.HopCountToTarget = estimateHopCountToTarget(item, res.SelectedURLPass)
	res.FailureClasses = classifyQualityFailures(res, item, a.ProofComparable())
	res.QualityPass = len(res.FailureClasses) == 0
	return res
}

type envSkippedAdapter struct {
	name, category, envVar string
}

func (a envSkippedAdapter) Name() string          { return a.name }
func (a envSkippedAdapter) Category() string      { return a.category }
func (a envSkippedAdapter) ProofComparable() bool { return false }
func (a envSkippedAdapter) Availability() (bool, string) {
	if strings.TrimSpace(os.Getenv(a.envVar)) == "" {
		return false, "missing_env:" + a.envVar
	}
	return false, "adapter_not_implemented"
}
func (a envSkippedAdapter) Supports(seededCase) bool { return false }
func (a envSkippedAdapter) Run(context.Context, seededCase) competitorResult {
	return competitorResult{Skipped: true, SkipReason: "adapter_not_implemented"}
}

func summarizeByCompetitor(rows []caseResult, adapters []adapter) []competitorSummary {
	type agg struct {
		category                                                                       string
		configured                                                                     bool
		caseCount, supported, runtimeOK, qualityPass, selectedPass, proofPass, skipped int
		factCoverageTotal                                                              float64
		claimToSourceTotal                                                             int
		claimToSourceCount                                                             int
		postProcessingTotal                                                            int
		cachedCount                                                                    int
		hopTotal                                                                       int
		hopCount                                                                       int
		latencyTotal, bytesTotal                                                       int64
	}
	by := map[string]*agg{}
	for _, a := range adapters {
		configured, _ := a.Availability()
		by[a.Name()] = &agg{category: a.Category(), configured: configured}
	}
	for _, row := range rows {
		for _, res := range row.Results {
			a := by[res.Competitor]
			if a == nil {
				continue
			}
			a.caseCount++
			if res.Supported {
				a.supported++
			}
			if res.Skipped {
				a.skipped++
				continue
			}
			if res.RuntimeOK {
				a.runtimeOK++
			}
			if res.QualityPass {
				a.qualityPass++
			}
			if res.SelectedURLPass {
				a.selectedPass++
			}
			if res.ProofUsable {
				a.proofPass++
			}
			a.factCoverageTotal += res.FactCoverage
			a.postProcessingTotal += res.PostProcessingBurden
			if res.ClaimToSourceSteps != nil {
				a.claimToSourceTotal += *res.ClaimToSourceSteps
				a.claimToSourceCount++
			}
			if res.Cached {
				a.cachedCount++
			}
			if res.HopCountToTarget != nil {
				a.hopTotal += *res.HopCountToTarget
				a.hopCount++
			}
			a.latencyTotal += res.LatencyMS
			a.bytesTotal += int64(res.PacketBytes)
		}
	}
	out := make([]competitorSummary, 0, len(by))
	for name, a := range by {
		den := max(a.caseCount-a.skipped, 1)
		out = append(out, competitorSummary{
			Competitor:                name,
			Category:                  a.category,
			CaseCount:                 a.caseCount,
			Configured:                a.configured,
			SupportedRate:             evalutil.Ratio(a.supported, max(a.caseCount, 1)),
			RuntimeSuccessRate:        evalutil.Ratio(a.runtimeOK, den),
			QualityPassRate:           evalutil.Ratio(a.qualityPass, den),
			SelectedURLPassRate:       evalutil.Ratio(a.selectedPass, den),
			ProofUsabilityRate:        evalutil.Ratio(a.proofPass, den),
			FactCoverageRate:          a.factCoverageTotal / float64(den),
			ClaimToSourceCoverageRate: evalutil.Ratio(a.claimToSourceCount, den),
			AvgClaimToSourceSteps:     averageFloat64(float64(a.claimToSourceTotal), a.claimToSourceCount),
			AvgPostProcessingBurden:   averageFloat64(float64(a.postProcessingTotal), den),
			CachedReuseRate:           evalutil.Ratio(a.cachedCount, den),
			CachedReuseCount:          a.cachedCount,
			AvgHopCountToTarget:       averageFloat64(float64(a.hopTotal), a.hopCount),
			AvgLatencyMS:              averageInt64(a.latencyTotal, den),
			AvgPacketBytes:            averageInt64(a.bytesTotal, den),
			SkippedCases:              a.skipped,
		})
	}
	var baselineBytes int64
	for i := range out {
		if out[i].Competitor == "jina" && out[i].AvgPacketBytes > 0 {
			baselineBytes = out[i].AvgPacketBytes
			break
		}
	}
	if baselineBytes > 0 {
		for i := range out {
			out[i].PacketReductionVsBaseline = computePacketReductionVsBaseline(out[i].AvgPacketBytes, baselineBytes)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Competitor < out[j].Competitor })
	return out
}

func classifyQualityFailures(res competitorResult, item seededCase, requireProof bool) []string {
	fails := make([]string, 0, 5)
	if !res.SelectedURLPass {
		fails = append(fails, "wrong_selected_url")
	}
	if !res.SummaryPresent {
		fails = append(fails, "missing_summary")
	}
	if res.ChunkCount == 0 {
		fails = append(fails, "empty_packet")
	}
	if len(item.MustContainFacts) > 0 && len(res.MissingFacts) > 0 {
		fails = append(fails, "missing_facts")
	}
	if requireProof && item.MustExposeProof && !res.ProofUsable {
		fails = append(fails, "proof_not_actionable")
	}
	return fails
}

func classifyExecutionError(errText string) string {
	text := strings.ToLower(strings.TrimSpace(errText))
	switch {
	case text == "":
		return "runtime_error"
	case strings.Contains(text, "context deadline exceeded"), strings.Contains(text, "timeout"):
		return "network_timeout"
	case strings.Contains(text, "tls"), strings.Contains(text, "certificate"):
		return "network_tls_error"
	case strings.Contains(text, "connection reset"), strings.Contains(text, "connection refused"), strings.Contains(text, "no such host"):
		return "network_connect_error"
	default:
		return "runtime_error"
	}
}

func buildNeedleBinary() (string, func(), error) {
	tempDir, err := os.MkdirTemp("", "needlex-competitive-benchmark-*")
	if err != nil {
		return "", nil, err
	}
	binaryPath := filepath.Join(tempDir, "needle")
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/needle")
	root, err := findRepoRoot()
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return "", nil, err
	}
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", nil, err
	}
	return binaryPath, func() { _ = os.RemoveAll(tempDir) }, nil
}

func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found from current working directory")
		}
		dir = parent
	}
}

func runNeedleJSON(binaryPath string, args ...string) ([]byte, []byte, error) {
	cmd := exec.Command(binaryPath, args...)
	raw, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, raw, errors.New(strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, raw, err
	}
	return raw, raw, nil
}

func verifyNeedleProof(binaryPath string, chunks []compactChunk, required bool) bool {
	if !required {
		return true
	}
	if len(chunks) == 0 || strings.TrimSpace(chunks[0].ProofRef) == "" {
		return false
	}
	payload, _, err := runNeedleJSON(binaryPath, "proof", chunks[0].ProofRef, "--json")
	if err != nil {
		return false
	}
	var out proofPayload
	if err := json.Unmarshal(payload, &out); err != nil {
		return false
	}
	return len(out.ProofRecords) > 0 && strings.TrimSpace(out.ProofRecords[0].Proof.SourceSpan.Selector) != ""
}

func averageInt64(total int64, count int) int64 {
	if count <= 0 {
		return 0
	}
	return total / int64(count)
}

func averageFloat64(total float64, count int) float64 {
	if count <= 0 {
		return 0
	}
	return total / float64(count)
}

func computePacketReductionVsBaseline(avgBytes int64, baselineBytes int64) float64 {
	if baselineBytes <= 0 {
		return 0
	}
	return float64(baselineBytes-avgBytes) / float64(baselineBytes)
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func sameCanonicalURL(a, b string) bool { return canonicalizeURL(a) == canonicalizeURL(b) }

func canonicalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return strings.TrimRight(strings.ToLower(raw), "/")
	}
	host := strings.ToLower(strings.TrimSpace(u.Host))
	host = strings.TrimPrefix(host, "www.")
	path := strings.TrimRight(u.EscapedPath(), "/")
	if path == "" {
		path = "/"
	}
	return host + path
}

func ioReadAll(respBody interface{ Read([]byte) (int, error) }) ([]byte, error) {
	return io.ReadAll(respBody)
}

func (a firecrawlAdapter) runScrape(ctx context.Context, item seededCase) competitorResult {
	body := map[string]any{
		"url":             item.SeedURL,
		"formats":         []string{"markdown", "summary"},
		"onlyMainContent": true,
		"timeout":         30000,
	}
	var out struct {
		Success bool `json:"success"`
		Data    struct {
			Markdown string `json:"markdown"`
			Summary  string `json:"summary"`
			Metadata struct {
				SourceURL string `json:"sourceURL"`
				URL       string `json:"url"`
			} `json:"metadata"`
		} `json:"data"`
	}
	raw, err := doJSONRequest(ctx, "POST", "https://api.firecrawl.dev/v2/scrape", os.Getenv("FIRECRAWL_API_KEY"), body, &out)
	if err != nil {
		return competitorResult{Error: err.Error(), FailureClasses: []string{classifyExecutionError(err.Error())}}
	}
	actualURL := firstNonEmpty(out.Data.Metadata.URL, out.Data.Metadata.SourceURL, item.SeedURL)
	res := competitorResult{
		RuntimeOK:       true,
		ActualURL:       strings.TrimSpace(actualURL),
		SelectedURLPass: sameCanonicalURL(actualURL, item.ExpectedURL),
		SummaryPresent:  strings.TrimSpace(firstNonEmpty(out.Data.Summary, out.Data.Markdown)) != "",
		PacketBytes:     len(raw),
		ChunkCount:      countSyntheticChunks(firstNonEmpty(out.Data.Summary, out.Data.Markdown)),
		ProofUsable:     false,
	}
	text := firstNonEmpty(out.Data.Summary, out.Data.Markdown)
	res.FactCoverage, res.CoveredFacts, res.MissingFacts = evaluateFactCoverage(text, item.MustContainFacts)
	res.ClaimToSourceSteps = estimateClaimToSourceSteps(res)
	res.PostProcessingBurden = estimatePostProcessingBurden(item, res)
	res.HopCountToTarget = estimateHopCountToTarget(item, res.SelectedURLPass)
	res.FailureClasses = classifyQualityFailures(res, item, a.ProofComparable())
	res.QualityPass = len(res.FailureClasses) == 0
	return res
}

func (a firecrawlAdapter) runSearch(ctx context.Context, item seededCase) competitorResult {
	domain := canonicalDomain(item.SeedURL)
	query := strings.TrimSpace("site:" + domain + " " + item.Goal)
	body := map[string]any{
		"query":   query,
		"limit":   3,
		"sources": []string{"web"},
		"scrapeOptions": map[string]any{
			"formats":         []string{"markdown", "summary"},
			"onlyMainContent": true,
		},
	}
	var out struct {
		Success bool `json:"success"`
		Data    struct {
			Web []struct {
				Title       string `json:"title"`
				Description string `json:"description"`
				URL         string `json:"url"`
				Markdown    string `json:"markdown"`
				Metadata    struct {
					URL       string `json:"url"`
					SourceURL string `json:"sourceURL"`
				} `json:"metadata"`
			} `json:"web"`
		} `json:"data"`
	}
	raw, err := doJSONRequest(ctx, "POST", "https://api.firecrawl.dev/v2/search", os.Getenv("FIRECRAWL_API_KEY"), body, &out)
	if err != nil {
		return competitorResult{Error: err.Error(), FailureClasses: []string{classifyExecutionError(err.Error())}}
	}
	if len(out.Data.Web) == 0 {
		return competitorResult{RuntimeOK: true, Error: "empty search results", FailureClasses: []string{"empty_packet"}}
	}
	top := out.Data.Web[0]
	actualURL := firstNonEmpty(top.URL, top.Metadata.URL, top.Metadata.SourceURL)
	content := firstNonEmpty(top.Description, top.Markdown, top.Title)
	res := competitorResult{
		RuntimeOK:       true,
		ActualURL:       strings.TrimSpace(actualURL),
		SelectedURLPass: sameCanonicalURL(actualURL, item.ExpectedURL),
		SummaryPresent:  strings.TrimSpace(content) != "",
		PacketBytes:     len(raw),
		ChunkCount:      countSyntheticChunks(content),
		ProofUsable:     false,
	}
	res.FactCoverage, res.CoveredFacts, res.MissingFacts = evaluateFactCoverage(content, item.MustContainFacts)
	res.ClaimToSourceSteps = estimateClaimToSourceSteps(res)
	res.PostProcessingBurden = estimatePostProcessingBurden(item, res)
	res.HopCountToTarget = estimateHopCountToTarget(item, res.SelectedURLPass)
	res.FailureClasses = classifyQualityFailures(res, item, a.ProofComparable())
	res.QualityPass = len(res.FailureClasses) == 0
	return res
}

func (a tavilyAdapter) runExtract(ctx context.Context, item seededCase) competitorResult {
	body := map[string]any{
		"urls":          []string{item.SeedURL},
		"extract_depth": "basic",
		"format":        "markdown",
		"timeout":       20,
	}
	var out struct {
		Results []struct {
			URL        string `json:"url"`
			RawContent string `json:"raw_content"`
		} `json:"results"`
		ResponseTime float64 `json:"response_time"`
	}
	raw, err := doJSONRequest(ctx, "POST", "https://api.tavily.com/extract", os.Getenv("TAVILY_API_KEY"), body, &out)
	if err != nil {
		return competitorResult{Error: err.Error(), FailureClasses: []string{classifyExecutionError(err.Error())}}
	}
	if len(out.Results) == 0 {
		return competitorResult{RuntimeOK: true, Error: "empty extract results", FailureClasses: []string{"empty_packet"}}
	}
	top := out.Results[0]
	res := competitorResult{
		RuntimeOK:       true,
		ActualURL:       strings.TrimSpace(firstNonEmpty(top.URL, item.SeedURL)),
		SelectedURLPass: sameCanonicalURL(top.URL, item.ExpectedURL),
		SummaryPresent:  strings.TrimSpace(top.RawContent) != "",
		PacketBytes:     len(raw),
		ChunkCount:      countSyntheticChunks(top.RawContent),
		LatencyMS:       int64(out.ResponseTime * 1000),
		ProofUsable:     false,
	}
	res.FactCoverage, res.CoveredFacts, res.MissingFacts = evaluateFactCoverage(top.RawContent, item.MustContainFacts)
	res.ClaimToSourceSteps = estimateClaimToSourceSteps(res)
	res.PostProcessingBurden = estimatePostProcessingBurden(item, res)
	res.HopCountToTarget = estimateHopCountToTarget(item, res.SelectedURLPass)
	res.FailureClasses = classifyQualityFailures(res, item, a.ProofComparable())
	res.QualityPass = len(res.FailureClasses) == 0
	return res
}

func (a tavilyAdapter) runSearch(ctx context.Context, item seededCase) competitorResult {
	body := map[string]any{
		"query":               item.Goal,
		"search_depth":        "basic",
		"max_results":         3,
		"include_domains":     []string{canonicalDomain(item.SeedURL)},
		"include_answer":      false,
		"include_favicon":     false,
		"include_raw_content": false,
	}
	var out struct {
		Results []struct {
			Title   string  `json:"title"`
			URL     string  `json:"url"`
			Content string  `json:"content"`
			Score   float64 `json:"score"`
		} `json:"results"`
		ResponseTime float64 `json:"response_time"`
	}
	raw, err := doJSONRequest(ctx, "POST", "https://api.tavily.com/search", os.Getenv("TAVILY_API_KEY"), body, &out)
	if err != nil {
		return competitorResult{Error: err.Error(), FailureClasses: []string{classifyExecutionError(err.Error())}}
	}
	if len(out.Results) == 0 {
		return competitorResult{RuntimeOK: true, Error: "empty search results", FailureClasses: []string{"empty_packet"}}
	}
	top := out.Results[0]
	content := firstNonEmpty(top.Content, top.Title)
	res := competitorResult{
		RuntimeOK:       true,
		ActualURL:       strings.TrimSpace(top.URL),
		SelectedURLPass: sameCanonicalURL(top.URL, item.ExpectedURL),
		SummaryPresent:  strings.TrimSpace(content) != "",
		PacketBytes:     len(raw),
		ChunkCount:      countSyntheticChunks(content),
		LatencyMS:       int64(out.ResponseTime * 1000),
		ProofUsable:     false,
	}
	res.FactCoverage, res.CoveredFacts, res.MissingFacts = evaluateFactCoverage(content, item.MustContainFacts)
	res.ClaimToSourceSteps = estimateClaimToSourceSteps(res)
	res.PostProcessingBurden = estimatePostProcessingBurden(item, res)
	res.HopCountToTarget = estimateHopCountToTarget(item, res.SelectedURLPass)
	res.FailureClasses = classifyQualityFailures(res, item, a.ProofComparable())
	res.QualityPass = len(res.FailureClasses) == 0
	return res
}

func (a exaAdapter) runSearch(ctx context.Context, item seededCase) competitorResult {
	body := map[string]any{
		"query":         "site:" + canonicalDomain(item.SeedURL) + " " + item.Goal,
		"type":          "auto",
		"numResults":    3,
		"useAutoprompt": false,
	}
	var out struct {
		Results []struct {
			URL     string `json:"url"`
			Title   string `json:"title"`
			Text    string `json:"text"`
			Summary string `json:"summary"`
		} `json:"results"`
	}
	raw, err := doJSONRequestWithHeaders(ctx, "POST", "https://api.exa.ai/search", map[string]string{
		"x-api-key":    strings.TrimSpace(os.Getenv("EXA_API_KEY")),
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}, body, &out)
	if err != nil {
		return competitorResult{Error: err.Error(), FailureClasses: []string{classifyExecutionError(err.Error())}}
	}
	if len(out.Results) == 0 {
		return competitorResult{RuntimeOK: true, Error: "empty search results", FailureClasses: []string{"empty_packet"}}
	}
	top := out.Results[0]
	content := firstNonEmpty(top.Summary, top.Text, top.Title)
	res := competitorResult{
		RuntimeOK:       true,
		ActualURL:       strings.TrimSpace(top.URL),
		SelectedURLPass: sameCanonicalURL(top.URL, item.ExpectedURL),
		SummaryPresent:  strings.TrimSpace(content) != "",
		PacketBytes:     len(raw),
		ChunkCount:      countSyntheticChunks(content),
		ProofUsable:     false,
	}
	res.FactCoverage, res.CoveredFacts, res.MissingFacts = evaluateFactCoverage(content, item.MustContainFacts)
	res.ClaimToSourceSteps = estimateClaimToSourceSteps(res)
	res.PostProcessingBurden = estimatePostProcessingBurden(item, res)
	res.HopCountToTarget = estimateHopCountToTarget(item, res.SelectedURLPass)
	res.FailureClasses = classifyQualityFailures(res, item, a.ProofComparable())
	res.QualityPass = len(res.FailureClasses) == 0
	return res
}

func (a exaAdapter) runContents(ctx context.Context, item seededCase) competitorResult {
	body := map[string]any{
		"urls": []string{item.SeedURL},
		"text": true,
	}
	var out struct {
		Results []struct {
			URL     string `json:"url"`
			Title   string `json:"title"`
			Text    string `json:"text"`
			Summary string `json:"summary"`
		} `json:"results"`
	}
	raw, err := doJSONRequestWithHeaders(ctx, "POST", "https://api.exa.ai/contents", map[string]string{
		"x-api-key":    strings.TrimSpace(os.Getenv("EXA_API_KEY")),
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}, body, &out)
	if err != nil {
		return competitorResult{Error: err.Error(), FailureClasses: []string{classifyExecutionError(err.Error())}}
	}
	if len(out.Results) == 0 {
		return competitorResult{RuntimeOK: true, Error: "empty contents results", FailureClasses: []string{"empty_packet"}}
	}
	top := out.Results[0]
	content := firstNonEmpty(top.Summary, top.Text, top.Title)
	res := competitorResult{
		RuntimeOK:       true,
		ActualURL:       strings.TrimSpace(firstNonEmpty(top.URL, item.SeedURL)),
		SelectedURLPass: sameCanonicalURL(firstNonEmpty(top.URL, item.SeedURL), item.ExpectedURL),
		SummaryPresent:  strings.TrimSpace(content) != "",
		PacketBytes:     len(raw),
		ChunkCount:      countSyntheticChunks(content),
		ProofUsable:     false,
	}
	res.FactCoverage, res.CoveredFacts, res.MissingFacts = evaluateFactCoverage(content, item.MustContainFacts)
	res.ClaimToSourceSteps = estimateClaimToSourceSteps(res)
	res.PostProcessingBurden = estimatePostProcessingBurden(item, res)
	res.HopCountToTarget = estimateHopCountToTarget(item, res.SelectedURLPass)
	res.FailureClasses = classifyQualityFailures(res, item, a.ProofComparable())
	res.QualityPass = len(res.FailureClasses) == 0
	return res
}

func (a braveAdapter) runSearch(ctx context.Context, item seededCase) competitorResult {
	params := url.Values{}
	params.Set("q", "site:"+canonicalDomain(item.SeedURL)+" "+item.Goal)
	params.Set("count", "5")
	params.Set("search_lang", item.Language)
	var out struct {
		Web struct {
			Results []struct {
				URL         string `json:"url"`
				Title       string `json:"title"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	raw, err := doGETJSONWithHeaders(ctx, "https://api.search.brave.com/res/v1/web/search?"+params.Encode(), map[string]string{
		"Accept":               "application/json",
		"Accept-Encoding":      "gzip",
		"X-Subscription-Token": strings.TrimSpace(os.Getenv("BRAVE_SEARCH_API_KEY")),
	}, &out)
	if err != nil {
		return competitorResult{Error: err.Error(), FailureClasses: []string{classifyExecutionError(err.Error())}}
	}
	if len(out.Web.Results) == 0 {
		return competitorResult{RuntimeOK: true, Error: "empty search results", FailureClasses: []string{"empty_packet"}}
	}
	top := out.Web.Results[0]
	content := firstNonEmpty(top.Description, top.Title)
	res := competitorResult{
		RuntimeOK:       true,
		ActualURL:       strings.TrimSpace(top.URL),
		SelectedURLPass: sameCanonicalURL(top.URL, item.ExpectedURL),
		SummaryPresent:  strings.TrimSpace(content) != "",
		PacketBytes:     len(raw),
		ChunkCount:      countSyntheticChunks(content),
		ProofUsable:     false,
	}
	res.FactCoverage, res.CoveredFacts, res.MissingFacts = evaluateFactCoverage(content, item.MustContainFacts)
	res.ClaimToSourceSteps = estimateClaimToSourceSteps(res)
	res.PostProcessingBurden = estimatePostProcessingBurden(item, res)
	res.HopCountToTarget = estimateHopCountToTarget(item, res.SelectedURLPass)
	res.FailureClasses = classifyQualityFailures(res, item, a.ProofComparable())
	res.QualityPass = len(res.FailureClasses) == 0
	return res
}

func doJSONRequest(ctx context.Context, method, endpoint, apiKey string, body any, out any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 35 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return raw, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return raw, err
	}
	return raw, nil
}

func doJSONRequestWithHeaders(ctx context.Context, method, endpoint string, headers map[string]string, body any, out any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	client := &http.Client{Timeout: 35 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return raw, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return raw, err
	}
	return raw, nil
}

func doGETJSONWithHeaders(ctx context.Context, endpoint string, headers map[string]string, out any) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	client := &http.Client{Timeout: 35 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return raw, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return raw, err
	}
	return raw, nil
}

func canonicalDomain(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(raw)), "www.")
	}
	host := strings.ToLower(strings.TrimSpace(u.Host))
	return strings.TrimPrefix(host, "www.")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func countSyntheticChunks(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	return 1
}

func buildNeedleText(summary string, chunks []compactChunk) string {
	parts := make([]string, 0, 1+len(chunks))
	if trimmed := strings.TrimSpace(summary); trimmed != "" {
		parts = append(parts, trimmed)
	}
	for _, chunk := range chunks {
		if trimmed := strings.TrimSpace(chunk.Text); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, "\n")
}

func evaluateFactCoverage(text string, facts []string) (float64, []string, []string) {
	if len(facts) == 0 {
		return 1, nil, nil
	}
	normText := normalizeFactText(text)
	covered := make([]string, 0, len(facts))
	missing := make([]string, 0, len(facts))
	for _, fact := range facts {
		normFact := normalizeFactText(fact)
		if normFact != "" && strings.Contains(normText, normFact) {
			covered = append(covered, fact)
			continue
		}
		missing = append(missing, fact)
	}
	return float64(len(covered)) / float64(len(facts)), covered, missing
}

func normalizeFactText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	lastSpace := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func estimateHopCountToTarget(item seededCase, selectedURLPass bool) *int {
	if !selectedURLPass {
		return nil
	}
	hops := 0
	if item.TaskType == "same_site_query_routing" && !sameCanonicalURL(item.SeedURL, item.ExpectedURL) {
		hops = 1
	}
	return &hops
}

func estimateClaimToSourceSteps(res competitorResult) *int {
	if !res.SummaryPresent {
		return nil
	}
	steps := 2
	if res.ProofUsable {
		steps = 1
	}
	return &steps
}

func estimatePostProcessingBurden(item seededCase, res competitorResult) int {
	burden := 0
	if !res.SummaryPresent {
		burden++
	}
	if res.ChunkCount <= 1 && res.PacketBytes > 8000 {
		burden++
	}
	if len(res.SelectionWhy) == 0 && item.TaskType == "same_site_query_routing" {
		burden++
	}
	if !res.ProofUsable && item.MustExposeProof {
		burden++
	}
	if !res.SelectedURLPass {
		burden++
	}
	return burden
}

func enrichDerivedMetrics(item seededCase, res *competitorResult) {
	if res == nil {
		return
	}
	if res.ClaimToSourceSteps == nil {
		res.ClaimToSourceSteps = estimateClaimToSourceSteps(*res)
	}
	if res.PostProcessingBurden == 0 {
		res.PostProcessingBurden = estimatePostProcessingBurden(item, *res)
	}
	if res.HopCountToTarget == nil {
		res.HopCountToTarget = estimateHopCountToTarget(item, res.SelectedURLPass)
	}
}

func joinChunkTexts(chunks []struct {
	Text string `json:"text"`
}) string {
	parts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		if trimmed := strings.TrimSpace(chunk.Text); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, "\n")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
