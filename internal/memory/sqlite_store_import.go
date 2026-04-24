package memory

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/platform"
)

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

func importTopicNodes(ctx context.Context, store SQLiteStore, path string) (int, error) {
	return readJSONL(path, func(line []byte) error {
		var row exportTopicNodeRow
		if err := json.Unmarshal(line, &row); err != nil {
			return err
		}
		observedAt, _ := time.Parse(time.RFC3339Nano, row.ObservedAt)
		updatedAt, _ := time.Parse(time.RFC3339Nano, row.UpdatedAt)
		conn, err := store.open(ctx)
		if err != nil {
			return err
		}
		defer platform.Close(conn)
		vector, err := encodeVector(row.Vector)
		if err != nil {
			return err
		}
		_, err = conn.ExecContext(ctx, `
INSERT INTO topic_nodes (
  topic_key, host, root_path, representative_url, representative_title, semantic_summary,
  language, support_count, child_count, topic_depth, observed_at, updated_at, vector
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(topic_key) DO UPDATE SET
  host=excluded.host,
  root_path=excluded.root_path,
  representative_url=excluded.representative_url,
  representative_title=excluded.representative_title,
  semantic_summary=excluded.semantic_summary,
  language=excluded.language,
  support_count=excluded.support_count,
  child_count=excluded.child_count,
  topic_depth=excluded.topic_depth,
  observed_at=excluded.observed_at,
  updated_at=excluded.updated_at,
  vector=excluded.vector
`, row.TopicKey, row.Host, row.RootPath, row.RepresentativeURL, row.RepresentativeTitle, row.SemanticSummary, row.Language, row.SupportCount, row.ChildCount, row.TopicDepth, observedAt.UTC().Format(time.RFC3339Nano), updatedAt.UTC().Format(time.RFC3339Nano), vector)
		if err != nil {
			return fmt.Errorf("upsert topic node import: %w", err)
		}
		return nil
	})
}

func readJSONL(path string, consume func([]byte) error) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open jsonl import %s: %w", path, err)
	}
	defer platform.Close(file)
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
