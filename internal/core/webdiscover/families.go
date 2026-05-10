package webdiscover

import (
	"net/url"
	"path"
	"slices"
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
)

func CanonicalizeCandidateFamilies(candidates []discoverycore.Candidate) []discoverycore.Candidate {
	if len(candidates) < 2 {
		return candidates
	}
	out := append([]discoverycore.Candidate{}, candidates...)
	boosts := make([]float64, len(out))
	reasons := make([]string, len(out))
	for i := range out {
		for j := range out {
			if i == j {
				continue
			}
			left := out[i]
			right := out[j]
			if !SameCandidateFamily(left.URL, right.URL) {
				continue
			}
			leftDepth := discoverycore.URLPathDepth(left.URL)
			rightDepth := discoverycore.URLPathDepth(right.URL)
			if rightDepth >= leftDepth {
				continue
			}
			if left.Score-right.Score > 0.35 {
				continue
			}
			boost := 0.12
			reason := "same_family_shallow_preference"
			if SameHost(left.URL, right.URL) && rightDepth == 0 {
				boost = 0.28
				reason = "same_host_canonical_root"
			} else if rightDepth == 0 {
				boost = 0.18
				reason = "same_family_canonical_root"
			}
			if boost > boosts[j] {
				boosts[j] = boost
				reasons[j] = reason
			}
		}
	}
	for i, boost := range boosts {
		if boost == 0 {
			continue
		}
		out[i].Score += boost
		out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, reasons[i])
	}
	discoverycore.SortCandidates(out)
	return out
}

func DampenWeakProvenanceTraps(candidates []discoverycore.Candidate) []discoverycore.Candidate {
	if len(candidates) < 2 {
		return candidates
	}
	strongFamilies := strongEvidenceFamilies(candidates)
	if len(strongFamilies) == 0 {
		return candidates
	}
	out := append([]discoverycore.Candidate{}, candidates...)
	for i := range out {
		if hasAnyReason(out[i], "weak_canonical_root_context_penalty", "weak_recovered_family_context_penalty") {
			continue
		}
		switch {
		case weakCanonicalRootTrap(out[i], strongFamilies):
			out[i].Score -= weakCanonicalRootPenalty(out[i])
			out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, "weak_canonical_root_context_penalty")
		case weakRecoveredFamilyTrap(out[i], strongFamilies):
			out[i].Score -= 0.42
			out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, "weak_recovered_family_context_penalty")
		default:
			continue
		}
	}
	discoverycore.SortCandidates(out)
	return out
}

func DampenCrossFamilyMirrorRoutes(candidates []discoverycore.Candidate) []discoverycore.Candidate {
	if len(candidates) < 2 {
		return candidates
	}
	out := append([]discoverycore.Candidate{}, candidates...)
	for i := range out {
		penalty := crossFamilyMirrorRoutePenalty(out[i], out)
		if penalty == 0 {
			continue
		}
		out[i].Score -= penalty
		out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, "cross_family_mirror_route_penalty")
	}
	discoverycore.SortCandidates(out)
	return out
}

func PromoteRecoveredCanonicalOrigins(candidates []discoverycore.Candidate) []discoverycore.Candidate {
	if len(candidates) < 2 {
		return candidates
	}
	out := append([]discoverycore.Candidate{}, candidates...)
	for i := range out {
		if !discoverycore.IsCanonicalHomeURL(out[i].URL) {
			continue
		}
		if !hasAnyReason(out[i], "external_family_recovery") || !hasAnyReason(out[i], "page_expand") {
			continue
		}
		if !hasAnyReason(out[i], "semantic_goal_alignment", "semantic_evidence_probe", "semantic_root_identity_probe", "host_root_identity_probe", "identity_reference", "semantic_family_alignment", "semantic_custodian_alignment") {
			continue
		}
		out[i].Score += 0.22
		out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, "recovered_canonical_origin")
	}
	discoverycore.SortCandidates(out)
	return out
}

func strongEvidenceFamilies(candidates []discoverycore.Candidate) map[string]struct{} {
	out := map[string]struct{}{}
	for _, candidate := range candidates {
		if !hasAnyReason(candidate, "semantic_root_identity_probe", "host_root_identity_probe", "host_root_candidate", "identity_reference", "semantic_family_alignment", "semantic_custodian_alignment", "semantic_quorum_provider_fusion") {
			continue
		}
		family, ok := CandidateFamily(candidate.URL)
		if !ok || family == "" {
			continue
		}
		out[family] = struct{}{}
	}
	return out
}

func weakCanonicalRootTrap(candidate discoverycore.Candidate, strongFamilies map[string]struct{}) bool {
	if !discoverycore.IsCanonicalHomeURL(candidate.URL) {
		return false
	}
	if !hasAnyReason(candidate, "same_host_canonical_root", "same_family_canonical_root") {
		return false
	}
	if hasAnyReason(candidate, "semantic_root_identity_probe", "host_root_identity_probe", "host_root_candidate", "identity_reference", "semantic_family_alignment", "semantic_custodian_alignment", "semantic_quorum_provider_fusion") {
		return false
	}
	family, ok := CandidateFamily(candidate.URL)
	if !ok || family == "" {
		return false
	}
	if _, ok := strongFamilies[family]; ok {
		return false
	}
	return true
}

