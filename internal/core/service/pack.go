package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/pipeline"
	"github.com/josepavese/needlex/internal/proof"
)

type rankedSegment struct {
	segment pipeline.Segment
	chunk   core.Chunk
	index   int
}

func (s *Service) pack(recorder *proof.Recorder, req ReadRequest, document core.Document, segments []pipeline.Segment) (core.ResultPack, []proof.ProofRecord, error) {
	const stage = "pack"
	if err := recorder.StageStarted(stage, segments, s.now().UTC()); err != nil {
		return core.ResultPack{}, nil, err
	}

	ranked := rankSegments(document.ID, req.Objective, segments)
	selected := selectProfile(ranked, req.Profile)
	chunks := collectChunks(selected)
	proofRecords, err := buildProofRecords(selected)
	if err != nil {
		recorder.Error(stage, "NX_PROOF_BUILD_FAILED", err.Error(), nil, s.now().UTC())
		return core.ResultPack{}, nil, err
	}

	resultPack := core.ResultPack{
		Objective: objectiveOrDefault(req.Objective),
		Profile:   req.Profile,
		Chunks:    chunks,
		Sources: []core.SourceRef{
			{
				DocumentID: document.ID,
				URL:        document.FinalURL,
				Title:      document.Title,
				ChunkIDs:   chunkIDs(chunks),
			},
		},
		Outline:    buildOutline(selected),
		Links:      []string{document.FinalURL},
		ProofRefs:  proofIDs(proofRecords),
		CostReport: core.CostReport{LanePath: []int{0}},
	}

	if err := resultPack.Validate(); err != nil {
		return core.ResultPack{}, nil, err
	}

	if err := recorder.StageCompleted(stage, resultPack, len(chunks), map[string]string{
		"profile": req.Profile,
		"chunks":  fmt.Sprintf("%d", len(chunks)),
	}, s.now().UTC()); err != nil {
		return core.ResultPack{}, nil, err
	}

	return resultPack, proofRecords, nil
}

func resolveProfile(profile string) (string, error) {
	profile = strings.TrimSpace(strings.ToLower(profile))
	if profile == "" {
		return core.ProfileStandard, nil
	}
	switch profile {
	case core.ProfileTiny, core.ProfileStandard, core.ProfileDeep:
		return profile, nil
	default:
		return "", fmt.Errorf("unsupported profile %q", profile)
	}
}

func rankSegments(docID, objective string, segments []pipeline.Segment) []rankedSegment {
	ranked := make([]rankedSegment, 0, len(segments))
	for index, segment := range segments {
		fingerprint := prefixedHash("fp", docID, strings.Join(segment.HeadingPath, "/"), segment.Text)
		score := segmentScore(segment, objective, index, len(segments))
		confidence := segmentConfidence(segment, objective)
		ranked = append(ranked, rankedSegment{
			segment: segment,
			index:   index,
			chunk: core.Chunk{
				ID:          prefixedHash("chk", docID, fingerprint),
				DocID:       docID,
				Text:        segment.Text,
				HeadingPath: append([]string{}, segment.HeadingPath...),
				Score:       score,
				Fingerprint: fingerprint,
				Confidence:  confidence,
			},
		})
	}

	slices.SortStableFunc(ranked, func(a, b rankedSegment) int {
		switch {
		case a.chunk.Score > b.chunk.Score:
			return -1
		case a.chunk.Score < b.chunk.Score:
			return 1
		case a.index < b.index:
			return -1
		case a.index > b.index:
			return 1
		default:
			return 0
		}
	})

	return ranked
}

func selectProfile(ranked []rankedSegment, profile string) []rankedSegment {
	limit := len(ranked)
	switch profile {
	case core.ProfileTiny:
		limit = min(limit, 3)
	case core.ProfileStandard:
		limit = min(limit, 6)
	case core.ProfileDeep:
		limit = len(ranked)
	}
	return append([]rankedSegment{}, ranked[:limit]...)
}

