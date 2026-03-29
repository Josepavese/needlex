package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/pipeline"
	"github.com/josepavese/needlex/internal/proof"
)

type rankedSegment struct {
	segment pipeline.Segment
	chunk   core.Chunk
	index   int
	ir      segmentIREvidence
}
type segmentIREvidence struct {
	kindMatch, embedded, headingBacked bool
	averageNodeDepth                   float64
}

func (s *Service) pack(recorder *proof.Recorder, req ReadRequest, document core.Document, webIR core.WebIR, segments []pipeline.Segment) (core.ResultPack, []proof.ProofRecord, error) {
	const stage = "pack"
	if err := recorder.StageStarted(stage, segments, s.now().UTC()); err != nil {
		return core.ResultPack{}, nil, err
	}

	ranked := rankSegments(document.ID, req.Objective, webIR, segments)
	ranked = applyContaminationPenalty(ranked, req.Objective)
	ranked = applyFingerprintNoveltyBias(ranked, req.StableFingerprints)
	intelSummary := s.analyzeRanked(recorder, req, ranked)
	selected := selectProfile(ranked, req.Profile)
	selected = s.applyIntel(req, selected, intelSummary.Decisions)
	selected, irPolicy := applyIRSelectionPolicy(ranked, selected, webIR, req.Profile)
	if req.Profile == core.ProfileTiny {
		selected = deduplicateSelected(selected, req.StableFingerprints)
	}
	selected = applyStableReadGate(ranked, selected, req.StableFingerprints)
	selected, reuseEligible, reuseApplied, reusedSet := applyPartialSelectionReuse(ranked, selected, req.StableFingerprints)
	annotateSelectionReuse(intelSummary.Decisions, selected, reusedSet)
	chunks := collectChunks(selected)
	proofRecords, err := buildProofRecords(selected, intelSummary.Decisions)
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
		CostReport: core.CostReport{LanePath: lanePath(intelSummary.MaxLane)},
	}
	stableHits, novelHits := fingerprintStability(chunks, req.StableFingerprints)

	if err := resultPack.Validate(); err != nil {
		return core.ResultPack{}, nil, err
	}

	metadata := map[string]string{
		"profile":                   req.Profile,
		"chunks":                    fmt.Sprintf("%d", len(chunks)),
		"page_type":                 intelSummary.PageType,
		"difficulty":                intelSummary.Difficulty,
		"noise_level":               intelSummary.NoiseLevel,
		"escalation_count":          fmt.Sprintf("%d", intelSummary.EscalationCount),
		"web_ir_nodes":              fmt.Sprintf("%d", webIR.NodeCount),
		"stable_fp_hits":            fmt.Sprintf("%d", stableHits),
		"novel_fp_hits":             fmt.Sprintf("%d", novelHits),
		"delta_class":               deltaClassFromHits(stableHits, novelHits),
		"reuse_mode":                reuseMode(req.StableFingerprints),
		"reuse_eligible":            fmt.Sprintf("%d", reuseEligible),
		"reuse_applied":             fmt.Sprintf("%d", reuseApplied),
		"reuse_recomputed":          fmt.Sprintf("%d", len(chunks)-reuseApplied),
		"selected_ir_embedded_hits": fmt.Sprintf("%d", irEmbeddedHits(selected)),
		"selected_ir_heading_hits":  fmt.Sprintf("%d", irHeadingHits(selected)),
		"selected_ir_shallow_hits":  fmt.Sprintf("%d", irShallowHits(selected)),
	}
	for key, value := range webIRSelectionPolicyMetadata(irPolicy) {
		metadata[key] = value
	}
	if err := recorder.StageCompleted(stage, resultPack, len(chunks), metadata, s.now().UTC()); err != nil {
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

func rankSegments(docID, objective string, webIR core.WebIR, segments []pipeline.Segment) []rankedSegment {
	irEvidence := buildIRSegmentEvidence(webIR)
	ranked := make([]rankedSegment, 0, len(segments))
	for index, segment := range segments {
		fingerprint := prefixedHash("fp", docID, strings.Join(segment.HeadingPath, "/"), segment.Text)
		segmentIR := irEvidence.forSegment(segment)
		score := segmentScore(segment, objective, index, len(segments), segmentIR)
		confidence := segmentConfidence(segment, objective)
		ranked = append(ranked, rankedSegment{
			segment: segment,
			index:   index,
			ir:      segmentIR,
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

	sortRankedSegments(ranked)
	return ranked
}

func selectProfile(ranked []rankedSegment, profile string) []rankedSegment {
	limit := len(ranked)
	switch profile {
	case core.ProfileTiny:
		limit = min(limit, 2)
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

func buildProofRecords(selected []rankedSegment, decisions map[string]intel.Decision) ([]proof.ProofRecord, error) {
	records := make([]proof.ProofRecord, 0, len(selected))
	for _, item := range selected {
		decision := decisions[item.chunk.Fingerprint]
		decision.RiskFlags = append(decision.RiskFlags, irRiskFlags(item.ir)...)
		decision.TransformChain = append(decision.TransformChain, irTransformMarks(item.ir)...)
		record, err := proof.BuildProofRecord(proof.BuildInput{
			Chunk:            item.chunk,
			Segment:          item.segment,
			Lane:             decision.Lane,
			TransformChain:   append([]string{"acquire:v1", "reduce:v1", "segment:v1", "web_ir:v1"}, decision.TransformChain...),
			ModelInvocations: decision.ModelInvocations,
			RiskFlags:        decision.RiskFlags,
		})
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func buildCostReport(trace proof.RunTrace, lanePath []int) core.CostReport {
	latency := int64(0)
	if !trace.FinishedAt.IsZero() {
		latency = trace.FinishedAt.Sub(trace.StartedAt).Milliseconds()
	}
	if len(lanePath) == 0 {
		lanePath = []int{0}
	}
	return core.CostReport{LatencyMS: latency, TokenIn: 0, TokenOut: 0, LanePath: append([]int{}, lanePath...)}
}

func fingerprintStability(chunks []core.Chunk, stable []string) (int, int) {
	seen := map[string]struct{}{}
	for _, fp := range stable {
		seen[fp] = struct{}{}
	}
	stableHits := 0
	for _, chunk := range chunks {
		if _, ok := seen[chunk.Fingerprint]; ok {
			stableHits++
		}
	}
	return stableHits, len(chunks) - stableHits
}

func applyFingerprintNoveltyBias(ranked []rankedSegment, stable []string) []rankedSegment {
	if len(ranked) == 0 || len(stable) == 0 {
		return ranked
	}
	seen, out := map[string]struct{}{}, append([]rankedSegment{}, ranked...)
	for _, fp := range stable {
		seen[fp] = struct{}{}
	}
	for i := range out {
		if _, ok := seen[out[i].chunk.Fingerprint]; ok {
			out[i].chunk.Score -= 0.05
		} else {
			out[i].chunk.Score += 0.05
		}
	}
	sortRankedSegments(out)
	return out
}

func applyStableReadGate(ranked, selected []rankedSegment, stable []string) []rankedSegment {
	if len(selected) == 0 || len(stable) == 0 {
		return selected
	}
	if stableHits, _ := fingerprintStability(collectChunks(selected), stable); stableHits > 0 {
		return selected
	}
	seen := map[string]struct{}{}
	for _, fp := range stable {
		seen[fp] = struct{}{}
	}
	for _, item := range ranked {
		if _, ok := seen[item.chunk.Fingerprint]; ok {
			out := append([]rankedSegment{}, selected...)
			out[len(out)-1] = item
			return out
		}
	}
	return selected
}

func objectiveOrDefault(objective string) string {
	if strings.TrimSpace(objective) != "" {
		return objective
	}
	return "read"
}

func sortRankedSegments(ranked []rankedSegment) {
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
}

func deltaClassFromHits(stableHits, novelHits int) string {
	switch {
	case stableHits > 0 && novelHits == 0:
		return "stable"
	case stableHits == 0 && novelHits > 0:
		return "changed"
	default:
		return "mixed"
	}
}

func reuseMode(stable []string) string {
	if len(stable) == 0 {
		return "fresh"
	}
	return "delta_aware"
}

func (s *Service) analyzeRanked(recorder *proof.Recorder, req ReadRequest, ranked []rankedSegment) intel.Summary {
	inputs := make([]intel.Input, 0, len(ranked))
	for _, item := range ranked {
		inputs = append(inputs, intel.Input{
			Fingerprint: item.chunk.Fingerprint,
			Text:        item.chunk.Text,
			HeadingPath: append([]string{}, item.chunk.HeadingPath...),
			Score:       item.chunk.Score,
			Confidence:  item.chunk.Confidence,
		})
	}

	summary := intel.New(s.cfg).Analyze(req.Objective, inputs, intel.Hints{
		ForceLane: req.ForceLane,
		Profile:   req.Profile,
	})
	for _, decision := range summary.Decisions {
		if decision.Lane == 0 {
			continue
		}
		recorder.EscalationTriggered(
			"pack",
			decision.ReasonCode,
			"lane escalation triggered by local ambiguity policy",
			decision.Lane,
			decision.Metadata(),
			s.now().UTC(),
		)
	}
	return summary
}

func lanePath(maxLane int) []int {
	path := []int{0}
	if maxLane <= 0 {
		return path
	}
	for lane := 1; lane <= maxLane; lane++ {
		path = append(path, lane)
	}
	return path
}

func (s *Service) applyIntel(req ReadRequest, selected []rankedSegment, decisions map[string]intel.Decision) []rankedSegment {
	out := make([]rankedSegment, 0, len(selected))
	for _, item := range selected {
		decision := decisions[item.chunk.Fingerprint]
		decision.RiskFlags = append(decision.RiskFlags, contaminationRiskFlags(item.chunk.Text, req.Objective)...)
		if decision.Lane >= 2 {
			extracted := intel.Extract(s.cfg, decision, item.chunk, req.Objective)
			if strings.TrimSpace(extracted.Text) != "" {
				item.chunk.Text = extracted.Text
			}
			if extracted.Invocation.Model != "" {
				decision.ModelInvocations = append(decision.ModelInvocations, extracted.Invocation)
			}
			decision.RiskFlags = append(decision.RiskFlags, extracted.AdditionalRisk...)
		}
		if decision.Lane >= 3 {
			formatted := intel.Format(s.cfg, decision, item.chunk, req.Profile)
			if strings.TrimSpace(formatted.Text) != "" {
				item.chunk.Text = formatted.Text
			}
			if formatted.Invocation.Model != "" {
				decision.ModelInvocations = append(decision.ModelInvocations, formatted.Invocation)
			}
			decision.RiskFlags = append(decision.RiskFlags, formatted.AdditionalRisk...)
		}
		if req.Profile == core.ProfileTiny {
			if compacted, changed := compactTinyText(item.chunk.Text, req.Objective); changed {
				item.chunk.Text = compacted
				decision.RiskFlags = append(decision.RiskFlags, "tiny_compaction")
				decision.TransformChain = append(decision.TransformChain, "pack:tiny_compact:v1")
			}
		}
		decisions[item.chunk.Fingerprint] = decision
		out = append(out, item)
	}
	return out
}

func prefixedHash(prefix string, parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return prefix + "_" + hex.EncodeToString(digest[:])[:16]
}
