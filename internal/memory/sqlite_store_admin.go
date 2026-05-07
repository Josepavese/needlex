package memory

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/platform"
)

func (s SQLiteStore) GetStats(ctx context.Context) (Stats, error) {
	conn, err := s.open(ctx)
	if err != nil {
		return Stats{}, err
	}
	defer platform.Close(conn)
	stats := Stats{DBPath: s.dbPath}
	for query, target := range map[string]*int{
		"SELECT COUNT(*) FROM documents":               &stats.DocumentCount,
		"SELECT COUNT(*) FROM edges":                   &stats.EdgeCount,
		"SELECT COUNT(*) FROM embeddings":              &stats.EmbeddingCount,
		"SELECT COUNT(*) FROM topic_nodes":             &stats.TopicNodeCount,
		"SELECT COUNT(*) FROM semantic_families":       &stats.SemanticFamilyCount,
		"SELECT COUNT(*) FROM semantic_family_members": &stats.SemanticMemberCount,
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
	var vectorEngine sql.NullString
	if err := conn.QueryRowContext(ctx, "SELECT value FROM memory_state WHERE key = 'vector_engine'").Scan(&vectorEngine); err == nil && vectorEngine.Valid {
		stats.VectorEngine = vectorEngine.String
	}
	stats.VectorDimensions, err = distinctEmbeddingDimensions(ctx, conn)
	if err != nil {
		return Stats{}, err
	}
	return stats, nil
}

func (s SQLiteStore) Prune(ctx context.Context, policy PrunePolicy) error {
	conn, err := s.open(ctx)
	if err != nil {
		return err
	}
	defer platform.Close(conn)
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
	defer platform.Close(conn)
	if _, err := conn.ExecContext(ctx, `DELETE FROM topic_nodes`); err != nil {
		return fmt.Errorf("clear topic nodes during rebuild: %w", err)
	}
	rows, err := conn.QueryContext(ctx, `
SELECT DISTINCT d.host, d.path, e.model
FROM documents d
JOIN embeddings e ON e.document_url = d.url
ORDER BY d.observed_at DESC
`)
	if err != nil {
		return fmt.Errorf("load documents for topic rebuild: %w", err)
	}
	var docs [][3]string
	for rows.Next() {
		var host, path, vectorSpace string
		if err := rows.Scan(&host, &path, &vectorSpace); err != nil {
			platform.Close(rows)
			return fmt.Errorf("scan document during topic rebuild: %w", err)
		}
		docs = append(docs, [3]string{host, path, vectorSpace})
	}
	if err := rows.Err(); err != nil {
		platform.Close(rows)
		return fmt.Errorf("iterate documents during topic rebuild: %w", err)
	}
	platform.Close(rows)
	for _, item := range docs {
		for _, rootPath := range topicRootPaths(item[1]) {
			row, ok, err := loadTopicNodeRow(ctx, conn, item[2], item[0], rootPath)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			if err := upsertTopicNodeRow(ctx, conn, row); err != nil {
				return err
			}
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for key, value := range map[string]string{
		"vector_index_rebuilt_at": now,
		"vector_engine":           "exact",
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

func distinctEmbeddingDimensions(ctx context.Context, conn *sql.DB) ([]int, error) {
	rows, err := conn.QueryContext(ctx, `SELECT DISTINCT dimension FROM embeddings ORDER BY dimension ASC`)
	if err != nil {
		return nil, fmt.Errorf("query embedding dimensions: %w", err)
	}
	defer platform.Close(rows)
	out := []int{}
	for rows.Next() {
		var dimension int
		if err := rows.Scan(&dimension); err != nil {
			return nil, fmt.Errorf("scan embedding dimension: %w", err)
		}
		out = append(out, dimension)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate embedding dimensions: %w", err)
	}
	return out, nil
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
	defer platform.Close(conn)
	if err := os.MkdirAll(cleanDir, 0o755); err != nil {
		return ExportStats{}, fmt.Errorf("create memory export dir: %w", err)
	}
	stats := ExportStats{
		DocumentsPath:     filepath.Join(cleanDir, "documents.jsonl"),
		EdgesPath:         filepath.Join(cleanDir, "edges.jsonl"),
		EmbeddingsPath:    filepath.Join(cleanDir, "embeddings.jsonl"),
		TopicNodesPath:    filepath.Join(cleanDir, "topic_nodes.jsonl"),
		FamiliesPath:      filepath.Join(cleanDir, "semantic_families.jsonl"),
		FamilyMembersPath: filepath.Join(cleanDir, "semantic_family_members.jsonl"),
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
	if stats.TopicNodeCount, err = exportTopicNodes(ctx, conn, stats.TopicNodesPath); err != nil {
		return ExportStats{}, err
	}
	if stats.FamilyCount, err = exportSemanticFamilies(ctx, conn, stats.FamiliesPath); err != nil {
		return ExportStats{}, err
	}
	if stats.MemberCount, err = exportSemanticFamilyMembers(ctx, conn, stats.FamilyMembersPath); err != nil {
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
	if count, err := importTopicNodes(ctx, s, filepath.Join(cleanDir, "topic_nodes.jsonl")); err != nil {
		return ImportStats{}, err
	} else {
		stats.TopicNodeCount = count
	}
	if count, err := importSemanticFamilies(ctx, s, filepath.Join(cleanDir, "semantic_families.jsonl")); err != nil {
		return ImportStats{}, err
	} else {
		stats.FamilyCount = count
	}
	if count, err := importSemanticFamilyMembers(ctx, s, filepath.Join(cleanDir, "semantic_family_members.jsonl")); err != nil {
		return ImportStats{}, err
	} else {
		stats.MemberCount = count
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
	if err := platform.ConfigureSQLite(ctx, db); err != nil {
		platform.Close(db)
		return nil, fmt.Errorf("configure discovery db: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		platform.Close(db)
		return nil, fmt.Errorf("ping discovery db: %w", err)
	}
	if err := s.ensureSchema(ctx, db); err != nil {
		platform.Close(db)
		return nil, err
	}
	return db, nil
}

func (s SQLiteStore) ensureSchema(ctx context.Context, db *sql.DB) error {
	for _, stmt := range sqliteSchemaStatements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensure discovery schema: %w", err)
		}
	}
	if err := ensureColumn(ctx, db, "topic_nodes", "vector_space", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(ctx, db, "semantic_families", "vector_space", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_topic_nodes_vector_space ON topic_nodes(vector_space)`,
		`CREATE INDEX IF NOT EXISTS idx_semantic_families_vector_space ON semantic_families(vector_space)`,
		`CREATE INDEX IF NOT EXISTS idx_embeddings_model ON embeddings(model)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensure discovery schema index: %w", err)
		}
	}
	return nil
}

func ensureColumn(ctx context.Context, db *sql.DB, table, column, definition string) error {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return fmt.Errorf("inspect %s schema: %w", table, err)
	}
	defer platform.Close(rows)
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			return fmt.Errorf("scan %s schema: %w", table, err)
		}
		if strings.EqualFold(name, column) {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate %s schema: %w", table, err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+column+` `+definition); err != nil {
		return fmt.Errorf("add %s.%s column: %w", table, column, err)
	}
	return nil
}

func withTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer platform.Rollback(tx)
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
