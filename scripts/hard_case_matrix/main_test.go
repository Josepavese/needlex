package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
	coreservice "github.com/josepavese/needlex/internal/core/service"
)

type corpus struct {
	Version          string            `json:"version"`
	FamilyThresholds []familyThreshold `json:"family_thresholds,omitempty"`
	Acceptance       acceptancePolicy  `json:"acceptance"`
	Cases            []matrixCase      `json:"cases"`
}

type familyThreshold struct {
	Family           string  `json:"family"`
	MaxAvgLossiness  float64 `json:"max_avg_lossiness"`
	MaxLossinessRisk float64 `json:"max_lossiness_risk"`
}

type matrixCase struct {
	Name               string   `json:"name"`
	Family             string   `json:"family"`
	Objective          string   `json:"objective"`
	ObjectiveTerms     []string `json:"objective_terms,omitempty"`
	Profile            string   `json:"profile"`
	CompareForceLane   int      `json:"compare_force_lane"`
	MinCompareFidelity float64  `json:"min_compare_fidelity"`
	ExpectedTerms      []string `json:"expected_terms"`
	HTMLInline         string   `json:"html_inline,omitempty"`
	FixturePath        string   `json:"fixture_path,omitempty"`
}

type metrics struct {
	MaxLane        int     `json:"max_lane"`
	LanePath       []int   `json:"lane_path"`
	Invocations    int     `json:"invocations"`
	Fidelity       float64 `json:"fidelity"`
	SignalDensity  float64 `json:"signal_density"`
	ObjectiveScore float64 `json:"objective_score"`
	Tokens         int     `json:"tokens"`
	PackedText     string  `json:"packed_text"`
}

type matrixRow struct {
	Name             string   `json:"name"`
	Family           string   `json:"family"`
	Objective        string   `json:"objective"`
	Profile          string   `json:"profile"`
	ExpectedTerms    []string `json:"expected_terms"`
	CompareForceLane int      `json:"compare_force_lane"`
	Baseline         metrics  `json:"baseline"`
	Compare          metrics  `json:"compare"`
	LossinessRisk    float64  `json:"lossiness_risk"`
	LossinessLevel   string   `json:"lossiness_level"`
	Pass             bool     `json:"pass"`
	Reasons          []string `json:"reasons,omitempty"`
}

type familySummary struct {
	Family               string  `json:"family"`
	CaseCount            int     `json:"case_count"`
	PassCount            int     `json:"pass_count"`
	AvgBaselineSignal    float64 `json:"avg_baseline_signal"`
	AvgCompareSignal     float64 `json:"avg_compare_signal"`
	AvgBaselineObjective float64 `json:"avg_baseline_objective"`
	AvgCompareObjective  float64 `json:"avg_compare_objective"`
	AvgLossinessRisk     float64 `json:"avg_lossiness_risk"`
	MaxLossinessRisk     float64 `json:"max_lossiness_risk"`
}

type report struct {
	GeneratedAtUTC string           `json:"generated_at_utc"`
	CorpusVersion  string           `json:"corpus_version"`
	Rows           []matrixRow      `json:"rows"`
	FamilySummary  []familySummary  `json:"family_summary"`
	Acceptance     acceptanceResult `json:"acceptance"`
	Alerts         []string         `json:"alerts,omitempty"`
	Regressions    []string         `json:"regressions,omitempty"`
}

type acceptancePolicy struct {
	MinPassRate          float64            `json:"min_pass_rate"`
	MinLaneLiftRate      float64            `json:"min_lane_lift_rate"`
	MinObjectiveLiftAvg  float64            `json:"min_objective_lift_avg"`
	MaxMediumOrHighRisk  float64            `json:"max_medium_or_high_risk_rate"`
	RequiredFamilies     []string           `json:"required_families,omitempty"`
	FailureClassMap      []failureClassRule `json:"failure_class_map,omitempty"`
	AllowUnclassified    bool               `json:"allow_unclassified"`
	RequireHigherLaneFor []string           `json:"require_higher_lane_for,omitempty"`
}

