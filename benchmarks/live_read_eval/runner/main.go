package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
	coreservice "github.com/josepavese/needlex/internal/core/service"
	"github.com/josepavese/needlex/internal/evalutil"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/proof"
)

type evalCase struct {
	Name      string `json:"name"`
	Family    string `json:"family,omitempty"`
	URL       string `json:"url"`
	TimeoutMS int64  `json:"timeout_ms"`
	Objective string `json:"objective,omitempty"`
}

type readMetrics struct {
	Backend            string   `json:"backend,omitempty"`
	Success            bool     `json:"success"`
	Error              string   `json:"error,omitempty"`
	Title              string   `json:"title,omitempty"`
	LatencyMS          int64    `json:"latency_ms"`
	ChunkCount         int      `json:"chunk_count"`
	ContextAlignment   float64  `json:"context_alignment,omitempty"`
	NoiseHits          int      `json:"noise_hits"`
	WebIRVersion       string   `json:"web_ir_version,omitempty"`
	WebIRNodeCount     int      `json:"web_ir_node_count,omitempty"`
	WebIRShortRatio    float64  `json:"web_ir_short_ratio,omitempty"`
	WebIRHeadingRatio  float64  `json:"web_ir_heading_ratio,omitempty"`
	WebIREmbeddedCount int      `json:"web_ir_embedded_count,omitempty"`
	ReuseMode          string   `json:"reuse_mode,omitempty"`
	ReuseEligible      int      `json:"reuse_eligible,omitempty"`
	ReuseApplied       int      `json:"reuse_applied,omitempty"`
	ReuseRecomputed    int      `json:"reuse_recomputed,omitempty"`
	StableFPHits       int      `json:"stable_fp_hits,omitempty"`
	NovelFPHits        int      `json:"novel_fp_hits,omitempty"`
	Invocations        int      `json:"invocations,omitempty"`
	Interventions      int      `json:"interventions,omitempty"`
	Accepted           int      `json:"accepted_interventions,omitempty"`
	Rejected           int      `json:"rejected_interventions,omitempty"`
	PatchEffects       []string `json:"patch_effects,omitempty"`
	ValidatorMessages  []string `json:"validator_messages,omitempty"`
}

type caseResult struct {
	Name               string      `json:"name"`
	Family             string      `json:"family,omitempty"`
	URL                string      `json:"url"`
	Objective          string      `json:"objective,omitempty"`
	Baseline           readMetrics `json:"baseline"`
	Compare            readMetrics `json:"compare,omitempty"`
	ExternalNoiseHits  int         `json:"external_noise_hits,omitempty"`
	ExternalTextLength int         `json:"external_text_length,omitempty"`
}

type familySummary struct {
	Family                string  `json:"family"`
	CaseCount             int     `json:"case_count"`
	BaselineSuccessRate   float64 `json:"baseline_success_rate"`
	CompareSuccessRate    float64 `json:"compare_success_rate,omitempty"`
	AvgBaselineLatencyMS  int64   `json:"avg_baseline_latency_ms"`
	AvgCompareLatencyMS   int64   `json:"avg_compare_latency_ms,omitempty"`
	AvgBaselineContext    float64 `json:"avg_baseline_context_alignment,omitempty"`
	AvgCompareContext     float64 `json:"avg_compare_context_alignment,omitempty"`
	RuntimeErrorCount     int     `json:"runtime_error_count,omitempty"`
	AcceptedInterventions int     `json:"accepted_interventions,omitempty"`
	RejectedInterventions int     `json:"rejected_interventions,omitempty"`
}

type reportSummary struct {
	CaseCount             int             `json:"case_count"`
	BaselineSuccessRate   float64         `json:"baseline_success_rate"`
	CompareSuccessRate    float64         `json:"compare_success_rate,omitempty"`
	AvgBaselineLatencyMS  int64           `json:"avg_baseline_latency_ms"`
	AvgCompareLatencyMS   int64           `json:"avg_compare_latency_ms,omitempty"`
	AvgBaselineContext    float64         `json:"avg_baseline_context_alignment,omitempty"`
	AvgCompareContext     float64         `json:"avg_compare_context_alignment,omitempty"`
	RuntimeErrorCases     []string        `json:"runtime_error_cases,omitempty"`
	FailureClusters       []string        `json:"failure_clusters,omitempty"`
	Families              []familySummary `json:"families,omitempty"`
	AcceptedInterventions int             `json:"accepted_interventions,omitempty"`
	RejectedInterventions int             `json:"rejected_interventions,omitempty"`
}

