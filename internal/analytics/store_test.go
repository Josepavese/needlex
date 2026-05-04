package analytics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/platform"
)

func TestSQLiteStoreAppendRunAndReports(t *testing.T) {
	root := t.TempDir()
	store := NewSQLiteStore(root)
	startedAt := time.Now().UTC().Add(-2 * time.Second)
	completedAt := startedAt.Add(1500 * time.Millisecond)
	err := store.AppendRun(context.Background(), RunRecord{
		RunID:                "run_1",
		StartedAt:            startedAt,
		CompletedAt:          completedAt,
		Operation:            "query",
		Surface:              "cli",
		Profile:              "standard",
		GoalHash:             "goal_hash",
		GoalLengthChars:      21,
		DiscoveryMode:        "web_search",
		SelectedURL:          "https://example.com/docs",
		Provider:             "discovery_memory_same_site",
		Success:              true,
		TraceID:              "trace_1",
		LatencyMS:            1500,
		PacketBytes:          640,
		FinalContextChars:    400,
		ChunkCount:           2,
		SourceCount:          1,
		LinkCount:            3,
		ProofRefCount:        2,
		ProofUsable:          true,
		PublicBootstrapUsed:  false,
		LocalMemoryUsed:      true,
		TopicNodeUsed:        true,
		SameSiteRecoveryUsed: true,
		CandidateCount:       5,
		RawFetchChars:        2400,
		RawFetchBytes:        2400,
		ReducedChars:         900,
		ReducedNodeCount:     18,
		MemoryDocumentCount:  12,
		MemoryEmbeddingCount: 10,
		MemoryTopicNodeCount: 4,
	}, []StageEvent{
		{
			RunID:       "run_1",
			Stage:       "acquire",
			StartedAt:   startedAt,
			CompletedAt: startedAt.Add(100 * time.Millisecond),
			LatencyMS:   100,
			ItemCount:   1,
			Status:      "completed",
			Metadata:    map[string]string{"raw_chars": "2400"},
		},
	})
	if err != nil {
		t.Fatalf("append run: %v", err)
	}

	stats, err := store.Stats(context.Background())
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.RunCount != 1 || stats.QueryRuns != 1 || stats.StageEventCount != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if stats.TotalAgentCharsSaved != 2000 || stats.TotalAgentTokensSaved != 500 || stats.EstimatedCostSavedUSD.At5USDPerMillionTokens != 0.0025 {
		t.Fatalf("unexpected value stats: %+v", stats)
	}

	report, err := store.ValueReport(context.Background())
	if err != nil {
		t.Fatalf("value report: %v", err)
	}
	if report.TotalRuns != 1 || report.TotalAgentCharsSaved <= 0 || report.TotalPublicBootstrapsAvoided != 1 || report.TotalTopicRootCorrections != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if report.TokenEstimateMethod != TokenEstimateMethod || report.CharsPerTokenEstimate != CharsPerToken {
		t.Fatalf("unexpected token estimate policy: %+v", report)
	}
	if report.TotalAgentTokensSaved != 500 || report.TotalAgentTokensDelivered != 100 || report.TotalRawTokensEstimated != 600 {
		t.Fatalf("unexpected token estimates: %+v", report)
	}
	if report.EstimatedCostSavedUSD.At5USDPerMillionTokens != 0.0025 {
		t.Fatalf("unexpected cost estimate: %+v", report.EstimatedCostSavedUSD)
	}
	if report.ContextCompressionFactor != 6 {
		t.Fatalf("unexpected compression factor: %+v", report)
	}

	recent, err := store.RecentRuns(context.Background(), 5)
	if err != nil {
		t.Fatalf("recent runs: %v", err)
	}
	if len(recent) != 1 {
		t.Fatalf("expected one recent run, got %d", len(recent))
	}
	if recent[0].CharsSaved <= 0 || recent[0].TokensSavedEstimated != 500 || !recent[0].LocalMemoryUsed || recent[0].PublicBootstrapUsed {
		t.Fatalf("unexpected recent run: %+v", recent[0])
	}

	hosts, err := store.Hosts(context.Background(), 10)
	if err != nil {
		t.Fatalf("hosts: %v", err)
	}
	if len(hosts) != 1 || hosts[0].Host != "example.com" || hosts[0].TotalAgentTokensSaved != 500 {
		t.Fatalf("unexpected hosts: %+v", hosts)
	}

	providers, err := store.Providers(context.Background(), 10)
	if err != nil {
		t.Fatalf("providers: %v", err)
	}
	if len(providers) != 1 || providers[0].Provider != "discovery_memory_same_site" || providers[0].TotalAgentTokensSaved != 500 {
		t.Fatalf("unexpected providers: %+v", providers)
	}

	failures, err := store.Failures(context.Background(), 10)
	if err != nil {
		t.Fatalf("failures: %v", err)
	}
	if len(failures) != 0 {
		t.Fatalf("unexpected failures: %+v", failures)
	}

	daily, err := store.Daily(context.Background(), 10)
	if err != nil {
		t.Fatalf("daily: %v", err)
	}
	if len(daily) != 1 || daily[0].RunCount != 1 || daily[0].TotalAgentTokensSaved != 500 {
		t.Fatalf("unexpected daily: %+v", daily)
	}

	exportDir := t.TempDir()
	exported, err := store.ExportJSON(context.Background(), exportDir)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	for _, path := range []string{exported.RunsPath, exported.StagesPath, exported.HostsPath, exported.ProvidersPath, exported.DailyPath, exported.ValueReportPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected export file %s: %v", filepath.Base(path), err)
		}
	}
}

