package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
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
	flag.StringVar(&casesPath, "cases", "benchmarks/corpora/seeded-corpus-v1.json", "seeded corpus path")
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
		payload, raw, err := runNeedleJSON(binaryPath, "query", item.SeedURL, "--goal", item.Goal, "--json")
		if err != nil {
			row.Error = err.Error()
			row.FailureClasses = append(row.FailureClasses, classifyExecutionError(err.Error()))
			return row
		}
		row.RuntimeOK = true
		var out compactQuery
		if err := json.Unmarshal(payload, &out); err != nil {
			row.Error = err.Error()
			row.FailureClasses = append(row.FailureClasses, "decode_error")
			return row
		}
		row.ActualURL = strings.TrimSpace(out.SelectedURL)
		row.SelectedURLPass = sameCanonicalURL(row.ActualURL, item.ExpectedURL)
		row.SummaryPresent = strings.TrimSpace(out.Summary) != ""
		row.ChunkCount = len(out.Chunks)
		row.PacketBytes = len(raw)
		row.LatencyMS = out.CostReport.LatencyMS
		row.UncertaintyLevel = uncertaintyLevelFromMap(out.Uncertainty)
		row.SelectionWhy = append([]string{}, out.SelectionWhy...)
		row.ProofRef, row.ProofUsable = verifyProof(binaryPath, out.Chunks, item.MustExposeProof)
	case "read_page_understanding", "read_then_answer":
		payload, raw, err := runNeedleJSON(binaryPath, "read", item.SeedURL, "--json")
		if err != nil {
			row.Error = err.Error()
			row.FailureClasses = append(row.FailureClasses, classifyExecutionError(err.Error()))
			return row
		}
		row.RuntimeOK = true
		var out compactRead
		if err := json.Unmarshal(payload, &out); err != nil {
			row.Error = err.Error()
			row.FailureClasses = append(row.FailureClasses, "decode_error")
			return row
		}
		row.ActualURL = strings.TrimSpace(out.URL)
		row.SelectedURLPass = sameCanonicalURL(row.ActualURL, item.ExpectedURL)
		row.SummaryPresent = strings.TrimSpace(out.Summary) != ""
		row.ChunkCount = len(out.Chunks)
		row.PacketBytes = len(raw)
		row.LatencyMS = out.CostReport.LatencyMS
		row.UncertaintyLevel = uncertaintyLevelFromMap(out.Uncertainty)
		row.ProofRef, row.ProofUsable = verifyProof(binaryPath, out.Chunks, item.MustExposeProof)
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

func runNeedleJSON(binaryPath string, args ...string) ([]byte, []byte, error) {
	cmd := exec.Command(binaryPath, args...)
	raw, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, raw, fmt.Errorf("%s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, raw, err
	}
	return raw, raw, nil
}

func verifyProof(binaryPath string, chunks []compactChunk, required bool) (string, bool) {
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
	payload, _, err := runNeedleJSON(binaryPath, "proof", proofRef, "--json")
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
	failureCounts := make(map[string]int)
	familyAgg := make(map[string][]caseResult)
	var runtimeOKCount, qualityPassCount, passCount, urlPassCount, proofPassCount int
	var latencyTotal, bytesTotal int64
	for _, row := range results {
		familyAgg[row.Family] = append(familyAgg[row.Family], row)
		if row.RuntimeOK {
			runtimeOKCount++
		}
		if row.QualityPass {
			qualityPassCount++
		}
		if row.Pass {
			passCount++
		}
		if row.SelectedURLPass {
			urlPassCount++
		}
		if row.ProofUsable {
			proofPassCount++
		}
		latencyTotal += row.LatencyMS
		bytesTotal += int64(row.PacketBytes)
		for _, failure := range row.FailureClasses {
			failureCounts[failure]++
		}
	}
	families := make([]familySummary, 0, len(familyAgg))
	for family, rows := range familyAgg {
		var fRuntimeOK, fQualityPass, fPass, fURLPass, fProofPass int
		var fLatency, fBytes int64
		for _, row := range rows {
			if row.RuntimeOK {
				fRuntimeOK++
			}
			if row.QualityPass {
				fQualityPass++
			}
			if row.Pass {
				fPass++
			}
			if row.SelectedURLPass {
				fURLPass++
			}
			if row.ProofUsable {
				fProofPass++
			}
			fLatency += row.LatencyMS
			fBytes += int64(row.PacketBytes)
		}
		families = append(families, familySummary{
			Family:              family,
			CaseCount:           len(rows),
			RuntimeSuccessRate:  evalutil.Ratio(fRuntimeOK, len(rows)),
			QualityPassRate:     evalutil.Ratio(fQualityPass, len(rows)),
			PassRate:            evalutil.Ratio(fPass, len(rows)),
			SelectedURLPassRate: evalutil.Ratio(fURLPass, len(rows)),
			ProofUsabilityRate:  evalutil.Ratio(fProofPass, len(rows)),
			AvgLatencyMS:        averageInt64(fLatency, len(rows)),
			AvgPacketBytes:      averageInt64(fBytes, len(rows)),
		})
	}
	return summary{
		CaseCount:           len(results),
		RuntimeSuccessRate:  evalutil.Ratio(runtimeOKCount, len(results)),
		QualityPassRate:     evalutil.Ratio(qualityPassCount, len(results)),
		PassRate:            evalutil.Ratio(passCount, len(results)),
		SelectedURLPassRate: evalutil.Ratio(urlPassCount, len(results)),
		ProofUsabilityRate:  evalutil.Ratio(proofPassCount, len(results)),
		AvgLatencyMS:        averageInt64(latencyTotal, len(results)),
		AvgPacketBytes:      averageInt64(bytesTotal, len(results)),
		FailureClassCounts:  failureCounts,
		FamilyBreakdown:     families,
	}
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
