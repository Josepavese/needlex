package queryflow

import (
	"fmt"
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
)

type FingerprintEvidence struct {
	TraceID         string
	Stable, Novelty float64
	Changed         bool
}

func RerankCandidatesWithFingerprintEvidence(candidates []discoverycore.Candidate, seedURL string, seedEvidence FingerprintEvidence, loader func(string) (FingerprintEvidence, bool)) []discoverycore.Candidate {
	if len(candidates) < 2 {
		return candidates
	}
	out := append([]discoverycore.Candidate{}, candidates...)
	for i := range out {
		evidence, ok := FingerprintEvidence{}, out[i].URL == seedURL && strings.TrimSpace(seedEvidence.TraceID) != ""
		if out[i].URL == seedURL {
			evidence = seedEvidence
		} else if loader != nil {
			evidence, ok = loader(out[i].URL)
		}
		if ok {
			out[i] = applyFingerprintEvidenceToCandidate(out[i], evidence, out[i].URL == seedURL)
		}
	}
	discoverycore.SortCandidates(out)
	return out
}

func ShouldEscalateRewrite(selectedURL string, candidates []discoverycore.Candidate) bool {
	if strings.TrimSpace(selectedURL) == "" || len(candidates) == 0 {
		return true
	}
	if len(candidates) < 2 {
		return false
	}
	return SelectionDelta(candidates) <= 0.25
}

func SelectionDelta(candidates []discoverycore.Candidate) float64 {
	if len(candidates) < 2 {
		return 0
	}
	first := candidates[0].Score
	second := candidates[1].Score
	if first < second {
		first, second = second, first
	}
	return first - second
}

func FinalizeDiscoveryResult(candidates []discoverycore.Candidate, selected, seedURL string, seedEvidence FingerprintEvidence, loader func(string) (FingerprintEvidence, bool)) ([]discoverycore.Candidate, string) {
	candidates = RerankCandidatesWithFingerprintEvidence(candidates, seedURL, seedEvidence, loader)
	selected = selectedURL(candidates, selected)
	return candidates, selected
}

func selectedURL(candidates []discoverycore.Candidate, fallback string) string {
	if len(candidates) == 0 {
		return fallback
	}
	return candidates[0].URL
}

func applyFingerprintEvidenceToCandidate(candidate discoverycore.Candidate, evidence FingerprintEvidence, isSeed bool) discoverycore.Candidate {
	if strings.TrimSpace(evidence.TraceID) == "" {
		return candidate
	}
	candidate.Metadata = mergeMetadata(candidate.Metadata, map[string]string{
		"candidate_latest_trace_id": strings.TrimSpace(evidence.TraceID),
		"candidate_stable_ratio":    fmt.Sprintf("%.3f", evidence.Stable),
		"candidate_novelty_ratio":   fmt.Sprintf("%.3f", evidence.Novelty),
		"candidate_changed":         fmt.Sprintf("%t", evidence.Changed),
	})
	switch {
	case evidence.Stable >= 0.80 && !evidence.Changed:
		candidate.Score -= 0.20
		if isSeed {
			candidate.Reason = discoverycore.AppendUniqueReason(candidate.Reason, "stable_seed_penalty")
		} else {
			candidate.Reason = discoverycore.AppendUniqueReason(candidate.Reason, "stable_candidate_penalty")
		}
	case evidence.Novelty > 0.20 || evidence.Changed:
		candidate.Score += 0.20
		if isSeed {
			candidate.Reason = discoverycore.AppendUniqueReason(candidate.Reason, "novel_seed_bias")
		} else {
			candidate.Reason = discoverycore.AppendUniqueReason(candidate.Reason, "novel_candidate_bias")
		}
	}
	return candidate
}

func mergeMetadata(existing, incoming map[string]string) map[string]string {
	if len(existing) == 0 && len(incoming) == 0 {
		return nil
	}
	out := make(map[string]string, len(existing)+len(incoming))
	for key, value := range existing {
		out[key] = value
	}
	for key, value := range incoming {
		out[key] = value
	}
	return out
}