type report struct {
	GeneratedAtUTC   string        `json:"generated_at_utc"`
	ExternalBaseline string        `json:"external_baseline,omitempty"`
	CompareEnabled   bool          `json:"compare_enabled"`
	CompareBackend   string        `json:"compare_backend,omitempty"`
	Summary          reportSummary `json:"summary"`
	Results          []caseResult  `json:"results"`
	Regressions      []string      `json:"regressions,omitempty"`
}

func main() {
	var (
		outPath        string
		baselinePath   string
		casesPath      string
		updateBaseline bool
	)
	flag.StringVar(&outPath, "out", "improvements/live-read-latest.json", "output report path")
	flag.StringVar(&baselinePath, "baseline", "improvements/live-read-baseline.json", "baseline report path")
	flag.StringVar(&casesPath, "cases", envOr("NEEDLEX_LIVE_READ_CASES", ""), "optional cases JSON path")
	flag.BoolVar(&updateBaseline, "update-baseline", false, "overwrite baseline with latest report")
	flag.Parse()

	externalCommand := strings.TrimSpace(os.Getenv("NEEDLEX_EXTERNAL_BASELINE_CMD"))
	compareEnabled := useLiveCompare()
	compareBackend := strings.TrimSpace(os.Getenv("NEEDLEX_MODELS_BACKEND"))
	if !compareEnabled {
		compareBackend = ""
	}

	cases, err := loadCases(casesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load cases: %v\n", err)
		os.Exit(1)
	}
	results := make([]caseResult, 0, len(cases))
	fmt.Printf("[live] %s evaluation start cases=%d compare=%t backend=%s\n", time.Now().Format("15:04:05"), len(cases), compareEnabled, nonEmpty(compareBackend, "noop"))
	for i, item := range cases {
		fmt.Printf("[live] %s case %d/%d start name=%s family=%s objective=%s\n", time.Now().Format("15:04:05"), i+1, len(cases), item.Name, nonEmpty(item.Family, "uncategorized"), nonEmpty(item.Objective, "-"))
		result := runCase(item, externalCommand, compareEnabled)
		results = append(results, result)
		fmt.Printf("[live] %s case %d/%d done name=%s baseline=%s latency=%dms context=%.2f",
			time.Now().Format("15:04:05"),
			i+1,
			len(cases),
			item.Name,
			statusLabel(result.Baseline.Success),
			result.Baseline.LatencyMS,
			result.Baseline.ContextAlignment,
		)
		if compareEnabled {
			fmt.Printf(" compare=%s latency=%dms context=%.2f interventions=%d accepted=%d rejected=%d",
				statusLabel(result.Compare.Success),
				result.Compare.LatencyMS,
				result.Compare.ContextAlignment,
				result.Compare.Interventions,
				result.Compare.Accepted,
				result.Compare.Rejected,
			)
		}
		fmt.Println()
	}

	rep := report{
		GeneratedAtUTC:   time.Now().UTC().Format(time.RFC3339),
		ExternalBaseline: externalCommand,
		CompareEnabled:   compareEnabled,
		CompareBackend:   compareBackend,
		Summary:          summarizeResults(results, compareEnabled),
		Results:          results,
	}

	if prior, err := loadReport(baselinePath); err == nil {
		rep.Regressions = compareReports(prior, rep)
	}

	if err := evalutil.WriteJSON(outPath, rep); err != nil {
		fmt.Fprintf(os.Stderr, "write report: %v\n", err)
		os.Exit(1)
	}
	if updateBaseline {
		if err := evalutil.WriteJSON(baselinePath, rep); err != nil {
			fmt.Fprintf(os.Stderr, "write baseline: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Printf("Live read evaluation written to %s\n", outPath)
	if updateBaseline {
		fmt.Printf("Baseline updated at %s\n", baselinePath)
	}
	for _, result := range rep.Results {
		fmt.Printf("- %s: baseline=%s context=%.2f noise=%d latency=%dms",
			result.Name,
			statusLabel(result.Baseline.Success),
			result.Baseline.ContextAlignment,
			result.Baseline.NoiseHits,
			result.Baseline.LatencyMS,
		)
		if rep.CompareEnabled {
			fmt.Printf(" compare=%s backend=%s context=%.2f noise=%d latency=%dms interventions=%d accepted=%d rejected=%d effects=%s",
				statusLabel(result.Compare.Success),
				nonEmpty(result.Compare.Backend, "noop"),
				result.Compare.ContextAlignment,
				result.Compare.NoiseHits,
				result.Compare.LatencyMS,
				result.Compare.Interventions,
				result.Compare.Accepted,
				result.Compare.Rejected,
				strings.Join(result.Compare.PatchEffects, ","),
			)
		}
		fmt.Println()
	}
	if len(rep.Regressions) > 0 {
		fmt.Println("Regressions detected:")
		for _, issue := range rep.Regressions {
			fmt.Printf("  - %s\n", issue)
		}
		os.Exit(2)
	}
}

func defaultCases() []evalCase {
	return []evalCase{
		{
			Name:      "carratellire",
			Family:    "corporate",
			URL:       "https://carratellire.com/",
			TimeoutMS: 12000,
			Objective: "company profile",
		},
		{
			Name:      "cnf",
			Family:    "corporate",
			URL:       "https://www.cnfsrl.it/",
			TimeoutMS: 12000,
			Objective: "capability summary",
		},
		{
			Name:      "halfpocket",
			Family:    "corporate",
			URL:       "https://halfpocket.net/",
			TimeoutMS: 12000,
			Objective: "service offering",
		},
	}
}

func loadCases(path string) ([]evalCase, error) {
	if strings.TrimSpace(path) == "" {
		return defaultCases(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases []evalCase
	if err := json.Unmarshal(data, &cases); err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("no cases found in %s", path)
	}
	return cases, nil
}

func runCase(item evalCase, externalCommand string, compareEnabled bool) caseResult {
	result := caseResult{
		Name:      item.Name,
		Family:    strings.TrimSpace(item.Family),
		URL:       item.URL,
		Objective: item.Objective,
	}

	result.Baseline = runReadVariant(item, false)
	if compareEnabled {
		result.Compare = runReadVariant(item, true)
	}
	if externalCommand != "" {
		if externalText, extErr := runExternalBaseline(item.URL, item.TimeoutMS, externalCommand); extErr == nil {
			result.ExternalTextLength = len(externalText)
			result.ExternalNoiseHits = noiseHits(externalText)
		}
	}
	return result
}

func runReadVariant(item evalCase, useLiveBackend bool) readMetrics {
	cfg := config.Defaults()
	cfg.Runtime.TimeoutMS = item.TimeoutMS
	cfg.Budget.MaxLatencyMS = maxInt64(cfg.Budget.MaxLatencyMS, item.TimeoutMS)
	clientTimeoutMS := item.TimeoutMS
	backend := "noop"

	if useLiveBackend {
		liveCfg, err := config.Load("")
		if err != nil {
			return readMetrics{Backend: "config_error", Error: err.Error()}
		}
		liveCfg.Runtime.TimeoutMS = item.TimeoutMS
		liveCfg.Budget.MaxLatencyMS = maxInt64(liveCfg.Budget.MaxLatencyMS, item.TimeoutMS)
		cfg = liveCfg
		backend = strings.TrimSpace(cfg.Models.Backend)
		clientTimeoutMS = maxInt64(clientTimeoutMS, cfg.Budget.MaxLatencyMS)
		clientTimeoutMS = maxInt64(clientTimeoutMS, cfg.Models.MicroTimeoutMS)
		clientTimeoutMS = maxInt64(clientTimeoutMS, cfg.Models.StructuredTimeoutMS)
		clientTimeoutMS = maxInt64(clientTimeoutMS, cfg.Models.SpecialistTimeoutMS)
	}
	client := &http.Client{Timeout: time.Duration(clientTimeoutMS) * time.Millisecond}

	svc, err := coreservice.New(cfg, client)
	if err != nil {
		return readMetrics{Backend: backend, Error: err.Error()}
	}

	storeRoot, err := os.MkdirTemp("", "needlex-live-read-*")
	if err != nil {
		return readMetrics{Backend: backend, Error: err.Error()}
	}
	defer os.RemoveAll(storeRoot)

	readReq := coreservice.ReadRequest{
		URL:        item.URL,
		Objective:  item.Objective,
		Profile:    core.ProfileStandard,
		RenderHint: true,
	}
	readResp, err := svc.Read(context.Background(), coreservice.PrepareReadRequestWithLocalState(storeRoot, readReq))
	if err != nil {
		return readMetrics{Backend: backend, Error: err.Error()}
	}
	coreservice.ObserveReadResponseWithLocalState(storeRoot, readReq, readResp)
	warmReq := coreservice.PrepareReadRequestWithLocalState(storeRoot, readReq)
	warmResp, err := svc.Read(context.Background(), warmReq)
	if err != nil {
		return readMetrics{Backend: backend, Error: err.Error()}
	}

	text := mergeChunkText(warmResp.ResultPack.Chunks)
	semanticAlignment := semanticAlignmentForChunks(item.Objective, warmResp.ResultPack.Chunks, client)
	interventions, accepted, rejected := traceInterventions(warmResp.Trace)
	patchEffects, validatorMessages := proofInvocationDiagnostics(warmResp.ProofRecords)

	return readMetrics{
		Backend:            backend,
		Success:            true,
		Title:              warmResp.Document.Title,
		LatencyMS:          warmResp.ResultPack.CostReport.LatencyMS,
		ChunkCount:         len(warmResp.ResultPack.Chunks),
		ContextAlignment:   semanticAlignment,
		NoiseHits:          noiseHits(text),
		WebIRVersion:       warmResp.WebIR.Version,
		WebIRNodeCount:     warmResp.WebIR.NodeCount,
		WebIRShortRatio:    warmResp.WebIR.Signals.ShortTextRatio,
		WebIRHeadingRatio:  warmResp.WebIR.Signals.HeadingRatio,
		WebIREmbeddedCount: warmResp.WebIR.Signals.EmbeddedNodeCount,
		ReuseMode:          stageMetadata(warmResp.Trace, "pack", "reuse_mode"),
		ReuseEligible:      stageMetadataInt(warmResp.Trace, "pack", "reuse_eligible"),
		ReuseApplied:       stageMetadataInt(warmResp.Trace, "pack", "reuse_applied"),
		ReuseRecomputed:    stageMetadataInt(warmResp.Trace, "pack", "reuse_recomputed"),
		StableFPHits:       stageMetadataInt(warmResp.Trace, "pack", "stable_fp_hits"),
		NovelFPHits:        stageMetadataInt(warmResp.Trace, "pack", "novel_fp_hits"),
		Invocations:        proofInvocationCount(warmResp.ProofRecords),
		Interventions:      interventions,
		Accepted:           accepted,
		Rejected:           rejected,
		PatchEffects:       patchEffects,
		ValidatorMessages:  validatorMessages,
	}
}

func stageMetadata(trace proof.RunTrace, stage, key string) string {
	for _, snapshot := range trace.Stages {
		if snapshot.Stage == stage {
			return strings.TrimSpace(snapshot.Metadata[key])
		}
	}
	return ""
}

func stageMetadataInt(trace proof.RunTrace, stage, key string) int {
	value, _ := strconv.Atoi(stageMetadata(trace, stage, key))
	return value
}

func mergeChunkText(chunks []core.Chunk) string {
	parts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		if strings.TrimSpace(chunk.Text) == "" {
			continue
		}
		parts = append(parts, chunk.Text)
	}
	return strings.ToLower(strings.Join(parts, "\n"))
}

func semanticAlignmentForChunks(objective string, chunks []core.Chunk, client *http.Client) float64 {
	objective = strings.TrimSpace(objective)
	if objective == "" || len(chunks) == 0 {
		return 0
	}
	cfg, err := config.Load("")
	if err != nil || !cfg.Semantic.Enabled || strings.TrimSpace(cfg.Semantic.Model) == "" {
		return 0
	}
	aligner := intel.NewSemanticAligner(cfg, client)
	candidates := make([]intel.SemanticCandidate, 0, minInt(len(chunks), cfg.Semantic.MaxCandidates))
	for idx, chunk := range chunks {
		if idx >= cfg.Semantic.MaxCandidates {
			break
		}
		text := strings.TrimSpace(chunk.Text)
		if text == "" {
			continue
		}
		candidates = append(candidates, intel.SemanticCandidate{
			ID:      nonEmpty(chunk.ID, fmt.Sprintf("chunk-%d", idx)),
			Heading: append([]string(nil), chunk.HeadingPath...),
			Text:    text,
		})
	}
	if len(candidates) == 0 {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Semantic.TimeoutMS)*time.Millisecond)
	defer cancel()
	alignment, err := aligner.Align(ctx, objective, candidates)
	if err != nil {
		return 0
	}
	return alignment.TopSimilarity
}

func noiseHits(text string) int {
	noiseTerms := []string{"casino", "aams", "22bet", "gambling", "scommesse"}
	hits := 0
	for _, term := range noiseTerms {
		if strings.Contains(text, term) {
			hits++
		}
	}
	return hits
}

func runExternalBaseline(url string, timeoutMS int64, command string) (string, error) {
	client := &http.Client{Timeout: time.Duration(timeoutMS) * time.Millisecond}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	rawHTML, err := io.ReadAll(io.LimitReader(resp.Body, 4_000_000))
	if err != nil {
		return "", err
	}

	cmd := exec.Command("bash", "-lc", command)
	cmd.Stdin = bytes.NewReader(rawHTML)
	cmd.Dir = "."
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(string(output))), nil
}

func loadReport(path string) (report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return report{}, err
	}
	var rep report
	if err := json.Unmarshal(data, &rep); err != nil {
		return report{}, err
	}
	return rep, nil
}

