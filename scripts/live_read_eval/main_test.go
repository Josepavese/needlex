package main

import (
	"strings"
	"testing"
)

func TestCompareReportsDetectsWebIRRegressions(t *testing.T) {
	previous := report{
		Results: []caseResult{
			{
				Name:            "site",
				Success:         true,
				KeywordCoverage: 1.0,
				NoiseHits:       0,
				WebIRVersion:    "web_ir.v1",
				WebIRNodeCount:  10,
			},
		},
	}
	current := report{
		Results: []caseResult{
			{
				Name:            "site",
				Success:         true,
				KeywordCoverage: 1.0,
				NoiseHits:       0,
				WebIRVersion:    "web_ir.v2",
				WebIRNodeCount:  3,
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
				Name:            "site",
				Success:         true,
				KeywordCoverage: 1.0,
				NoiseHits:       0,
			},
		},
	}
	current := report{
		Results: []caseResult{
			{
				Name:            "site",
				Success:         true,
				KeywordCoverage: 1.0,
				NoiseHits:       0,
				WebIRVersion:    "web_ir.v1",
				WebIRNodeCount:  7,
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
