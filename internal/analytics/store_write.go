package analytics

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/josepavese/needlex/internal/platform"
	_ "modernc.org/sqlite"
)

const upsertRunSQL = `
INSERT INTO analytics_runs (
  run_id, started_at, completed_at, operation, surface, profile, goal_hash, goal_length_chars,
  discovery_mode, seed_present, host, selected_url, provider, failure_class, success, trace_id, latency_ms,
  packet_bytes, final_context_chars, chunk_count, source_count, link_count, proof_ref_count,
  proof_usable, public_bootstrap_used, local_memory_used, topic_node_used, same_site_recovery_used,
  candidate_count, raw_fetch_chars, raw_fetch_bytes, reduced_chars, reduced_node_count,
  memory_document_count, memory_embedding_count, memory_topic_node_count
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
  failure_class=excluded.failure_class,
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
`

func runRecordArgs(run RunRecord) []any {
	return []any{
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
		run.FailureClass,
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
	}
}

func (s SQLiteStore) insertStageEvents(ctx context.Context, tx *sql.Tx, stages []StageEvent) error {
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
	return nil
}

func (s SQLiteStore) Stats(ctx context.Context) (Stats, error) {
	conn, err := s.open(ctx)
	if err != nil {
		return Stats{}, err
	}
	defer platform.Close(conn)
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
	if err := conn.QueryRowContext(ctx, `
SELECT
  COALESCE(SUM(raw_fetch_chars), 0),
  COALESCE(SUM(MAX(raw_fetch_chars - final_context_chars, 0)), 0)
FROM analytics_runs
`).Scan(&out.TotalRawCharsProcessed, &out.TotalAgentCharsSaved); err != nil {
		return Stats{}, fmt.Errorf("query analytics value stats: %w", err)
	}
	out.TokenEstimateMethod = TokenEstimateMethod
	out.CharsPerTokenEstimate = CharsPerToken
	out.TotalAgentTokensSaved = TokenEstimateFromChars(out.TotalAgentCharsSaved)
	out.EstimatedCostSavedUSD = CostSavingsFromTokens(out.TotalAgentTokensSaved)
	cacheCounters, err := s.embeddingCacheCounters(ctx, conn)
	if err != nil {
		return Stats{}, err
	}
	out.EmbeddingCache = cacheCounters
	if stat, err := os.Stat(s.dbPath); err == nil {
		out.DBSizeBytes = stat.Size()
	}
	return out, nil
}

func (s SQLiteStore) embeddingCacheCounters(ctx context.Context, conn *sql.DB) (EmbeddingCacheCounters, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT metadata_json
FROM analytics_stage_events
WHERE stage = 'embedding.cache'
`)
	if err != nil {
		return EmbeddingCacheCounters{}, fmt.Errorf("query embedding cache analytics: %w", err)
	}
	defer platform.Close(rows)
	var out EmbeddingCacheCounters
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return EmbeddingCacheCounters{}, fmt.Errorf("scan embedding cache analytics: %w", err)
		}
		var meta map[string]string
		if err := json.Unmarshal([]byte(raw), &meta); err != nil {
			continue
		}
		out.Hits += uint64(atoi(meta["hits"]))
		out.Misses += uint64(atoi(meta["misses"]))
		out.Writes += uint64(atoi(meta["writes"]))
		out.NegativeHits += uint64(atoi(meta["negative_hits"]))
		out.StaleHits += uint64(atoi(meta["stale_hits"]))
		out.Evictions += uint64(atoi(meta["evictions"]))
		out.EvictedBytes += uint64(atoi(meta["evicted_bytes"]))
	}
	if err := rows.Err(); err != nil {
		return EmbeddingCacheCounters{}, fmt.Errorf("iterate embedding cache analytics: %w", err)
	}
	totalLookups := out.Hits + out.Misses
	if totalLookups > 0 {
		out.HitRate = float64(out.Hits) / float64(totalLookups)
	}
	return out, nil
}

func (s SQLiteStore) ValueReport(ctx context.Context) (ValueReport, error) {
	conn, err := s.open(ctx)
	if err != nil {
		return ValueReport{}, err
	}
	defer platform.Close(conn)
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
	out.TotalAgentCharsDelivered = totalFinalContext
	out.TokenEstimateMethod = TokenEstimateMethod
	out.CharsPerTokenEstimate = CharsPerToken
	out.TotalRawTokensEstimated = TokenEstimateFromChars(totalRawChars)
	out.TotalAgentTokensSaved = TokenEstimateFromChars(out.TotalAgentCharsSaved)
	out.TotalAgentTokensDelivered = TokenEstimateFromChars(totalFinalContext)
	out.EstimatedCostSavedUSD = CostSavingsFromTokens(out.TotalAgentTokensSaved)
	if totalFinalContext > 0 {
		out.ContextCompressionFactor = float64(totalRawChars) / float64(totalFinalContext)
	}
	if out.TotalRuns > 0 {
		out.ProofBackedRate = float64(out.TotalProofBackedPackets) / float64(out.TotalRuns)
		out.WarmLikeReuseRate = float64(out.TotalMemoryReuseEvents) / float64(out.TotalRuns)
	}
	return out, nil
}
