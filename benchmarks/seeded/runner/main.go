package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/josepavese/needlex/benchmarks/internal/evalutil"
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
	Kind        string         `json:"kind"`
	URL         string         `json:"url"`
	Summary     string         `json:"summary"`
	Uncertainty map[string]any `json:"uncertainty"`
	Chunks      []compactChunk `json:"chunks"`
	CostReport  compactCost    `json:"cost_report"`
}

type compactQuery struct {
	Kind         string         `json:"kind"`
	SelectedURL  string         `json:"selected_url"`
	Summary      string         `json:"summary"`
	Uncertainty  map[string]any `json:"uncertainty"`
	SelectionWhy []string       `json:"selection_why"`
	Chunks       []compactChunk `json:"chunks"`
	CostReport   compactCost    `json:"cost_report"`
}

type proofPayload struct {
	TraceID      string `json:"trace_id"`
	ProofRecords []struct {
		ID    string `json:"id"`
		Proof struct {
			SourceSpan struct {
				Selector string `json:"selector"`
			} `json:"source_span"`
		} `json:"proof"`
	} `json:"proof_records"`
}

type caseResult struct {
	ID               string   `json:"id"`
	Family           string   `json:"family"`
	Language         string   `json:"language"`
	TaskType         string   `json:"task_type"`
	SeedURL          string   `json:"seed_url"`
	Goal             string   `json:"goal,omitempty"`
	ExpectedURL      string   `json:"expected_url"`
	ActualURL        string   `json:"actual_url,omitempty"`
	RuntimeOK        bool     `json:"runtime_ok"`
	QualityPass      bool     `json:"quality_pass"`
	Pass             bool     `json:"pass"`
	SelectedURLPass  bool     `json:"selected_url_pass"`
	SummaryPresent   bool     `json:"summary_present"`
	ProofUsable      bool     `json:"proof_usable"`
	ProofRef         string   `json:"proof_ref,omitempty"`
	ChunkCount       int      `json:"chunk_count"`
	PacketBytes      int      `json:"packet_bytes"`
	LatencyMS        int64    `json:"latency_ms"`
	UncertaintyLevel string   `json:"uncertainty_level,omitempty"`
	SelectionWhy     []string `json:"selection_why,omitempty"`
	FailureClasses   []string `json:"failure_classes,omitempty"`
	Error            string   `json:"error,omitempty"`
	Notes            string   `json:"notes,omitempty"`
}

type familySummary struct {
	Family              string  `json:"family"`
	CaseCount           int     `json:"case_count"`
	RuntimeSuccessRate  float64 `json:"runtime_success_rate"`
	QualityPassRate     float64 `json:"quality_pass_rate"`
	PassRate            float64 `json:"pass_rate"`
	SelectedURLPassRate float64 `json:"selected_url_pass_rate"`
	ProofUsabilityRate  float64 `json:"proof_usability_rate"`
	AvgLatencyMS        int64   `json:"avg_latency_ms"`
	AvgPacketBytes      int64   `json:"avg_packet_bytes"`
}

type summary struct {
	CaseCount           int             `json:"case_count"`
	RuntimeSuccessRate  float64         `json:"runtime_success_rate"`
	QualityPassRate     float64         `json:"quality_pass_rate"`
	PassRate            float64         `json:"pass_rate"`
	SelectedURLPassRate float64         `json:"selected_url_pass_rate"`
	ProofUsabilityRate  float64         `json:"proof_usability_rate"`
	AvgLatencyMS        int64           `json:"avg_latency_ms"`
	AvgPacketBytes      int64           `json:"avg_packet_bytes"`
	FailureClassCounts  map[string]int  `json:"failure_class_counts,omitempty"`
	FamilyBreakdown     []familySummary `json:"family_breakdown,omitempty"`
}