type acceptanceResult struct {
	Passed               bool                 `json:"passed"`
	PassRate             float64              `json:"pass_rate"`
	LaneLiftRate         float64              `json:"lane_lift_rate"`
	ObjectiveLiftAvg     float64              `json:"objective_lift_avg"`
	MediumOrHighRiskRate float64              `json:"medium_or_high_risk_rate"`
	Failures             []string             `json:"failures,omitempty"`
	FailureClassCounts   []failureClassCount  `json:"failure_class_counts,omitempty"`
	Thresholds           acceptancePolicyView `json:"thresholds"`
}

type acceptancePolicyView struct {
	MinPassRate         float64 `json:"min_pass_rate"`
	MinLaneLiftRate     float64 `json:"min_lane_lift_rate"`
	MinObjectiveLiftAvg float64 `json:"min_objective_lift_avg"`
	MaxMediumOrHighRisk float64 `json:"max_medium_or_high_risk_rate"`
}

type failureClassRule struct {
	ID              string   `json:"id"`
	Description     string   `json:"description"`
	IntegrationGate string   `json:"integration_gate"`
	MatchAny        []string `json:"match_any"`
}

type failureClassCount struct {
	ID              string `json:"id"`
	Description     string `json:"description"`
	IntegrationGate string `json:"integration_gate"`
	Count           int    `json:"count"`
}

func TestGenerateReportFromCorpus(t *testing.T) {
	withRepoRoot(t)
	rep, err := generateReport("testdata/benchmark/hard-case-corpus-v2.json")
	if err != nil {
		t.Fatalf("generate report: %v", err)
	}
	if rep.CorpusVersion != "hard-case-corpus-v2" {
		t.Fatalf("unexpected corpus version %q", rep.CorpusVersion)
	}
	if len(rep.Rows) != 6 {
		t.Fatalf("expected 6 rows, got %d", len(rep.Rows))
	}
	if len(rep.FamilySummary) == 0 {
		t.Fatal("expected family summary")
	}
	for _, row := range rep.Rows {
		if !row.Pass {
			t.Fatalf("expected passing row for %s, got reasons %#v", row.Name, row.Reasons)
		}
		if row.Compare.PackedText == "" {
			t.Fatalf("expected packed text for %s", row.Name)
		}
	}
	if !rep.Acceptance.Passed {
		t.Fatalf("expected acceptance passed, got failures %#v", rep.Acceptance.Failures)
	}
}

func TestExportHardCaseMatrix(t *testing.T) {
	outPath := strings.TrimSpace(os.Getenv("NEEDLEX_HARD_CASE_MATRIX_OUT"))
	if outPath == "" {
		t.Skip("set NEEDLEX_HARD_CASE_MATRIX_OUT to export matrix report")
	}
	withRepoRoot(t)

	corpusPath := getenvOr("NEEDLEX_HARD_CASE_MATRIX_CORPUS", "testdata/benchmark/hard-case-corpus-v2.json")
	baselinePath := getenvOr("NEEDLEX_HARD_CASE_MATRIX_BASELINE", "improvements/hard-case-matrix-baseline.json")
	updateBaseline := strings.EqualFold(strings.TrimSpace(os.Getenv("NEEDLEX_HARD_CASE_MATRIX_UPDATE_BASELINE")), "1") || strings.EqualFold(strings.TrimSpace(os.Getenv("NEEDLEX_HARD_CASE_MATRIX_UPDATE_BASELINE")), "true")

	rep, err := generateReport(corpusPath)
	if err != nil {
		t.Fatalf("generate report: %v", err)
	}
	if prior, err := loadReport(baselinePath); err == nil {
		rep.Regressions = compareReports(prior, rep)
	}
	for _, alert := range rep.Alerts {
		t.Logf("alert: %s", alert)
	}
	if err := writeReport(outPath, rep); err != nil {
		t.Fatalf("write report: %v", err)
	}
	if updateBaseline {
		if err := writeReport(baselinePath, rep); err != nil {
			t.Fatalf("write baseline: %v", err)
		}
	}
	for _, row := range rep.Rows {
		t.Logf("%s family=%s baseline(lane=%d signal=%.4f objective=%.4f tokens=%d) compare(lane=%d signal=%.4f objective=%.4f tokens=%d) risk=%.3f/%s pass=%v",
			row.Name,
			row.Family,
			row.Baseline.MaxLane,
			row.Baseline.SignalDensity,
			row.Baseline.ObjectiveScore,
			row.Baseline.Tokens,
			row.Compare.MaxLane,
			row.Compare.SignalDensity,
			row.Compare.ObjectiveScore,
			row.Compare.Tokens,
			row.LossinessRisk,
			row.LossinessLevel,
			row.Pass,
		)
	}
	for _, family := range rep.FamilySummary {
		t.Logf("family=%s cases=%d pass=%d avg_compare_signal=%.4f avg_compare_objective=%.4f avg_lossiness=%.3f",
			family.Family,
			family.CaseCount,
			family.PassCount,
			family.AvgCompareSignal,
			family.AvgCompareObjective,
			family.AvgLossinessRisk,
		)
	}
	if len(rep.Alerts) > 0 {
		t.Fatalf("hard-case matrix family alerts detected: %d", len(rep.Alerts))
	}
	if len(rep.Regressions) > 0 {
		for _, issue := range rep.Regressions {
			t.Logf("regression: %s", issue)
		}
		t.Fatalf("hard-case matrix regressions detected: %d", len(rep.Regressions))
	}
	if !rep.Acceptance.Passed {
		for _, issue := range rep.Acceptance.Failures {
			t.Logf("acceptance failure: %s", issue)
		}
		t.Fatalf("hard-case matrix acceptance failed")
	}
}