func compareReports(previous, current report) []string {
	prevIndex := map[string]caseResult{}
	for _, item := range previous.Results {
		prevIndex[item.Name] = item
	}

	regressions := []string{}
	for _, now := range current.Results {
		before, ok := prevIndex[now.Name]
		if !ok {
			continue
		}
		beforeBase := normalizeLegacyBaseline(before)
		nowBase := now.Baseline
		if beforeBase.Success && !nowBase.Success {
			regressions = append(regressions, fmt.Sprintf("%s baseline regressed: previously successful, now failed (%s)", now.Name, nowBase.Error))
		}
		if nowBase.Success && beforeBase.Success {
			if nowBase.NoiseHits > beforeBase.NoiseHits+1 {
				regressions = append(regressions, fmt.Sprintf("%s baseline regressed noise hits %d -> %d", now.Name, beforeBase.NoiseHits, nowBase.NoiseHits))
			}
			if beforeBase.WebIRVersion != "" && nowBase.WebIRVersion != beforeBase.WebIRVersion {
				regressions = append(regressions, fmt.Sprintf("%s baseline regressed web_ir version %s -> %s", now.Name, beforeBase.WebIRVersion, nowBase.WebIRVersion))
			}
			if beforeBase.WebIRNodeCount >= 6 && nowBase.WebIRNodeCount*2 < beforeBase.WebIRNodeCount {
				regressions = append(regressions, fmt.Sprintf("%s baseline regressed web_ir node count %d -> %d", now.Name, beforeBase.WebIRNodeCount, nowBase.WebIRNodeCount))
			}
			if nowBase.WebIRNodeCount == 0 {
				regressions = append(regressions, fmt.Sprintf("%s baseline regressed web_ir node count to zero", now.Name))
			}
		}
		if current.CompareEnabled && now.Compare.Success && beforeBase.ContextAlignment > 0 && now.Compare.ContextAlignment+0.05 < beforeBase.ContextAlignment {
			regressions = append(regressions, fmt.Sprintf("%s compare regressed context alignment %.2f -> %.2f", now.Name, beforeBase.ContextAlignment, now.Compare.ContextAlignment))
		}
	}
	return regressions
}

