package service

import (
	"context"
	"slices"
	"strings"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/intel"
)

func buildResolveAmbiguityInput(objective string, webIR core.WebIR, candidates []rankedSegment, decisions map[string]intel.Decision) (intel.ResolveAmbiguityInput, bool) {
	if len(candidates) < 2 {
		return intel.ResolveAmbiguityInput{}, false
	}
	out := intel.ResolveAmbiguityInput{
		Objective:    strings.TrimSpace(objective),
		FailureClass: intel.FailureClassAmbiguityConflict,
		Candidates:   make([]intel.AmbiguityCandidate, 0, len(candidates)),
	}
	if ambiguityRoutingSuppressed(webIR, candidates, decisions) {
		return intel.ResolveAmbiguityInput{}, false
	}
	ambiguousCandidates := 0
	for _, item := range candidates {
		decision := decisions[item.chunk.Fingerprint]
		if !candidateNeedsAmbiguityRoute(webIR, item, decision) {
			continue
		}
		ambiguousCandidates++
		out.Candidates = append(out.Candidates, intel.AmbiguityCandidate{
			ChunkID:     item.chunk.ID,
			Fingerprint: item.chunk.Fingerprint,
			Text:        item.chunk.Text,
			HeadingPath: append([]string{}, item.chunk.HeadingPath...),
			Score:       item.chunk.Score,
			Confidence:  item.chunk.Confidence,
			RiskFlags:   append([]string{}, decision.RiskFlags...),
			IRSignals:   candidateIRSignalsFromEvidence(item.ir),
		})
	}
	if ambiguousCandidates < 2 {
		return intel.ResolveAmbiguityInput{}, false
	}
	if err := out.Validate(); err != nil {
		return intel.ResolveAmbiguityInput{}, false
	}
	return out, true
}

func ambiguityRoutingSuppressed(webIR core.WebIR, candidates []rankedSegment, decisions map[string]intel.Decision) bool {
	if coverageAnchorSuppressed(candidates, decisions) {
		return true
	}
	if structuralDominanceSuppressed(candidates, decisions) {
		return true
	}
	switch strings.TrimSpace(webIR.Signals.SubstrateClass) {
	case "embedded_app_payload":
		return true
	case "theme_heavy_wordpress":
		return ambiguityStructurallyResolved(candidates, decisions, 0.82)
	default:
		return ambiguityStructurallyResolved(candidates, decisions, 0.86)
	}
}

func structuralDominanceSuppressed(candidates []rankedSegment, decisions map[string]intel.Decision) bool {
	if len(candidates) < 2 {
		return false
	}
	dominant := candidates[0]
	if dominant.chunk.Confidence < 0.90 || dominant.chunk.Score < 0.92 {
		return false
	}
	if !hasStructuralEvidence(dominant) {
		return false
	}
	if hasStrictAmbiguityRisk(decisions[dominant.chunk.Fingerprint].RiskFlags) {
		return false
	}
	for _, candidate := range candidates[1:] {
		if hasStructuralEvidence(candidate) && candidate.chunk.Score >= dominant.chunk.Score-0.05 && candidate.chunk.Confidence >= dominant.chunk.Confidence-0.05 {
			return false
		}
		if hasStrictAmbiguityRisk(decisions[candidate.chunk.Fingerprint].RiskFlags) && candidate.chunk.Score >= dominant.chunk.Score-0.08 {
			return false
		}
	}
	return true
}

func coverageAnchorSuppressed(candidates []rankedSegment, decisions map[string]intel.Decision) bool {
	if len(candidates) < 2 {
		return false
	}
	anchor := candidates[0]
	anchorDecision := decisions[anchor.chunk.Fingerprint]
	if anchor.chunk.Confidence < 0.88 || anchor.chunk.Score < 0.90 {
		return false
	}
	if !hasStructuralEvidence(anchor) {
		return false
	}
	if slices.Contains(anchorDecision.RiskFlags, "coverage_gap") || hasStrictAmbiguityRisk(anchorDecision.RiskFlags) {
		return false
	}
	for _, candidate := range candidates[1:] {
		decision := decisions[candidate.chunk.Fingerprint]
		if hasStrictAmbiguityRisk(decision.RiskFlags) {
			return false
		}
	}
	return true
}

func hasStructuralEvidence(item rankedSegment) bool {
	if item.ir.headingBacked || item.ir.kindMatch || item.ir.embedded {
		return true
	}
	return len(item.chunk.HeadingPath) > 0
}

func ambiguityStructurallyResolved(candidates []rankedSegment, decisions map[string]intel.Decision, minConfidence float64) bool {
	for _, item := range candidates {
		decision := decisions[item.chunk.Fingerprint]
		if item.chunk.Confidence < minConfidence || item.chunk.Score < minConfidence {
			continue
		}
		if !hasStructuralEvidence(item) {
			continue
		}
		if hasStrictAmbiguityRisk(decision.RiskFlags) {
			continue
		}
		return true
	}
	return false
}

func candidateNeedsAmbiguityRoute(webIR core.WebIR, item rankedSegment, decision intel.Decision) bool {
	if strings.TrimSpace(webIR.Signals.SubstrateClass) == "theme_heavy_wordpress" && !hasStrictAmbiguityRisk(decision.RiskFlags) {
		return false
	}
	if decision.Lane >= 1 && hasStrictAmbiguityRisk(decision.RiskFlags) {
		return true
	}
	if item.chunk.Confidence < 0.80 {
		return true
	}
	if item.chunk.Score < 0.84 {
		return true
	}
	for _, flag := range decision.RiskFlags {
		switch strings.TrimSpace(flag) {
		case "high_ambiguity", "coverage_gap", "short_text", "low_confidence":
			return true
		}
	}
	return false
}

func hasStrictAmbiguityRisk(flags []string) bool {
	for _, flag := range flags {
		switch strings.TrimSpace(flag) {
		case "high_ambiguity", "coverage_gap":
			return true
		}
	}
	return false
}

func candidateIRSignalsFromEvidence(e segmentIREvidence) []string {
	signals := []string{}
	if e.kindMatch {
		signals = append(signals, "kind_match")
	}
	if e.embedded {
		signals = append(signals, "embedded")
	}
	if e.headingBacked {
		signals = append(signals, "heading_backed")
	}
	if e.averageNodeDepth > 0 && e.averageNodeDepth <= 4 {
		signals = append(signals, "shallow_depth")
	}
	return intel.CandidateIRSignals(signals)
}

func (s *Service) semanticSuppressesAmbiguity(ctx context.Context, objective string, input intel.ResolveAmbiguityInput) (bool, string) {
	if s.semantic == nil {
		return false, ""
	}
	candidates := make([]intel.SemanticCandidate, 0, min(len(input.Candidates), s.cfg.Semantic.MaxCandidates))
	for _, candidate := range input.Candidates {
		if len(candidates) == s.cfg.Semantic.MaxCandidates {
			break
		}
		candidates = append(candidates, intel.SemanticCandidate{ID: candidate.ChunkID, Heading: candidate.HeadingPath, Text: candidate.Text})
	}
	out, err := s.semantic.Align(ctx, objective, candidates)
	if err != nil {
		return false, ""
	}
	return out.Suppressed, out.Reason
}
