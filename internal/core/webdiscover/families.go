package webdiscover

import (
	"slices"
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
)

func CanonicalizeCandidateFamilies(candidates []discoverycore.Candidate) []discoverycore.Candidate {
	if len(candidates) < 2 {
		return candidates
	}
	out := append([]discoverycore.Candidate{}, candidates...)
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
			out[j].Score += boost
			out[j].Reason = discoverycore.AppendUniqueReason(out[j].Reason, reason)
		}
	}
	discoverycore.SortCandidates(out)
	return out
}

func SameCandidateFamily(leftURL, rightURL string) bool {
	leftDomain, leftErr := discoverycore.RegistrableDomain(leftURL)
	rightDomain, rightErr := discoverycore.RegistrableDomain(rightURL)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return leftDomain == rightDomain
}

func CandidateFamily(rawURL string) (string, bool) {
	if family, err := discoverycore.RegistrableDomain(rawURL); err == nil && strings.TrimSpace(family) != "" {
		return family, true
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
