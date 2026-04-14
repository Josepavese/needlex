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
	"time"

	"github.com/josepavese/needlex/internal/platform"
	_ "modernc.org/sqlite"
)

type RunRecord struct {
	RunID                string
	StartedAt            time.Time
	CompletedAt          time.Time
	Operation            string
	Surface              string
	Profile              string
	GoalHash             string
	GoalLengthChars      int
	DiscoveryMode        string
	SeedPresent          bool
	Host                 string
	SelectedURL          string
	Provider             string
	Success              bool
	TraceID              string
	LatencyMS            int64
	PacketBytes          int
	FinalContextChars    int
	ChunkCount           int
	SourceCount          int
	LinkCount            int
	ProofRefCount        int
	ProofUsable          bool
	PublicBootstrapUsed  bool
	LocalMemoryUsed      bool
	TopicNodeUsed        bool
	SameSiteRecoveryUsed bool
	CandidateCount       int
	RawFetchChars        int
	RawFetchBytes        int
	ReducedChars         int
	ReducedNodeCount     int
	MemoryDocumentCount  int
	MemoryEmbeddingCount int
	MemoryTopicNodeCount int
}

type StageEvent struct {
	RunID       string
	Stage       string
	StartedAt   time.Time
	CompletedAt time.Time
	LatencyMS   int64
	ItemCount   int
	Status      string
	Metadata    map[string]string
}

type Stats struct {
	RunCount        int       `json:"run_count"`
	SuccessfulRuns  int       `json:"successful_runs"`
	QueryRuns       int       `json:"query_runs"`
	ReadRuns        int       `json:"read_runs"`
	CrawlRuns       int       `json:"crawl_runs"`
	StageEventCount int       `json:"stage_event_count"`
	LastRunAt       time.Time `json:"last_run_at,omitempty"`
	DBPath          string    `json:"db_path"`
	DBSizeBytes     int64     `json:"db_size_bytes"`
}

type ValueReport struct {
	TotalRuns                    int     `json:"total_runs"`
	SuccessfulRuns               int     `json:"successful_runs"`
	TotalRawCharsProcessed       int64   `json:"total_raw_chars_processed"`
	TotalAgentCharsSaved         int64   `json:"total_agent_chars_saved"`
	TotalProofBackedPackets      int     `json:"total_proof_backed_packets"`
	TotalPublicBootstrapsAvoided int     `json:"total_public_bootstraps_avoided"`
	TotalMemoryReuseEvents       int     `json:"total_memory_reuse_events"`
	TotalTopicRootCorrections    int     `json:"total_topic_root_corrections"`
	TotalLinksExplored           int64   `json:"total_links_explored"`
	TotalSourcesVisited          int64   `json:"total_sources_visited"`
	AvgLatencyMS                 int64   `json:"avg_latency_ms"`
	ContextCompressionRatio      float64 `json:"context_compression_ratio"`
	ProofBackedRate              float64 `json:"proof_backed_rate"`
	WarmLikeReuseRate            float64 `json:"warm_like_reuse_rate"`
}

type RecentRun struct {
	RunID               string    `json:"run_id"`
	CompletedAt         time.Time `json:"completed_at"`
	Operation           string    `json:"operation"`
	Surface             string    `json:"surface"`
	SelectedURL         string    `json:"selected_url,omitempty"`
	Provider            string    `json:"provider,omitempty"`
	Success             bool      `json:"success"`
	LatencyMS           int64     `json:"latency_ms"`
	CharsSaved          int       `json:"chars_saved"`
	ProofUsable         bool      `json:"proof_usable"`
	LocalMemoryUsed     bool      `json:"local_memory_used"`
	PublicBootstrapUsed bool      `json:"public_bootstrap_used"`
}

