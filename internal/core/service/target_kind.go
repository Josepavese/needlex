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

type targetKindPrototype struct {
	Kind string
	ID   string
	Text string
}

func (s *Service) applyTargetKindRerank(ctx context.Context, goal string, candidates []DiscoverCandidate) []DiscoverCandidate {
	if strings.TrimSpace(goal) == "" || len(candidates) == 0 {
		return candidates
	}
	profile := s.inferTargetKindProfile(ctx, goal)
	if strings.TrimSpace(profile.Kind) == "" {
		return candidates
	}
	out := append([]DiscoverCandidate{}, candidates...)
	weight := targetKindWeight(profile)
	if weight <= 0 {
		return out
	}
	targetText := targetKindArchetypeText(profile.Kind)
	semanticCompatibility := s.scoreCandidateSetToGoal(ctx, targetText, targetKindSemanticCandidates(out))
	for i := range out {
		topology := targetKindTopologyCompatibility(profile.Kind, out[i].URL)
		semantic := semanticCompatibility[out[i].URL]
		compatibility := targetKindCandidateCompatibility(profile.Kind, semantic, topology)
		if compatibility == 0 {
			continue
		}
		out[i].Score += compatibility * weight
		if compatibility > 0 {
			out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, "target_kind_semantic_alignment")
			if topology > 0 {
				out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, "target_kind_topology_alignment")
			}
		} else {
			out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, "target_kind_topology_mismatch")
		}
		out[i].Metadata = discoverycore.MergeMetadata(out[i].Metadata, map[string]string{
			"target_kind":                    profile.Kind,
			"target_kind_similarity":         strconv.FormatFloat(profile.Similarity, 'f', 3, 64),
			"target_kind_margin":             strconv.FormatFloat(profile.Margin, 'f', 3, 64),
			"target_kind_compatibility":      strconv.FormatFloat(compatibility, 'f', 3, 64),
			"target_kind_candidate_semantic": strconv.FormatFloat(semantic, 'f', 3, 64),
			"target_kind_candidate_topology": strconv.FormatFloat(topology, 'f', 3, 64),
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
	byKind := map[string]float64{}
	for _, item := range scored {
		kind := targetKindFromPrototypeID(item.ID)
		if kind == "" {
			continue
		}
		if item.Similarity > byKind[kind] {
			byKind[kind] = item.Similarity
		}
	}
	if len(byKind) == 0 {
		return targetKindProfile{}
	}
	best := targetKindProfile{}
	second := 0.0
	for kind, similarity := range byKind {
		if similarity > best.Similarity {
			second = best.Similarity
			best = targetKindProfile{Kind: kind, Similarity: similarity}
			continue
		}
		if similarity > second {
			second = similarity
		}
	}
	if best.Similarity <= 0 {
		return targetKindProfile{}
	}
	if canonical, ok := byKind[targetKindCanonicalHome]; ok && shouldPreferCloseCanonicalHome(best, canonical) {
		return targetKindProfile{
			Kind:       targetKindCanonicalHome,
			Similarity: canonical,
			Margin:     max(canonical-secondBestExcluding(byKind, targetKindCanonicalHome), 0.04),
		}
	}
	return targetKindProfile{
		Kind:       best.Kind,
		Similarity: best.Similarity,
		Margin:     max(best.Similarity-second, 0),
	}
}

func shouldPreferCloseCanonicalHome(best targetKindProfile, canonical float64) bool {
	if best.Kind == targetKindCanonicalHome {
		return false
	}
	if canonical < 0.40 {
		return false
	}
	switch best.Kind {
	case targetKindAPIReference, targetKindDownloadRelease:
		return false
	}
	return best.Similarity-canonical <= 0.08
}

func secondBestExcluding(scores map[string]float64, excluded string) float64 {
	best := 0.0
	for kind, score := range scores {
		if kind == excluded {
			continue
		}
		best = max(best, score)
	}
	return best
}

func targetKindArchetypes() []intel.SemanticCandidate {
	prototypes := targetKindPrototypes()
	out := make([]intel.SemanticCandidate, 0, len(prototypes))
	for _, prototype := range prototypes {
		out = append(out, intel.SemanticCandidate{
			ID:   targetKindPrototypeID(prototype.Kind, prototype.ID),
			Text: prototype.Text,
		})
	}
	return out
}

func targetKindPrototypes() []targetKindPrototype {
	return []targetKindPrototype{
		{Kind: targetKindCanonicalHome, ID: "home", Text: "home"},
		{Kind: targetKindCanonicalHome, ID: "home_page", Text: "home page"},
		{Kind: targetKindCanonicalHome, ID: "official_homepage", Text: "official homepage"},
		{Kind: targetKindCanonicalHome, ID: "main_site", Text: "main site"},
		{Kind: targetKindCanonicalHome, ID: "what_is_entity", Text: "what is this software project product or organization"},
		{Kind: targetKindCanonicalHome, ID: "broad_overview", Text: "broad overview of a named entity"},
		{Kind: targetKindCanonicalHome, ID: "positioning", Text: "public positioning overview and primary introduction for an entity"},
		{Kind: targetKindCanonicalHome, ID: "multilingual_home", Text: "Página principal oficial, page d'accueil institutionnelle, homepage canonica, 公式入口."},
		{Kind: targetKindDocsLanding, ID: "docs", Text: "technical documentation reference manual API docs"},
		{Kind: targetKindDocsLanding, ID: "knowledge_index", Text: "documentation index reference collection maintained knowledge corpus"},
		{Kind: targetKindSpecificDocument, ID: "specific_page", Text: "specific article section page exact document bounded evidence unit"},
		{Kind: targetKindAPIReference, ID: "api_contract", Text: "API reference endpoint method schema parameters integration contract"},
		{Kind: targetKindLearningPath, ID: "guide", Text: "guide"},
		{Kind: targetKindLearningPath, ID: "tutorial", Text: "tutorial"},
		{Kind: targetKindLearningPath, ID: "getting_started", Text: "step by step tutorial getting started onboarding learning path"},
		{Kind: targetKindProductSolutionPage, ID: "capability", Text: "product service solution use case feature capability offering surface"},
		{Kind: targetKindDownloadRelease, ID: "artifact", Text: "download release package install binary changelog distribution artifact"},
		{Kind: targetKindOrganizationAbout, ID: "organization", Text: "about company organization mission team governance legal entity accountability"},
		{Kind: targetKindOrganizationAbout, ID: "institution", Text: "institutional profile foundation mission governance team accountability"},
	}
}

func targetKindPrototypeID(kind, id string) string {
	return strings.TrimSpace(kind) + "#" + strings.TrimSpace(id)
}

func targetKindFromPrototypeID(id string) string {
	before, _, ok := strings.Cut(strings.TrimSpace(id), "#")
	if !ok {
		return strings.TrimSpace(id)
	}
	return strings.TrimSpace(before)
}

func targetKindArchetypeText(kind string) string {
	texts := make([]string, 0, 4)
	for _, prototype := range targetKindPrototypes() {
		if prototype.Kind == kind {
			texts = append(texts, prototype.Text)
		}
	}
	return discoverycore.JoinNonEmpty(texts...)
}

func targetKindWeight(profile targetKindProfile) float64 {
	if profile.Kind == targetKindCanonicalHome || profile.Kind == targetKindOrganizationAbout {
		if profile.Similarity < 0.36 {
			return 0
		}
		if profile.Margin < 0.04 && profile.Similarity < 0.50 {
			return 0
		}
	}
	weight := 0.40 + min(profile.Similarity, 1.0)*0.75
	if profile.Margin < 0.03 {
		weight *= 0.55
	}
	return min(weight, 1.10)
}

func targetKindSemanticCandidates(candidates []DiscoverCandidate) []intel.SemanticCandidate {
	out := make([]intel.SemanticCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, intel.SemanticCandidate{
			ID:   candidate.URL,
			Text: targetKindCandidateText(candidate),
		})
	}
	return out
}

func targetKindCandidateText(candidate DiscoverCandidate) string {
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
	case targetKindCanonicalHome:
		if topology > 0 {
			return topology + semantic*0.20
		}
		if semantic >= 0.35 && topology > -0.30 {
			return semantic*0.25 + topology*0.25
		}
		return topology
	case targetKindOrganizationAbout:
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
	case targetKindCanonicalHome:
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
	case targetKindOrganizationAbout:
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