func withRepoRoot(t *testing.T) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(filepath.Join("..", "..")); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
}

func getenvOr(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func generateReport(corpusPath string) (report, error) {
	data, err := os.ReadFile(corpusPath)
	if err != nil {
		return report{}, err
	}
	var c corpus
	if err := json.Unmarshal(data, &c); err != nil {
		return report{}, err
	}
	rows := make([]matrixRow, 0, len(c.Cases))
	regressions := make([]string, 0)
	for _, item := range c.Cases {
		row, err := runCase(item)
		if err != nil {
			return report{}, fmt.Errorf("%s: %w", item.Name, err)
		}
		rows = append(rows, row)
		if !row.Pass {
			regressions = append(regressions, row.Name+": "+strings.Join(row.Reasons, "; "))
		}
	}
	summary := buildFamilySummary(rows)
	acceptance := evaluateAcceptance(rows, summary, c.Acceptance)
	return report{
		GeneratedAtUTC: time.Now().UTC().Format(time.RFC3339),
		CorpusVersion:  c.Version,
		Rows:           rows,
		FamilySummary:  summary,
		Acceptance:     acceptance,
		Alerts:         evaluateFamilyThresholds(summary, c.FamilyThresholds),
		Regressions:    regressions,
	}, nil
}

func runCase(item matrixCase) (matrixRow, error) {
	html, err := loadCaseHTML(item)
	if err != nil {
		return matrixRow{}, err
	}
	baseline, err := runReadMetrics(html, item.ExpectedTerms, item.ObjectiveTerms, coreservice.ReadRequest{Objective: item.Objective, Profile: item.Profile})
	if err != nil {
		return matrixRow{}, err
	}
	compare, err := runReadMetrics(html, item.ExpectedTerms, item.ObjectiveTerms, coreservice.ReadRequest{Objective: item.Objective, Profile: item.Profile, ForceLane: item.CompareForceLane})
	if err != nil {
		return matrixRow{}, err
	}
	row := matrixRow{
		Name:             item.Name,
		Family:           item.Family,
		Objective:        item.Objective,
		Profile:          item.Profile,
		ExpectedTerms:    item.ExpectedTerms,
		CompareForceLane: item.CompareForceLane,
		Baseline:         baseline,
		Compare:          compare,
	}
	row.LossinessRisk = lossinessRisk(row.Baseline, row.Compare)
	row.LossinessLevel = lossinessLevel(row.LossinessRisk)
	row.Pass, row.Reasons = evaluateRow(item, row)
	return row, nil
}

func loadCaseHTML(item matrixCase) (string, error) {
	if strings.TrimSpace(item.HTMLInline) != "" {
		return item.HTMLInline, nil
	}
	if strings.TrimSpace(item.FixturePath) == "" {
		return "", errors.New("missing html_inline or fixture_path")
	}
	data, err := os.ReadFile(item.FixturePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func runReadMetrics(html string, expected []string, objectiveTerms []string, req coreservice.ReadRequest) (metrics, error) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, html)
	}))
	defer server.Close()

	svc, err := coreservice.New(config.Defaults(), server.Client())
	if err != nil {
		return metrics{}, err
	}
	svcReq := req
	svcReq.URL = server.URL
	resp, err := svc.Read(context.Background(), svcReq)
	if err != nil {
		return metrics{}, err
	}
	packed := mergeChunkText(resp.ResultPack.Chunks)
	return metrics{
		MaxLane:        maxLane(resp.ResultPack.CostReport.LanePath),
		LanePath:       append([]int(nil), resp.ResultPack.CostReport.LanePath...),
		Invocations:    len(resp.ProofRecords[0].Proof.ModelInvocations),
		Fidelity:       fidelityScore(packed, expected),
		SignalDensity:  signalDensity(packed, expected),
		ObjectiveScore: objectiveSignalDensity(packed, svcReq.Objective, objectiveTerms),
		Tokens:         tokenCount(packed),
		PackedText:     packed,
	}, nil
}