type HostRollup struct {
	Host                    string  `json:"host"`
	RunCount                int     `json:"run_count"`
	SuccessfulRuns          int     `json:"successful_runs"`
	AvgLatencyMS            int64   `json:"avg_latency_ms"`
	TotalAgentCharsSaved    int64   `json:"total_agent_chars_saved"`
	ProofBackedRate         float64 `json:"proof_backed_rate"`
	PublicBootstrapUsedRate float64 `json:"public_bootstrap_used_rate"`
	LocalMemoryUsedRate     float64 `json:"local_memory_used_rate"`
}

type ProviderRollup struct {
	Provider                string  `json:"provider"`
	RunCount                int     `json:"run_count"`
	SuccessfulRuns          int     `json:"successful_runs"`
	AvgLatencyMS            int64   `json:"avg_latency_ms"`
	TotalAgentCharsSaved    int64   `json:"total_agent_chars_saved"`
	ProofBackedRate         float64 `json:"proof_backed_rate"`
	PublicBootstrapUsedRate float64 `json:"public_bootstrap_used_rate"`
	LocalMemoryUsedRate     float64 `json:"local_memory_used_rate"`
}

type SQLiteStore struct {
	dbPath string
}

func NewSQLiteStore(root string) SQLiteStore {
	cleanRoot := strings.TrimSpace(root)
	if cleanRoot == "" {
		cleanRoot = platform.DefaultStateRoot()
	}
	return SQLiteStore{dbPath: filepath.Join(cleanRoot, "analytics", "analytics.db")}
}

func (s SQLiteStore) DBPath() string { return s.dbPath }

