package memory

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/platform"
)

const semanticFamilyJoinThreshold = 0.78

type semanticFamilyRow struct {
	FamilyID            string
	VectorSpace         string
	RepresentativeURL   string
	RepresentativeTitle string
	SemanticSummary     string
	SupportCount        int
	ContradictionCount  int
	Confidence          float64
	ObservedAt          string
	UpdatedAt           string
	Vector              []byte
}

func (s SQLiteStore) UpsertSemanticFamilyEvidence(ctx context.Context, doc Document, vector []float32, vectorSpace string) error {
	vectorSpace = strings.TrimSpace(vectorSpace)
	if len(vector) == 0 || strings.TrimSpace(doc.URL) == "" || vectorSpace == "" {
		return nil
	}
	conn, err := s.open(ctx)
	if err != nil {
		return err
	}
	defer platform.Close(conn)
	row, ok, similarity, err := nearestSemanticFamily(ctx, conn, vector, vectorSpace)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if !ok || similarity < semanticFamilyJoinThreshold {
		row = newSemanticFamilyRow(doc, vector, vectorSpace, now)
	} else {
		row, err = mergeSemanticFamilyRow(row, doc, vector, similarity, now)
		if err != nil {
			return err
		}
	}
	if err := upsertSemanticFamilyRow(ctx, conn, row); err != nil {
		return err
	}
	return upsertSemanticFamilyMember(ctx, conn, row.FamilyID, doc, max(similarity, row.Confidence), now)
}

