package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/platform"
	_ "modernc.org/sqlite"
)

func (s SQLiteStore) RecentRuns(ctx context.Context, limit int) ([]RecentRun, error) {
	if limit <= 0 {
		limit = 20
	}
	conn, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	defer platform.Close(conn)
	rows, err := conn.QueryContext(ctx, `
SELECT run_id, completed_at, operation, surface, selected_url, provider, failure_class, success, latency_ms,
       MAX(raw_fetch_chars - final_context_chars, 0), proof_usable, local_memory_used, public_bootstrap_used
FROM analytics_runs
ORDER BY completed_at DESC
LIMIT ?
`, limit)
	if err != nil {
		return nil, fmt.Errorf("query analytics recent runs: %w", err)
	}
	defer platform.Close(rows)
	out := make([]RecentRun, 0, limit)
	for rows.Next() {
		var item RecentRun
		var completedAt string
		var success, proofUsable, localMemoryUsed, publicBootstrapUsed int
		if err := rows.Scan(&item.RunID, &completedAt, &item.Operation, &item.Surface, &item.SelectedURL, &item.Provider, &item.FailureClass, &success, &item.LatencyMS, &item.CharsSaved, &proofUsable, &localMemoryUsed, &publicBootstrapUsed); err != nil {
			return nil, fmt.Errorf("scan analytics recent run: %w", err)
		}
		item.CompletedAt, _ = time.Parse(time.RFC3339Nano, completedAt)
		item.Success = success == 1
		item.TokensSavedEstimated = TokenEstimateFromChars(int64(item.CharsSaved))
		item.ProofUsable = proofUsable == 1
		item.LocalMemoryUsed = localMemoryUsed == 1
		item.PublicBootstrapUsed = publicBootstrapUsed == 1
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate analytics recent runs: %w", err)
	}
	return out, nil
}

const rollupMetricsSelect = `
  COUNT(*),
  COALESCE(SUM(success), 0),
  COALESCE(CAST(AVG(latency_ms) AS INTEGER), 0),
  COALESCE(SUM(MAX(raw_fetch_chars - final_context_chars, 0)), 0),
  COALESCE(AVG(CAST(proof_usable AS REAL)), 0),
  COALESCE(AVG(CAST(public_bootstrap_used AS REAL)), 0),
  COALESCE(AVG(CAST(local_memory_used AS REAL)), 0)
`

func (s SQLiteStore) Hosts(ctx context.Context, limit int) ([]HostRollup, error) {
	return dimensionRollups(ctx, s, "host", limit, func(label string, metrics RollupMetrics) HostRollup {
		return HostRollup{Host: label, RollupMetrics: metrics}
	})
}

func (s SQLiteStore) Providers(ctx context.Context, limit int) ([]ProviderRollup, error) {
	return dimensionRollups(ctx, s, "provider", limit, func(label string, metrics RollupMetrics) ProviderRollup {
		return ProviderRollup{Provider: label, RollupMetrics: metrics}
	})
}

// dimension is an adapter-owned column, supplied only by Hosts and Providers.
func dimensionRollups[T any](ctx context.Context, s SQLiteStore, dimension string, limit int, build func(string, RollupMetrics) T) ([]T, error) {
	if limit <= 0 {
		limit = 20
	}
	conn, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	defer platform.Close(conn)
	query := fmt.Sprintf(`SELECT %[1]s,`+rollupMetricsSelect+`
FROM analytics_runs
WHERE TRIM(COALESCE(%[1]s, '')) != ''
GROUP BY %[1]s
ORDER BY COUNT(*) DESC, %[1]s ASC
LIMIT ?`, dimension)
	rows, err := conn.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query analytics %ss: %w", dimension, err)
	}
	defer platform.Close(rows)
	out := []T{}
	for rows.Next() {
		label, metrics, err := readRollup(rows)
		if err != nil {
			return nil, fmt.Errorf("scan analytics %s rollup: %w", dimension, err)
		}
		out = append(out, build(label, metrics))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate analytics %s rollups: %w", dimension, err)
	}
	return out, nil
}

func readRollup(rows *sql.Rows) (string, RollupMetrics, error) {
	var label string
	var metrics RollupMetrics
	err := rows.Scan(&label, &metrics.RunCount, &metrics.SuccessfulRuns, &metrics.AvgLatencyMS, &metrics.TotalAgentCharsSaved, &metrics.ProofBackedRate, &metrics.PublicBootstrapUsedRate, &metrics.LocalMemoryUsedRate)
	metrics.TotalAgentTokensSaved = TokenEstimateFromChars(metrics.TotalAgentCharsSaved)
	return label, metrics, err
}

func (s SQLiteStore) Failures(ctx context.Context, limit int) ([]FailureRollup, error) {
	if limit <= 0 {
		limit = 20
	}
	conn, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	defer platform.Close(conn)
	rows, err := conn.QueryContext(ctx, `
SELECT
  failure_class,
  COUNT(*),
  COALESCE(CAST(AVG(latency_ms) AS INTEGER), 0),
  COALESCE(MAX(completed_at), '')
FROM analytics_runs
WHERE success = 0 AND TRIM(COALESCE(failure_class, '')) != ''
GROUP BY failure_class
ORDER BY COUNT(*) DESC, failure_class ASC
LIMIT ?
`, limit)
	if err != nil {
		return nil, fmt.Errorf("query analytics failures: %w", err)
	}
	defer platform.Close(rows)
	out := []FailureRollup{}
	for rows.Next() {
		var item FailureRollup
		if err := rows.Scan(&item.FailureClass, &item.RunCount, &item.AvgLatencyMS, &item.LastSeenAt); err != nil {
			return nil, fmt.Errorf("scan analytics failure rollup: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate analytics failure rollups: %w", err)
	}
	return out, nil
}

func (s SQLiteStore) Daily(ctx context.Context, limit int) ([]DailyRollup, error) {
	if limit <= 0 {
		limit = 30
	}
	conn, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	defer platform.Close(conn)
	rows, err := conn.QueryContext(ctx, `
SELECT
  substr(completed_at, 1, 10) AS day,
`+rollupMetricsSelect+`FROM analytics_runs
GROUP BY substr(completed_at, 1, 10)
ORDER BY day DESC
LIMIT ?
`, limit)
	if err != nil {
		return nil, fmt.Errorf("query analytics daily rollups: %w", err)
	}
	defer platform.Close(rows)
	out := []DailyRollup{}
	for rows.Next() {
		label, metrics, err := readRollup(rows)
		if err != nil {
			return nil, fmt.Errorf("scan analytics daily rollup: %w", err)
		}
		out = append(out, DailyRollup{Day: label, RollupMetrics: metrics})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate analytics daily rollups: %w", err)
	}
	return out, nil
}

func (s SQLiteStore) ExportJSON(ctx context.Context, outDir string) (ExportStats, error) {
	outDir = strings.TrimSpace(outDir)
	if outDir == "" {
		return ExportStats{}, fmt.Errorf("analytics export requires out_dir")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return ExportStats{}, fmt.Errorf("create analytics export dir: %w", err)
	}
	conn, err := s.open(ctx)
	if err != nil {
		return ExportStats{}, err
	}
	defer platform.Close(conn)
	bundle, err := s.loadExportBundle(ctx)
	if err != nil {
		return ExportStats{}, err
	}
	result := buildExportStats(outDir, bundle)
	if err := s.writeExportArtifacts(ctx, conn, result, bundle); err != nil {
		return ExportStats{}, err
	}
	return result, nil
}