func summarizeResults(results []caseResult, compareEnabled bool) reportSummary {
	summary := reportSummary{
		CaseCount: len(results),
	}
	if len(results) == 0 {
		return summary
	}
	families := map[string]*familyAgg{}
	runtimeErrorSet := map[string]struct{}{}
	clusterSet := map[string]struct{}{}
	baselineSuccess := 0
	compareSuccess := 0
	var baselineLatency int64
	var compareLatency int64
	var baselineContext float64
	var compareContext float64

	for _, item := range results {
		if item.Baseline.Success {
			baselineSuccess++
		}
		baselineLatency += item.Baseline.LatencyMS
		baselineContext += item.Baseline.ContextAlignment
		summary.AcceptedInterventions += item.Compare.Accepted
		summary.RejectedInterventions += item.Compare.Rejected
		if compareEnabled && item.Compare.Success {
			compareSuccess++
		}
		if compareEnabled {
			compareLatency += item.Compare.LatencyMS
			compareContext += item.Compare.ContextAlignment
		}

		family := nonEmpty(strings.TrimSpace(item.Family), "uncategorized")
		agg := families[family]
		if agg == nil {
			agg = &familyAgg{name: family}
			families[family] = agg
		}
		agg.count++
		if item.Baseline.Success {
			agg.baselineSuccess++
		}
		agg.baselineLatency += item.Baseline.LatencyMS
		agg.baselineContext += item.Baseline.ContextAlignment
		if compareEnabled {
			if item.Compare.Success {
				agg.compareSuccess++
			}
			agg.compareLatency += item.Compare.LatencyMS
			agg.compareContext += item.Compare.ContextAlignment
			agg.accepted += item.Compare.Accepted
			agg.rejected += item.Compare.Rejected
		}

		if compareEnabled && strings.TrimSpace(item.Compare.Error) != "" {
			runtimeErrorSet[item.Name] = struct{}{}
			agg.runtimeErrors++
			clusterSet[classifyCompareFailure(item.Compare.Error, item.Compare.ValidatorMessages)] = struct{}{}
		}
	}

	summary.BaselineSuccessRate = float64(baselineSuccess) / float64(len(results))
	summary.AvgBaselineLatencyMS = baselineLatency / int64(len(results))
	summary.AvgBaselineContext = baselineContext / float64(len(results))
	if compareEnabled {
		summary.CompareSuccessRate = float64(compareSuccess) / float64(len(results))
		summary.AvgCompareLatencyMS = compareLatency / int64(len(results))
		summary.AvgCompareContext = compareContext / float64(len(results))
	}
	for name := range runtimeErrorSet {
		summary.RuntimeErrorCases = append(summary.RuntimeErrorCases, name)
	}
	slices.Sort(summary.RuntimeErrorCases)
	for cluster := range clusterSet {
		summary.FailureClusters = append(summary.FailureClusters, cluster)
	}
	slices.Sort(summary.FailureClusters)
	for _, family := range sortedFamilyKeys(families) {
		agg := families[family]
		entry := familySummary{
			Family:               agg.name,
			CaseCount:            agg.count,
			BaselineSuccessRate:  float64(agg.baselineSuccess) / float64(agg.count),
			AvgBaselineLatencyMS: agg.baselineLatency / int64(agg.count),
			AvgBaselineContext:   agg.baselineContext / float64(agg.count),
		}
		if compareEnabled {
			entry.CompareSuccessRate = float64(agg.compareSuccess) / float64(agg.count)
			entry.AvgCompareLatencyMS = agg.compareLatency / int64(agg.count)
			entry.AvgCompareContext = agg.compareContext / float64(agg.count)
			entry.RuntimeErrorCount = agg.runtimeErrors
			entry.AcceptedInterventions = agg.accepted
			entry.RejectedInterventions = agg.rejected
		}
		summary.Families = append(summary.Families, entry)
	}
	return summary
}

