package service

import (
	"slices"
	"strconv"
	"strings"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/pipeline"
)

type webIRSegmentEvidence struct {
	byPath map[string]core.WebIRNode
}

func buildIRSegmentEvidence(webIR core.WebIR) webIRSegmentEvidence {
	byPath := make(map[string]core.WebIRNode, len(webIR.Nodes))
	for _, node := range webIR.Nodes {
		byPath[node.Path] = node
	}
	return webIRSegmentEvidence{byPath: byPath}
}

func (e webIRSegmentEvidence) forSegment(segment pipeline.Segment) segmentIREvidence {
	if len(segment.NodePaths) == 0 || len(e.byPath) == 0 {
		return segmentIREvidence{}
	}
	var out segmentIREvidence
	depthSum := 0
	depthCount := 0
	for _, path := range segment.NodePaths {
		node, ok := e.byPath[path]
		if !ok {
			continue
		}
		if node.Kind == segment.Kind {
			out.kindMatch = true
		}
		if node.Kind == "heading" {
			out.headingBacked = true
		}
		if strings.HasPrefix(node.Path, "/embedded/") {
			out.embedded = true
		}
		if node.Depth > 0 {
			depthSum += node.Depth
			depthCount++
		}
	}
	if depthCount > 0 {
		out.averageNodeDepth = float64(depthSum) / float64(depthCount)
	}
	return out
}

type webIRSelectionPolicyReport struct {
	EmbeddedAnchorRequired bool
	EmbeddedAnchorApplied  bool
	HeadingAnchorRequired  bool
	HeadingAnchorApplied   bool
	NoiseSwapApplied       bool
}

func applyIRSelectionPolicy(ranked, selected []rankedSegment, webIR core.WebIR, profile string) ([]rankedSegment, webIRSelectionPolicyReport) {
	report := webIRSelectionPolicyReport{
		EmbeddedAnchorRequired: webIR.Signals.EmbeddedNodeCount > 0,
		HeadingAnchorRequired:  webIR.Signals.HeadingRatio >= 0.15,
	}
	if len(ranked) == 0 || len(selected) == 0 {
		return selected, report
	}

	out := append([]rankedSegment{}, selected...)

	if report.EmbeddedAnchorRequired && !hasEvidence(out, func(e segmentIREvidence) bool { return e.embedded }) {
		candidate, ok := firstRankedCandidate(ranked, out, func(item rankedSegment) bool { return item.ir.embedded })
		if ok {
			out = replaceWeakestForEvidence(out, candidate, func(item rankedSegment) bool { return !item.ir.embedded })
			report.EmbeddedAnchorApplied = true
		}
	}

	if report.HeadingAnchorRequired && !hasEvidence(out, func(e segmentIREvidence) bool { return e.headingBacked }) {
		candidate, ok := firstRankedCandidate(ranked, out, func(item rankedSegment) bool { return item.ir.headingBacked })
		if ok {
			out = replaceWeakestForEvidence(out, candidate, func(item rankedSegment) bool { return !item.ir.headingBacked })
			report.HeadingAnchorApplied = true
		}
	}

	highShortText := webIR.Signals.ShortTextRatio >= 0.80
	if highShortText && profile != core.ProfileDeep {
		if weakIdx := weakIRSelectionIndex(out); weakIdx >= 0 {
			candidate, ok := firstRankedCandidate(ranked, out, strongerIRCandidate)
			if ok {
				out[weakIdx] = candidate
				report.NoiseSwapApplied = true
			}
		}
	}

	return out, report
}

func webIRSelectionPolicyMetadata(report webIRSelectionPolicyReport) map[string]string {
	return map[string]string{
		"web_ir_policy_embedded_required": strconv.FormatBool(report.EmbeddedAnchorRequired),
		"web_ir_policy_embedded_applied":  strconv.FormatBool(report.EmbeddedAnchorApplied),
		"web_ir_policy_heading_required":  strconv.FormatBool(report.HeadingAnchorRequired),
		"web_ir_policy_heading_applied":   strconv.FormatBool(report.HeadingAnchorApplied),
		"web_ir_policy_noise_swap":        strconv.FormatBool(report.NoiseSwapApplied),
	}
}

