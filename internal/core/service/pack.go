package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

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

func (s *Service) pack(recorder *proof.Recorder, req ReadRequest, document core.Document, dom pipeline.SimplifiedDOM, webIR core.WebIR, segments []pipeline.Segment) (core.ResultPack, []proof.ProofRecord, error) {
	const stage = "pack"
	if err := recorder.StageStarted(stage, segments, s.now().UTC()); err != nil {
		return core.ResultPack{}, nil, err
	}
	intelSummary, selected, taskExecutions, irPolicy, reuseEligible, reuseApplied, err := s.preparePackSelection(recorder, req, dom, webIR, document.ID, segments)
	if err != nil {
		return core.ResultPack{}, nil, err
	}
	chunks := collectChunks(selected)
	proofRecords, err := buildProofRecords(selected, intelSummary.Decisions)
	if err != nil {
		recorder.Error(stage, "NX_PROOF_BUILD_FAILED", err.Error(), nil, s.now().UTC())
		return core.ResultPack{}, nil, err
	}
	resultPack, stableHits, novelHits, err := finalizePackResult(document, req, chunks, selected, proofRecords, intelSummary)
	if err != nil {
		return core.ResultPack{}, nil, err
	}
	return completePackStage(recorder, req, webIR, selected, taskExecutions, irPolicy, resultPack, proofRecords, intelSummary, stableHits, novelHits, reuseEligible, reuseApplied, s.now().UTC())
}

func finalizePackResult(document core.Document, req ReadRequest, chunks []core.Chunk, selected []rankedSegment, proofRecords []proof.ProofRecord, intelSummary intel.Summary) (core.ResultPack, int, int, error) {
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
		return core.ResultPack{}, 0, 0, err
	}
	return resultPack, stableHits, novelHits, nil
}

func completePackStage(recorder *proof.Recorder, req ReadRequest, webIR core.WebIR, selected []rankedSegment, taskExecutions []executedIntelTask, irPolicy webIRSelectionPolicyReport, resultPack core.ResultPack, proofRecords []proof.ProofRecord, intelSummary intel.Summary, stableHits, novelHits, reuseEligible, reuseApplied int, completedAt time.Time) (core.ResultPack, []proof.ProofRecord, error) {
	metadata := packStageMetadata(req, webIR, resultPack.Chunks, selected, intelSummary, stableHits, novelHits, reuseEligible, reuseApplied)
	for key, value := range webIRSelectionPolicyMetadata(irPolicy) {
		metadata[key] = value
	}
	for key, value := range intelTaskPlanMetadata(taskExecutions) {
		metadata[key] = value
	}
	if err := recorder.StageCompleted("pack", resultPack, len(resultPack.Chunks), metadata, completedAt); err != nil {
		return core.ResultPack{}, nil, err
	}
	return resultPack, proofRecords, nil
}

func (s *Service) preparePackSelection(recorder *proof.Recorder, req ReadRequest, dom pipeline.SimplifiedDOM, webIR core.WebIR, documentID string, segments []pipeline.Segment) (intel.Summary, []rankedSegment, []executedIntelTask, webIRSelectionPolicyReport, int, int, error) {
	ranked := s.rankSegments(documentID, req.Objective, webIR, segments)
	ranked = applyBoilerplatePenalty(ranked)
	ranked = applySubordinateFragmentDemotion(ranked)
	ranked = applyIndexLikeDemotion(ranked)
	ranked = applyCodeLikeDemotion(ranked)
	ranked = applyFingerprintNoveltyBias(ranked, req.StableFingerprints)
	intelSummary := s.analyzeRanked(recorder, req, ranked)
	selected := selectProfile(ranked, req.Profile)
	traceCtx := recorder.Trace()
	selected, taskExecutions, err := s.executeIntelTasks(context.Background(), recorder, req, webIR, selected, intelSummary.Decisions, intel.ModelTraceContext{
		TraceID:    traceCtx.TraceID,
		RunID:      traceCtx.RunID,
		ReasonCode: "NX_INTEL_TASK_ROUTED",
	})
	if err != nil {
		recorder.Error("pack", "NX_INTEL_TASK_EXEC_FAILED", err.Error(), nil, s.now().UTC())
		return intel.Summary{}, nil, nil, webIRSelectionPolicyReport{}, 0, 0, err
	}
	selected = s.applyIntel(req, selected, intelSummary.Decisions)
	selected, irPolicy := applyIRSelectionPolicy(ranked, selected, webIR, req.Profile)
	if req.Profile == core.ProfileTiny {
		selected = deduplicateSelected(selected, req.StableFingerprints)
	}
	selected = applyStableReadGate(ranked, selected, req.StableFingerprints)
	selected, reuseEligible, reuseApplied, reusedSet := applyPartialSelectionReuse(ranked, selected, req.StableFingerprints)
	annotateSelectionReuse(intelSummary.Decisions, selected, reusedSet)
	sortRankedSegments(selected)
	return intelSummary, selected, taskExecutions, irPolicy, reuseEligible, reuseApplied, nil
}

