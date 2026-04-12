package memory

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/platform"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	root   string
	dbPath string
}

func NewSQLiteStore(root, relativePath string) SQLiteStore {
	cleanRoot := strings.TrimSpace(root)
	if cleanRoot == "" {
		cleanRoot = platform.DefaultStateRoot()
	}
	cleanPath := strings.TrimSpace(relativePath)
	if cleanPath == "" {
		cleanPath = "discovery/discovery.db"
	}
	if filepath.IsAbs(cleanPath) {
		return SQLiteStore{root: cleanRoot, dbPath: cleanPath}
	}
	return SQLiteStore{root: cleanRoot, dbPath: filepath.Join(cleanRoot, cleanPath)}
}

func (s SQLiteStore) DBPath() string {
	return s.dbPath
}

func (s SQLiteStore) UpsertDocument(ctx context.Context, doc Document) error {
	if strings.TrimSpace(doc.URL) == "" {
		return fmt.Errorf("document url must not be empty")
	}
	if doc.ObservedAt.IsZero() {
		doc.ObservedAt = time.Now().UTC()
	}
	if doc.UpdatedAt.IsZero() {
		doc.UpdatedAt = doc.ObservedAt
	}
	conn, err := s.open(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.ExecContext(ctx, `
INSERT INTO documents (
  url, final_url, host, path, title, semantic_summary, language,
  locality_hints_json, entity_hints_json, category_hints_json, proof_refs_json,
  last_trace_id, source_kind, stable_ratio, novelty_ratio, changed_recently,
  observed_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(url) DO UPDATE SET
  final_url=excluded.final_url,
  host=excluded.host,
  path=excluded.path,
  title=excluded.title,
  semantic_summary=excluded.semantic_summary,
  language=excluded.language,
  locality_hints_json=excluded.locality_hints_json,
  entity_hints_json=excluded.entity_hints_json,
  category_hints_json=excluded.category_hints_json,
  proof_refs_json=excluded.proof_refs_json,
  last_trace_id=excluded.last_trace_id,
  source_kind=excluded.source_kind,
  stable_ratio=excluded.stable_ratio,
  novelty_ratio=excluded.novelty_ratio,
  changed_recently=excluded.changed_recently,
  observed_at=excluded.observed_at,
  updated_at=excluded.updated_at
`,
		doc.URL,
		firstNonEmpty(doc.FinalURL, doc.URL),
		doc.Host,
		doc.Path,
		doc.Title,
		doc.SemanticSummary,
		doc.Language,
		mustJSON(doc.LocalityHints),
		mustJSON(doc.EntityHints),
		mustJSON(doc.CategoryHints),
		mustJSON(doc.ProofRefs),
		doc.LastTraceID,
		doc.SourceKind,
		doc.StableRatio,
		doc.NoveltyRatio,
		boolInt(doc.ChangedRecently),
		doc.ObservedAt.UTC().Format(time.RFC3339Nano),
		doc.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("upsert discovery document: %w", err)
	}
	return nil
}

func (s SQLiteStore) UpsertEdges(ctx context.Context, edges []Edge) error {
	if len(edges) == 0 {
		return nil
	}
	conn, err := s.open(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin discovery edges tx: %w", err)
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO edges (source_url, target_url, anchor_text, same_host, trace_ref, observed_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(source_url, target_url, anchor_text) DO UPDATE SET
  same_host=excluded.same_host,
  trace_ref=excluded.trace_ref,
  observed_at=excluded.observed_at
`)
	if err != nil {
		return fmt.Errorf("prepare discovery edges upsert: %w", err)
	}
	defer stmt.Close()
	for _, edge := range edges {
		if strings.TrimSpace(edge.SourceURL) == "" || strings.TrimSpace(edge.TargetURL) == "" || strings.TrimSpace(edge.AnchorText) == "" {
			continue
		}
		observedAt := edge.ObservedAt
		if observedAt.IsZero() {
			observedAt = time.Now().UTC()
		}
		if _, err := stmt.ExecContext(ctx, edge.SourceURL, edge.TargetURL, edge.AnchorText, boolInt(edge.SameHost), edge.TraceRef, observedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("upsert discovery edge: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit discovery edges tx: %w", err)
	}
	return nil
}

func (s SQLiteStore) UpsertEmbedding(ctx context.Context, emb Embedding, vector []float32) error {
	if strings.TrimSpace(emb.DocumentURL) == "" {
		return fmt.Errorf("embedding document_url must not be empty")
	}
	if emb.CreatedAt.IsZero() {
		emb.CreatedAt = time.Now().UTC()
	}
	if emb.UpdatedAt.IsZero() {
		emb.UpdatedAt = emb.CreatedAt
	}
	blob, err := encodeVector(vector)
	if err != nil {
		return fmt.Errorf("encode embedding vector: %w", err)
	}
	conn, err := s.open(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.ExecContext(ctx, `
INSERT INTO embeddings (embedding_ref, document_url, model, backend, input_text, dimension, vector, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(embedding_ref) DO UPDATE SET
  document_url=excluded.document_url,
  model=excluded.model,
  backend=excluded.backend,
  input_text=excluded.input_text,
  dimension=excluded.dimension,
  vector=excluded.vector,
  updated_at=excluded.updated_at
`,
		emb.EmbeddingRef,
		emb.DocumentURL,
		emb.Model,
		emb.Backend,
		emb.InputText,
		emb.Dimension,
		blob,
		emb.CreatedAt.UTC().Format(time.RFC3339Nano),
		emb.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("upsert discovery embedding: %w", err)
	}
	return nil
}

func (s SQLiteStore) SearchByVector(ctx context.Context, vector []float32, limit int, domainHints []string) ([]Candidate, error) {
	if len(vector) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}
	conn, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	rows, err := conn.QueryContext(ctx, `
SELECT d.url, d.title, d.host, d.proof_refs_json, d.last_trace_id, e.vector
     , d.source_kind, d.stable_ratio, d.novelty_ratio, d.changed_recently, d.observed_at
FROM documents d
JOIN embeddings e ON e.document_url = d.url
`)
	if err != nil {
		return nil, fmt.Errorf("query discovery embeddings: %w", err)
	}
	defer rows.Close()
	hints := normalizeDomainHints(domainHints)
	out := make([]Candidate, 0, limit)
	for rows.Next() {
		var rawURL, title, host, rawProofRefs, traceRef, sourceKind, observedAtRaw string
		var rawVector []byte
		var stableRatio, noveltyRatio float64
		var changedRecently int
		if err := rows.Scan(&rawURL, &title, &host, &rawProofRefs, &traceRef, &rawVector, &sourceKind, &stableRatio, &noveltyRatio, &changedRecently, &observedAtRaw); err != nil {
			return nil, fmt.Errorf("scan discovery embedding row: %w", err)
		}
		storedVector, err := decodeVector(rawVector)
		if err != nil {
			return nil, fmt.Errorf("decode discovery vector: %w", err)
		}
		similarity := cosineSimilarity(vector, storedVector)
		if similarity <= 0 {
			continue
		}
		reasons := []string{"semantic_goal_alignment", "local_memory_hit"}
		score := similarity * 3
		if hasDomainHint(host, hints) {
			score += 0.2
			reasons = append(reasons, "domain_hint_match")
		}
		if observedAt, ok := parseObservedAt(observedAtRaw); ok {
			recencyBoost := recentObservationBoost(observedAt)
			if recencyBoost > 0 {
				score += recencyBoost
				reasons = append(reasons, "recent_local_evidence")
			}
		}
		if stableRatio > 0 {
			score += stableRatio * 0.08
			reasons = append(reasons, "stable_page")
		}
		if noveltyRatio > 0 {
			score += noveltyRatio * 0.08
			reasons = append(reasons, "novel_page")
		}
		if changedRecently == 1 {
			score += 0.06
			reasons = append(reasons, "changed_recently")
		}
		proofRefs := decodeStringSlice(rawProofRefs)
		candidate := Candidate{
			URL:             rawURL,
			Title:           title,
			Host:            host,
			Score:           score,
			Reasons:         reasons,
			TraceRef:        traceRef,
			Source:          firstNonEmpty(sourceKind, "discovery_memory"),
			Distance:        1 - similarity,
			ObservedAt:      parseObservedAtOrZero(observedAtRaw),
			StableRatio:     stableRatio,
			NoveltyRatio:    noveltyRatio,
			ChangedRecently: changedRecently == 1,
		}
		if len(proofRefs) > 0 {
			candidate.ProofRef = proofRefs[0]
			candidate.Score += 0.08
			candidate.Reasons = append(candidate.Reasons, "proof_backed_page")
		}
		out = append(out, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate discovery embeddings: %w", err)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].URL < out[j].URL
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s SQLiteStore) ExpandNeighbors(ctx context.Context, urls []string, limit int) ([]Candidate, error) {
	clean := compactURLs(urls)
	if len(clean) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}
	conn, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	out := make([]Candidate, 0, limit)
	seen := map[string]struct{}{}
	for _, sourceURL := range clean {
		rows, err := conn.QueryContext(ctx, `
SELECT d.url, d.title, d.host, d.proof_refs_json, d.last_trace_id, e.anchor_text, e.same_host
FROM edges e
JOIN documents d ON d.url = e.target_url
WHERE e.source_url = ?
ORDER BY e.same_host DESC, e.observed_at DESC
LIMIT ?
`, sourceURL, limit)
		if err != nil {
			return nil, fmt.Errorf("expand discovery neighbors: %w", err)
		}
		for rows.Next() {
			var rawURL, title, host, rawProofRefs, traceRef, anchor string
			var sameHost int
			if err := rows.Scan(&rawURL, &title, &host, &rawProofRefs, &traceRef, &anchor, &sameHost); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan discovery neighbor: %w", err)
			}
			if _, ok := seen[rawURL]; ok {
				continue
			}
			seen[rawURL] = struct{}{}
			proofRefs := decodeStringSlice(rawProofRefs)
			reasons := []string{"graph_neighbor"}
			score := 0.7
			if sameHost == 1 {
				score += 0.2
				reasons = append(reasons, "same_host")
			}
			candidate := Candidate{URL: rawURL, Title: firstNonEmpty(strings.TrimSpace(anchor), title), Host: host, Score: score, Reasons: reasons, TraceRef: traceRef, Source: "discovery_memory_graph"}
			if len(proofRefs) > 0 {
				candidate.ProofRef = proofRefs[0]
			}
			out = append(out, candidate)
		}
		rows.Close()
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].URL < out[j].URL
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s SQLiteStore) ExpandHosts(ctx context.Context, hosts []string, limit int) ([]Candidate, error) {
	cleanHosts := normalizeDomainHints(hosts)
	if len(cleanHosts) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}
	conn, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	out := make([]Candidate, 0, limit)
	seen := map[string]struct{}{}
	for _, host := range cleanHosts {
		rows, err := conn.QueryContext(ctx, `
SELECT url, title, host, proof_refs_json, last_trace_id, source_kind, stable_ratio, novelty_ratio, changed_recently, observed_at
FROM documents
WHERE host = ?
ORDER BY observed_at DESC, LENGTH(path) ASC
LIMIT ?
`, host, limit)
		if err != nil {
			return nil, fmt.Errorf("expand discovery hosts: %w", err)
		}
		for rows.Next() {
			var rawURL, title, rowHost, rawProofRefs, traceRef, sourceKind, observedAtRaw string
			var stableRatio, noveltyRatio float64
			var changedRecently int
			if err := rows.Scan(&rawURL, &title, &rowHost, &rawProofRefs, &traceRef, &sourceKind, &stableRatio, &noveltyRatio, &changedRecently, &observedAtRaw); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan discovery host expansion row: %w", err)
			}
			if _, ok := seen[rawURL]; ok {
				continue
			}
			seen[rawURL] = struct{}{}
			reasons := []string{"host_memory_recall"}
			score := 0.78
			if observedAt, ok := parseObservedAt(observedAtRaw); ok {
				recencyBoost := recentObservationBoost(observedAt)
				if recencyBoost > 0 {
					score += recencyBoost
					reasons = append(reasons, "recent_local_evidence")
				}
			}
			if stableRatio > 0 {
				score += stableRatio * 0.05
				reasons = append(reasons, "stable_page")
			}
			if noveltyRatio > 0 {
				score += noveltyRatio * 0.05
				reasons = append(reasons, "novel_page")
			}
			candidate := Candidate{
				URL:             rawURL,
				Title:           title,
				Host:            rowHost,
				Score:           score,
				Reasons:         reasons,
				TraceRef:        traceRef,
				Source:          firstNonEmpty(sourceKind, "discovery_memory_host"),
				ObservedAt:      parseObservedAtOrZero(observedAtRaw),
				StableRatio:     stableRatio,
				NoveltyRatio:    noveltyRatio,
				ChangedRecently: changedRecently == 1,
			}
			proofRefs := decodeStringSlice(rawProofRefs)
			if len(proofRefs) > 0 {
				candidate.ProofRef = proofRefs[0]
				candidate.Score += 0.06
				candidate.Reasons = append(candidate.Reasons, "proof_backed_page")
			}
			out = append(out, candidate)
		}
		rows.Close()
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].URL < out[j].URL
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s SQLiteStore) GetStats(ctx context.Context) (Stats, error) {
	conn, err := s.open(ctx)
	if err != nil {
		return Stats{}, err
	}
	defer conn.Close()
	stats := Stats{DBPath: s.dbPath}
	for query, target := range map[string]*int{
		"SELECT COUNT(*) FROM documents":  &stats.DocumentCount,
		"SELECT COUNT(*) FROM edges":      &stats.EdgeCount,
		"SELECT COUNT(*) FROM embeddings": &stats.EmbeddingCount,
	} {
		if err := conn.QueryRowContext(ctx, query).Scan(target); err != nil {
			return Stats{}, fmt.Errorf("query discovery stats: %w", err)
		}
	}
	var lastObserved sql.NullString
	if err := conn.QueryRowContext(ctx, "SELECT MAX(observed_at) FROM documents").Scan(&lastObserved); err != nil {
		return Stats{}, fmt.Errorf("query discovery last_observed_at: %w", err)
	}
	if lastObserved.Valid {
		stats.LastObservedAt, _ = time.Parse(time.RFC3339Nano, lastObserved.String)
	}
	var lastRebuild sql.NullString
	if err := conn.QueryRowContext(ctx, "SELECT value FROM memory_state WHERE key = 'vector_index_rebuilt_at'").Scan(&lastRebuild); err == nil && lastRebuild.Valid {
		stats.LastRebuildAt, _ = time.Parse(time.RFC3339Nano, lastRebuild.String)
	}
	return stats, nil
}

func (s SQLiteStore) Prune(ctx context.Context, policy PrunePolicy) error {
	conn, err := s.open(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	return withTx(ctx, conn, func(tx *sql.Tx) error {
		if err := pruneTable(ctx, tx, "documents", "url", "observed_at", policy.MaxDocuments); err != nil {
			return err
		}
		if err := pruneTable(ctx, tx, "edges", "source_url || '|' || target_url || '|' || anchor_text", "observed_at", policy.MaxEdges); err != nil {
			return err
		}
		if err := pruneTable(ctx, tx, "embeddings", "embedding_ref", "updated_at", policy.MaxEmbeddings); err != nil {
			return err
		}
		return nil
	})
}

func (s SQLiteStore) RebuildIndex(ctx context.Context) error {
	conn, err := s.open(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for key, value := range map[string]string{
		"vector_index_rebuilt_at": now,
		"vector_engine":           "linear_fallback",
	} {
		if _, err := conn.ExecContext(ctx, `
INSERT INTO memory_state (key, value, updated_at) VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at
`, key, value, now); err != nil {
			return fmt.Errorf("rebuild discovery memory index state: %w", err)
		}
	}
	return nil
}

func (s SQLiteStore) ExportJSONL(ctx context.Context, dir string) (ExportStats, error) {
	cleanDir := strings.TrimSpace(dir)
	if cleanDir == "" {
		return ExportStats{}, fmt.Errorf("export dir must not be empty")
	}
	conn, err := s.open(ctx)
	if err != nil {
		return ExportStats{}, err
	}
	defer conn.Close()
	if err := os.MkdirAll(cleanDir, 0o755); err != nil {
		return ExportStats{}, fmt.Errorf("create memory export dir: %w", err)
	}
	stats := ExportStats{
		DocumentsPath:  filepath.Join(cleanDir, "documents.jsonl"),
		EdgesPath:      filepath.Join(cleanDir, "edges.jsonl"),
		EmbeddingsPath: filepath.Join(cleanDir, "embeddings.jsonl"),
	}
	if stats.DocumentCount, err = exportDocuments(ctx, conn, stats.DocumentsPath); err != nil {
		return ExportStats{}, err
	}
	if stats.EdgeCount, err = exportEdges(ctx, conn, stats.EdgesPath); err != nil {
		return ExportStats{}, err
	}
	if stats.EmbeddingCount, err = exportEmbeddings(ctx, conn, stats.EmbeddingsPath); err != nil {
		return ExportStats{}, err
	}
	return stats, nil
}

func (s SQLiteStore) ImportJSONL(ctx context.Context, dir string) (ImportStats, error) {
	cleanDir := strings.TrimSpace(dir)
	if cleanDir == "" {
		return ImportStats{}, fmt.Errorf("import dir must not be empty")
	}
	stats := ImportStats{}
	if count, err := importDocuments(ctx, s, filepath.Join(cleanDir, "documents.jsonl")); err != nil {
		return ImportStats{}, err
	} else {
		stats.DocumentCount = count
	}
	if count, err := importEdges(ctx, s, filepath.Join(cleanDir, "edges.jsonl")); err != nil {
		return ImportStats{}, err
	} else {
		stats.EdgeCount = count
	}
	if count, err := importEmbeddings(ctx, s, filepath.Join(cleanDir, "embeddings.jsonl")); err != nil {
		return ImportStats{}, err
	} else {
		stats.EmbeddingCount = count
	}
	return stats, nil
}

func (s SQLiteStore) open(ctx context.Context) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(s.dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create discovery db dir: %w", err)
	}
	db, err := sql.Open("sqlite", s.dbPath)
	if err != nil {
		return nil, fmt.Errorf("open discovery db: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping discovery db: %w", err)
	}
	if err := s.ensureSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (s SQLiteStore) ensureSchema(ctx context.Context, db *sql.DB) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS documents (
		  url TEXT PRIMARY KEY,
		  final_url TEXT NOT NULL,
		  host TEXT NOT NULL,
		  path TEXT NOT NULL,
		  title TEXT NOT NULL,
		  semantic_summary TEXT NOT NULL,
		  language TEXT,
		  locality_hints_json TEXT NOT NULL,
		  entity_hints_json TEXT NOT NULL,
		  category_hints_json TEXT NOT NULL,
		  proof_refs_json TEXT NOT NULL,
		  last_trace_id TEXT NOT NULL,
		  source_kind TEXT NOT NULL,
		  stable_ratio REAL NOT NULL,
		  novelty_ratio REAL NOT NULL,
		  changed_recently INTEGER NOT NULL,
		  observed_at TEXT NOT NULL,
		  updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_documents_host ON documents(host)`,
		`CREATE INDEX IF NOT EXISTS idx_documents_observed_at ON documents(observed_at)`,
		`CREATE TABLE IF NOT EXISTS edges (
		  source_url TEXT NOT NULL,
		  target_url TEXT NOT NULL,
		  anchor_text TEXT NOT NULL,
		  same_host INTEGER NOT NULL,
		  trace_ref TEXT NOT NULL,
		  observed_at TEXT NOT NULL,
		  PRIMARY KEY (source_url, target_url, anchor_text)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_edges_source_url ON edges(source_url)`,
		`CREATE INDEX IF NOT EXISTS idx_edges_target_url ON edges(target_url)`,
		`CREATE TABLE IF NOT EXISTS embeddings (
		  embedding_ref TEXT PRIMARY KEY,
		  document_url TEXT NOT NULL UNIQUE,
		  model TEXT NOT NULL,
		  backend TEXT NOT NULL,
		  input_text TEXT NOT NULL,
		  dimension INTEGER NOT NULL,
		  vector BLOB NOT NULL,
		  created_at TEXT NOT NULL,
		  updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS memory_state (
		  key TEXT PRIMARY KEY,
		  value TEXT NOT NULL,
		  updated_at TEXT NOT NULL
		)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensure discovery schema: %w", err)
		}
	}
	return nil
}

func withTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func pruneTable(ctx context.Context, tx *sql.Tx, table, keyExpr, orderColumn string, maxCount int) error {
	if maxCount <= 0 {
		return nil
	}
	query := fmt.Sprintf(`
DELETE FROM %s
WHERE %s IN (
  SELECT %s FROM %s
  ORDER BY %s DESC
  LIMIT -1 OFFSET ?
)`, table, keyExpr, keyExpr, table, orderColumn)
	if _, err := tx.ExecContext(ctx, query, maxCount); err != nil {
		return fmt.Errorf("prune %s: %w", table, err)
	}
	return nil
}

func mustJSON(values []string) string {
	data, _ := json.Marshal(compactStrings(values))
	return string(data)
}

func decodeStringSlice(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return compactStrings(out)
}

func compactStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func compactURLs(values []string) []string {
	return compactStrings(values)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func normalizeDomainHints(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range compactStrings(values) {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func hasDomainHint(host string, hints []string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	for _, hint := range hints {
		if hint == host {
			return true
		}
		if strings.HasSuffix(host, "."+hint) {
			return true
		}
	}
	return false
}

func parseObservedAt(raw string) (time.Time, bool) {
	value, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw))
	if err != nil || value.IsZero() {
		return time.Time{}, false
	}
	return value, true
}

func parseObservedAtOrZero(raw string) time.Time {
	value, _ := parseObservedAt(raw)
	return value
}

func recentObservationBoost(observedAt time.Time) float64 {
	if observedAt.IsZero() {
		return 0
	}
	age := time.Since(observedAt)
	switch {
	case age <= 24*time.Hour:
		return 0.12
	case age <= 7*24*time.Hour:
		return 0.08
	case age <= 30*24*time.Hour:
		return 0.04
	default:
		return 0
	}
}

type exportDocumentRow struct {
	URL             string   `json:"url"`
	FinalURL        string   `json:"final_url"`
	Host            string   `json:"host"`
	Path            string   `json:"path"`
	Title           string   `json:"title"`
	SemanticSummary string   `json:"semantic_summary"`
	Language        string   `json:"language,omitempty"`
	LocalityHints   []string `json:"locality_hints,omitempty"`
	EntityHints     []string `json:"entity_hints,omitempty"`
	CategoryHints   []string `json:"category_hints,omitempty"`
	ProofRefs       []string `json:"proof_refs,omitempty"`
	LastTraceID     string   `json:"last_trace_id,omitempty"`
	SourceKind      string   `json:"source_kind"`
	StableRatio     float64  `json:"stable_ratio,omitempty"`
	NoveltyRatio    float64  `json:"novelty_ratio,omitempty"`
	ChangedRecently bool     `json:"changed_recently,omitempty"`
	ObservedAt      string   `json:"observed_at"`
	UpdatedAt       string   `json:"updated_at"`
}

type exportEdgeRow struct {
	SourceURL  string `json:"source_url"`
	TargetURL  string `json:"target_url"`
	AnchorText string `json:"anchor_text"`
	SameHost   bool   `json:"same_host"`
	TraceRef   string `json:"trace_ref,omitempty"`
	ObservedAt string `json:"observed_at"`
}

type exportEmbeddingRow struct {
	EmbeddingRef string    `json:"embedding_ref"`
	DocumentURL  string    `json:"document_url"`
	Model        string    `json:"model"`
	Backend      string    `json:"backend"`
	InputText    string    `json:"input_text"`
	Dimension    int       `json:"dimension"`
	Vector       []float32 `json:"vector"`
	CreatedAt    string    `json:"created_at"`
	UpdatedAt    string    `json:"updated_at"`
}

func exportDocuments(ctx context.Context, conn *sql.DB, path string) (int, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT url, final_url, host, path, title, semantic_summary, language,
       locality_hints_json, entity_hints_json, category_hints_json, proof_refs_json,
       last_trace_id, source_kind, stable_ratio, novelty_ratio, changed_recently,
       observed_at, updated_at
FROM documents
ORDER BY observed_at DESC, url ASC
`)
	if err != nil {
		return 0, fmt.Errorf("query discovery documents export: %w", err)
	}
	defer rows.Close()
	return writeJSONL(path, rows, func() (exportDocumentRow, error) {
		var row exportDocumentRow
		var rawLocality, rawEntity, rawCategory, rawProof string
		var changed int
		if err := rows.Scan(
			&row.URL, &row.FinalURL, &row.Host, &row.Path, &row.Title, &row.SemanticSummary, &row.Language,
			&rawLocality, &rawEntity, &rawCategory, &rawProof,
			&row.LastTraceID, &row.SourceKind, &row.StableRatio, &row.NoveltyRatio, &changed,
			&row.ObservedAt, &row.UpdatedAt,
		); err != nil {
			return exportDocumentRow{}, err
		}
		row.LocalityHints = decodeStringSlice(rawLocality)
		row.EntityHints = decodeStringSlice(rawEntity)
		row.CategoryHints = decodeStringSlice(rawCategory)
		row.ProofRefs = decodeStringSlice(rawProof)
		row.ChangedRecently = changed == 1
		return row, nil
	})
}

func exportEdges(ctx context.Context, conn *sql.DB, path string) (int, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT source_url, target_url, anchor_text, same_host, trace_ref, observed_at
FROM edges
ORDER BY observed_at DESC, source_url ASC, target_url ASC
`)
	if err != nil {
		return 0, fmt.Errorf("query discovery edges export: %w", err)
	}
	defer rows.Close()
	return writeJSONL(path, rows, func() (exportEdgeRow, error) {
		var row exportEdgeRow
		var sameHost int
		if err := rows.Scan(&row.SourceURL, &row.TargetURL, &row.AnchorText, &sameHost, &row.TraceRef, &row.ObservedAt); err != nil {
			return exportEdgeRow{}, err
		}
		row.SameHost = sameHost == 1
		return row, nil
	})
}

func exportEmbeddings(ctx context.Context, conn *sql.DB, path string) (int, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT embedding_ref, document_url, model, backend, input_text, dimension, vector, created_at, updated_at
FROM embeddings
ORDER BY updated_at DESC, embedding_ref ASC
`)
	if err != nil {
		return 0, fmt.Errorf("query discovery embeddings export: %w", err)
	}
	defer rows.Close()
	return writeJSONL(path, rows, func() (exportEmbeddingRow, error) {
		var row exportEmbeddingRow
		var rawVector []byte
		if err := rows.Scan(&row.EmbeddingRef, &row.DocumentURL, &row.Model, &row.Backend, &row.InputText, &row.Dimension, &rawVector, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return exportEmbeddingRow{}, err
		}
		vector, err := decodeVector(rawVector)
		if err != nil {
			return exportEmbeddingRow{}, err
		}
		row.Vector = vector
		return row, nil
	})
}

func writeJSONL[T any](path string, rows *sql.Rows, next func() (T, error)) (int, error) {
	file, err := os.Create(path)
	if err != nil {
		return 0, fmt.Errorf("create jsonl export %s: %w", path, err)
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	defer writer.Flush()
	count := 0
	for rows.Next() {
		row, err := next()
		if err != nil {
			return count, fmt.Errorf("scan jsonl export row: %w", err)
		}
		data, err := json.Marshal(row)
		if err != nil {
			return count, fmt.Errorf("encode jsonl export row: %w", err)
		}
		if _, err := writer.Write(append(data, '\n')); err != nil {
			return count, fmt.Errorf("write jsonl export row: %w", err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return count, fmt.Errorf("iterate jsonl export rows: %w", err)
	}
	return count, nil
}

func importDocuments(ctx context.Context, store SQLiteStore, path string) (int, error) {
	return readJSONL(path, func(line []byte) error {
		var row exportDocumentRow
		if err := json.Unmarshal(line, &row); err != nil {
			return err
		}
		observedAt, _ := time.Parse(time.RFC3339Nano, row.ObservedAt)
		updatedAt, _ := time.Parse(time.RFC3339Nano, row.UpdatedAt)
		return store.UpsertDocument(ctx, Document{
			URL:             row.URL,
			FinalURL:        row.FinalURL,
			Host:            row.Host,
			Path:            row.Path,
			Title:           row.Title,
			SemanticSummary: row.SemanticSummary,
			Language:        row.Language,
			LocalityHints:   row.LocalityHints,
			EntityHints:     row.EntityHints,
			CategoryHints:   row.CategoryHints,
			ProofRefs:       row.ProofRefs,
			LastTraceID:     row.LastTraceID,
			SourceKind:      row.SourceKind,
			StableRatio:     row.StableRatio,
			NoveltyRatio:    row.NoveltyRatio,
			ChangedRecently: row.ChangedRecently,
			ObservedAt:      observedAt,
			UpdatedAt:       updatedAt,
		})
	})
}

func importEdges(ctx context.Context, store SQLiteStore, path string) (int, error) {
	buffer := make([]Edge, 0, 32)
	count, err := readJSONL(path, func(line []byte) error {
		var row exportEdgeRow
		if err := json.Unmarshal(line, &row); err != nil {
			return err
		}
		observedAt, _ := time.Parse(time.RFC3339Nano, row.ObservedAt)
		buffer = append(buffer, Edge{
			SourceURL:  row.SourceURL,
			TargetURL:  row.TargetURL,
			AnchorText: row.AnchorText,
			SameHost:   row.SameHost,
			TraceRef:   row.TraceRef,
			ObservedAt: observedAt,
		})
		return nil
	})
	if err != nil {
		return count, err
	}
	if err := store.UpsertEdges(ctx, buffer); err != nil {
		return count, err
	}
	return count, nil
}

func importEmbeddings(ctx context.Context, store SQLiteStore, path string) (int, error) {
	return readJSONL(path, func(line []byte) error {
		var row exportEmbeddingRow
		if err := json.Unmarshal(line, &row); err != nil {
			return err
		}
		createdAt, _ := time.Parse(time.RFC3339Nano, row.CreatedAt)
		updatedAt, _ := time.Parse(time.RFC3339Nano, row.UpdatedAt)
		return store.UpsertEmbedding(ctx, Embedding{
			EmbeddingRef: row.EmbeddingRef,
			DocumentURL:  row.DocumentURL,
			Model:        row.Model,
			Backend:      row.Backend,
			InputText:    row.InputText,
			Dimension:    row.Dimension,
			CreatedAt:    createdAt,
			UpdatedAt:    updatedAt,
		}, row.Vector)
	})
}

func readJSONL(path string, consume func([]byte) error) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open jsonl import %s: %w", path, err)
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	count := 0
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return count, fmt.Errorf("read jsonl import line: %w", err)
		}
		line = []byte(strings.TrimSpace(string(line)))
		if len(line) > 0 {
			if err := consume(line); err != nil {
				return count, fmt.Errorf("consume jsonl import row: %w", err)
			}
			count++
		}
		if err == io.EOF {
			break
		}
	}
	return count, nil
}

func hostPath(raw string) (string, string) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", ""
	}
	return strings.ToLower(parsed.Hostname()), firstNonEmpty(parsed.EscapedPath(), "/")
}