func sortedFamilyKeys(m map[string]*familyAgg) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

type familyAgg struct {
	name            string
	count           int
	baselineSuccess int
	compareSuccess  int
	baselineLatency int64
	compareLatency  int64
	baselineContext float64
	compareContext  float64
	runtimeErrors   int
	accepted        int
	rejected        int
}

func classifyCompareFailure(err string, validatorMessages []string) string {
	text := strings.ToLower(strings.TrimSpace(err + " " + strings.Join(validatorMessages, " ")))
	switch {
	case strings.Contains(text, "deadline"), strings.Contains(text, "timeout"):
		return "timeout"
	case strings.Contains(text, "502"), strings.Contains(text, "bad gateway"):
		return "upstream_gateway"
	case strings.Contains(text, "connection refused"), strings.Contains(text, "dial tcp"):
		return "upstream_connectivity"
	default:
		return "other_runtime_error"
	}
}

func normalizeLegacyBaseline(item caseResult) readMetrics {
	if item.Baseline.Success || item.Baseline.Error != "" || item.Baseline.Title != "" {
		return item.Baseline
	}
	return readMetrics{
		Success:            item.Baseline.Success || item.Baseline.Error == "",
		Error:              item.Baseline.Error,
		Title:              item.Baseline.Title,
		LatencyMS:          item.Baseline.LatencyMS,
		ChunkCount:         item.Baseline.ChunkCount,
		ContextAlignment:   item.Baseline.ContextAlignment,
		NoiseHits:          item.Baseline.NoiseHits,
		WebIRVersion:       item.Baseline.WebIRVersion,
		WebIRNodeCount:     item.Baseline.WebIRNodeCount,
		WebIRShortRatio:    item.Baseline.WebIRShortRatio,
		WebIRHeadingRatio:  item.Baseline.WebIRHeadingRatio,
		WebIREmbeddedCount: item.Baseline.WebIREmbeddedCount,
	}
}

