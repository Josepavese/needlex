package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareReportsDetectsWebIRRegressions(t *testing.T) {
	previous := report{
		Results: []caseResult{
			{
				Name: "site",
				Baseline: readMetrics{
					Success:          true,
					ContextAlignment: 1.0,
					NoiseHits:        0,
					WebIRVersion:     "web_ir.v1",
					WebIRNodeCount:   10,
				},
			},
		},
	}
	current := report{
		Results: []caseResult{
			{
				Name: "site",
				Baseline: readMetrics{
					Success:          true,
					ContextAlignment: 1.0,
					NoiseHits:        0,
					WebIRVersion:     "web_ir.v2",
					WebIRNodeCount:   3,
				},
			},
		},
	}

	issues := compareReports(previous, current)
	joined := strings.Join(issues, "\n")
	if !strings.Contains(joined, "web_ir version") {
		t.Fatalf("expected web_ir version regression, got %v", issues)
	}
	if !strings.Contains(joined, "web_ir node count") {
		t.Fatalf("expected web_ir node regression, got %v", issues)
	}
}

func TestCompareReportsDoesNotFlagMissingHistoricalWebIRFields(t *testing.T) {
	previous := report{
		Results: []caseResult{
			{
				Name: "site",
				Baseline: readMetrics{
					Success:          true,
					ContextAlignment: 1.0,
					NoiseHits:        0,
				},
			},
		},
	}
	current := report{
		Results: []caseResult{
			{
				Name: "site",
				Baseline: readMetrics{
					Success:          true,
					ContextAlignment: 1.0,
					NoiseHits:        0,
					WebIRVersion:     "web_ir.v1",
					WebIRNodeCount:   7,
				},
			},
		},
	}

	issues := compareReports(previous, current)
	for _, issue := range issues {
		if strings.Contains(issue, "web_ir") {
			t.Fatalf("expected no web_ir regression for legacy baseline, got %v", issues)
		}
	}
}

func TestDefaultCasesUseStableTimeout(t *testing.T) {
	cases := defaultCases()
	if cases[0].TimeoutMS != 12000 {
		t.Fatalf("expected default timeout 12000, got %d", cases[0].TimeoutMS)
	}
}

func TestLoadCasesFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cases.json")
	payload, err := json.Marshal([]evalCase{{Name: "demo", URL: "https://example.com", TimeoutMS: 1000, Objective: "demo objective"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write cases: %v", err)
	}
	cases, err := loadCases(path)
	if err != nil {
		t.Fatalf("load cases: %v", err)
	}
	if len(cases) != 1 || cases[0].Name != "demo" {
		t.Fatalf("unexpected cases %#v", cases)
	}
}

func TestSummarizeResultsAggregatesFamiliesAndFailures(t *testing.T) {
	rep := summarizeResults([]caseResult{
		{
			Name:   "corp-1",
			Family: "corporate",
			Baseline: readMetrics{
				Success:          true,
				LatencyMS:        100,
				ContextAlignment: 0.82,
			},
			Compare: readMetrics{
				Success:           false,
				Error:             "context deadline exceeded",
				LatencyMS:         500,
				ContextAlignment:  0.71,
				Accepted:          0,
				Rejected:          1,
				ValidatorMessages: []string{"context deadline exceeded"},
			},
		},
		{
			Name:   "docs-1",
			Family: "docs",
			Baseline: readMetrics{
				Success:          true,
				LatencyMS:        50,
				ContextAlignment: 0.64,
			},
			Compare: readMetrics{
				Success:          true,
				LatencyMS:        60,
				ContextAlignment: 0.77,
				Accepted:         1,
			},
		},
	}, true)
	if rep.CaseCount != 2 {
		t.Fatalf("expected 2 cases, got %#v", rep)
	}
	if rep.BaselineSuccessRate != 1 {
		t.Fatalf("expected baseline success rate 1, got %#v", rep)
	}
	if rep.CompareSuccessRate != 0.5 {
		t.Fatalf("expected compare success rate 0.5, got %#v", rep)
	}
	if len(rep.RuntimeErrorCases) != 1 || rep.RuntimeErrorCases[0] != "corp-1" {
		t.Fatalf("expected runtime error case classification, got %#v", rep.RuntimeErrorCases)
	}
	if len(rep.FailureClusters) != 1 || rep.FailureClusters[0] != "timeout" {
		t.Fatalf("expected timeout failure cluster, got %#v", rep.FailureClusters)
	}
	if len(rep.Families) != 2 {
		t.Fatalf("expected two family summaries, got %#v", rep.Families)
	}
	if rep.AcceptedInterventions != 1 || rep.RejectedInterventions != 1 {
		t.Fatalf("unexpected intervention totals %#v", rep)
	}
	if rep.AvgBaselineContext <= 0 || rep.AvgCompareContext <= 0 {
		t.Fatalf("expected semantic aggregation, got %#v", rep)
	}
}

func TestLoadCasesRetainsFamily(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cases.json")
	payload, err := json.Marshal([]evalCase{{Name: "demo", Family: "docs", URL: "https://example.com", TimeoutMS: 1000, Objective: "demo objective"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write cases: %v", err)
	}
	cases, err := loadCases(path)
	if err != nil {
		t.Fatalf("load cases: %v", err)
	}
	if cases[0].Family != "docs" {
		t.Fatalf("expected family retained, got %#v", cases[0])
	}
}
