package analytics

import (
	"context"
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

func (s SQLiteStore) Hosts(ctx context.Context, limit int) ([]HostRollup, error) {
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
  host,
  COUNT(*),
  COALESCE(SUM(success), 0),
  COALESCE(CAST(AVG(latency_ms) AS INTEGER), 0),
  COALESCE(SUM(MAX(raw_fetch_chars - final_context_chars, 0)), 0),
  COALESCE(AVG(CAST(proof_usable AS REAL)), 0),
  COALESCE(AVG(CAST(public_bootstrap_used AS REAL)), 0),
  COALESCE(AVG(CAST(local_memory_used AS REAL)), 0)
FROM analytics_runs
WHERE TRIM(COALESCE(host, '')) != ''
GROUP BY host
ORDER BY COUNT(*) DESC, host ASC
LIMIT ?
`, limit)
	if err != nil {
		return nil, fmt.Errorf("query analytics hosts: %w", err)
	}
	defer platform.Close(rows)
	out := []HostRollup{}
	for rows.Next() {
		var item HostRollup
		if err := rows.Scan(&item.Host, &item.RunCount, &item.SuccessfulRuns, &item.AvgLatencyMS, &item.TotalAgentCharsSaved, &item.ProofBackedRate, &item.PublicBootstrapUsedRate, &item.LocalMemoryUsedRate); err != nil {
			return nil, fmt.Errorf("scan analytics host rollup: %w", err)
		}
		item.TotalAgentTokensSaved = TokenEstimateFromChars(item.TotalAgentCharsSaved)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate analytics host rollups: %w", err)
	}
	return out, nil
}

func (s SQLiteStore) Providers(ctx context.Context, limit int) ([]ProviderRollup, error) {
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
  provider,
  COUNT(*),
  COALESCE(SUM(success), 0),
  COALESCE(CAST(AVG(latency_ms) AS INTEGER), 0),
  COALESCE(SUM(MAX(raw_fetch_chars - final_context_chars, 0)), 0),
  COALESCE(AVG(CAST(proof_usable AS REAL)), 0),
  COALESCE(AVG(CAST(public_bootstrap_used AS REAL)), 0),
  COALESCE(AVG(CAST(local_memory_used AS REAL)), 0)
FROM analytics_runs
WHERE TRIM(COALESCE(provider, '')) != ''
GROUP BY provider
ORDER BY COUNT(*) DESC, provider ASC
LIMIT ?
`, limit)
	if err != nil {
		return nil, fmt.Errorf("query analytics providers: %w", err)
	}
	defer platform.Close(rows)
	out := []ProviderRollup{}
	for rows.Next() {
		var item ProviderRollup
		if err := rows.Scan(&item.Provider, &item.RunCount, &item.SuccessfulRuns, &item.AvgLatencyMS, &item.TotalAgentCharsSaved, &item.ProofBackedRate, &item.PublicBootstrapUsedRate, &item.LocalMemoryUsedRate); err != nil {
			return nil, fmt.Errorf("scan analytics provider rollup: %w", err)
		}
		item.TotalAgentTokensSaved = TokenEstimateFromChars(item.TotalAgentCharsSaved)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate analytics provider rollups: %w", err)
	}
	return out, nil
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
  COUNT(*),
  COALESCE(SUM(success), 0),
  COALESCE(CAST(AVG(latency_ms) AS INTEGER), 0),
  COALESCE(SUM(MAX(raw_fetch_chars - final_context_chars, 0)), 0),
  COALESCE(AVG(CAST(proof_usable AS REAL)), 0),
  COALESCE(AVG(CAST(public_bootstrap_used AS REAL)), 0),
  COALESCE(AVG(CAST(local_memory_used AS REAL)), 0)
FROM analytics_runs
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
		var item DailyRollup
		if err := rows.Scan(&item.Day, &item.RunCount, &item.SuccessfulRuns, &item.AvgLatencyMS, &item.TotalAgentCharsSaved, &item.ProofBackedRate, &item.PublicBootstrapUsedRate, &item.LocalMemoryUsedRate); err != nil {
			return nil, fmt.Errorf("scan analytics daily rollup: %w", err)
		}
		item.TotalAgentTokensSaved = TokenEstimateFromChars(item.TotalAgentCharsSaved)
		out = append(out, item)
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