func packStageMetadata(req ReadRequest, webIR core.WebIR, chunks []core.Chunk, selected []rankedSegment, summary intel.Summary, stableHits, novelHits, reuseEligible, reuseApplied int) map[string]string {
	return map[string]string{
		"profile":                   req.Profile,
		"chunks":                    fmt.Sprintf("%d", len(chunks)),
		"page_type":                 summary.PageType,
		"substrate_class":           webIR.Signals.SubstrateClass,
		"difficulty":                summary.Difficulty,
		"noise_level":               summary.NoiseLevel,
		"escalation_count":          fmt.Sprintf("%d", summary.EscalationCount),
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
}

func (s *Service) rankSegments(docID, objective string, webIR core.WebIR, segments []pipeline.Segment) []rankedSegment {
	irEvidence := buildIRSegmentEvidence(webIR)
	alignments := s.segmentSemanticAlignment(objective, segments)
	ranked := make([]rankedSegment, 0, len(segments))
	for index, segment := range segments {
		fingerprint := prefixedHash("fp", docID, strings.Join(segment.HeadingPath, "/"), segment.Text)
		segmentIR := irEvidence.forSegment(segment)
		contextAlignment := alignments[fingerprint]
		score := segmentScore(segment, contextAlignment, index, len(segments), segmentIR)
		confidence := segmentConfidence(segment, contextAlignment)
		ranked = append(ranked, rankedSegment{
			segment: segment,
			index:   index,
			ir:      segmentIR,
			chunk: core.Chunk{
				ID:               prefixedHash("chk", docID, fingerprint),
				DocID:            docID,
				Text:             segment.Text,
				HeadingPath:      append([]string{}, segment.HeadingPath...),
				ContextAlignment: contextAlignment,
				Score:            score,
				Fingerprint:      fingerprint,
				Confidence:       confidence,
			},
		})
	}

	sortRankedSegments(ranked)
	return ranked
}

func (s *Service) segmentSemanticAlignment(objective string, segments []pipeline.Segment) map[string]float64 {
	out := make(map[string]float64, len(segments))
	objective = strings.TrimSpace(objective)
	if objective == "" || len(segments) == 0 {
		return out
	}
	candidates := make([]intel.SemanticCandidate, 0, len(segments))
	for _, segment := range segments {
		fingerprint := prefixedHash("fp", "", strings.Join(segment.HeadingPath, "/"), segment.Text)
		candidates = append(candidates, intel.SemanticCandidate{
			ID:      fingerprint,
			Heading: append([]string{}, segment.HeadingPath...),
			Text:    segment.Text,
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(s.cfg.Semantic.TimeoutMS)*time.Millisecond)
	defer cancel()
	scores, err := s.semantic.Score(ctx, objective, candidates)
	if err != nil {
		return out
	}
	for _, score := range scores {
		out[score.ID] = score.Similarity
	}
	return out
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
		enrichDecisionWithIR(&decision, item.ir)
		record, err := proof.BuildProofRecord(proof.BuildInput{
			Chunk:            item.chunk,
			Segment:          item.segment,
			Lane:             decision.Lane,
			TransformChain:   append(baseTransformChain(), decision.TransformChain...),
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

func baseTransformChain() []string {
	return []string{"acquire:v1", "reduce:v1", "segment:v1", "web_ir:v1"}
}

func enrichDecisionWithIR(decision *intel.Decision, ir segmentIREvidence) {
	decision.RiskFlags, decision.TransformChain = append(decision.RiskFlags, irRiskFlags(ir)...), append(decision.TransformChain, irTransformMarks(ir)...)
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
	seen := fingerprintSet(stable)
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
	seen, out := fingerprintSet(stable), append([]rankedSegment{}, ranked...)
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
	seen := fingerprintSet(stable)
	for _, item := range ranked {
		if _, ok := seen[item.chunk.Fingerprint]; ok {
			out := append([]rankedSegment{}, selected...)
			out[len(out)-1] = item
			return out
		}
	}
	return selected
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

func (s *Service) analyzeRanked(recorder *proof.Recorder, req ReadRequest, ranked []rankedSegment) intel.Summary {
	inputs := make([]intel.Input, 0, len(ranked))
	for _, item := range ranked {
		inputs = append(inputs, intel.Input{
			Fingerprint:      item.chunk.Fingerprint,
			Text:             item.chunk.Text,
			HeadingPath:      append([]string{}, item.chunk.HeadingPath...),
			ContextAlignment: item.chunk.ContextAlignment,
			Score:            item.chunk.Score,
			Confidence:       item.chunk.Confidence,
		})
	}

	summary := intel.New(s.cfg).Analyze(inputs, intel.Hints{
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

func (s *Service) applyIntel(req ReadRequest, selected []rankedSegment, decisions map[string]intel.Decision) []rankedSegment {
	out := make([]rankedSegment, 0, len(selected))
	for _, item := range selected {
		decision := decisions[item.chunk.Fingerprint]
		if decision.Lane >= 2 {
			extracted := intel.Extract(s.cfg, s.semantic, decision, item.chunk, req.Objective, req.Profile)
			applyIntelTextResult(&item.chunk, &decision, extracted.Text, extracted.Invocation, extracted.AdditionalRisk)
		}
		if decision.Lane >= 3 {
			formatted := intel.Format(s.cfg, decision, item.chunk, req.Profile)
			applyIntelTextResult(&item.chunk, &decision, formatted.Text, formatted.Invocation, formatted.AdditionalRisk)
		}
		if req.Profile == core.ProfileTiny {
			if compacted, changed := s.compactTinyText(item.chunk.Text, req.Objective); changed {
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
