package service

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/intel"
)

const (
	targetKindCanonicalHome       = "canonical_home"
	targetKindDocsLanding         = "docs_landing"
	targetKindSpecificDocument    = "specific_document"
	targetKindAPIReference        = "api_reference"
	targetKindLearningPath        = "learning_path"
	targetKindProductSolutionPage = "product_solution_page"
	targetKindDownloadRelease     = "download_release"
	targetKindOrganizationAbout   = "organization_about"
)

type targetKindProfile struct {
	Kind       string
	Similarity float64
	Margin     float64
}

func (s *Service) applyTargetKindRerank(ctx context.Context, goal string, candidates []DiscoverCandidate) []DiscoverCandidate {
	if strings.TrimSpace(goal) == "" || len(candidates) == 0 {
		return candidates
	}
	profile := s.inferTargetKindProfile(ctx, goal)
	if strings.TrimSpace(profile.Kind) == "" {
		return candidates
	}
	if profile.Kind != targetKindCanonicalHome && profile.Kind != targetKindOrganizationAbout {
		return candidates
	}
	out := append([]DiscoverCandidate{}, candidates...)
	weight := targetKindWeight(profile)
	if weight <= 0 {
		return out
	}
	for i := range out {
		compatibility := targetKindTopologyCompatibility(profile.Kind, out[i].URL)
		if compatibility == 0 {
			continue
		}
		out[i].Score += compatibility * weight
		if compatibility > 0 {
			out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, "target_kind_topology_alignment")
		} else {
			out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, "target_kind_topology_mismatch")
		}
		out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, "target_kind_semantic_alignment")
		out[i].Metadata = discoverycore.MergeMetadata(out[i].Metadata, map[string]string{
			"target_kind":               profile.Kind,
			"target_kind_similarity":    strconv.FormatFloat(profile.Similarity, 'f', 3, 64),
			"target_kind_margin":        strconv.FormatFloat(profile.Margin, 'f', 3, 64),
			"target_kind_compatibility": strconv.FormatFloat(compatibility, 'f', 3, 64),
		})
	}
	discoverycore.SortCandidates(out)
	return out
}

func (s *Service) inferTargetKindProfile(ctx context.Context, goal string) targetKindProfile {
	archetypes := targetKindArchetypes()
	scored, err := s.semantic.Score(ctx, goal, archetypes)
	if err != nil || len(scored) == 0 {
		return targetKindProfile{}
	}
	best := scored[0]
	second := 0.0
	for _, item := range scored[1:] {
		if item.Similarity > best.Similarity {
			second = best.Similarity
			best = item
			continue
		}
		if item.Similarity > second {
			second = item.Similarity
		}
	}
	if best.Similarity <= 0 {
		return targetKindProfile{}
	}
	return targetKindProfile{
		Kind:       best.ID,
		Similarity: best.Similarity,
		Margin:     max(best.Similarity-second, 0),
	}
}

func targetKindArchetypes() []intel.SemanticCandidate {
	return []intel.SemanticCandidate{
		{ID: targetKindCanonicalHome, Text: "Entity origin surface expressing identity, ownership, scope, positioning, and global entry point. Página principal oficial, page d'accueil institutionnelle, homepage canonica, 公式入口."},
		{ID: targetKindDocsLanding, Text: "Maintained knowledge corpus entry: reference collection, manuals, specifications, documentation family, technical knowledge index. Documentación, documentazione, référence, Dokumentation, 技術文書."},
		{ID: targetKindSpecificDocument, Text: "Granular record or section: a precise document, chapter, article, clause, page, or bounded evidence unit inside a larger corpus. Documento específico, sezione dettagliata, section précise."},
		{ID: targetKindAPIReference, Text: "Machine-facing interface contract: protocol, endpoint, method, schema, parameter, data contract, integration boundary, programmatic surface."},
		{ID: targetKindLearningPath, Text: "Progressive instructional pathway: onboarding, setup sequence, guided explanation, practical learning route, first-use path, percorso introduttivo, parcours d'apprentissage."},
		{ID: targetKindProductSolutionPage, Text: "Capability or offering surface: product, service, solution, use case, feature family, platform capability, commercial or functional presentation."},
		{ID: targetKindDownloadRelease, Text: "Artifact acquisition and release provenance: package, binary, archive, changelog, version record, installable distribution, release channel."},
		{ID: targetKindOrganizationAbout, Text: "Institutional identity and governance surface: organization profile, mission, legal entity, foundation, team, governance, accountability, chi siamo, à propos."},
	}
}

func targetKindWeight(profile targetKindProfile) float64 {
	if (profile.Kind == targetKindCanonicalHome || profile.Kind == targetKindOrganizationAbout) && (profile.Similarity < 0.45 || profile.Margin < 0.12) {
		return 0
	}
	weight := 0.40 + min(profile.Similarity, 1.0)*0.75
	if profile.Margin < 0.03 {
		weight *= 0.55
	}
	return min(weight, 1.10)
}

func targetKindTopologyCompatibility(kind, rawURL string) float64 {
	depth := discoverycore.URLPathDepth(rawURL)
	home := discoverycore.IsCanonicalHomeURL(rawURL)
	resourceClass := discoverycore.ResourceClass(rawURL)
	htmlLike := resourceClass == discoverycore.ResourceClassHTMLLike
	queryPenalty := targetKindQueryPenalty(rawURL)

	if kind != targetKindCanonicalHome && kind != targetKindOrganizationAbout {
		return 0
	}
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
}

func targetKindQueryPenalty(rawURL string) float64 {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || strings.TrimSpace(parsed.RawQuery) == "" {
		return 0
	}
	return -0.12
}
