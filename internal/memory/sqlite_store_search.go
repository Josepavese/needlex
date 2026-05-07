package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/josepavese/needlex/internal/core/vectorindex"
	"github.com/josepavese/needlex/internal/platform"
)

func (s SQLiteStore) SearchTopicNodes(ctx context.Context, vector []float32, vectorSpace string, limit int, domainHints []string) ([]Candidate, error) {
	vectorSpace = strings.TrimSpace(vectorSpace)
	if len(vector) == 0 || vectorSpace == "" {
		return nil, nil
	}
	limit = normalizedCandidateLimit(limit)
	conn, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	defer platform.Close(conn)
	rows, err := conn.QueryContext(ctx, `
SELECT topic_key, host, root_path, representative_url, representative_title, semantic_summary,
       language, support_count, child_count, topic_depth, observed_at, updated_at, vector, vector_space
FROM topic_nodes
WHERE vector_space = ?
`, vectorSpace)
	if err != nil {
		return nil, fmt.Errorf("query topic nodes: %w", err)
	}
	defer platform.Close(rows)
	hints := normalizeDomainHints(domainHints)
	out := make([]Candidate, 0, limit)
	for rows.Next() {
		row, err := scanTopicNodeRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan topic node: %w", err)
		}
		candidate, ok, err := topicNodeCandidate(row, vector, hints)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		out = append(out, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate topic nodes: %w", err)
	}
	return rankMemoryCandidates(out, limit), nil
}

func (s SQLiteStore) SearchByVector(ctx context.Context, vector []float32, vectorSpace string, limit int, domainHints []string) ([]Candidate, error) {
	vectorSpace = strings.TrimSpace(vectorSpace)
	if len(vector) == 0 || vectorSpace == "" {
		return nil, nil
	}
	limit = normalizedCandidateLimit(limit)
	conn, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	defer platform.Close(conn)
	rows, err := conn.QueryContext(ctx, `
SELECT d.url, d.title, d.host, d.proof_refs_json, d.last_trace_id, e.vector
     , d.source_kind, d.stable_ratio, d.novelty_ratio, d.changed_recently, d.observed_at
FROM documents d
JOIN embeddings e ON e.document_url = d.url
WHERE e.model = ?
`, vectorSpace)
	if err != nil {
		return nil, fmt.Errorf("query discovery embeddings: %w", err)
	}
	defer platform.Close(rows)
	hints := normalizeDomainHints(domainHints)
	rowsByURL := map[string]embeddingCandidateRow{}
	items := []vectorindex.Item{}
	for rows.Next() {
		row, err := scanEmbeddingCandidateRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan discovery embedding row: %w", err)
		}
		storedVector, err := decodeVector(row.RawVector)
		if err != nil {
			return nil, fmt.Errorf("decode discovery vector: %w", err)
		}
		if len(storedVector) != len(vector) {
			continue
		}
		rowsByURL[row.URL] = row
		items = append(items, vectorindex.Item{ID: row.URL, Vector: storedVector})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate discovery embeddings: %w", err)
	}
	index, err := vectorindex.NewExact(items)
	if err != nil {
		return nil, fmt.Errorf("build exact vector index: %w", err)
	}
	hits, err := index.Search(ctx, vector, vectorindex.SearchOptions{Limit: 0})
	if err != nil {
		return nil, fmt.Errorf("search exact vector index: %w", err)
	}
	out := make([]Candidate, 0, min(len(hits), limit))
	for _, hit := range hits {
		row := rowsByURL[hit.ID]
		candidate := vectorMemoryCandidateWithSimilarity(row, hit.Similarity, hints)
		if candidate.URL != "" {
			out = append(out, candidate)
		}
	}
	return rankMemoryCandidates(out, limit), nil
}

func normalizedCandidateLimit(limit int) int {
	if limit <= 0 {
		return 5
	}
	return limit
}

func scanTopicNodeRow(scanner rowScanner) (topicNodeRow, error) {
	var row topicNodeRow
	err := scanner.Scan(
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
		&row.ObservedAt,
		&row.UpdatedAt,
		&row.Vector,
		&row.VectorSpace,
	)
	return row, err
}

func scanMemoryDocumentRow(scanner rowScanner) (memoryDocumentRow, error) {
	var row memoryDocumentRow
	err := scanner.Scan(
		&row.URL,
		&row.Title,
		&row.Host,
		&row.RawProofRefs,
		&row.TraceRef,
		&row.SourceKind,
		&row.StableRatio,
		&row.NoveltyRatio,
		&row.ChangedRecently,
		&row.ObservedAtRaw,
	)
	return row, err
}