func hasEvidence(selected []rankedSegment, predicate func(segmentIREvidence) bool) bool {
	for _, item := range selected {
		if predicate(item.ir) {
			return true
		}
	}
	return false
}

func firstRankedCandidate(ranked, selected []rankedSegment, predicate func(rankedSegment) bool) (rankedSegment, bool) {
	fingerprints := make([]string, 0, len(selected))
	for _, item := range selected {
		fingerprints = append(fingerprints, item.chunk.Fingerprint)
	}
	for _, item := range ranked {
		if slices.Contains(fingerprints, item.chunk.Fingerprint) {
			continue
		}
		if predicate(item) {
			return item, true
		}
	}
	return rankedSegment{}, false
}

func replaceWeakestForEvidence(selected []rankedSegment, candidate rankedSegment, weakPredicate func(rankedSegment) bool) []rankedSegment {
	replace := len(selected) - 1
	for i := len(selected) - 1; i >= 0; i-- {
		if weakPredicate(selected[i]) {
			replace = i
			break
		}
	}
	out := append([]rankedSegment{}, selected...)
	out[replace] = candidate
	return out
}

func weakIRSelectionIndex(selected []rankedSegment) int {
	for i := len(selected) - 1; i >= 0; i-- {
		item := selected[i]
		if len(strings.TrimSpace(item.chunk.Text)) >= 40 {
			continue
		}
		if item.ir.headingBacked || item.ir.embedded || item.ir.kindMatch {
			continue
		}
		return i
	}
	return -1
}

func strongerIRCandidate(item rankedSegment) bool {
	return item.ir.headingBacked || item.ir.embedded || item.ir.kindMatch || len(strings.TrimSpace(item.chunk.Text)) >= 80
}

func irEmbeddedHits(selected []rankedSegment) int {
	count := 0
	for _, item := range selected {
		if item.ir.embedded {
			count++
		}
	}
	return count
}

func irHeadingHits(selected []rankedSegment) int {
	count := 0
	for _, item := range selected {
		if item.ir.headingBacked {
			count++
		}
	}
	return count
}

func irShallowHits(selected []rankedSegment) int {
	count := 0
	for _, item := range selected {
		if item.ir.averageNodeDepth > 0 && item.ir.averageNodeDepth <= 4 {
			count++
		}
	}
	return count
}

func irRiskFlags(e segmentIREvidence) []string {
	out := []string{}
	if e.embedded {
		out = append(out, "ir_embedded")
	}
	if e.headingBacked {
		out = append(out, "ir_heading_backed")
	}
	if e.averageNodeDepth > 0 && e.averageNodeDepth <= 4 {
		out = append(out, "ir_shallow_depth")
	}
	if e.averageNodeDepth >= 9 {
		out = append(out, "ir_deep_path")
	}
	if !e.kindMatch && !e.embedded && !e.headingBacked {
		out = append(out, "ir_weak_support")
	}
	return out
}

func irTransformMarks(e segmentIREvidence) []string {
	out := []string{}
	if e.embedded {
		out = append(out, "web_ir:embedded:v1")
	}
	if e.headingBacked {
		out = append(out, "web_ir:heading_backed:v1")
	}
	if e.kindMatch {
		out = append(out, "web_ir:kind_match:v1")
	}
	if e.averageNodeDepth > 0 && e.averageNodeDepth <= 4 {
		out = append(out, "web_ir:depth:shallow:v1")
	}
	if len(out) == 0 {
		return []string{"web_ir:evidence:weak:v1"}
	}
	out = append(out, "web_ir:evidence:strong:v1")
	return out
}
