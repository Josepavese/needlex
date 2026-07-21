package analytics

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/josepavese/needlex/internal/platform"
	_ "modernc.org/sqlite"
)

type exportBundle struct {
	stats     Stats
	hosts     []HostRollup
	providers []ProviderRollup
	failures  []FailureRollup
	daily     []DailyRollup
	report    ValueReport
}

func (s SQLiteStore) loadExportBundle(ctx context.Context) (exportBundle, error) {
	stats, err := s.Stats(ctx)
	if err != nil {
		return exportBundle{}, err
	}
	hosts, err := s.Hosts(ctx, 1000)
	if err != nil {
		return exportBundle{}, err
	}
	providers, err := s.Providers(ctx, 1000)
	if err != nil {
		return exportBundle{}, err
	}
	failures, err := s.Failures(ctx, 1000)
	if err != nil {
		return exportBundle{}, err
	}
	daily, err := s.Daily(ctx, 1000)
	if err != nil {
		return exportBundle{}, err
	}
	report, err := s.ValueReport(ctx)
	if err != nil {
		return exportBundle{}, err
	}
	return exportBundle{
		stats:     stats,
		hosts:     hosts,
		providers: providers,
		failures:  failures,
		daily:     daily,
		report:    report,
	}, nil
}

func buildExportStats(outDir string, bundle exportBundle) ExportStats {
	return ExportStats{
		Directory:       outDir,
		RunsPath:        filepath.Join(outDir, "analytics_runs.jsonl"),
		StagesPath:      filepath.Join(outDir, "analytics_stage_events.jsonl"),
		HostsPath:       filepath.Join(outDir, "analytics_hosts.json"),
		ProvidersPath:   filepath.Join(outDir, "analytics_providers.json"),
		FailuresPath:    filepath.Join(outDir, "analytics_failures.json"),
		DailyPath:       filepath.Join(outDir, "analytics_daily.json"),
		ValueReportPath: filepath.Join(outDir, "analytics_value_report.json"),
		RunCount:        bundle.stats.RunCount,
		StageCount:      bundle.stats.StageEventCount,
		HostCount:       len(bundle.hosts),
		ProviderCount:   len(bundle.providers),
		FailureCount:    len(bundle.failures),
		DailyCount:      len(bundle.daily),
	}
}

func (s SQLiteStore) writeExportArtifacts(ctx context.Context, conn *sql.DB, stats ExportStats, bundle exportBundle) error {
	if err := exportJSONLQuery(ctx, conn, stats.RunsPath, `
SELECT
  run_id, started_at, completed_at, operation, surface, profile, goal_hash, goal_length_chars,
  discovery_mode, seed_present, host, selected_url, provider, failure_class, success, trace_id, latency_ms,
  packet_bytes, final_context_chars, chunk_count, source_count, link_count, proof_ref_count,
  proof_usable, public_bootstrap_used, local_memory_used, topic_node_used, same_site_recovery_used,
  candidate_count, raw_fetch_chars, raw_fetch_bytes, reduced_chars, reduced_node_count,
  memory_document_count, memory_embedding_count, memory_topic_node_count
FROM analytics_runs
ORDER BY completed_at DESC
`); err != nil {
		return err
	}
	if err := exportJSONLQuery(ctx, conn, stats.StagesPath, `
SELECT run_id, stage, started_at, completed_at, latency_ms, item_count, status, metadata_json
FROM analytics_stage_events
ORDER BY id ASC
`); err != nil {
		return err
	}
	if err := writeJSONFile(stats.HostsPath, bundle.hosts); err != nil {
		return err
	}
	if err := writeJSONFile(stats.ProvidersPath, bundle.providers); err != nil {
		return err
	}
	if err := writeJSONFile(stats.FailuresPath, bundle.failures); err != nil {
		return err
	}
	if err := writeJSONFile(stats.DailyPath, bundle.daily); err != nil {
		return err
	}
	if err := writeJSONFile(stats.ValueReportPath, bundle.report); err != nil {
		return err
	}
	return nil
}

func (s SQLiteStore) open(ctx context.Context) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(s.dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create analytics db dir: %w", err)
	}
	db, err := sql.Open("sqlite", s.dbPath)
	if err != nil {
		return nil, fmt.Errorf("open analytics db: %w", err)
	}
	if err := platform.ConfigureSQLite(ctx, db); err != nil {
		platform.Close(db)
		return nil, fmt.Errorf("configure analytics db: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		platform.Close(db)
		return nil, fmt.Errorf("ping analytics db: %w", err)
	}
	if err := s.ensureSchema(ctx, db); err != nil {
		platform.Close(db)
		return nil, err
	}
	return db, nil
}

func (s SQLiteStore) ensureSchema(ctx context.Context, db *sql.DB) error {
	for _, stmt := range analyticsSchemaStatements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensure analytics schema: %w", err)
		}
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE analytics_runs ADD COLUMN host TEXT NOT NULL DEFAULT ''`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return fmt.Errorf("ensure analytics schema: %w", err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE analytics_runs ADD COLUMN failure_class TEXT NOT NULL DEFAULT ''`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return fmt.Errorf("ensure analytics schema: %w", err)
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func hostFromURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Hostname())
}

func exportJSONLQuery(ctx context.Context, conn *sql.DB, path, query string) error {
	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("query analytics export rows: %w", err)
	}
	defer platform.Close(rows)
	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("analytics export columns: %w", err)
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create analytics export file: %w", err)
	}
	defer platform.Close(file)
	encoder := json.NewEncoder(file)
	for rows.Next() {
		values := make([]any, len(columns))
		scanTargets := make([]any, len(columns))
		for i := range values {
			scanTargets[i] = &values[i]
		}
		if err := rows.Scan(scanTargets...); err != nil {
			return fmt.Errorf("scan analytics export row: %w", err)
		}
		record := map[string]any{}
		for i, column := range columns {
			record[column] = normalizeSQLValue(values[i])
		}
		if err := encoder.Encode(record); err != nil {
			return fmt.Errorf("encode analytics export row: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate analytics export rows: %w", err)
	}
	return nil
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal analytics export json: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write analytics export json: %w", err)
	}
	return nil
}

func normalizeSQLValue(value any) any {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	default:
		return typed
	}
}
