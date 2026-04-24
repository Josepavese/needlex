package transport

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
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
	if !strings.Contains(stdout.String(), "Runs: 1") || !strings.Contains(stdout.String(), "Estimated Tokens Saved: 225") || !strings.Contains(stdout.String(), "DB Path:") {
		t.Fatalf("unexpected analytics stats output: %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = runner.Run([]string{"analytics", "value-report"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("analytics value-report exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Chars Saved for the Agent:") || !strings.Contains(stdout.String(), "Estimated Tokens Saved:") || !strings.Contains(stdout.String(), "Topic Roots Recovered: 1") {
		t.Fatalf("unexpected analytics value-report output: %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = runner.Run([]string{"analytics", "hosts"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("analytics hosts exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "example.com") || !strings.Contains(stdout.String(), "Estimated Tokens Saved: 225") {
		t.Fatalf("unexpected analytics hosts output: %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = runner.Run([]string{"analytics", "providers"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("analytics providers exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "discovery_memory_same_site") || !strings.Contains(stdout.String(), "Estimated Tokens Saved: 225") {
		t.Fatalf("unexpected analytics providers output: %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = runner.Run([]string{"analytics", "failures"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("analytics failures exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Failures: 0") {
		t.Fatalf("unexpected analytics failures output: %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = runner.Run([]string{"analytics", "daily"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("analytics daily exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Runs: 1") || !strings.Contains(stdout.String(), "Estimated Tokens Saved: 225") {
		t.Fatalf("unexpected analytics daily output: %q", stdout.String())
	}

	exportDir := t.TempDir()
	stdout.Reset()
	stderr.Reset()
	code = runner.Run([]string{"analytics", "export", "--out", exportDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("analytics export exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "analytics_runs.jsonl") || !strings.Contains(stdout.String(), "analytics_value_report.json") {
		t.Fatalf("unexpected analytics export output: %q", stdout.String())
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

func TestRunnerDoctorReportsStateRootAndDatabases(t *testing.T) {
	root := t.TempDir()
	runner := NewRunner()
	runner.storeRoot = root

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runner.Run([]string{"doctor"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doctor exit=%d stderr=%q", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Needle-X Doctor") || !strings.Contains(out, "State Root: "+root) || !strings.Contains(out, "Analytics DB:") || !strings.Contains(out, "Discovery DB:") {
		t.Fatalf("unexpected doctor output: %q", out)
	}

	stdout.Reset()
	stderr.Reset()
	code = runner.Run([]string{"doctor", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doctor json exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"state_root":`) || !strings.Contains(stdout.String(), `"analytics_db_path":`) {
		t.Fatalf("unexpected doctor json: %q", stdout.String())
	}
}

func TestRunnerSupportBundleExportsDiagnostics(t *testing.T) {
	root := t.TempDir()
	runner := NewRunner()
	runner.storeRoot = root
	_ = runner.logRuntimeError("read", "fetch.failed", errors.New("unexpected status code 403"), map[string]any{"url": "https://example.com"})

	outDir := filepath.Join(t.TempDir(), "bundle")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runner.Run([]string{"support", "bundle", "--out", outDir, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("support bundle exit=%d stderr=%q", code, stderr.String())
	}
	for _, path := range []string{
		filepath.Join(outDir, "manifest.json"),
		filepath.Join(outDir, "doctor.json"),
		filepath.Join(outDir, "runtime_log_stats.json"),
		filepath.Join(outDir, "runtime_log_tail.json"),
		filepath.Join(outDir, "analytics", "analytics_value_report.json"),
		filepath.Join(outDir, "logs", "needlex.jsonl"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected support artifact %s: %v", path, err)
		}
	}
	if !strings.Contains(stdout.String(), `"manifest_path"`) || !strings.Contains(stdout.String(), `"analytics_export"`) {
		t.Fatalf("unexpected support bundle json: %q", stdout.String())
	}
}