func (s SQLiteStore) SearchSemanticFamilies(ctx context.Context, vector []float32, vectorSpace string, limit int, domainHints []string) ([]Candidate, error) {
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
SELECT family_id, representative_url, representative_title, semantic_summary, support_count,
       contradiction_count, confidence, observed_at, updated_at, vector, vector_space
FROM semantic_families
WHERE vector_space = ?
`, vectorSpace)
	if err != nil {
		return nil, fmt.Errorf("query semantic families: %w", err)
	}
	defer platform.Close(rows)
	hints := normalizeDomainHints(domainHints)
	out := make([]Candidate, 0, limit)
	for rows.Next() {
		row, err := scanSemanticFamilyRow(rows)
		if err != nil {
			return nil, err
		}
		candidate, ok, err := semanticFamilyCandidate(row, vector, hints)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, candidate)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate semantic families: %w", err)
	}
	return rankMemoryCandidates(out, limit), nil
}

func nearestSemanticFamily(ctx context.Context, conn *sql.DB, vector []float32, vectorSpace string) (semanticFamilyRow, bool, float64, error) {
	rows, err := conn.QueryContext(ctx, `
SELECT family_id, representative_url, representative_title, semantic_summary, support_count,
       contradiction_count, confidence, observed_at, updated_at, vector, vector_space
FROM semantic_families
WHERE vector_space = ?
`, vectorSpace)
	if err != nil {
		return semanticFamilyRow{}, false, 0, fmt.Errorf("query nearest semantic family: %w", err)
	}
	defer platform.Close(rows)
	var best semanticFamilyRow
	bestScore := 0.0
	found := false
	for rows.Next() {
		row, err := scanSemanticFamilyRow(rows)
		if err != nil {
			return semanticFamilyRow{}, false, 0, err
		}
		storedVector, err := decodeVector(row.Vector)
		if err != nil {
			return semanticFamilyRow{}, false, 0, fmt.Errorf("decode semantic family vector: %w", err)
		}
		similarity := cosineSimilarity(vector, storedVector)
		if !found || similarity > bestScore {
			best = row
			bestScore = similarity
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		return semanticFamilyRow{}, false, 0, fmt.Errorf("iterate semantic families: %w", err)
	}
	return best, found, bestScore, nil
}

func scanSemanticFamilyRow(scanner rowScanner) (semanticFamilyRow, error) {
	var row semanticFamilyRow
	err := scanner.Scan(
		&row.FamilyID,
		&row.RepresentativeURL,
		&row.RepresentativeTitle,
		&row.SemanticSummary,
		&row.SupportCount,
		&row.ContradictionCount,
		&row.Confidence,
		&row.ObservedAt,
		&row.UpdatedAt,
		&row.Vector,
		&row.VectorSpace,
	)
	return row, err
}

func newSemanticFamilyRow(doc Document, vector []float32, vectorSpace, now string) semanticFamilyRow {
	observedAt := now
	if !doc.ObservedAt.IsZero() {
		observedAt = doc.ObservedAt.UTC().Format(time.RFC3339Nano)
	}
	return semanticFamilyRow{
		FamilyID:            prefixedHash("fam", vectorSpace, doc.URL, doc.Title, doc.SemanticSummary),
		VectorSpace:         vectorSpace,
		RepresentativeURL:   doc.URL,
		RepresentativeTitle: firstNonEmpty(doc.Title, doc.SemanticSummary),
		SemanticSummary:     firstNonEmpty(doc.SemanticSummary, doc.Title),
		SupportCount:        1,
		Confidence:          0.64,
		ObservedAt:          observedAt,
		UpdatedAt:           now,
		Vector:              mustEncodeVector(vector),
	}
}

func mergeSemanticFamilyRow(row semanticFamilyRow, doc Document, vector []float32, similarity float64, now string) (semanticFamilyRow, error) {
	storedVector, err := decodeVector(row.Vector)
	if err != nil {
		return semanticFamilyRow{}, fmt.Errorf("decode semantic family vector: %w", err)
	}
	row.Vector = mustEncodeVector(mergeFamilyCentroid(storedVector, vector, row.SupportCount))
	row.SupportCount++
	row.Confidence = min(0.98, max(row.Confidence, similarity)*0.92+0.06)
	row.SemanticSummary = compactFamilySummary(row.SemanticSummary, doc.SemanticSummary)
	if betterFamilyRepresentative(doc, row) {
		row.RepresentativeURL = doc.URL
		row.RepresentativeTitle = firstNonEmpty(doc.Title, doc.SemanticSummary)
	}
	row.UpdatedAt = now
	return row, nil
}

func mergeFamilyCentroid(current, incoming []float32, supportCount int) []float32 {
	if len(current) == 0 || len(current) != len(incoming) {
		return incoming
	}
	weight := float32(maxInt(1, supportCount))
	out := make([]float32, len(current))
	for i := range current {
		out[i] = (current[i]*weight + incoming[i]) / (weight + 1)
	}
	return out
}

func betterFamilyRepresentative(doc Document, row semanticFamilyRow) bool {
	if strings.TrimSpace(row.RepresentativeURL) == "" {
		return true
	}
	if doc.StableRatio > 0.72 && doc.StableRatio > row.Confidence {
		return true
	}
	return row.SupportCount <= 1 && strings.TrimSpace(doc.Title) != ""
}

func compactFamilySummary(existing, incoming string) string {
	joined := strings.Join(compactStrings([]string{existing, incoming}), " ")
	joined = strings.Join(strings.Fields(joined), " ")
	if len(joined) > 900 {
		return strings.TrimSpace(joined[:900])
	}
	return joined
}

func upsertSemanticFamilyRow(ctx context.Context, conn *sql.DB, row semanticFamilyRow) error {
	_, err := conn.ExecContext(ctx, `
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
`, row.FamilyID, row.VectorSpace, row.RepresentativeURL, row.RepresentativeTitle, row.SemanticSummary, row.SupportCount, row.ContradictionCount, row.Confidence, row.ObservedAt, row.UpdatedAt, row.Vector)
	if err != nil {
		return fmt.Errorf("upsert semantic family: %w", err)
	}
	return nil
}

func upsertSemanticFamilyMember(ctx context.Context, conn *sql.DB, familyID string, doc Document, confidence float64, now string) error {
	_, err := conn.ExecContext(ctx, `
INSERT INTO semantic_family_members (family_id, resource_url, role, evidence_kind, confidence, trace_ref, observed_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(family_id, resource_url, evidence_kind) DO UPDATE SET
  role=excluded.role,
  confidence=max(semantic_family_members.confidence, excluded.confidence),
  trace_ref=excluded.trace_ref,
  observed_at=excluded.observed_at
`, familyID, doc.URL, "observed_resource", firstNonEmpty(doc.SourceKind, "read"), clampUnitInterval(confidence), doc.LastTraceID, now)
	if err != nil {
		return fmt.Errorf("upsert semantic family member: %w", err)
	}
	return nil
}

func semanticFamilyCandidate(row semanticFamilyRow, vector []float32, hints []string) (Candidate, bool, error) {
	storedVector, err := decodeVector(row.Vector)
	if err != nil {
		return Candidate{}, false, fmt.Errorf("decode semantic family vector: %w", err)
	}
	similarity := cosineSimilarity(vector, storedVector)
	if similarity <= 0 {
		return Candidate{}, false, nil
	}
	score := similarity*3.35 + topicSupportBoost(row.SupportCount, 0) + row.Confidence*0.24 - float64(row.ContradictionCount)*0.18
	reasons := []string{"entity_family_graph_recall", "semantic_family_memory"}
	host, _ := hostPath(row.RepresentativeURL)
	score, reasons = applyDomainAndRecencyEvidence(score, reasons, host, row.ObservedAt, hints)
	return Candidate{
		URL:        row.RepresentativeURL,
		Title:      firstNonEmpty(row.RepresentativeTitle, row.SemanticSummary),
		Host:       host,
		Score:      score,
		Reasons:    reasons,
		Distance:   1 - similarity,
		Source:     "discovery_memory_family_graph",
		ObservedAt: parseObservedAtOrZero(row.ObservedAt),
	}, true, nil
}