type seededSummaryAgg struct {
	failureCounts                                                   map[string]int
	families                                                        map[string][]caseResult
	runtimeOKCount, qualityPassCount, passCount, urlPass, proofPass int
	latencyTotal, bytesTotal                                        int64
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
	flag.StringVar(&outPath, "out", "improvements/seeded-benchmark-latest.json", "output report path")
	flag.StringVar(&casesPath, "cases", "benchmarks/corpora/seeded-corpus-v2.json", "seeded corpus path")
	flag.Parse()

	c, err := loadCorpus(casesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load corpus: %v\n", err)
		os.Exit(1)
	}

	binaryPath, cleanup, err := buildNeedleBinary()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build needle: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	results := make([]caseResult, 0, len(c.Cases))
	for i, item := range c.Cases {
		fmt.Printf("[seeded] %s case %d/%d start id=%s family=%s task=%s\n", time.Now().Format("15:04:05"), i+1, len(c.Cases), item.ID, item.Family, item.TaskType)
		row := runCase(binaryPath, item)
		results = append(results, row)
		fmt.Printf("[seeded] %s case %d/%d done id=%s pass=%t url_pass=%t proof=%t latency=%dms bytes=%d\n",
			time.Now().Format("15:04:05"), i+1, len(c.Cases), item.ID, row.Pass, row.SelectedURLPass, row.ProofUsable, row.LatencyMS, row.PacketBytes)
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
	fmt.Printf("Seeded benchmark written to %s\n", outPath)
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

func buildNeedleBinary() (string, func(), error) {
	tempDir, err := os.MkdirTemp("", "needlex-seeded-benchmark-*")
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

func runCase(binaryPath string, item seededCase) caseResult {
	stateRoot, err := os.MkdirTemp("", "needlex-seeded-state-*")
	if err != nil {
		return caseResult{ID: item.ID, Error: err.Error(), FailureClasses: []string{"state_setup_failed"}}
	}
	defer func() { _ = os.RemoveAll(stateRoot) }()

	row := caseResult{
		ID:          item.ID,
		Family:      item.Family,
		Language:    item.Language,
		TaskType:    item.TaskType,
		SeedURL:     item.SeedURL,
		Goal:        item.Goal,
		ExpectedURL: item.ExpectedURL,
		Notes:       item.Notes,
	}

	switch item.TaskType {
	case "same_site_query_routing":
		row = runSeededQuery(binaryPath, stateRoot, item, row)
	case "read_page_understanding", "read_then_answer":
		row = runSeededRead(binaryPath, stateRoot, item, row)
	default:
		row.Error = "unsupported task type"
		row.FailureClasses = append(row.FailureClasses, "unsupported_task_type")
		return row
	}

	row.FailureClasses = classifyQualityFailures(row, item)
	row.QualityPass = len(row.FailureClasses) == 0
	row.Pass = len(row.FailureClasses) == 0
	return row
}

func runSeededQuery(binaryPath, stateRoot string, item seededCase, row caseResult) caseResult {
	payload, raw, err := runNeedleJSON(binaryPath, stateRoot, "query", item.SeedURL, "--goal", item.Goal, "--json")
	if err != nil {
		row.Error = err.Error()
		row.FailureClasses = append(row.FailureClasses, classifyExecutionError(err.Error()))
		return row
	}
	var out compactQuery
	if err := json.Unmarshal(payload, &out); err != nil {
		row.Error = err.Error()
		row.FailureClasses = append(row.FailureClasses, "decode_error")
		return row
	}
	row.RuntimeOK = true
	row.ActualURL = strings.TrimSpace(out.SelectedURL)
	row.SelectedURLPass = sameCanonicalURL(row.ActualURL, item.ExpectedURL)
	row.SummaryPresent = strings.TrimSpace(out.Summary) != ""
	row.ChunkCount = len(out.Chunks)
	row.PacketBytes = len(raw)
	row.LatencyMS = out.CostReport.LatencyMS
	row.UncertaintyLevel = uncertaintyLevelFromMap(out.Uncertainty)
	row.SelectionWhy = append([]string{}, out.SelectionWhy...)
	row.ProofRef, row.ProofUsable = verifyProof(binaryPath, stateRoot, out.Chunks, item.MustExposeProof)
	return row
}

func runSeededRead(binaryPath, stateRoot string, item seededCase, row caseResult) caseResult {
	payload, raw, err := runNeedleJSON(binaryPath, stateRoot, "read", item.SeedURL, "--json")
	if err != nil {
		row.Error = err.Error()
		row.FailureClasses = append(row.FailureClasses, classifyExecutionError(err.Error()))
		return row
	}
	var out compactRead
	if err := json.Unmarshal(payload, &out); err != nil {
		row.Error = err.Error()
		row.FailureClasses = append(row.FailureClasses, "decode_error")
		return row
	}
	row.RuntimeOK = true
	row.ActualURL = strings.TrimSpace(out.URL)
	row.SelectedURLPass = sameCanonicalURL(row.ActualURL, item.ExpectedURL)
	row.SummaryPresent = strings.TrimSpace(out.Summary) != ""
	row.ChunkCount = len(out.Chunks)
	row.PacketBytes = len(raw)
	row.LatencyMS = out.CostReport.LatencyMS
	row.UncertaintyLevel = uncertaintyLevelFromMap(out.Uncertainty)
	row.ProofRef, row.ProofUsable = verifyProof(binaryPath, stateRoot, out.Chunks, item.MustExposeProof)
	return row
}

func runNeedleJSON(binaryPath, stateRoot string, args ...string) ([]byte, []byte, error) {
	cmd := exec.Command(binaryPath, args...)
	cmd.Env = append(os.Environ(), "NEEDLEX_HOME="+stateRoot)
	raw, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, raw, fmt.Errorf("%s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, raw, err
	}
	return raw, raw, nil
}

func verifyProof(binaryPath, stateRoot string, chunks []compactChunk, required bool) (string, bool) {
	if !required {
		return "", true
	}
	if len(chunks) == 0 {
		return "", false
	}
	proofRef := strings.TrimSpace(chunks[0].ProofRef)
	if proofRef == "" {
		return "", false
	}
	payload, _, err := runNeedleJSON(binaryPath, stateRoot, "proof", proofRef, "--json")
	if err != nil {
		return proofRef, false
	}
	var out proofPayload
	if err := json.Unmarshal(payload, &out); err != nil {
		return proofRef, false
	}
	if len(out.ProofRecords) == 0 {
		return proofRef, false
	}
	return proofRef, strings.TrimSpace(out.ProofRecords[0].Proof.SourceSpan.Selector) != ""
}

func classifyQualityFailures(row caseResult, item seededCase) []string {
	failures := make([]string, 0, 4)
	if !row.SelectedURLPass {
		failures = append(failures, "wrong_selected_url")
	}
	if !row.SummaryPresent {
		failures = append(failures, "missing_summary")
	}
	if row.ChunkCount == 0 {
		failures = append(failures, "empty_packet")
	}
	if item.MustExposeProof && !row.ProofUsable {
		failures = append(failures, "proof_not_actionable")
	}
	return failures
}

func classifyExecutionError(errText string) string {
	text := strings.ToLower(strings.TrimSpace(errText))
	switch {
	case text == "":
		return "runtime_error"
	case strings.Contains(text, "context deadline exceeded"), strings.Contains(text, "i/o timeout"), strings.Contains(text, "timeout"):
		return "network_timeout"
	case strings.Contains(text, "tls"), strings.Contains(text, "certificate"):
		return "network_tls_error"
	case strings.Contains(text, "connection reset"), strings.Contains(text, "connection refused"), strings.Contains(text, "no such host"):
		return "network_connect_error"
	default:
		return "runtime_error"
	}
}

func uncertaintyLevelFromMap(value map[string]any) string {
	if value == nil {
		return ""
	}
	if level, ok := value["level"].(string); ok {
		return strings.TrimSpace(level)
	}
	return ""
}

func summarize(results []caseResult) summary {
	agg := seededSummaryAgg{failureCounts: map[string]int{}, families: map[string][]caseResult{}}
	for _, row := range results {
		agg.record(row)
	}
	return summary{
		CaseCount:           len(results),
		RuntimeSuccessRate:  evalutil.Ratio(agg.runtimeOKCount, len(results)),
		QualityPassRate:     evalutil.Ratio(agg.qualityPassCount, len(results)),
		PassRate:            evalutil.Ratio(agg.passCount, len(results)),
		SelectedURLPassRate: evalutil.Ratio(agg.urlPass, len(results)),
		ProofUsabilityRate:  evalutil.Ratio(agg.proofPass, len(results)),
		AvgLatencyMS:        averageInt64(agg.latencyTotal, len(results)),
		AvgPacketBytes:      averageInt64(agg.bytesTotal, len(results)),
		FailureClassCounts:  agg.failureCounts,
		FamilyBreakdown:     seededFamilySummaries(agg.families),
	}
}

func (a *seededSummaryAgg) record(row caseResult) {
	a.families[row.Family] = append(a.families[row.Family], row)
	if row.RuntimeOK {
		a.runtimeOKCount++
	}
	if row.QualityPass {
		a.qualityPassCount++
	}
	if row.Pass {
		a.passCount++
	}
	if row.SelectedURLPass {
		a.urlPass++
	}
	if row.ProofUsable {
		a.proofPass++
	}
	a.latencyTotal += row.LatencyMS
	a.bytesTotal += int64(row.PacketBytes)
	for _, failure := range row.FailureClasses {
		a.failureCounts[failure]++
	}
}

func seededFamilySummaries(familyAgg map[string][]caseResult) []familySummary {
	families := make([]familySummary, 0, len(familyAgg))
	for family, rows := range familyAgg {
		families = append(families, summarizeSeededFamily(family, rows))
	}
	return families
}

func summarizeSeededFamily(family string, rows []caseResult) familySummary {
	agg := seededSummaryAgg{}
	for _, row := range rows {
		agg.recordFamilyOnly(row)
	}
	return familySummary{
		Family:              family,
		CaseCount:           len(rows),
		RuntimeSuccessRate:  evalutil.Ratio(agg.runtimeOKCount, len(rows)),
		QualityPassRate:     evalutil.Ratio(agg.qualityPassCount, len(rows)),
		PassRate:            evalutil.Ratio(agg.passCount, len(rows)),
		SelectedURLPassRate: evalutil.Ratio(agg.urlPass, len(rows)),
		ProofUsabilityRate:  evalutil.Ratio(agg.proofPass, len(rows)),
		AvgLatencyMS:        averageInt64(agg.latencyTotal, len(rows)),
		AvgPacketBytes:      averageInt64(agg.bytesTotal, len(rows)),
	}
}

func (a *seededSummaryAgg) recordFamilyOnly(row caseResult) {
	if row.RuntimeOK {
		a.runtimeOKCount++
	}
	if row.QualityPass {
		a.qualityPassCount++
	}
	if row.Pass {
		a.passCount++
	}
	if row.SelectedURLPass {
		a.urlPass++
	}
	if row.ProofUsable {
		a.proofPass++
	}
	a.latencyTotal += row.LatencyMS
	a.bytesTotal += int64(row.PacketBytes)
}

func averageInt64(total int64, count int) int64 {
	if count == 0 {
		return 0
	}
	return total / int64(count)
}

func sameCanonicalURL(a, b string) bool {
	return canonicalizeURL(a) == canonicalizeURL(b)
}

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
