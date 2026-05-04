package memory

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/platform"
)

func loadTopicNodeRow(ctx context.Context, conn *sql.DB, host, rootPath string) (topicNodeRow, bool, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT d.url, d.title, d.path, d.semantic_summary, d.language, d.observed_at, e.vector
FROM documents d
JOIN embeddings e ON e.document_url = d.url
WHERE d.host = ? AND (d.path = ? OR d.path LIKE ?)
ORDER BY LENGTH(d.path) ASC, d.observed_at DESC, d.url ASC
`, host, rootPath, rootPath+"/%")
	if err != nil {
		return topicNodeRow{}, false, fmt.Errorf("load topic node descendants: %w", err)
	}
	defer platform.Close(rows)
	docs := make([]topicDoc, 0, 8)
	for rows.Next() {
		var item topicDoc
		var rawVector []byte
		if err := rows.Scan(&item.URL, &item.Title, &item.Path, &item.Summary, &item.Language, &item.ObservedAt, &rawVector); err != nil {
			return topicNodeRow{}, false, fmt.Errorf("scan topic node descendant: %w", err)
		}
		vector, err := decodeVector(rawVector)
		if err != nil {
			return topicNodeRow{}, false, fmt.Errorf("decode topic node descendant vector: %w", err)
		}
		item.Vector = vector
		docs = append(docs, item)
	}
	if err := rows.Err(); err != nil {
		return topicNodeRow{}, false, fmt.Errorf("iterate topic node descendants: %w", err)
	}
	if len(docs) == 0 {
		return topicNodeRow{}, false, nil
	}
	rep := docs[0]
	observedAt := rep.ObservedAt
	if parsed, ok := parseObservedAt(observedAt); ok {
		for _, item := range docs[1:] {
			if candidateAt, ok := parseObservedAt(item.ObservedAt); ok && candidateAt.After(parsed) {
				observedAt = item.ObservedAt
				parsed = candidateAt
			}
		}
	}
	return topicNodeRow{
		TopicKey:            topicNodeKey(host, rootPath),
		Host:                host,
		RootPath:            rootPath,
		RepresentativeURL:   rep.URL,
		RepresentativeTitle: rep.Title,
		SemanticSummary:     buildTopicSummary(rootPath, docs),
		Language:            firstNonEmpty(rep.Language),
		SupportCount:        len(docs),
		ChildCount:          maxInt(0, len(docs)-1),
		TopicDepth:          pathDepth(rootPath),
		ObservedAt:          observedAt,
		UpdatedAt:           time.Now().UTC().Format(time.RFC3339Nano),
		Vector:              mustEncodeVector(averageTopicVector(docs)),
	}, true, nil
}

func upsertTopicNodeRow(ctx context.Context, conn *sql.DB, row topicNodeRow) error {
	_, err := conn.ExecContext(ctx, `
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
`, row.TopicKey, row.Host, row.RootPath, row.RepresentativeURL, row.RepresentativeTitle, row.SemanticSummary, row.Language, row.SupportCount, row.ChildCount, row.TopicDepth, row.ObservedAt, row.UpdatedAt, row.Vector)
	if err != nil {
		return fmt.Errorf("upsert topic node: %w", err)
	}
	return nil
}

func topicRootPaths(path string) []string {
	ancestors := inclusiveAncestorPaths(path)
	if len(ancestors) == 0 {
		return nil
	}
	out := make([]string, 0, len(ancestors))
	for _, item := range ancestors {
		if pathDepth(item) < 2 {
			continue
		}
		out = append(out, item)
	}
	return out
}

func topicNodeKey(host, rootPath string) string {
	return strings.ToLower(strings.TrimSpace(host)) + "|" + strings.TrimSpace(rootPath)
}

func buildTopicSummary(rootPath string, docs []topicDoc) string {
	parts := make([]string, 0, minInt(len(docs), 4))
	for i, item := range docs {
		if i >= 4 {
			break
		}
		parts = append(parts, firstNonEmpty(item.Title, item.Summary))
		if summary := strings.TrimSpace(item.Summary); summary != "" && summary != strings.TrimSpace(item.Title) {
			parts = append(parts, summary)
		}
	}
	joined := strings.Join(compactStrings(parts), " ")
	joined = strings.Join(strings.Fields(joined), " ")
	if len(joined) > 800 {
		joined = strings.TrimSpace(joined[:800])
	}
	if joined == "" {
		joined = strings.TrimSpace(rootPath)
	}
	return joined
}

func averageTopicVector(docs []topicDoc) []float32 {
	if len(docs) == 0 {
		return nil
	}
	dim := len(docs[0].Vector)
	if dim == 0 {
		return nil
	}
	acc := make([]float32, dim)
	var total float32
	for i, item := range docs {
		weight := float32(1.0)
		if i == 0 {
			weight = 1.75
		}
		if pathDepth(item.Path) == pathDepth(docs[0].Path) {
			weight += 0.25
		}
		for j := 0; j < dim && j < len(item.Vector); j++ {
			acc[j] += item.Vector[j] * weight
		}
		total += weight
	}
	if total <= 0 {
		return acc
	}
	for i := range acc {
		acc[i] /= total
	}
	return acc
}

func topicSupportBoost(supportCount, childCount int) float64 {
	score := 0.18 * min(4, float64(maxInt(0, supportCount-1)))
	score += 0.12 * min(3, float64(childCount))
	return score
}

func topicDepthBoost(depth int) float64 {
	if depth <= 0 {
		return 0
	}
	return 0.42 / float64(depth+1)
}

func pathDepth(path string) int {
	clean := strings.Trim(strings.TrimSpace(path), "/")
	if clean == "" {
		return 0
	}
	return len(strings.Split(clean, "/"))
}

func mustEncodeVector(vector []float32) []byte {
	blob, _ := encodeVector(vector)
	return blob
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func hostPath(raw string) (string, string) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", ""
	}
	return strings.ToLower(parsed.Hostname()), firstNonEmpty(parsed.EscapedPath(), "/")
}