func evaluateRow(item matrixCase, row matrixRow) (bool, []string) {
	reasons := make([]string, 0)
	if row.Compare.MaxLane < item.CompareForceLane {
		reasons = append(reasons, fmt.Sprintf("compare max lane %d < forced lane %d", row.Compare.MaxLane, item.CompareForceLane))
	}
	if row.Compare.Invocations <= row.Baseline.Invocations {
		reasons = append(reasons, "compare path did not increase invocation depth")
	}
	if row.Compare.SignalDensity < row.Baseline.SignalDensity {
		reasons = append(reasons, fmt.Sprintf("signal density regressed %.4f -> %.4f", row.Baseline.SignalDensity, row.Compare.SignalDensity))
	}
	if row.Compare.ObjectiveScore < row.Baseline.ObjectiveScore {
		reasons = append(reasons, fmt.Sprintf("objective score regressed %.4f -> %.4f", row.Baseline.ObjectiveScore, row.Compare.ObjectiveScore))
	}
	if row.Compare.Fidelity < item.MinCompareFidelity {
		reasons = append(reasons, fmt.Sprintf("compare fidelity %.2f < threshold %.2f", row.Compare.Fidelity, item.MinCompareFidelity))
	}
	if item.Profile == core.ProfileTiny && row.Compare.Tokens > row.Baseline.Tokens {
		reasons = append(reasons, fmt.Sprintf("tiny token budget regressed %d -> %d", row.Baseline.Tokens, row.Compare.Tokens))
	}
	return len(reasons) == 0, reasons
}

func lossinessRisk(baseline, compare metrics) float64 {
	fidelityLoss := baseline.Fidelity - compare.Fidelity
	if fidelityLoss < 0 {
		fidelityLoss = 0
	}
	tokenDropRatio := 0.0
	if baseline.Tokens > 0 && compare.Tokens < baseline.Tokens {
		tokenDropRatio = float64(baseline.Tokens-compare.Tokens) / float64(baseline.Tokens)
	}
	objectiveRegression := baseline.ObjectiveScore - compare.ObjectiveScore
	if objectiveRegression < 0 {
		objectiveRegression = 0
	}
	signalRegression := baseline.SignalDensity - compare.SignalDensity
	if signalRegression < 0 {
		signalRegression = 0
	}
	return fidelityLoss*0.7 + tokenDropRatio*0.3 + objectiveRegression + signalRegression
}

func lossinessLevel(risk float64) string {
	switch {
	case risk >= 0.45:
		return "high"
	case risk >= 0.20:
		return "medium"
	default:
		return "low"
	}
}

