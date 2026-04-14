package transport

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/analytics"
)

func TestRunnerAnalyticsStatsAndValueReport(t *testing.T) {
	root := t.TempDir()
	store := analytics.NewSQLiteStore(root)
	startedAt := time.Now().UTC().Add(-time.Second)
	completedAt := startedAt.Add(500 * time.Millisecond)
	if err := store.AppendRun(context.Background(), analytics.RunRecord{
		RunID:                "run_1",
		StartedAt:            startedAt,
		CompletedAt:          completedAt,
		Operation:            "query",
		Surface:              "cli",
		Profile:              "standard",
		GoalHash:             "goal_hash",
		GoalLengthChars:      11,
		DiscoveryMode:        "web_search",
		SelectedURL:          "https://example.com",
		Provider:             "discovery_memory_same_site",
		Success:              true,
		TraceID:              "trace_1",
		LatencyMS:            500,
		PacketBytes:          200,
		FinalContextChars:    100,
		ChunkCount:           1,
		SourceCount:          1,
		LinkCount:            2,
		ProofRefCount:        1,
		ProofUsable:          true,
		PublicBootstrapUsed:  false,
		LocalMemoryUsed:      true,
		TopicNodeUsed:        true,
		SameSiteRecoveryUsed: true,
		RawFetchChars:        1000,
		RawFetchBytes:        1000,
	}, nil); err != nil {
		t.Fatalf("seed analytics db: %v", err)
	}

	runner := NewRunner()
	runner.storeRoot = root

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runner.Run([]string{"analytics", "stats"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("analytics stats exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Runs: 1") || !strings.Contains(stdout.String(), "DB Path:") {
		t.Fatalf("unexpected analytics stats output: %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = runner.Run([]string{"analytics", "value-report"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("analytics value-report exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Chars Saved for the Agent:") || !strings.Contains(stdout.String(), "Topic Roots Recovered: 1") {
		t.Fatalf("unexpected analytics value-report output: %q", stdout.String())
	}
}

func TestRunnerHelpListsAnalyticsCommand(t *testing.T) {
	runner := NewRunner()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runner.Run([]string{"help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "needlex analytics") {
		t.Fatalf("expected help to include analytics command, got %q", stdout.String())
	}
}
