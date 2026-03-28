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
	"path/filepath"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
	coreservice "github.com/josepavese/needlex/internal/core/service"
)

type evalCase struct {
	Name          string   `json:"name"`
	URL           string   `json:"url"`
	TimeoutMS     int64    `json:"timeout_ms"`
	ExpectedTerms []string `json:"expected_terms"`
}

type caseResult struct {
	Name               string  `json:"name"`
	URL                string  `json:"url"`
	Success            bool    `json:"success"`
	Error              string  `json:"error,omitempty"`
	Title              string  `json:"title,omitempty"`
	LatencyMS          int64   `json:"latency_ms"`
	ChunkCount         int     `json:"chunk_count"`
	KeywordCoverage    float64 `json:"keyword_coverage"`
	NoiseHits          int     `json:"noise_hits"`
	ExternalCoverage   float64 `json:"external_coverage,omitempty"`
	ExternalNoiseHits  int     `json:"external_noise_hits,omitempty"`
	ExternalTextLength int     `json:"external_text_length,omitempty"`
}

type report struct {
	GeneratedAtUTC   string       `json:"generated_at_utc"`
	ExternalBaseline string       `json:"external_baseline,omitempty"`
	Results          []caseResult `json:"results"`
	Regressions      []string     `json:"regressions,omitempty"`
}

func main() {
	var (
		outPath        string
		baselinePath   string
		updateBaseline bool
	)
	flag.StringVar(&outPath, "out", "improvements/live-read-latest.json", "output report path")
	flag.StringVar(&baselinePath, "baseline", "improvements/live-read-baseline.json", "baseline report path")
	flag.BoolVar(&updateBaseline, "update-baseline", false, "overwrite baseline with latest report")
	flag.Parse()

	externalCommand := strings.TrimSpace(os.Getenv("NEEDLEX_EXTERNAL_BASELINE_CMD"))
	cases := defaultCases()
	results := make([]caseResult, 0, len(cases))
	for _, item := range cases {
		result := runCase(item, externalCommand)
		results = append(results, result)
	}

	rep := report{
		GeneratedAtUTC:   time.Now().UTC().Format(time.RFC3339),
		ExternalBaseline: externalCommand,
		Results:          results,
	}

	if prior, err := loadReport(baselinePath); err == nil {
		rep.Regressions = compareReports(prior, rep)
	}

	if err := writeReport(outPath, rep); err != nil {
		fmt.Fprintf(os.Stderr, "write report: %v\n", err)
		os.Exit(1)
	}
	if updateBaseline {
		if err := writeReport(baselinePath, rep); err != nil {
			fmt.Fprintf(os.Stderr, "write baseline: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Printf("Live read evaluation written to %s\n", outPath)
	if updateBaseline {
		fmt.Printf("Baseline updated at %s\n", baselinePath)
	}
	for _, result := range rep.Results {
		status := "ok"
		if !result.Success {
			status = "fail"
		}
		fmt.Printf("- %s: %s chunks=%d coverage=%.2f noise=%d latency=%dms\n",
			result.Name, status, result.ChunkCount, result.KeywordCoverage, result.NoiseHits, result.LatencyMS)
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
			Name:          "carratellire",
			URL:           "https://carratellire.com/",
			TimeoutMS:     12000,
			ExpectedTerms: []string{"carratelli", "immobiliare", "lusso"},
		},
		{
			Name:          "cnf",
			URL:           "https://www.cnfsrl.it/",
			TimeoutMS:     12000,
			ExpectedTerms: []string{"intralogistica", "magazzino", "carrelli"},
		},
		{
			Name:          "halfpocket",
			URL:           "https://halfpocket.net/",
			TimeoutMS:     12000,
			ExpectedTerms: []string{"siti web", "sviluppo", "marketing"},
		},
	}
}

func runCase(item evalCase, externalCommand string) caseResult {
	result := caseResult{
		Name: item.Name,
		URL:  item.URL,
	}

	cfg := config.Defaults()
	cfg.Runtime.TimeoutMS = item.TimeoutMS
	client := &http.Client{
		Timeout: time.Duration(item.TimeoutMS) * time.Millisecond,
	}
	svc, err := coreservice.New(cfg, client)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	readResp, err := svc.Read(context.Background(), coreservice.ReadRequest{
		URL:        item.URL,
		Profile:    core.ProfileStandard,
		RenderHint: true,
	})
	if err != nil {
		result.Error = err.Error()
		return result
	}

	result.Success = true
	result.Title = readResp.Document.Title
	result.LatencyMS = readResp.ResultPack.CostReport.LatencyMS
	result.ChunkCount = len(readResp.ResultPack.Chunks)

	text := mergeChunkText(readResp.ResultPack.Chunks)
	result.KeywordCoverage = keywordCoverage(text, item.ExpectedTerms)
	result.NoiseHits = noiseHits(text)

	if externalCommand != "" {
		if externalText, extErr := runExternalBaseline(item.URL, item.TimeoutMS, externalCommand); extErr == nil {
			result.ExternalTextLength = len(externalText)
			result.ExternalCoverage = keywordCoverage(externalText, item.ExpectedTerms)
			result.ExternalNoiseHits = noiseHits(externalText)
		}
	}

	return result
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

func keywordCoverage(text string, terms []string) float64 {
	if len(terms) == 0 {
		return 1
	}
	matches := 0
	for _, term := range terms {
		term = strings.ToLower(strings.TrimSpace(term))
		if term == "" {
			continue
		}
		if strings.Contains(text, term) {
			matches++
		}
	}
	return float64(matches) / float64(len(terms))
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

func writeReport(path string, rep report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
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
		if before.Success && !now.Success {
			regressions = append(regressions, fmt.Sprintf("%s regressed: previously successful, now failed (%s)", now.Name, now.Error))
		}
		if now.Success && before.Success {
			if now.KeywordCoverage+0.15 < before.KeywordCoverage {
				regressions = append(regressions, fmt.Sprintf("%s regressed keyword coverage %.2f -> %.2f", now.Name, before.KeywordCoverage, now.KeywordCoverage))
			}
			if now.NoiseHits > before.NoiseHits+1 {
				regressions = append(regressions, fmt.Sprintf("%s regressed noise hits %d -> %d", now.Name, before.NoiseHits, now.NoiseHits))
			}
		}
	}
	return regressions
}
