package memory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/platform"
)

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
	defer platform.Close(conn)
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
	defer platform.Close(conn)
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin discovery edges tx: %w", err)
	}
	defer platform.Rollback(tx)
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
	defer platform.Close(stmt)
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
	defer platform.Close(conn)
	_, err = conn.ExecContext(ctx, `
INSERT OR REPLACE INTO embeddings (embedding_ref, document_url, model, backend, input_text, dimension, vector, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
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

func (s SQLiteStore) ReusableEmbeddingVector(ctx context.Context, emb Embedding) ([]float32, bool, error) {
	conn, err := s.open(ctx)
	if err != nil {
		return nil, false, err
	}
	defer platform.Close(conn)
	var raw []byte
	var dimension int
	err = conn.QueryRowContext(ctx, `
SELECT vector, dimension
FROM embeddings
WHERE embedding_ref = ? AND document_url = ? AND model = ? AND backend = ? AND input_text = ?
	`, emb.EmbeddingRef, emb.DocumentURL, emb.Model, emb.Backend, emb.InputText).Scan(&raw, &dimension)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("query reusable embedding: %w", err)
	}
	vector, err := decodeVector(raw)
	if err != nil {
		return nil, false, fmt.Errorf("decode reusable embedding: %w", err)
	}
	if len(vector) != dimension {
		return nil, false, nil
	}
	return vector, true, nil
}

func (s SQLiteStore) RefreshTopicNodes(ctx context.Context, doc Document, vectorSpace string) error {
	vectorSpace = strings.TrimSpace(vectorSpace)
	host := strings.TrimSpace(strings.ToLower(doc.Host))
	path := firstNonEmpty(doc.Path, "/")
	if vectorSpace == "" || host == "" || strings.TrimSpace(path) == "" || path == "/" {
		return nil
	}
	conn, err := s.open(ctx)
	if err != nil {
		return err
	}
	defer platform.Close(conn)
	for _, rootPath := range topicRootPaths(path) {
		row, ok, err := loadTopicNodeRow(ctx, conn, vectorSpace, host, rootPath)
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
	return nil
}
