package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/evalutil"
)

type corpus struct {
	Version string       `json:"version"`
	Cases   []seededCase `json:"cases"`
}

type seededCase struct {
	ID             string `json:"id"`
	Family         string `json:"family"`
	Language       string `json:"language"`
	SeedURL        string `json:"seed_url"`
	TaskType       string `json:"task_type"`
	Goal           string `json:"goal,omitempty"`
	ExpectedURL    string `json:"expected_url"`
	ExpectedDomain string `json:"expected_domain"`
	Notes          string `json:"notes,omitempty"`
}

type compactQuery struct {
	Kind        string `json:"kind"`
	Goal        string `json:"goal"`
	SelectedURL string `json:"selected_url"`
	Provider    string `json:"provider,omitempty"`
	Summary     string `json:"summary,omitempty"`
	TraceID     string `json:"trace_id,omitempty"`
	Chunks      []struct {
		ProofRef string `json:"proof_ref,omitempty"`
	} `json:"chunks"`
	CostReport struct {
		LatencyMS int64 `json:"latency_ms"`
	} `json:"cost_report"`
}

type compactMemoryStats struct {
	Stats struct {
		DocumentCount  int    `json:"document_count"`
		EdgeCount      int    `json:"edge_count"`
		EmbeddingCount int    `json:"embedding_count"`
		DBPath         string `json:"db_path"`
	} `json:"stats"`
}

type stageResult struct {
	RuntimeOK      bool     `json:"runtime_ok"`
	SelectedURL    string   `json:"selected_url,omitempty"`
	Provider       string   `json:"provider,omitempty"`
	SelectedPass   bool     `json:"selected_pass"`
	SummaryPresent bool     `json:"summary_present"`
	ProofUsable    bool     `json:"proof_usable"`
	PacketBytes    int      `json:"packet_bytes"`
	LatencyMS      int64    `json:"latency_ms"`
	MemoryDocs     int      `json:"memory_docs"`
	FailureClasses []string `json:"failure_classes,omitempty"`
	Error          string   `json:"error,omitempty"`
}

type caseResult struct {
	ID          string      `json:"id"`
	Family      string      `json:"family"`
	Language    string      `json:"language"`
	Goal        string      `json:"goal"`
	ExpectedURL string      `json:"expected_url"`
	Cold        stageResult `json:"cold"`
	Warm        stageResult `json:"warm"`
	Improved    bool        `json:"improved"`
}