func TestSQLiteStoreOpenAppliesOperationalPragmas(t *testing.T) {
	store := NewSQLiteStore(t.TempDir())
	conn, err := store.open(context.Background())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer platform.Close(conn)

	var busyTimeout int
	if err := conn.QueryRowContext(context.Background(), `PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatalf("read busy_timeout: %v", err)
	}
	if busyTimeout < 5000 {
		t.Fatalf("expected busy_timeout >= 5000, got %d", busyTimeout)
	}

	var journalMode string
	if err := conn.QueryRowContext(context.Background(), `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		t.Fatalf("expected WAL journal mode, got %q", journalMode)
	}
}

func TestObserveFailureRecordsFailedAttempt(t *testing.T) {
	root := t.TempDir()
	store := NewSQLiteStore(root)
	startedAt := time.Now().UTC().Add(-250 * time.Millisecond)

	err := ObserveFailure(context.Background(), store, FailureObservation{
		Operation:     "query",
		Surface:       "mcp",
		Profile:       "standard",
		Goal:          "find protocol docs",
		SeedURL:       "https://example.com/missing",
		DiscoveryMode: "off",
		StartedAt:     startedAt,
		Err:           fmt.Errorf("unexpected status code 404"),
	})
	if err != nil {
		t.Fatalf("ObserveFailure() error = %v", err)
	}

	stats, err := store.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if stats.RunCount != 1 || stats.SuccessfulRuns != 0 || stats.QueryRuns != 1 || stats.StageEventCount != 1 {
		t.Fatalf("unexpected failed stats: %+v", stats)
	}

	recent, err := store.RecentRuns(context.Background(), 1)
	if err != nil {
		t.Fatalf("RecentRuns() error = %v", err)
	}
	if len(recent) != 1 {
		t.Fatalf("expected one recent run, got %d", len(recent))
	}
	if recent[0].Success || recent[0].Surface != "mcp" || recent[0].Provider != "error:upstream_not_found" || recent[0].FailureClass != "upstream_not_found" {
		t.Fatalf("unexpected failed run: %+v", recent[0])
	}

	failures, err := store.Failures(context.Background(), 10)
	if err != nil {
		t.Fatalf("Failures() error = %v", err)
	}
	if len(failures) != 1 || failures[0].FailureClass != "upstream_not_found" || failures[0].RunCount != 1 {
		t.Fatalf("unexpected failure rollups: %+v", failures)
	}
}
