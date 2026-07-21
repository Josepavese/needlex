package targetkind

import (
	"net/url"
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/intel"
)

func targetKindSemanticCandidates(candidates []discoverycore.Candidate) []intel.SemanticCandidate {
	out := make([]intel.SemanticCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, intel.SemanticCandidate{
			ID:   candidate.URL,
			Text: targetKindCandidateText(candidate),
		})
	}
	return out
}

func targetKindCandidateText(candidate discoverycore.Candidate) string {
	return discoverycore.JoinNonEmpty(
		candidate.Metadata["host_root_title"],
		candidate.Metadata["host_root_context"],
		candidate.Metadata["page_title"],
		candidate.Metadata["web_ir_context"],
		strings.TrimSpace(candidate.Label),
		candidate.Metadata["resource_class"],
	)
}

func targetKindCandidateCompatibility(kind string, semantic, topology float64) float64 {
	switch kind {
	case CanonicalHome:
		if topology > 0 {
			return topology + semantic*0.20
		}
		if semantic >= 0.35 && topology > -0.30 {
			return semantic*0.25 + topology*0.25
		}
		return topology
	case OrganizationAbout:
		if semantic <= 0 {
			return 0
		}
		compatibility := semantic*1.25 + topology*0.45
		if semantic < 0.22 {
			compatibility -= 0.18
		}
		return compatibility
	default:
		return 0
	}
}

func targetKindTopologyCompatibility(kind, rawURL string) float64 {
	depth := discoverycore.URLPathDepth(rawURL)
	home := discoverycore.IsCanonicalHomeURL(rawURL)
	resourceClass := discoverycore.ResourceClass(rawURL)
	htmlLike := resourceClass == discoverycore.ResourceClassHTMLLike
	queryPenalty := targetKindQueryPenalty(rawURL)

	switch kind {
	case CanonicalHome:
		switch {
		case home:
			return 1.15 + queryPenalty
		case depth == 1 && htmlLike:
			return -0.18 + queryPenalty
		case depth == 2 && htmlLike:
			return -0.58 + queryPenalty
		case htmlLike:
			return -0.82 + queryPenalty
		default:
			return -0.90 + queryPenalty
		}
	case OrganizationAbout:
		switch {
		case home:
			return -0.22 + queryPenalty
		case depth == 1 && htmlLike:
			return 0.22 + queryPenalty
		case depth == 2 && htmlLike:
			return 0.16 + queryPenalty
		case htmlLike:
			return -0.10 + queryPenalty
		default:
			return -0.40 + queryPenalty
		}
	default:
		return 0
	}
}

func targetKindQueryPenalty(rawURL string) float64 {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || strings.TrimSpace(parsed.RawQuery) == "" {
		return 0
	}
	return -0.12
}
