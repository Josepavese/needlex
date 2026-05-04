package memory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/josepavese/needlex/internal/platform"
)

func (s SQLiteStore) ExpandAncestorRoots(ctx context.Context, urls []string, limit int) ([]Candidate, error) {
	clean := compactURLs(urls)
	if len(clean) == 0 {
		return nil, nil
	}
	limit = normalizedCandidateLimit(limit)
	conn, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	defer platform.Close(conn)
	out := make([]Candidate, 0, limit)
	seen := map[string]struct{}{}
	for _, rawURL := range clean {
		host, path := hostPath(rawURL)
		for _, ancestor := range inclusiveAncestorPaths(path) {
			if len(out) >= limit {
				break
			}
			candidate, ok, err := loadAncestorRootCandidate(ctx, conn, host, ancestor)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			if _, ok := seen[candidate.URL]; ok {
				continue
			}
			seen[candidate.URL] = struct{}{}
			out = append(out, candidate)
		}
	}
	return rankMemoryCandidates(out, limit), nil
}

func loadAncestorRootCandidate(ctx context.Context, conn *sql.DB, host, ancestor string) (Candidate, bool, error) {
	row, ok, err := loadMemoryDocumentRow(ctx, conn, `
SELECT url, title, host, proof_refs_json, last_trace_id, source_kind, stable_ratio, novelty_ratio, changed_recently, observed_at
FROM documents
WHERE host = ? AND path = ?
LIMIT 1
`, host, ancestor)
	if err != nil || !ok {
		return Candidate{}, ok, err
	}
	supportCount, err := ancestorSupportCount(ctx, conn, host, ancestor)
	if err != nil {
		return Candidate{}, false, err
	}
	if supportCount < 2 {
		return Candidate{}, false, nil
	}
	return ancestorRootCandidate(row, supportCount), true, nil
}

func loadMemoryDocumentRow(ctx context.Context, conn *sql.DB, query string, args ...any) (memoryDocumentRow, bool, error) {
	row, err := scanMemoryDocumentRow(conn.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return memoryDocumentRow{}, false, nil
	}
	if err != nil {
		return memoryDocumentRow{}, false, fmt.Errorf("load memory document row: %w", err)
	}
	return row, true, nil
}

func ancestorSupportCount(ctx context.Context, conn *sql.DB, host, ancestor string) (int, error) {
	var supportCount int
	prefix := ancestor + "/%"
	err := conn.QueryRowContext(ctx, `
SELECT COUNT(*) FROM documents
WHERE host = ? AND (path = ? OR path LIKE ?)
`, host, ancestor, prefix).Scan(&supportCount)
	if err != nil {
		return 0, fmt.Errorf("count ancestor root descendants: %w", err)
	}
	return supportCount, nil
}

func ancestorRootCandidate(row memoryDocumentRow, supportCount int) Candidate {
	score := 1.4 + min(0.9, float64(supportCount-1)*0.32)
	reasons := []string{"family_root_inference"}
	score, reasons = applyDocumentEvidence(score, reasons, row, nil, 0.05, false)
	reasons = append(reasons, "semantic_family_support")
	candidate := Candidate{
		URL:             row.URL,
		Title:           row.Title,
		Host:            row.Host,
		Score:           score,
		Reasons:         reasons,
		TraceRef:        row.TraceRef,
		Source:          firstNonEmpty(row.SourceKind, "discovery_memory_root"),
		ObservedAt:      parseObservedAtOrZero(row.ObservedAtRaw),
		StableRatio:     row.StableRatio,
		NoveltyRatio:    row.NoveltyRatio,
		ChangedRecently: row.ChangedRecently == 1,
	}
	return addProofEvidence(candidate, row.RawProofRefs, 0.06)
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
	defer platform.Close(conn)
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
				platform.Close(rows)
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
		platform.Close(rows)
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

func ancestorPaths(path string) []string {
	clean := strings.Trim(strings.TrimSpace(path), "/")
	if clean == "" {
		return nil
	}
	parts := strings.Split(clean, "/")
	if len(parts) <= 1 {
		return nil
	}
	out := make([]string, 0, len(parts)-1)
	for i := len(parts) - 1; i >= 1; i-- {
		out = append(out, "/"+strings.Join(parts[:i], "/"))
	}
	return out
}

func inclusiveAncestorPaths(path string) []string {
	clean := strings.Trim(strings.TrimSpace(path), "/")
	if clean == "" {
		return nil
	}
	out := []string{"/" + clean}
	return append(out, ancestorPaths(path)...)
}

func (s SQLiteStore) ExpandHosts(ctx context.Context, hosts []string, limit int) ([]Candidate, error) {
	cleanHosts := normalizeDomainHints(hosts)
	if len(cleanHosts) == 0 {
		return nil, nil
	}
	limit = normalizedCandidateLimit(limit)
	conn, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	defer platform.Close(conn)
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
			row, err := scanMemoryDocumentRow(rows)
			if err != nil {
				platform.Close(rows)
				return nil, fmt.Errorf("scan discovery host expansion row: %w", err)
			}
			if _, ok := seen[row.URL]; ok {
				continue
			}
			seen[row.URL] = struct{}{}
			candidate := hostExpansionCandidate(row)
			out = append(out, candidate)
		}
		platform.Close(rows)
	}
	return rankMemoryCandidates(out, limit), nil
}

func hostExpansionCandidate(row memoryDocumentRow) Candidate {
	score, reasons := applyDocumentEvidence(0.78, []string{"host_memory_recall"}, row, nil, 0.05, false)
	candidate := Candidate{
		URL:             row.URL,
		Title:           row.Title,
		Host:            row.Host,
		Score:           score,
		Reasons:         reasons,
		TraceRef:        row.TraceRef,
		Source:          firstNonEmpty(row.SourceKind, "discovery_memory_host"),
		ObservedAt:      parseObservedAtOrZero(row.ObservedAtRaw),
		StableRatio:     row.StableRatio,
		NoveltyRatio:    row.NoveltyRatio,
		ChangedRecently: row.ChangedRecently == 1,
	}
	return addProofEvidence(candidate, row.RawProofRefs, 0.06)
}
