package service

import (
	"strings"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/proof"
)

func deduplicateSelected(selected []rankedSegment, stable []string) []rankedSegment {
	if len(selected) <= 1 {
		return selected
	}
	stableSet := fingerprintSet(stable)
	out := make([]rankedSegment, 0, len(selected))
	kept := map[string]int{}
	for _, item := range selected {
		norm := normalizeChunkForDedup(item.chunk.Text)
		if norm == "" {
			continue
		}
		if idx, ok := kept[norm]; ok {
			if _, stable := stableSet[out[idx].chunk.Fingerprint]; stable {
				if _, stable := stableSet[item.chunk.Fingerprint]; !stable {
					out[idx] = item
				}
			}
			continue
		}
		if idx := nearDuplicateIndex(norm, out); idx >= 0 {
			if _, stable := stableSet[out[idx].chunk.Fingerprint]; stable {
				if _, stable := stableSet[item.chunk.Fingerprint]; !stable {
					out[idx] = item
				}
			}
			continue
		}
		kept[norm] = len(out)
		out = append(out, item)
	}
	if len(out) == 0 {
		return selected[:1]
	}
	return out
}

func applyPartialSelectionReuse(ranked, selected []rankedSegment, stable []string) ([]rankedSegment, int, int, map[string]struct{}) {
	if len(selected) == 0 || len(stable) == 0 {
		return selected, 0, 0, nil
	}
	byFP, ordered := map[string]rankedSegment{}, make([]rankedSegment, 0, len(stable))
	for _, item := range ranked {
		byFP[item.chunk.Fingerprint] = item
	}
	for _, fp := range stable {
		if item, ok := byFP[fp]; ok {
			ordered = append(ordered, item)
		}
	}
	eligible := len(ordered)
	if eligible == 0 {
		return selected, 0, 0, nil
	}
	if stableSelected, _ := fingerprintStability(collectChunks(selected), stable); stableSelected*3 < len(selected)*2 {
		return selected, eligible, 0, nil
	}
	limit, budget := len(selected), max(1, len(selected)/2)
	if budget > eligible {
		budget = eligible
	}
	out, seen, reused := make([]rankedSegment, 0, limit), map[string]struct{}{}, map[string]struct{}{}
	add := func(item rankedSegment, mark bool) {
		if len(out) >= limit {
			return
		}
		if _, ok := seen[item.chunk.Fingerprint]; ok {
			return
		}
		seen[item.chunk.Fingerprint] = struct{}{}
		if mark {
			reused[item.chunk.Fingerprint] = struct{}{}
		}
		out = append(out, item)
	}
	for _, item := range ordered[:budget] {
		add(item, true)
	}
	for _, items := range [][]rankedSegment{selected, ranked} {
		for _, item := range items {
			add(item, false)
		}
	}
	return out, eligible, len(reused), reused
}

func annotateSelectionReuse(decisions map[string]intel.Decision, selected []rankedSegment, reused map[string]struct{}) {
	if len(reused) == 0 {
		return
	}
	for _, item := range selected {
		decision := decisions[item.chunk.Fingerprint]
		if _, ok := reused[item.chunk.Fingerprint]; ok {
			decision.RiskFlags = append(decision.RiskFlags, "selection_reused")
			decision.TransformChain = append(decision.TransformChain, "pack:reuse:v1")
		} else {
			decision.RiskFlags = append(decision.RiskFlags, "selection_recomputed")
		}
		decisions[item.chunk.Fingerprint] = decision
	}
}

func buildOutline(selected []rankedSegment) []string {
	out, seen := make([]string, 0, len(selected)), map[string]struct{}{}
	for _, item := range selected {
		if len(item.chunk.HeadingPath) == 0 {
			continue
		}
		label := normalizeOutlineLabel(item.chunk.HeadingPath)
		if label == "" {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	return out
}

func proofIDs(records []proof.ProofRecord) []string {
	return collectStrings(records, func(record proof.ProofRecord) string { return record.ID })
}

func chunkIDs(chunks []core.Chunk) []string {
	return collectStrings(chunks, func(chunk core.Chunk) string { return chunk.ID })
}

func collectStrings[T any](items []T, value func(T) string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, value(item))
	}
	return out
}

func normalizeChunkForDedup(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return ""
	}
	return strings.Join(strings.Fields(text), " ")
}

func nearDuplicateIndex(candidate string, existing []rankedSegment) int {
	for i, item := range existing {
		norm := normalizeChunkForDedup(item.chunk.Text)
		if candidate == norm {
			return i
		}
		short, long := candidate, norm
		if len(short) > len(long) {
			short, long = long, short
		}
		if len(short) < 40 {
			continue
		}
		if strings.HasPrefix(long, short) || strings.HasSuffix(long, short) {
			ratio := float64(len(short)) / float64(len(long))
			if ratio >= 0.70 {
				return i
			}
		}
	}
	return -1
}

func normalizeOutlineLabel(path []string) string {
	if len(path) == 0 {
		return ""
	}
	parts := make([]string, 0, len(path))
	for _, item := range path {
		label := strings.TrimSpace(item)
		if label == "" {
			continue
		}
		sub := splitBreadcrumb(label)
		if len(sub) > 2 {
			sub = sub[len(sub)-2:]
		}
		if hasSuffixSequence(parts, sub) {
			continue
		}
		for _, piece := range sub {
			piece = strings.Join(strings.Fields(strings.TrimSpace(piece)), " ")
			if piece == "" {
				continue
			}
			if len(parts) > 0 && parts[len(parts)-1] == piece {
				continue
			}
			parts = append(parts, piece)
		}
	}
	return strings.Join(parts, " > ")
}

func hasSuffixSequence(parts, seq []string) bool {
	if len(seq) == 0 || len(parts) < len(seq) {
		return false
	}
	start := len(parts) - len(seq)
	for idx := range seq {
		if parts[start+idx] != strings.TrimSpace(seq[idx]) {
			return false
		}
	}
	return true
}

func splitBreadcrumb(value string) []string {
	raw := strings.Split(value, ">")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return []string{strings.TrimSpace(value)}
	}
	return out
}
