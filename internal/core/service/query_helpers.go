package service

import (
	"fmt"
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/core/queryplan"
)

func queryPlanCandidates(candidates []DiscoverCandidate) []queryplan.Candidate {
	out := make([]queryplan.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, queryPlanCandidate(candidate))
	}
	return out
}

func queryPlanCandidate(candidate DiscoverCandidate) queryplan.Candidate {
	return queryplan.Candidate{
		URL:      candidate.URL,
		Score:    candidate.Score,
		Reason:   append([]string{}, candidate.Reason...),
		Metadata: discoverycore.MergeMetadata(nil, candidate.Metadata),
	}
}

func resolveDiscoveryMode(mode string) (string, error) {
	mode = normalizeDiscoveryMode(mode)
	if mode == "" {
		return QueryDiscoverySameSite, nil
	}
	switch mode {
	case QueryDiscoveryOff, QueryDiscoverySameSite, QueryDiscoveryWeb:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported discovery_mode %q; valid values: %s", mode, strings.Join(validDiscoveryModes(), ", "))
	}
}

func normalizeDiscoveryMode(mode string) string {
	mode = strings.TrimSpace(strings.ToLower(mode))
	switch mode {
	case "", QueryDiscoveryOff, QueryDiscoverySameSite, QueryDiscoveryWeb:
		return mode
	case "same-site", "same_site", "same site", "same-site-links", "same_site_link", "same site links":
		return QueryDiscoverySameSite
	case "web-search", "web search":
		return QueryDiscoveryWeb
	default:
		return mode
	}
}

func validDiscoveryModes() []string {
	return []string{QueryDiscoverySameSite, QueryDiscoveryWeb, QueryDiscoveryOff}
}