func weakCanonicalRootPenalty(candidate discoverycore.Candidate) float64 {
	penalty := 1.18
	if !hasAnyReason(candidate, "page_title_probe", "web_ir_probe") {
		penalty += 0.22
	}
	return penalty
}

func weakRecoveredFamilyTrap(candidate discoverycore.Candidate, strongFamilies map[string]struct{}) bool {
	if !hasAnyReason(candidate, "same_family_child_recovery", "page_expand_child_context") {
		return false
	}
	if hasAnyReason(candidate, "semantic_root_identity_probe", "host_root_identity_probe", "host_root_candidate", "identity_reference", "semantic_family_alignment", "semantic_custodian_alignment", "semantic_quorum_provider_fusion") {
		return false
	}
	family, ok := CandidateFamily(candidate.URL)
	if !ok || family == "" {
		return false
	}
	if _, ok := strongFamilies[family]; ok {
		return false
	}
	return true
}

func crossFamilyMirrorRoutePenalty(candidate discoverycore.Candidate, candidates []discoverycore.Candidate) float64 {
	if discoverycore.URLPathDepth(candidate.URL) < 2 {
		return 0
	}
	candidateFamily, ok := CandidateFamily(candidate.URL)
	if !ok || candidateFamily == "" {
		return 0
	}
	if !hasAnyReason(candidate, "page_title_probe", "web_ir_probe", "same_family_child_recovery", "page_expand_child_context") {
		return 0
	}
	penalty := 0.0
	for _, anchor := range candidates {
		anchorFamily, ok := CandidateFamily(anchor.URL)
		if !ok || anchorFamily == "" || anchorFamily == candidateFamily {
			continue
		}
		if !hasAnyReason(anchor, "semantic_root_identity_probe", "host_root_identity_probe", "host_root_candidate", "identity_reference", "semantic_family_alignment", "semantic_custodian_alignment", "semantic_quorum_provider_fusion") {
			continue
		}
		switch {
		case URLPathEmbedsHost(candidate.URL, anchor.URL):
			penalty = max(penalty, 0.58)
		case CrossFamilyRouteDescendant(candidate.URL, anchor.URL):
			penalty = max(penalty, 0.34)
		}
	}
	return penalty
}

func URLPathEmbedsHost(candidateURL, anchorURL string) bool {
	anchorHost, ok := discoverycore.Hostname(anchorURL)
	if !ok || anchorHost == "" {
		return false
	}
	parsed, err := url.Parse(strings.TrimSpace(candidateURL))
	if err != nil {
		return false
	}
	cleanPath := strings.ToLower(strings.Trim(parsed.EscapedPath(), "/"))
	if cleanPath == "" {
		return false
	}
	return strings.Contains(cleanPath, strings.ToLower(anchorHost))
}

func CrossFamilyRouteDescendant(candidateURL, anchorURL string) bool {
	candidatePath, ok := normalizedURLPath(candidateURL)
	if !ok || candidatePath == "/" {
		return false
	}
	anchorPath, ok := normalizedURLPath(anchorURL)
	if !ok {
		return false
	}
	if anchorPath == "/" {
		return false
	}
	return strings.HasPrefix(candidatePath, strings.TrimRight(anchorPath, "/")+"/")
}

func normalizedURLPath(rawURL string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", false
	}
	clean := strings.TrimSpace(parsed.EscapedPath())
	if clean == "" || clean == "/" {
		return "/", true
	}
	return path.Clean("/" + strings.Trim(clean, "/")), true
}

func SameCandidateFamily(leftURL, rightURL string) bool {
	leftDomain, leftOK := CandidateFamily(leftURL)
	rightDomain, rightOK := CandidateFamily(rightURL)
	if !leftOK || !rightOK {
		return false
	}
	return leftDomain == rightDomain
}

func CandidateFamily(rawURL string) (string, bool) {
	if family, err := discoverycore.RegistrableDomain(rawURL); err == nil && strings.TrimSpace(family) != "" {
		return family, true
	}
	if parsed, err := url.Parse(strings.TrimSpace(rawURL)); err == nil {
		if host := strings.TrimSpace(strings.ToLower(parsed.Host)); host != "" {
			return host, true
		}
	}
	if host, ok := discoverycore.Hostname(rawURL); ok {
		return host, true
	}
	return "", false
}

func SameHost(leftURL, rightURL string) bool {
	leftHost, leftOK := discoverycore.Hostname(leftURL)
	rightHost, rightOK := discoverycore.Hostname(rightURL)
	return leftOK && rightOK && leftHost == rightHost
}

func LocalSubstrateResolved(candidate discoverycore.Candidate) bool {
	if candidate.Score >= 1.8 {
		return true
	}
	return slices.Contains(candidate.Reason, "semantic_goal_alignment") ||
		slices.Contains(candidate.Reason, "structure_hint")
}

func hasAnyReason(candidate discoverycore.Candidate, reasons ...string) bool {
	for _, existing := range candidate.Reason {
		for _, wanted := range reasons {
			if strings.TrimSpace(existing) == wanted {
				return true
			}
		}
	}
	return false
}
