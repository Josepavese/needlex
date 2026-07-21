package analytics

import (
	"context"
	"database/sql"
	"fmt"
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
	FailureClass         string
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

type EmbeddingCacheCounters struct {
	Hits         uint64  `json:"hits"`
	Misses       uint64  `json:"misses"`
	Writes       uint64  `json:"writes"`
	NegativeHits uint64  `json:"negative_hits"`
	StaleHits    uint64  `json:"stale_hits"`
	Evictions    uint64  `json:"evictions"`
	EvictedBytes uint64  `json:"evicted_bytes"`
	HitRate      float64 `json:"hit_rate,omitempty"`
}

type Stats struct {
	RunCount               int                    `json:"run_count"`
	SuccessfulRuns         int                    `json:"successful_runs"`
	QueryRuns              int                    `json:"query_runs"`
	ReadRuns               int                    `json:"read_runs"`
	CrawlRuns              int                    `json:"crawl_runs"`
	StageEventCount        int                    `json:"stage_event_count"`
	TotalRawCharsProcessed int64                  `json:"total_raw_chars_processed"`
	TotalAgentCharsSaved   int64                  `json:"total_agent_chars_saved"`
	TokenEstimateMethod    string                 `json:"token_estimate_method"`
	CharsPerTokenEstimate  float64                `json:"chars_per_token_estimate"`
	TotalAgentTokensSaved  int64                  `json:"total_agent_tokens_saved_estimated"`
	EstimatedCostSavedUSD  CostSavingsUSD         `json:"estimated_cost_saved_usd"`
	EmbeddingCache         EmbeddingCacheCounters `json:"embedding_cache"`
	LastRunAt              time.Time              `json:"last_run_at,omitempty"`
	DBPath                 string                 `json:"db_path"`
	DBSizeBytes            int64                  `json:"db_size_bytes"`
}

type ValueReport struct {
	TotalRuns                    int            `json:"total_runs"`
	SuccessfulRuns               int            `json:"successful_runs"`
	TotalRawCharsProcessed       int64          `json:"total_raw_chars_processed"`
	TotalAgentCharsSaved         int64          `json:"total_agent_chars_saved"`
	TotalAgentCharsDelivered     int64          `json:"total_agent_chars_delivered"`
	TokenEstimateMethod          string         `json:"token_estimate_method"`
	CharsPerTokenEstimate        float64        `json:"chars_per_token_estimate"`
	TotalRawTokensEstimated      int64          `json:"total_raw_tokens_estimated"`
	TotalAgentTokensSaved        int64          `json:"total_agent_tokens_saved_estimated"`
	TotalAgentTokensDelivered    int64          `json:"total_agent_tokens_delivered_estimated"`
	EstimatedCostSavedUSD        CostSavingsUSD `json:"estimated_cost_saved_usd"`
	TotalProofBackedPackets      int            `json:"total_proof_backed_packets"`
	TotalPublicBootstrapsAvoided int            `json:"total_public_bootstraps_avoided"`
	TotalMemoryReuseEvents       int            `json:"total_memory_reuse_events"`
	TotalTopicRootCorrections    int            `json:"total_topic_root_corrections"`
	TotalLinksExplored           int64          `json:"total_links_explored"`
	TotalSourcesVisited          int64          `json:"total_sources_visited"`
	AvgLatencyMS                 int64          `json:"avg_latency_ms"`
	ContextCompressionRatio      float64        `json:"context_compression_ratio"`
	ContextCompressionFactor     float64        `json:"context_compression_factor"`
	ProofBackedRate              float64        `json:"proof_backed_rate"`
	WarmLikeReuseRate            float64        `json:"warm_like_reuse_rate"`
}

type RecentRun struct {
	RunID                string    `json:"run_id"`
	CompletedAt          time.Time `json:"completed_at"`
	Operation            string    `json:"operation"`
	Surface              string    `json:"surface"`
	SelectedURL          string    `json:"selected_url,omitempty"`
	Provider             string    `json:"provider,omitempty"`
	FailureClass         string    `json:"failure_class,omitempty"`
	Success              bool      `json:"success"`
	LatencyMS            int64     `json:"latency_ms"`
	CharsSaved           int       `json:"chars_saved"`
	TokensSavedEstimated int64     `json:"tokens_saved_estimated"`
	ProofUsable          bool      `json:"proof_usable"`
	LocalMemoryUsed      bool      `json:"local_memory_used"`
	PublicBootstrapUsed  bool      `json:"public_bootstrap_used"`
}

type HostRollup struct {
	Host                    string  `json:"host"`
	RunCount                int     `json:"run_count"`
	SuccessfulRuns          int     `json:"successful_runs"`
	AvgLatencyMS            int64   `json:"avg_latency_ms"`
	TotalAgentCharsSaved    int64   `json:"total_agent_chars_saved"`
	TotalAgentTokensSaved   int64   `json:"total_agent_tokens_saved_estimated"`
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
	TotalAgentTokensSaved   int64   `json:"total_agent_tokens_saved_estimated"`
	ProofBackedRate         float64 `json:"proof_backed_rate"`
	PublicBootstrapUsedRate float64 `json:"public_bootstrap_used_rate"`
	LocalMemoryUsedRate     float64 `json:"local_memory_used_rate"`
}

type FailureRollup struct {
	FailureClass string `json:"failure_class"`
	RunCount     int    `json:"run_count"`
	AvgLatencyMS int64  `json:"avg_latency_ms"`
	LastSeenAt   string `json:"last_seen_at,omitempty"`
}

type DailyRollup struct {
	Day                     string  `json:"day"`
	RunCount                int     `json:"run_count"`
	SuccessfulRuns          int     `json:"successful_runs"`
	AvgLatencyMS            int64   `json:"avg_latency_ms"`
	TotalAgentCharsSaved    int64   `json:"total_agent_chars_saved"`
	TotalAgentTokensSaved   int64   `json:"total_agent_tokens_saved_estimated"`
	ProofBackedRate         float64 `json:"proof_backed_rate"`
	PublicBootstrapUsedRate float64 `json:"public_bootstrap_used_rate"`
	LocalMemoryUsedRate     float64 `json:"local_memory_used_rate"`
}

type ExportStats struct {
	Directory       string `json:"directory"`
	RunsPath        string `json:"runs_path"`
	StagesPath      string `json:"stages_path"`
	HostsPath       string `json:"hosts_path"`
	ProvidersPath   string `json:"providers_path"`
	FailuresPath    string `json:"failures_path"`
	DailyPath       string `json:"daily_path"`
	ValueReportPath string `json:"value_report_path"`
	RunCount        int    `json:"run_count"`
	StageCount      int    `json:"stage_count"`
	HostCount       int    `json:"host_count"`
	ProviderCount   int    `json:"provider_count"`
	FailureCount    int    `json:"failure_count"`
	DailyCount      int    `json:"daily_count"`
}

type SQLiteStore struct {
	dbPath string
}

func NewSQLiteStore(root string) SQLiteStore {
	cleanRoot := strings.TrimSpace(root)
	if cleanRoot == "" {
		cleanRoot = platform.DefaultStateRoot()
	}
	return SQLiteStore{dbPath: platform.NewStateLayout(cleanRoot).AnalyticsDB}
}

func (s SQLiteStore) DBPath() string { return s.dbPath }

func (s SQLiteStore) AppendRun(ctx context.Context, run RunRecord, stages []StageEvent) error {
	run = normalizeRunRecord(run)
	conn, err := s.open(ctx)
	if err != nil {
		return err
	}
	defer platform.Close(conn)
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin analytics tx: %w", err)
	}
	defer platform.Rollback(tx)
	if err := s.upsertRun(ctx, tx, run); err != nil {
		return err
	}
	if err := s.insertStageEvents(ctx, tx, stages); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit analytics tx: %w", err)
	}
	return nil
}

func normalizeRunRecord(run RunRecord) RunRecord {
	if strings.TrimSpace(run.Host) == "" {
		run.Host = hostFromURL(run.SelectedURL)
	}
	return run
}

func (s SQLiteStore) upsertRun(ctx context.Context, tx *sql.Tx, run RunRecord) error {
	_, err := tx.ExecContext(ctx, upsertRunSQL, runRecordArgs(run)...)
	if err != nil {
		return fmt.Errorf("upsert analytics run: %w", err)
	}
	return nil
}