func buildFamilySummary(rows []matrixRow) []familySummary {
	type acc struct {
		caseCount            int
		passCount            int
		sumBaselineSignal    float64
		sumCompareSignal     float64
		sumBaselineObjective float64
		sumCompareObjective  float64
		sumLossinessRisk     float64
		maxLossinessRisk     float64
	}
	byFamily := map[string]*acc{}
	for _, row := range rows {
		name := row.Family
		if strings.TrimSpace(name) == "" {
			name = "uncategorized"
		}
		bucket := byFamily[name]
		if bucket == nil {
			bucket = &acc{}
			byFamily[name] = bucket
		}
		bucket.caseCount++
		if row.Pass {
			bucket.passCount++
		}
		bucket.sumBaselineSignal += row.Baseline.SignalDensity
		bucket.sumCompareSignal += row.Compare.SignalDensity
		bucket.sumBaselineObjective += row.Baseline.ObjectiveScore
		bucket.sumCompareObjective += row.Compare.ObjectiveScore
		bucket.sumLossinessRisk += row.LossinessRisk
		if row.LossinessRisk > bucket.maxLossinessRisk {
			bucket.maxLossinessRisk = row.LossinessRisk
		}
	}
	order := []string{"embedded", "forum", "tiny", "compaction", "uncategorized"}
	out := make([]familySummary, 0, len(byFamily))
	seen := map[string]struct{}{}
	appendFamily := func(name string) {
		bucket := byFamily[name]
		if bucket == nil {
			return
		}
		seen[name] = struct{}{}
		out = append(out, familySummary{
			Family:               name,
			CaseCount:            bucket.caseCount,
			PassCount:            bucket.passCount,
			AvgBaselineSignal:    bucket.sumBaselineSignal / float64(bucket.caseCount),
			AvgCompareSignal:     bucket.sumCompareSignal / float64(bucket.caseCount),
			AvgBaselineObjective: bucket.sumBaselineObjective / float64(bucket.caseCount),
			AvgCompareObjective:  bucket.sumCompareObjective / float64(bucket.caseCount),
			AvgLossinessRisk:     bucket.sumLossinessRisk / float64(bucket.caseCount),
			MaxLossinessRisk:     bucket.maxLossinessRisk,
		})
	}
	for _, name := range order {
		appendFamily(name)
	}
	for name := range byFamily {
		if _, ok := seen[name]; ok {
			continue
		}
		appendFamily(name)
	}
	return out
}

func evaluateFamilyThresholds(summary []familySummary, thresholds []familyThreshold) []string {
	if len(thresholds) == 0 {
		return nil
	}
	byFamily := make(map[string]familySummary, len(summary))
	for _, family := range summary {
		byFamily[family.Family] = family
	}
	alerts := make([]string, 0)
	for _, threshold := range thresholds {
		family, ok := byFamily[threshold.Family]
		if !ok {
			continue
		}
		if family.AvgLossinessRisk > threshold.MaxAvgLossiness {
			alerts = append(alerts, fmt.Sprintf("%s avg lossiness %.3f > %.3f", threshold.Family, family.AvgLossinessRisk, threshold.MaxAvgLossiness))
		}
		if family.MaxLossinessRisk > threshold.MaxLossinessRisk {
			alerts = append(alerts, fmt.Sprintf("%s max lossiness %.3f > %.3f", threshold.Family, family.MaxLossinessRisk, threshold.MaxLossinessRisk))
		}
	}
	return alerts
}

func mergeChunkText(chunks []core.Chunk) string {
	parts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		if strings.TrimSpace(chunk.Text) == "" {
			continue
		}
		parts = append(parts, chunk.Text)
	}
	return strings.Join(parts, "\n")
}

func fidelityScore(text string, expected []string) float64 {
	if len(expected) == 0 {
		return 1
	}
	matches := 0
	lower := strings.ToLower(text)
	for _, term := range expected {
		if strings.Contains(lower, strings.ToLower(strings.TrimSpace(term))) {
			matches++
		}
	}
	return float64(matches) / float64(len(expected))
}