func collectChunks(selected []rankedSegment) []core.Chunk {
	chunks := make([]core.Chunk, 0, len(selected))
	for _, item := range selected {
		chunks = append(chunks, item.chunk)
	}
	return chunks
}

func buildProofRecords(selected []rankedSegment) ([]proof.ProofRecord, error) {
	records := make([]proof.ProofRecord, 0, len(selected))
	for _, item := range selected {
		record, err := proof.BuildProofRecord(proof.BuildInput{
			Chunk:          item.chunk,
			Segment:        item.segment,
			Lane:           0,
			TransformChain: []string{"acquire:v1", "reduce:v1", "segment:v1", "pack:v2"},
		})
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func buildCostReport(trace proof.RunTrace) core.CostReport {
	latency := int64(0)
	if !trace.FinishedAt.IsZero() {
		latency = trace.FinishedAt.Sub(trace.StartedAt).Milliseconds()
	}
	return core.CostReport{
		LatencyMS: latency,
		TokenIn:   0,
		TokenOut:  0,
		LanePath:  []int{0},
	}
}

func buildOutline(selected []rankedSegment) []string {
	out := make([]string, 0, len(selected))
	seen := map[string]struct{}{}
	for _, item := range selected {
		if len(item.chunk.HeadingPath) == 0 {
			continue
		}
		label := strings.Join(item.chunk.HeadingPath, " > ")
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	return out
}

func proofIDs(records []proof.ProofRecord) []string {
	out := make([]string, 0, len(records))
	for _, record := range records {
		out = append(out, record.ID)
	}
	return out
}

func chunkIDs(chunks []core.Chunk) []string {
	out := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, chunk.ID)
	}
	return out
}

func objectiveOrDefault(objective string) string {
	if strings.TrimSpace(objective) == "" {
		return "read"
	}
	return objective
}

func segmentScore(segment pipeline.Segment, objective string, index, total int) float64 {
	kindWeight := map[string]float64{
		"paragraph":  0.80,
		"list_item":  0.74,
		"table_cell": 0.70,
		"code":       0.68,
	}
	base := kindWeight[segment.Kind]
	if base == 0 {
		base = 0.63
	}
	if len(segment.HeadingPath) > 0 {
		base += 0.05
	}
	if total > 1 {
		base += float64(total-index-1) / float64(total-1) * 0.08
	}
	base += objectiveCoverage(segment, objective) * 0.18
	if len(segment.Text) > 60 {
		base += 0.03
	}
	return clamp(base, 0, 1)
}

func segmentConfidence(segment pipeline.Segment, objective string) float64 {
	confidence := 0.75
	if len(segment.NodePaths) > 0 {
		confidence += 0.10
	}
	if len(segment.HeadingPath) > 0 {
		confidence += 0.04
	}
	if objectiveCoverage(segment, objective) > 0 {
		confidence += 0.06
	}
	if len(segment.Text) > 40 {
		confidence += 0.04
	}
	return clamp(confidence, 0, 0.99)
}

func objectiveCoverage(segment pipeline.Segment, objective string) float64 {
	tokens := objectiveTokens(objective)
	if len(tokens) == 0 {
		return 0
	}

	haystack := strings.ToLower(strings.Join(segment.HeadingPath, " ") + " " + segment.Text)
	matches := 0
	for _, token := range tokens {
		if strings.Contains(haystack, token) {
			matches++
		}
	}
	return float64(matches) / float64(len(tokens))
}

func objectiveTokens(objective string) []string {
	fields := strings.Fields(strings.ToLower(objective))
	out := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, field := range fields {
		token := strings.Trim(field, " \t\r\n,.;:!?()[]{}<>\"'")
		if len(token) < 3 {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	return out
}

func clamp(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func prefixedHash(prefix string, parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return prefix + "_" + hex.EncodeToString(digest[:])[:16]
}