type summary struct {
	CaseCount              int     `json:"case_count"`
	ColdRuntimeSuccessRate float64 `json:"cold_runtime_success_rate"`
	WarmRuntimeSuccessRate float64 `json:"warm_runtime_success_rate"`
	ColdSelectedPassRate   float64 `json:"cold_selected_pass_rate"`
	WarmSelectedPassRate   float64 `json:"warm_selected_pass_rate"`
	WarmMemoryProviderRate float64 `json:"warm_memory_provider_rate"`
	ImprovementRate        float64 `json:"improvement_rate"`
	AvgColdLatencyMS       int64   `json:"avg_cold_latency_ms"`
	AvgWarmLatencyMS       int64   `json:"avg_warm_latency_ms"`
	AvgColdPacketBytes     int64   `json:"avg_cold_packet_bytes"`
	AvgWarmPacketBytes     int64   `json:"avg_warm_packet_bytes"`
	AvgWarmMemoryDocuments float64 `json:"avg_warm_memory_documents"`
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
	flag.StringVar(&outPath, "out", "improvements/discovery-memory-benchmark-latest.json", "output report path")
	flag.StringVar(&casesPath, "cases", "benchmarks/corpora/seeded-corpus-v1.json", "seeded corpus path")
	flag.Parse()

	c, err := loadCorpus(casesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load corpus: %v\n", err)
		os.Exit(1)
	}
	cases := benchmarkCases(c.Cases)
	if len(cases) == 0 {
		fmt.Fprintln(os.Stderr, "no benchmarkable discovery-memory cases")
		os.Exit(1)
	}

	embeddingServer := newEmbeddingServer()
	defer embeddingServer.Close()

	binaryPath, cleanup, err := buildNeedleBinary()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build needle: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	results := make([]caseResult, 0, len(cases))
	for i, item := range cases {
		fmt.Printf("[memory-bench] %s case %d/%d start id=%s family=%s\n", time.Now().Format("15:04:05"), i+1, len(cases), item.ID, item.Family)
		row := runCase(binaryPath, embeddingServer.URL, item)
		results = append(results, row)
		fmt.Printf("[memory-bench] %s case %d/%d done id=%s cold_pass=%t warm_pass=%t warm_provider=%s\n", time.Now().Format("15:04:05"), i+1, len(cases), item.ID, row.Cold.SelectedPass, row.Warm.SelectedPass, row.Warm.Provider)
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
	fmt.Printf("Discovery Memory benchmark written to %s\n", outPath)
}

func benchmarkCases(all []seededCase) []seededCase {
	out := make([]seededCase, 0, len(all))
	for _, item := range all {
		if strings.TrimSpace(item.Goal) == "" || strings.TrimSpace(item.ExpectedURL) == "" {
			continue
		}
		switch item.TaskType {
		case "same_site_query_routing", "read_page_understanding", "read_then_answer":
			out = append(out, item)
		}
	}
	return out
}

func runCase(binaryPath, embeddingURL string, item seededCase) caseResult {
	coldDir, _ := os.MkdirTemp("", "needlex-memory-cold-*")
	defer os.RemoveAll(coldDir)
	warmDir, _ := os.MkdirTemp("", "needlex-memory-warm-*")
	defer os.RemoveAll(warmDir)

	coldConfig := mustWriteConfig(coldDir, embeddingURL)
	warmConfig := mustWriteConfig(warmDir, embeddingURL)

	cold := runQueryStage(binaryPath, coldDir, coldConfig, item)
	_ = runWarmup(binaryPath, warmDir, warmConfig, item.ExpectedURL)
	warm := runQueryStage(binaryPath, warmDir, warmConfig, item)

	return caseResult{
		ID:          item.ID,
		Family:      item.Family,
		Language:    item.Language,
		Goal:        item.Goal,
		ExpectedURL: item.ExpectedURL,
		Cold:        cold,
		Warm:        warm,
		Improved:    !cold.SelectedPass && warm.SelectedPass,
	}
}

func runWarmup(binaryPath, workDir, configPath, pageURL string) error {
	cmd := exec.Command(binaryPath, "read", pageURL, "--json", "--config", configPath)
	cmd.Dir = workDir
	_, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("%s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return err
	}
	return nil
}

func runQueryStage(binaryPath, workDir, configPath string, item seededCase) stageResult {
	out, err := runNeedle(binaryPath, workDir, "query", "--goal", item.Goal, "--json", "--config", configPath)
	if err != nil {
		return stageResult{Error: err.Error(), FailureClasses: []string{classifyExecError(err.Error())}}
	}
	var payload compactQuery
	if err := json.Unmarshal(out, &payload); err != nil {
		return stageResult{Error: err.Error(), FailureClasses: []string{"decode_error"}}
	}
	proofUsable := false
	if len(payload.Chunks) > 0 && strings.TrimSpace(payload.Chunks[0].ProofRef) != "" {
		proofUsable = true
	}
	stats := loadMemoryStats(binaryPath, workDir, configPath)
	selectedPass := sameCanonicalURL(payload.SelectedURL, item.ExpectedURL)
	failures := []string{}
	if !selectedPass {
		failures = append(failures, "wrong_selected_url")
	}
	if strings.TrimSpace(payload.Summary) == "" {
		failures = append(failures, "missing_summary")
	}
	return stageResult{
		RuntimeOK:      true,
		SelectedURL:    strings.TrimSpace(payload.SelectedURL),
		Provider:       strings.TrimSpace(payload.Provider),
		SelectedPass:   selectedPass,
		SummaryPresent: strings.TrimSpace(payload.Summary) != "",
		ProofUsable:    proofUsable,
		PacketBytes:    len(out),
		LatencyMS:      payload.CostReport.LatencyMS,
		MemoryDocs:     stats.Stats.DocumentCount,
		FailureClasses: failures,
	}
}

func loadMemoryStats(binaryPath, workDir, configPath string) compactMemoryStats {
	out, err := runNeedle(binaryPath, workDir, "memory", "stats", "--json", "--config", configPath)
	if err != nil {
		return compactMemoryStats{}
	}
	var stats compactMemoryStats
	_ = json.Unmarshal(out, &stats)
	return stats
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
	tempDir, err := os.MkdirTemp("", "needlex-memory-benchmark-*")
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

func mustWriteConfig(workDir, embeddingURL string) string {
	cfg := config.Defaults()
	cfg.Memory.Enabled = true
	cfg.Semantic.Enabled = true
	cfg.Semantic.Backend = "openai-embeddings"
	cfg.Semantic.BaseURL = embeddingURL
	cfg.Semantic.Model = "memory-benchmark-embed"
	cfg.Memory.EmbeddingBackend = cfg.Semantic.Backend
	cfg.Memory.EmbeddingModel = cfg.Semantic.Model
	path := filepath.Join(workDir, "needlex-memory-benchmark.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		panic(err)
	}
	return path
}

func runNeedle(binaryPath, workDir string, args ...string) ([]byte, error) {
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = workDir
	raw, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("%s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, err
	}
	return raw, nil
}

func sameCanonicalURL(actual, expected string) bool {
	return canonicalizeURL(actual) == canonicalizeURL(expected)
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

func classifyExecError(msg string) string {
	lowered := strings.ToLower(strings.TrimSpace(msg))
	switch {
	case strings.Contains(lowered, "context deadline exceeded"):
		return "network_timeout"
	case strings.Contains(lowered, "tls"):
		return "network_tls_error"
	case strings.Contains(lowered, "connect"):
		return "network_connect_error"
	default:
		return "runtime_error"
	}
}

func summarize(results []caseResult) summary {
	if len(results) == 0 {
		return summary{}
	}
	var coldRuntime, warmRuntime, coldPass, warmPass, warmMemoryProvider, improved int
	var coldLatency, warmLatency, coldBytes, warmBytes, warmDocs int64
	for _, r := range results {
		if r.Cold.RuntimeOK {
			coldRuntime++
		}
		if r.Warm.RuntimeOK {
			warmRuntime++
		}
		if r.Cold.SelectedPass {
			coldPass++
		}
		if r.Warm.SelectedPass {
			warmPass++
		}
		if r.Warm.Provider == "discovery_memory" {
			warmMemoryProvider++
		}
		if r.Improved {
			improved++
		}
		coldLatency += r.Cold.LatencyMS
		warmLatency += r.Warm.LatencyMS
		coldBytes += int64(r.Cold.PacketBytes)
		warmBytes += int64(r.Warm.PacketBytes)
		warmDocs += int64(r.Warm.MemoryDocs)
	}
	n := float64(len(results))
	return summary{
		CaseCount:              len(results),
		ColdRuntimeSuccessRate: float64(coldRuntime) / n,
		WarmRuntimeSuccessRate: float64(warmRuntime) / n,
		ColdSelectedPassRate:   float64(coldPass) / n,
		WarmSelectedPassRate:   float64(warmPass) / n,
		WarmMemoryProviderRate: float64(warmMemoryProvider) / n,
		ImprovementRate:        float64(improved) / n,
		AvgColdLatencyMS:       coldLatency / int64(len(results)),
		AvgWarmLatencyMS:       warmLatency / int64(len(results)),
		AvgColdPacketBytes:     coldBytes / int64(len(results)),
		AvgWarmPacketBytes:     warmBytes / int64(len(results)),
		AvgWarmMemoryDocuments: float64(warmDocs) / n,
	}
}

func newEmbeddingServer() *httptest.Server {
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
		inputs := embeddingInputs(payload.Input)
		data := make([]map[string]any, 0, len(inputs))
		for i, input := range inputs {
			data = append(data, map[string]any{
				"object":    "embedding",
				"index":     i,
				"embedding": embeddingVector(input),
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": data, "model": "memory-benchmark-embed"})
	}))
}

func embeddingInputs(raw any) []string {
	switch typed := raw.(type) {
	case string:
		return []string{typed}
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if value, ok := item.(string); ok {
				out = append(out, value)
			}
		}
		return out
	default:
		return nil
	}
}

func embeddingVector(text string) []float64 {
	const dims = 64
	vector := make([]float64, dims)
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	for _, token := range fields {
		if token == "" {
			continue
		}
		h := fnv.New32a()
		_, _ = h.Write([]byte(token))
		vector[int(h.Sum32()%dims)] += 1
	}
	return vector
}