func signalDensity(text string, expected []string) float64 {
	tokens := tokenCount(text)
	if tokens == 0 {
		return 0
	}
	matches := 0
	lower := strings.ToLower(text)
	for _, term := range expected {
		if strings.Contains(lower, strings.ToLower(strings.TrimSpace(term))) {
			matches++
		}
	}
	return float64(matches) / float64(tokens)
}

func objectiveSignalDensity(text, objective string, objectiveTerms []string) float64 {
	tokens := tokenCount(text)
	if tokens == 0 {
		return 0
	}
	matches := 0
	lower := strings.ToLower(text)
	terms := objectiveTerms
	if len(terms) == 0 {
		terms = uniqueTokens(objective)
	}
	for _, token := range terms {
		token = strings.ToLower(strings.TrimSpace(token))
		if token == "" {
			continue
		}
		if strings.Contains(lower, token) {
			matches++
		}
	}
	return float64(matches) / float64(tokens)
}

func uniqueTokens(value string) []string {
	seen := make(map[string]struct{})
	parts := strings.Fields(strings.ToLower(value))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		out = append(out, part)
	}
	return out
}

func tokenCount(text string) int {
	return len(strings.Fields(text))
}

func maxLane(path []int) int {
	max := 0
	for _, lane := range path {
		if lane > max {
			max = lane
		}
	}
	return max
}