func (s SQLiteStore) AppendRun(ctx context.Context, run RunRecord, stages []StageEvent) error {
	if strings.TrimSpace(run.Host) == "" {
		run.Host = hostFromURL(run.SelectedURL)
	}
	conn, err := s.open(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin analytics tx: %w", err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
INSERT INTO analytics_runs (
  run_id, started_at, completed_at, operation, surface, profile, goal_hash, goal_length_chars,
  discovery_mode, seed_present, host, selected_url, provider, success, trace_id, latency_ms,
  packet_bytes, final_context_chars, chunk_count, source_count, link_count, proof_ref_count,
  proof_usable, public_bootstrap_used, local_memory_used, topic_node_used, same_site_recovery_used,
  candidate_count, raw_fetch_chars, raw_fetch_bytes, reduced_chars, reduced_node_count,
  memory_document_count, memory_embedding_count, memory_topic_node_count
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(run_id) DO UPDATE SET
  completed_at=excluded.completed_at,
  operation=excluded.operation,
  surface=excluded.surface,
  profile=excluded.profile,
  goal_hash=excluded.goal_hash,
  goal_length_chars=excluded.goal_length_chars,
  discovery_mode=excluded.discovery_mode,
  seed_present=excluded.seed_present,
  host=excluded.host,
  selected_url=excluded.selected_url,
  provider=excluded.provider,
  success=excluded.success,
  trace_id=excluded.trace_id,
  latency_ms=excluded.latency_ms,
  packet_bytes=excluded.packet_bytes,
  final_context_chars=excluded.final_context_chars,
  chunk_count=excluded.chunk_count,
  source_count=excluded.source_count,
  link_count=excluded.link_count,
  proof_ref_count=excluded.proof_ref_count,
  proof_usable=excluded.proof_usable,
  public_bootstrap_used=excluded.public_bootstrap_used,
  local_memory_used=excluded.local_memory_used,
  topic_node_used=excluded.topic_node_used,
  same_site_recovery_used=excluded.same_site_recovery_used,
  candidate_count=excluded.candidate_count,
  raw_fetch_chars=excluded.raw_fetch_chars,
  raw_fetch_bytes=excluded.raw_fetch_bytes,
  reduced_chars=excluded.reduced_chars,
  reduced_node_count=excluded.reduced_node_count,
  memory_document_count=excluded.memory_document_count,
  memory_embedding_count=excluded.memory_embedding_count,
  memory_topic_node_count=excluded.memory_topic_node_count
`,
		run.RunID,
		run.StartedAt.UTC().Format(time.RFC3339Nano),
		run.CompletedAt.UTC().Format(time.RFC3339Nano),
		run.Operation,
		run.Surface,
		run.Profile,
		run.GoalHash,
		run.GoalLengthChars,
		run.DiscoveryMode,
		boolInt(run.SeedPresent),
		run.Host,
		run.SelectedURL,
		run.Provider,
		boolInt(run.Success),
		run.TraceID,
		run.LatencyMS,
		run.PacketBytes,
		run.FinalContextChars,
		run.ChunkCount,
		run.SourceCount,
		run.LinkCount,
		run.ProofRefCount,
		boolInt(run.ProofUsable),
		boolInt(run.PublicBootstrapUsed),
		boolInt(run.LocalMemoryUsed),
		boolInt(run.TopicNodeUsed),
		boolInt(run.SameSiteRecoveryUsed),
		run.CandidateCount,
		run.RawFetchChars,
		run.RawFetchBytes,
		run.ReducedChars,
		run.ReducedNodeCount,
		run.MemoryDocumentCount,
		run.MemoryEmbeddingCount,
		run.MemoryTopicNodeCount,
	)
	if err != nil {
		return fmt.Errorf("upsert analytics run: %w", err)
	}
	for _, stage := range stages {
		rawMeta, _ := json.Marshal(stage.Metadata)
		if _, err := tx.ExecContext(ctx, `
INSERT INTO analytics_stage_events (
  run_id, stage, started_at, completed_at, latency_ms, item_count, status, metadata_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, stage.RunID, stage.Stage, stage.StartedAt.UTC().Format(time.RFC3339Nano), stage.CompletedAt.UTC().Format(time.RFC3339Nano), stage.LatencyMS, stage.ItemCount, stage.Status, string(rawMeta)); err != nil {
			return fmt.Errorf("insert analytics stage event: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit analytics tx: %w", err)
	}
	return nil
}

func (s SQLiteStore) Stats(ctx context.Context) (Stats, error) {
	conn, err := s.open(ctx)
	if err != nil {
		return Stats{}, err
	}
	defer conn.Close()
	var out Stats
	out.DBPath = s.dbPath
	for query, target := range map[string]*int{
		"SELECT COUNT(*) FROM analytics_runs":                           &out.RunCount,
		"SELECT COUNT(*) FROM analytics_stage_events":                   &out.StageEventCount,
		"SELECT COUNT(*) FROM analytics_runs WHERE success = 1":         &out.SuccessfulRuns,
		"SELECT COUNT(*) FROM analytics_runs WHERE operation = 'query'": &out.QueryRuns,
		"SELECT COUNT(*) FROM analytics_runs WHERE operation = 'read'":  &out.ReadRuns,
		"SELECT COUNT(*) FROM analytics_runs WHERE operation = 'crawl'": &out.CrawlRuns,
	} {
		if err := conn.QueryRowContext(ctx, query).Scan(target); err != nil {
			return Stats{}, fmt.Errorf("query analytics stats: %w", err)
		}
	}
	var raw sql.NullString
	if err := conn.QueryRowContext(ctx, `SELECT MAX(completed_at) FROM analytics_runs`).Scan(&raw); err != nil {
		return Stats{}, fmt.Errorf("query analytics last run: %w", err)
	}
	if raw.Valid {
		out.LastRunAt, _ = time.Parse(time.RFC3339Nano, raw.String)
	}
	if stat, err := os.Stat(s.dbPath); err == nil {
		out.DBSizeBytes = stat.Size()
	}
	return out, nil
}

func (s SQLiteStore) ValueReport(ctx context.Context) (ValueReport, error) {
	conn, err := s.open(ctx)
	if err != nil {
		return ValueReport{}, err
	}
	defer conn.Close()
	var out ValueReport
	row := conn.QueryRowContext(ctx, `
SELECT
  COUNT(*),
  COALESCE(SUM(success), 0),
  COALESCE(SUM(raw_fetch_chars), 0),
  COALESCE(SUM(MAX(raw_fetch_chars - final_context_chars, 0)), 0),
  COALESCE(SUM(proof_usable), 0),
  COALESCE(SUM(CASE WHEN public_bootstrap_used = 0 THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(local_memory_used), 0),
  COALESCE(SUM(topic_node_used), 0),
  COALESCE(SUM(link_count), 0),
  COALESCE(SUM(source_count), 0),
  COALESCE(CAST(AVG(latency_ms) AS INTEGER), 0),
  COALESCE(SUM(final_context_chars), 0),
  COALESCE(SUM(raw_fetch_chars), 0)
FROM analytics_runs
`)
	var totalFinalContext, totalRawChars int64
	if err := row.Scan(
		&out.TotalRuns,
		&out.SuccessfulRuns,
		&out.TotalRawCharsProcessed,
		&out.TotalAgentCharsSaved,
		&out.TotalProofBackedPackets,
		&out.TotalPublicBootstrapsAvoided,
		&out.TotalMemoryReuseEvents,
		&out.TotalTopicRootCorrections,
		&out.TotalLinksExplored,
		&out.TotalSourcesVisited,
		&out.AvgLatencyMS,
		&totalFinalContext,
		&totalRawChars,
	); err != nil {
		return ValueReport{}, fmt.Errorf("query analytics value report: %w", err)
	}
	if totalRawChars > 0 {
		out.ContextCompressionRatio = float64(totalRawChars-totalFinalContext) / float64(totalRawChars)
	}
	if out.TotalRuns > 0 {
		out.ProofBackedRate = float64(out.TotalProofBackedPackets) / float64(out.TotalRuns)
		out.WarmLikeReuseRate = float64(out.TotalMemoryReuseEvents) / float64(out.TotalRuns)
	}
	return out, nil
}

func (s SQLiteStore) RecentRuns(ctx context.Context, limit int) ([]RecentRun, error) {
	if limit <= 0 {
		limit = 20
	}
	conn, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	rows, err := conn.QueryContext(ctx, `
SELECT run_id, completed_at, operation, surface, selected_url, provider, success, latency_ms,
       MAX(raw_fetch_chars - final_context_chars, 0), proof_usable, local_memory_used, public_bootstrap_used
FROM analytics_runs
ORDER BY completed_at DESC
LIMIT ?
`, limit)
	if err != nil {
		return nil, fmt.Errorf("query analytics recent runs: %w", err)
	}
	defer rows.Close()
	out := make([]RecentRun, 0, limit)
	for rows.Next() {
		var item RecentRun
		var completedAt string
		var success, proofUsable, localMemoryUsed, publicBootstrapUsed int
		if err := rows.Scan(&item.RunID, &completedAt, &item.Operation, &item.Surface, &item.SelectedURL, &item.Provider, &success, &item.LatencyMS, &item.CharsSaved, &proofUsable, &localMemoryUsed, &publicBootstrapUsed); err != nil {
			return nil, fmt.Errorf("scan analytics recent run: %w", err)
		}
		item.CompletedAt, _ = time.Parse(time.RFC3339Nano, completedAt)
		item.Success = success == 1
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
	defer conn.Close()
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
	defer rows.Close()
	out := []HostRollup{}
	for rows.Next() {
		var item HostRollup
		if err := rows.Scan(&item.Host, &item.RunCount, &item.SuccessfulRuns, &item.AvgLatencyMS, &item.TotalAgentCharsSaved, &item.ProofBackedRate, &item.PublicBootstrapUsedRate, &item.LocalMemoryUsedRate); err != nil {
			return nil, fmt.Errorf("scan analytics host rollup: %w", err)
		}
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
	defer conn.Close()
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
	defer rows.Close()
	out := []ProviderRollup{}
	for rows.Next() {
		var item ProviderRollup
		if err := rows.Scan(&item.Provider, &item.RunCount, &item.SuccessfulRuns, &item.AvgLatencyMS, &item.TotalAgentCharsSaved, &item.ProofBackedRate, &item.PublicBootstrapUsedRate, &item.LocalMemoryUsedRate); err != nil {
			return nil, fmt.Errorf("scan analytics provider rollup: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate analytics provider rollups: %w", err)
	}
	return out, nil
}

func (s SQLiteStore) open(ctx context.Context) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(s.dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create analytics db dir: %w", err)
	}
	db, err := sql.Open("sqlite", s.dbPath)
	if err != nil {
		return nil, fmt.Errorf("open analytics db: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping analytics db: %w", err)
	}
	if err := s.ensureSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (s SQLiteStore) ensureSchema(ctx context.Context, db *sql.DB) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS analytics_runs (
		  run_id TEXT PRIMARY KEY,
		  started_at TEXT NOT NULL,
		  completed_at TEXT NOT NULL,
		  operation TEXT NOT NULL,
		  surface TEXT NOT NULL,
		  profile TEXT NOT NULL,
		  goal_hash TEXT NOT NULL,
		  goal_length_chars INTEGER NOT NULL,
		  discovery_mode TEXT NOT NULL,
		  seed_present INTEGER NOT NULL,
		  host TEXT NOT NULL DEFAULT '',
		  selected_url TEXT NOT NULL,
		  provider TEXT NOT NULL,
		  success INTEGER NOT NULL,
		  trace_id TEXT NOT NULL,
		  latency_ms INTEGER NOT NULL,
		  packet_bytes INTEGER NOT NULL,
		  final_context_chars INTEGER NOT NULL,
		  chunk_count INTEGER NOT NULL,
		  source_count INTEGER NOT NULL,
		  link_count INTEGER NOT NULL,
		  proof_ref_count INTEGER NOT NULL,
		  proof_usable INTEGER NOT NULL,
		  public_bootstrap_used INTEGER NOT NULL,
		  local_memory_used INTEGER NOT NULL,
		  topic_node_used INTEGER NOT NULL,
		  same_site_recovery_used INTEGER NOT NULL,
		  candidate_count INTEGER NOT NULL,
		  raw_fetch_chars INTEGER NOT NULL,
		  raw_fetch_bytes INTEGER NOT NULL,
		  reduced_chars INTEGER NOT NULL,
		  reduced_node_count INTEGER NOT NULL,
		  memory_document_count INTEGER NOT NULL,
		  memory_embedding_count INTEGER NOT NULL,
		  memory_topic_node_count INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_analytics_runs_completed_at ON analytics_runs(completed_at)`,
		`CREATE INDEX IF NOT EXISTS idx_analytics_runs_operation ON analytics_runs(operation)`,
		`CREATE INDEX IF NOT EXISTS idx_analytics_runs_host ON analytics_runs(host)`,
		`CREATE INDEX IF NOT EXISTS idx_analytics_runs_provider ON analytics_runs(provider)`,
		`CREATE TABLE IF NOT EXISTS analytics_stage_events (
		  id INTEGER PRIMARY KEY AUTOINCREMENT,
		  run_id TEXT NOT NULL,
		  stage TEXT NOT NULL,
		  started_at TEXT NOT NULL,
		  completed_at TEXT NOT NULL,
		  latency_ms INTEGER NOT NULL,
		  item_count INTEGER NOT NULL,
		  status TEXT NOT NULL,
		  metadata_json TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_analytics_stage_events_run_id ON analytics_stage_events(run_id)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensure analytics schema: %w", err)
		}
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE analytics_runs ADD COLUMN host TEXT NOT NULL DEFAULT ''`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
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