func scanEmbeddingCandidateRow(scanner rowScanner) (embeddingCandidateRow, error) {
	var row embeddingCandidateRow
	err := scanner.Scan(
		&row.URL,
		&row.Title,
		&row.Host,
		&row.RawProofRefs,
		&row.TraceRef,
		&row.RawVector,
		&row.SourceKind,
		&row.StableRatio,
		&row.NoveltyRatio,
		&row.ChangedRecently,
		&row.ObservedAtRaw,
	)
	return row, err
}

func topicNodeCandidate(row topicNodeRow, vector []float32, hints []string) (Candidate, bool, error) {
	storedVector, err := decodeVector(row.Vector)
	if err != nil {
		return Candidate{}, false, fmt.Errorf("decode topic node vector: %w", err)
	}
	similarity := cosineSimilarity(vector, storedVector)
	if similarity <= 0 {
		return Candidate{}, false, nil
	}
	score := similarity*3.15 + topicSupportBoost(row.SupportCount, row.ChildCount) + topicDepthBoost(row.TopicDepth)
	reasons := []string{"semantic_goal_alignment", "local_memory_hit", "topic_node_retrieval"}
	score, reasons = applyDomainAndRecencyEvidence(score, reasons, row.Host, row.ObservedAt, hints)
	if row.ChildCount > 0 {
		reasons = append(reasons, "topic_child_coverage")
	}
	return Candidate{
		URL:        row.RepresentativeURL,
		Title:      firstNonEmpty(row.RepresentativeTitle, row.SemanticSummary),
		Host:       row.Host,
		Score:      score,
		Reasons:    reasons,
		Source:     "discovery_memory_topic",
		ObservedAt: parseObservedAtOrZero(row.ObservedAt),
	}, true, nil
}

func vectorMemoryCandidateWithSimilarity(row embeddingCandidateRow, similarity float64, hints []string) Candidate {
	if similarity <= 0 {
		return Candidate{}
	}
	score, reasons := applyDocumentEvidence(similarity*3, []string{"semantic_goal_alignment", "local_memory_hit"}, row.memoryDocumentRow, hints, 0.08, true)
	candidate := baseDocumentCandidate(row.memoryDocumentRow, score, reasons, firstNonEmpty(row.SourceKind, "discovery_memory"))
	candidate.Distance = 1 - similarity
	return candidate
}

func baseDocumentCandidate(row memoryDocumentRow, score float64, reasons []string, source string) Candidate {
	candidate := Candidate{
		URL:             row.URL,
		Title:           row.Title,
		Host:            row.Host,
		Score:           score,
		Reasons:         reasons,
		TraceRef:        row.TraceRef,
		Source:          source,
		ObservedAt:      parseObservedAtOrZero(row.ObservedAtRaw),
		StableRatio:     row.StableRatio,
		NoveltyRatio:    row.NoveltyRatio,
		ChangedRecently: row.ChangedRecently == 1,
	}
	return addProofEvidence(candidate, row.RawProofRefs, 0.08)
}

func applyDocumentEvidence(score float64, reasons []string, row memoryDocumentRow, hints []string, ratioWeight float64, includeChanged bool) (float64, []string) {
	score, reasons = applyDomainAndRecencyEvidence(score, reasons, row.Host, row.ObservedAtRaw, hints)
	if row.StableRatio > 0 {
		score += row.StableRatio * ratioWeight
		reasons = append(reasons, "stable_page")
	}
	if row.NoveltyRatio > 0 {
		score += row.NoveltyRatio * ratioWeight
		reasons = append(reasons, "novel_page")
	}
	if includeChanged && row.ChangedRecently == 1 {
		score += 0.06
		reasons = append(reasons, "changed_recently")
	}
	return score, reasons
}

func applyDomainAndRecencyEvidence(score float64, reasons []string, host, observedAtRaw string, hints []string) (float64, []string) {
	if hasDomainHint(host, hints) {
		score += 0.2
		reasons = append(reasons, "domain_identity_match")
	}
	if observedAt, ok := parseObservedAt(observedAtRaw); ok {
		if boost := recentObservationBoost(observedAt); boost > 0 {
			score += boost
			reasons = append(reasons, "recent_local_evidence")
		}
	}
	return score, reasons
}

func addProofEvidence(candidate Candidate, rawProofRefs string, boost float64) Candidate {
	proofRefs := decodeStringSlice(rawProofRefs)
	if len(proofRefs) == 0 {
		return candidate
	}
	candidate.ProofRef = proofRefs[0]
	candidate.Score += boost
	candidate.Reasons = append(candidate.Reasons, "proof_backed_page")
	return candidate
}

func rankMemoryCandidates(candidates []Candidate, limit int) []Candidate {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].URL < candidates[j].URL
		}
		return candidates[i].Score > candidates[j].Score
	})
	if len(candidates) > limit {
		return candidates[:limit]
	}
	return candidates
}