func writeReport(path string, rep report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
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

func compareReports(prior, current report) []string {
	prev := make(map[string]matrixRow, len(prior.Rows))
	for _, row := range prior.Rows {
		prev[row.Name] = row
	}
	issues := append([]string(nil), current.Regressions...)
	for _, row := range current.Rows {
		before, ok := prev[row.Name]
		if !ok {
			continue
		}
		if row.Compare.SignalDensity+1e-9 < before.Compare.SignalDensity {
			issues = append(issues, fmt.Sprintf("%s signal density regressed %.4f -> %.4f", row.Name, before.Compare.SignalDensity, row.Compare.SignalDensity))
		}
		if row.Compare.ObjectiveScore+1e-9 < before.Compare.ObjectiveScore {
			issues = append(issues, fmt.Sprintf("%s objective score regressed %.4f -> %.4f", row.Name, before.Compare.ObjectiveScore, row.Compare.ObjectiveScore))
		}
		if row.Profile == core.ProfileTiny && row.Compare.Tokens > before.Compare.Tokens {
			issues = append(issues, fmt.Sprintf("%s tiny token count regressed %d -> %d", row.Name, before.Compare.Tokens, row.Compare.Tokens))
		}
	}
	return issues
}

func evaluateAcceptance(rows []matrixRow, summary []familySummary, policy acceptancePolicy) acceptanceResult {
	res := acceptanceResult{
		Passed: true,
		Thresholds: acceptancePolicyView{
			MinPassRate:         policy.MinPassRate,
			MinLaneLiftRate:     policy.MinLaneLiftRate,
			MinObjectiveLiftAvg: policy.MinObjectiveLiftAvg,
			MaxMediumOrHighRisk: policy.MaxMediumOrHighRisk,
		},
	}
	if len(rows) == 0 {
		res.Passed = false
		res.Failures = []string{"no benchmark rows produced"}
		return res
	}

	passedCases := 0
	liftCases := 0
	mediumOrHighRisk := 0
	objectiveLiftSum := 0.0
	for _, row := range rows {
		if row.Pass {
			passedCases++
		}
		if row.LossinessLevel == "medium" || row.LossinessLevel == "high" {
			mediumOrHighRisk++
		}
		objectiveLift := row.Compare.ObjectiveScore - row.Baseline.ObjectiveScore
		objectiveLiftSum += objectiveLift
		if row.Compare.SignalDensity > row.Baseline.SignalDensity || row.Compare.ObjectiveScore > row.Baseline.ObjectiveScore {
			liftCases++
		}
	}

	res.PassRate = float64(passedCases) / float64(len(rows))
	res.LaneLiftRate = float64(liftCases) / float64(len(rows))
	res.ObjectiveLiftAvg = objectiveLiftSum / float64(len(rows))
	res.MediumOrHighRiskRate = float64(mediumOrHighRisk) / float64(len(rows))

	if res.PassRate+1e-9 < policy.MinPassRate {
		res.Failures = append(res.Failures, fmt.Sprintf("pass_rate %.3f < %.3f", res.PassRate, policy.MinPassRate))
	}
	if res.LaneLiftRate+1e-9 < policy.MinLaneLiftRate {
		res.Failures = append(res.Failures, fmt.Sprintf("lane_lift_rate %.3f < %.3f", res.LaneLiftRate, policy.MinLaneLiftRate))
	}
	if res.ObjectiveLiftAvg+1e-9 < policy.MinObjectiveLiftAvg {
		res.Failures = append(res.Failures, fmt.Sprintf("objective_lift_avg %.4f < %.4f", res.ObjectiveLiftAvg, policy.MinObjectiveLiftAvg))
	}
	if res.MediumOrHighRiskRate-1e-9 > policy.MaxMediumOrHighRisk {
		res.Failures = append(res.Failures, fmt.Sprintf("medium_or_high_risk_rate %.3f > %.3f", res.MediumOrHighRiskRate, policy.MaxMediumOrHighRisk))
	}

	if len(policy.RequiredFamilies) > 0 {
		seen := map[string]struct{}{}
		for _, family := range summary {
			seen[family.Family] = struct{}{}
		}
		for _, required := range policy.RequiredFamilies {
			required = strings.TrimSpace(required)
			if required == "" {
				continue
			}
			if _, ok := seen[required]; !ok {
				res.Failures = append(res.Failures, "required family missing: "+required)
			}
		}
	}

	if len(policy.RequireHigherLaneFor) > 0 {
		required := map[string]struct{}{}
		for _, family := range policy.RequireHigherLaneFor {
			family = strings.TrimSpace(family)
			if family == "" {
				continue
			}
			required[family] = struct{}{}
		}
		for _, row := range rows {
			if _, ok := required[row.Family]; !ok {
				continue
			}
			if row.Compare.MaxLane <= row.Baseline.MaxLane {
				res.Failures = append(res.Failures, fmt.Sprintf("family %s did not show higher lane activation on case %s", row.Family, row.Name))
			}
		}
	}

	res.FailureClassCounts = classifyAcceptanceFailures(res.Failures, policy)
	if !policy.AllowUnclassified {
		for _, entry := range res.FailureClassCounts {
			if entry.ID == "FC00_UNCLASSIFIED" && entry.Count > 0 {
				res.Failures = append(res.Failures, "unclassified failures present")
				break
			}
		}
	}
	res.Passed = len(res.Failures) == 0
	return res
}

func classifyAcceptanceFailures(failures []string, policy acceptancePolicy) []failureClassCount {
	if len(failures) == 0 {
		return nil
	}
	counts := map[string]failureClassCount{}
	for _, rule := range policy.FailureClassMap {
		id := strings.TrimSpace(rule.ID)
		if id == "" {
			continue
		}
		counts[id] = failureClassCount{
			ID:              id,
			Description:     strings.TrimSpace(rule.Description),
			IntegrationGate: strings.TrimSpace(rule.IntegrationGate),
			Count:           0,
		}
	}

	for _, failure := range failures {
		lower := strings.ToLower(failure)
		matched := false
		for _, rule := range policy.FailureClassMap {
			id := strings.TrimSpace(rule.ID)
			if id == "" {
				continue
			}
			for _, token := range rule.MatchAny {
				token = strings.ToLower(strings.TrimSpace(token))
				if token == "" {
					continue
				}
				if strings.Contains(lower, token) {
					entry := counts[id]
					entry.Count++
					counts[id] = entry
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if matched {
			continue
		}
		entry := counts["FC00_UNCLASSIFIED"]
		if entry.ID == "" {
			entry = failureClassCount{
				ID:              "FC00_UNCLASSIFIED",
				Description:     "failure does not match any configured class",
				IntegrationGate: "block_slm_rollout",
				Count:           0,
			}
		}
		entry.Count++
		counts["FC00_UNCLASSIFIED"] = entry
	}

	out := make([]failureClassCount, 0, len(counts))
	for _, entry := range counts {
		if entry.Count == 0 {
			continue
		}
		out = append(out, entry)
	}
	return out
}
