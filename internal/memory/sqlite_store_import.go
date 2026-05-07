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
  topic_key, vector_space, host, root_path, representative_url, representative_title, semantic_summary,
  language, support_count, child_count, topic_depth, observed_at, updated_at, vector
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(topic_key) DO UPDATE SET
  vector_space=excluded.vector_space,
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
`, row.TopicKey, row.VectorSpace, row.Host, row.RootPath, row.RepresentativeURL, row.RepresentativeTitle, row.SemanticSummary, row.Language, row.SupportCount, row.ChildCount, row.TopicDepth, observedAt.UTC().Format(time.RFC3339Nano), updatedAt.UTC().Format(time.RFC3339Nano), vector)
		if err != nil {
			return fmt.Errorf("upsert topic node import: %w", err)
		}
		return nil
	})
}

func importSemanticFamilies(ctx context.Context, store SQLiteStore, path string) (int, error) {
	return readOptionalJSONL(path, func(line []byte) error {
		var row exportSemanticFamilyRow
		if err := json.Unmarshal(line, &row); err != nil {
			return err
		}
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
INSERT INTO semantic_families (
  family_id, vector_space, representative_url, representative_title, semantic_summary, support_count,
  contradiction_count, confidence, observed_at, updated_at, vector
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(family_id) DO UPDATE SET
  vector_space=excluded.vector_space,
  representative_url=excluded.representative_url,
  representative_title=excluded.representative_title,
  semantic_summary=excluded.semantic_summary,
  support_count=excluded.support_count,
  contradiction_count=excluded.contradiction_count,
  confidence=excluded.confidence,
  observed_at=excluded.observed_at,
  updated_at=excluded.updated_at,
  vector=excluded.vector
`, row.FamilyID, row.VectorSpace, row.RepresentativeURL, row.RepresentativeTitle, row.SemanticSummary, row.SupportCount, row.ContradictionCount, row.Confidence, row.ObservedAt, row.UpdatedAt, vector)
		if err != nil {
			return fmt.Errorf("upsert semantic family import: %w", err)
		}
		return nil
	})
}

func importSemanticFamilyMembers(ctx context.Context, store SQLiteStore, path string) (int, error) {
	return readOptionalJSONL(path, func(line []byte) error {
		var row exportSemanticFamilyMemberRow
		if err := json.Unmarshal(line, &row); err != nil {
			return err
		}
		conn, err := store.open(ctx)
		if err != nil {
			return err
		}
		defer platform.Close(conn)
		_, err = conn.ExecContext(ctx, `
INSERT INTO semantic_family_members (family_id, resource_url, role, evidence_kind, confidence, trace_ref, observed_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(family_id, resource_url, evidence_kind) DO UPDATE SET
  role=excluded.role,
  confidence=excluded.confidence,
  trace_ref=excluded.trace_ref,
  observed_at=excluded.observed_at
`, row.FamilyID, row.ResourceURL, row.Role, row.EvidenceKind, row.Confidence, row.TraceRef, row.ObservedAt)
		if err != nil {
			return fmt.Errorf("upsert semantic family member import: %w", err)
		}
		return nil
	})
}

func readOptionalJSONL(path string, consume func([]byte) error) (int, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	return readJSONL(path, consume)
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