func proofInvocationCount(records []proof.ProofRecord) int {
	total := 0
	for _, record := range records {
		total += len(record.Proof.ModelInvocations)
	}
	return total
}

func proofInvocationDiagnostics(records []proof.ProofRecord) ([]string, []string) {
	effectSet := map[string]struct{}{}
	messageSet := map[string]struct{}{}
	for _, record := range records {
		for _, invocation := range record.Proof.ModelInvocations {
			if value := strings.TrimSpace(invocation.PatchEffect); value != "" {
				effectSet[value] = struct{}{}
			}
			if value := strings.TrimSpace(invocation.ValidatorMessage); value != "" {
				messageSet[value] = struct{}{}
			}
		}
	}
	effects := make([]string, 0, len(effectSet))
	for value := range effectSet {
		effects = append(effects, value)
	}
	slices.Sort(effects)
	messages := make([]string, 0, len(messageSet))
	for value := range messageSet {
		messages = append(messages, value)
	}
	slices.Sort(messages)
	return effects, messages
}

func traceInterventions(trace proof.RunTrace) (int, int, int) {
	total := 0
	accepted := 0
	rejected := 0
	for _, event := range trace.Events {
		if event.Type != "model_intervention" {
			continue
		}
		total++
		switch strings.TrimSpace(event.Data["patch_outcome"]) {
		case "accepted":
			accepted++
		case "rejected":
			rejected++
		}
	}
	return total, accepted, rejected
}

func useLiveCompare() bool {
	value := strings.TrimSpace(os.Getenv("NEEDLEX_LIVE_READ_USE_COMPARE"))
	return strings.EqualFold(value, "1") || strings.EqualFold(value, "true")
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func statusLabel(ok bool) string {
	if ok {
		return "ok"
	}
	return "fail"
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
