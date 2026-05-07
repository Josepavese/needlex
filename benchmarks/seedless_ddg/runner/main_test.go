package main

import "testing"

func TestSummarizeIncludesObservabilityByProfile(t *testing.T) {
	results := []caseResult{
		{
			ID: "ok",
			Runs: []runResult{{
				Profile:        "browser_like_semantic",
				RuntimeOK:      true,
				SelectedPass:   true,
				CandidateCount: 8,
				ExpectedRank:   2,
				LatencyMS:      1000,
			}},
			Delta: "browser_like_semantic",
		},
		{
			ID: "timeout",
			Runs: []runResult{{
				Profile:      "browser_like_semantic",
				RuntimeOK:    false,
				ErrorKind:    "benchmark_timeout",
				LatencyMS:    30000,
				SelectedPass: false,
			}},
			Delta: "browser_like_semantic",
		},
	}
	got := summarize(results, 1, 30000, seedlessConfigs{
		profiles:       []seedlessProfileConfig{{name: "browser_like_semantic"}},
		providerChains: []string{"ddg-bing"},
	})
	if got.AvgLatencyMSByProfile["browser_like_semantic"] != 15500 {
		t.Fatalf("unexpected avg latency: %#v", got.AvgLatencyMSByProfile)
	}
	if got.P95LatencyMSByProfile["browser_like_semantic"] != 30000 {
		t.Fatalf("unexpected p95 latency: %#v", got.P95LatencyMSByProfile)
	}
	if got.TimeoutRateByProfile["browser_like_semantic"] != 0.5 {
		t.Fatalf("unexpected timeout rate: %#v", got.TimeoutRateByProfile)
	}
	if got.AvgCandidateCountByProfile["browser_like_semantic"] != 8 {
		t.Fatalf("unexpected candidate count: %#v", got.AvgCandidateCountByProfile)
	}
	if got.AvgExpectedRankByProfile["browser_like_semantic"] != 2 {
		t.Fatalf("unexpected expected rank: %#v", got.AvgExpectedRankByProfile)
	}
	if got.ErrorKindsByProfile["browser_like_semantic"]["benchmark_timeout"] != 1 {
		t.Fatalf("unexpected error mix: %#v", got.ErrorKindsByProfile)
	}
}

func TestSummarizeBestProfileUsesPassRateBeforeCaseWinTieBreak(t *testing.T) {
	results := []caseResult{
		{
			ID: "both-pass-a",
			Runs: []runResult{
				{Profile: "standard", RuntimeOK: true, SelectedPass: true, PassCount: 3, AttemptCount: 3, RuntimePassCount: 3, LatencyMS: 1000},
				{Profile: "browser_like", RuntimeOK: true, SelectedPass: true, PassCount: 3, AttemptCount: 3, RuntimePassCount: 3, LatencyMS: 1200},
			},
			Delta: "standard",
		},
		{
			ID: "both-pass-b",
			Runs: []runResult{
				{Profile: "standard", RuntimeOK: true, SelectedPass: true, PassCount: 3, AttemptCount: 3, RuntimePassCount: 3, LatencyMS: 1000},
				{Profile: "browser_like", RuntimeOK: true, SelectedPass: true, PassCount: 3, AttemptCount: 3, RuntimePassCount: 3, LatencyMS: 1200},
			},
			Delta: "standard",
		},
		{
			ID: "browser-only",
			Runs: []runResult{
				{Profile: "standard", RuntimeOK: true, SelectedPass: false, PassCount: 0, AttemptCount: 3, RuntimePassCount: 3, LatencyMS: 1000},
				{Profile: "browser_like", RuntimeOK: true, SelectedPass: true, PassCount: 2, AttemptCount: 3, RuntimePassCount: 3, LatencyMS: 1200},
			},
			Delta: "browser_like",
		},
	}
	got := summarize(results, 3, 35000, seedlessConfigs{
		profiles:       []seedlessProfileConfig{{name: "standard"}, {name: "browser_like"}},
		providerChains: []string{"ddg-bing"},
	})
	if got.BestProfile != "browser_like" {
		t.Fatalf("expected pass-rate winner browser_like, got %q", got.BestProfile)
	}
	if got.BrowserLikeBeatsStandard != 1 {
		t.Fatalf("unexpected browser-like delta: %d", got.BrowserLikeBeatsStandard)
	}
	if got.ImprovementRate != float64(1)/float64(3) {
		t.Fatalf("unexpected improvement rate: %v", got.ImprovementRate)
	}
}

func TestCompareAllRunsUsesAttemptPassRatioBeforeProfileOrder(t *testing.T) {
	got := compareAllRuns(
		runResult{Profile: "standard", RuntimeOK: true, SelectedPass: true, PassCount: 2, AttemptCount: 3, RuntimePassCount: 3},
		runResult{Profile: "browser_like", RuntimeOK: true, SelectedPass: true, PassCount: 3, AttemptCount: 3, RuntimePassCount: 3},
	)
	if got != "browser_like" {
		t.Fatalf("expected browser_like to win by attempt pass ratio, got %q", got)
	}
}
