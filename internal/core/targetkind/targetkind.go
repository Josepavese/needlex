package targetkind

import (
	"context"
	"strconv"
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/intel"
)

const (
	CanonicalHome       = "canonical_home"
	DocsLanding         = "docs_landing"
	SpecificDocument    = "specific_document"
	APIReference        = "api_reference"
	LearningPath        = "learning_path"
	ProductSolutionPage = "product_solution_page"
	DownloadRelease     = "download_release"
	OrganizationAbout   = "organization_about"
)

type Profile struct {
	Kind       string
	Similarity float64
	Margin     float64
}

type targetKindPrototype struct {
	Kind string
	ID   string
	Text string
}

func Apply(ctx context.Context, semantic intel.SemanticAligner, goal string, candidates []discoverycore.Candidate) []discoverycore.Candidate {
	if strings.TrimSpace(goal) == "" || len(candidates) == 0 {
		return candidates
	}
	profile := Infer(ctx, semantic, goal)
	if strings.TrimSpace(profile.Kind) == "" {
		return candidates
	}
	out := append([]discoverycore.Candidate{}, candidates...)
	weight := targetKindWeight(profile)
	if weight <= 0 {
		return out
	}
	targetText := ArchetypeText(profile.Kind)
	semanticCompatibility := scoreCandidateSetToGoal(ctx, semantic, targetText, targetKindSemanticCandidates(out))
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

func Infer(ctx context.Context, semantic intel.SemanticAligner, goal string) Profile {
	archetypes := targetKindArchetypes()
	if semantic == nil {
		return Profile{}
	}
	scored, err := semantic.Score(ctx, goal, archetypes)
	if err != nil || len(scored) == 0 {
		return Profile{}
	}
	byKind := map[string]float64{}
	for _, item := range scored {
		kind := PrototypeKind(item.ID)
		if kind == "" {
			continue
		}
		if item.Similarity > byKind[kind] {
			byKind[kind] = item.Similarity
		}
	}
	if len(byKind) == 0 {
		return Profile{}
	}
	best := Profile{}
	second := 0.0
	for kind, similarity := range byKind {
		if similarity > best.Similarity {
			second = best.Similarity
			best = Profile{Kind: kind, Similarity: similarity}
			continue
		}
		if similarity > second {
			second = similarity
		}
	}
	if best.Similarity <= 0 {
		return Profile{}
	}
	if canonical, ok := byKind[CanonicalHome]; ok && shouldPreferCloseCanonicalHome(best, canonical) {
		return Profile{
			Kind:       CanonicalHome,
			Similarity: canonical,
			Margin:     max(canonical-secondBestExcluding(byKind, CanonicalHome), 0.04),
		}
	}
	return Profile{
		Kind:       best.Kind,
		Similarity: best.Similarity,
		Margin:     max(best.Similarity-second, 0),
	}
}

func WeakCanonicalHome(profile Profile) bool {
	return profile.Kind == CanonicalHome && profile.Similarity >= 0.36 && profile.Margin < 0.06 && profile.Similarity < 0.62
}

func scoreCandidateSetToGoal(ctx context.Context, semantic intel.SemanticAligner, goal string, candidates []intel.SemanticCandidate) map[string]float64 {
	if semantic == nil || len(candidates) == 0 || strings.TrimSpace(goal) == "" {
		return nil
	}
	scored, err := semantic.Score(ctx, goal, candidates)
	if err != nil || len(scored) == 0 {
		return nil
	}
	out := make(map[string]float64, len(scored))
	for _, item := range scored {
		out[item.ID] = max(item.Similarity, 0)
	}
	return out
}

func shouldPreferCloseCanonicalHome(best Profile, canonical float64) bool {
	if best.Kind == CanonicalHome {
		return false
	}
	if canonical < 0.40 {
		return false
	}
	switch best.Kind {
	case APIReference, DownloadRelease:
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
		{Kind: CanonicalHome, ID: "home", Text: "home"},
		{Kind: CanonicalHome, ID: "home_page", Text: "home page"},
		{Kind: CanonicalHome, ID: "official_homepage", Text: "official homepage"},
		{Kind: CanonicalHome, ID: "main_site", Text: "main site"},
		{Kind: CanonicalHome, ID: "what_is_entity", Text: "what is this software project product or organization"},
		{Kind: CanonicalHome, ID: "broad_overview", Text: "broad overview of a named entity"},
		{Kind: CanonicalHome, ID: "positioning", Text: "public positioning overview and primary introduction for an entity"},
		{Kind: CanonicalHome, ID: "multilingual_home", Text: "Página principal oficial, page d'accueil institutionnelle, homepage canonica, 公式入口."},
		{Kind: DocsLanding, ID: "docs", Text: "technical documentation reference manual API docs"},
		{Kind: DocsLanding, ID: "knowledge_index", Text: "documentation index reference collection maintained knowledge corpus"},
		{Kind: SpecificDocument, ID: "specific_page", Text: "specific article section page exact document bounded evidence unit"},
		{Kind: APIReference, ID: "api_contract", Text: "API reference endpoint method schema parameters integration contract"},
		{Kind: LearningPath, ID: "guide", Text: "guide"},
		{Kind: LearningPath, ID: "tutorial", Text: "tutorial"},
		{Kind: LearningPath, ID: "getting_started", Text: "step by step tutorial getting started onboarding learning path"},
		{Kind: ProductSolutionPage, ID: "capability", Text: "product service solution use case feature capability offering surface"},
		{Kind: DownloadRelease, ID: "artifact", Text: "download release package install binary changelog distribution artifact"},
		{Kind: OrganizationAbout, ID: "organization", Text: "about company organization mission team governance legal entity accountability"},
		{Kind: OrganizationAbout, ID: "institution", Text: "institutional profile foundation mission governance team accountability"},
	}
}

func targetKindPrototypeID(kind, id string) string {
	return strings.TrimSpace(kind) + "#" + strings.TrimSpace(id)
}

func PrototypeKind(id string) string {
	before, _, ok := strings.Cut(strings.TrimSpace(id), "#")
	if !ok {
		return strings.TrimSpace(id)
	}
	return strings.TrimSpace(before)
}

func ArchetypeText(kind string) string {
	texts := make([]string, 0, 4)
	for _, prototype := range targetKindPrototypes() {
		if prototype.Kind == kind {
			texts = append(texts, prototype.Text)
		}
	}
	return discoverycore.JoinNonEmpty(texts...)
}

func targetKindWeight(profile Profile) float64 {
	if profile.Kind == CanonicalHome || profile.Kind == OrganizationAbout {
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
