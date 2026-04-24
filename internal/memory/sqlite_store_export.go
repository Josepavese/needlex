package memory

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	"github.com/josepavese/needlex/internal/platform"
)

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

type exportTopicNodeRow struct {
	TopicKey            string    `json:"topic_key"`
	Host                string    `json:"host"`
	RootPath            string    `json:"root_path"`
	RepresentativeURL   string    `json:"representative_url"`
	RepresentativeTitle string    `json:"representative_title"`
	SemanticSummary     string    `json:"semantic_summary"`
	Language            string    `json:"language,omitempty"`
	SupportCount        int       `json:"support_count"`
	ChildCount          int       `json:"child_count"`
	TopicDepth          int       `json:"topic_depth"`
	Vector              []float32 `json:"vector"`
	ObservedAt          string    `json:"observed_at"`
	UpdatedAt           string    `json:"updated_at"`
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
	defer platform.Close(rows)
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
	defer platform.Close(rows)
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
	defer platform.Close(rows)
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

func exportTopicNodes(ctx context.Context, conn *sql.DB, path string) (int, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT topic_key, host, root_path, representative_url, representative_title, semantic_summary,
       language, support_count, child_count, topic_depth, vector, observed_at, updated_at
FROM topic_nodes
ORDER BY updated_at DESC, topic_key ASC
`)
	if err != nil {
		return 0, fmt.Errorf("query topic nodes export: %w", err)
	}
	defer platform.Close(rows)
	return writeJSONL(path, rows, func() (exportTopicNodeRow, error) {
		var row exportTopicNodeRow
		var rawVector []byte
		if err := rows.Scan(
			&row.TopicKey,
			&row.Host,
			&row.RootPath,
			&row.RepresentativeURL,
			&row.RepresentativeTitle,
			&row.SemanticSummary,
			&row.Language,
			&row.SupportCount,
			&row.ChildCount,
			&row.TopicDepth,
			&rawVector,
			&row.ObservedAt,
			&row.UpdatedAt,
		); err != nil {
			return exportTopicNodeRow{}, err
		}
		vector, err := decodeVector(rawVector)
		if err != nil {
			return exportTopicNodeRow{}, err
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
	defer platform.Close(file)
	writer := bufio.NewWriter(file)
	defer platform.Flush(writer)
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
