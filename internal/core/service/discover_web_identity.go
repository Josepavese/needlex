package service

import (
	"context"
	"strconv"
	"strings"

	"github.com/josepavese/needlex/internal/core"
	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/core/fetchpolicy"
	"github.com/josepavese/needlex/internal/core/webdiscover"
	"github.com/josepavese/needlex/internal/core/webirbuilder"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/pipeline"
)

func (s *Service) probeSemanticRootIdentity(ctx context.Context, goal, rawURL, title string, webIR core.WebIR) hostRootIdentityProbe {
	if !discoverycore.IsCanonicalHomeURL(rawURL) {
		return hostRootIdentityProbe{}
	}
	rootContext := webdiscover.IRContext(webIR, 900)
	semantics := s.hostRootSemanticScores(ctx, goal, rawURL, title, rootContext)
	if !semantics.HostRootIdentity() {
		return hostRootIdentityProbe{}
	}
	return hostRootIdentityProbe{
		URL:   strings.TrimSpace(rawURL),
		Title: strings.TrimSpace(title),
		Score: 0.28 + min(semantics.Goal*0.26+semantics.Origin*0.24, 0.38),
		Reasons: []string{
			"semantic_root_identity_probe",
		},
		Metadata: map[string]string{
			"semantic_root_goal_similarity":       strconv.FormatFloat(semantics.Goal, 'f', 3, 64),
			"semantic_root_origin_similarity":     strconv.FormatFloat(semantics.Origin, 'f', 3, 64),
			"semantic_root_derivative_similarity": strconv.FormatFloat(semantics.Derivative, 'f', 3, 64),
		},
	}
}

func (s *Service) probeHostRootIdentity(ctx context.Context, goal string, userAgent, rawURL string) (hostRootIdentityProbe, error) {
	rootURL, ok := webdiscover.HostRootURL(rawURL)
	if !ok || strings.TrimSpace(rootURL) == strings.TrimSpace(rawURL) {
		return hostRootIdentityProbe{}, nil
	}

	rawPage, err := s.acquirer.Acquire(ctx, fetchpolicy.Input(s.cfg, rootURL, pipeline.EffectiveUserAgent(userAgent, true), "", "", ""))
	if err != nil {
		return hostRootIdentityProbe{}, err
	}
	dom, err := s.reducer.Reduce(rawPage)
	if err != nil {
		return hostRootIdentityProbe{}, err
	}
	rootContext := webdiscover.IRContext(webirbuilder.Build(dom), 900)
	metadata := map[string]string{
		"host_root_url":     strings.TrimSpace(rawPage.FinalURL),
		"host_root_title":   strings.TrimSpace(dom.Title),
		"host_root_context": rootContext,
	}
	semantics := s.hostRootSemanticScores(ctx, goal, rawPage.FinalURL, dom.Title, rootContext)
	if semantics.Goal > 0 {
		metadata["host_root_goal_similarity"] = strconv.FormatFloat(semantics.Goal, 'f', 3, 64)
	}
	if semantics.Origin > 0 {
		metadata["host_root_origin_similarity"] = strconv.FormatFloat(semantics.Origin, 'f', 3, 64)
	}
	if semantics.Derivative > 0 {
		metadata["host_root_derivative_similarity"] = strconv.FormatFloat(semantics.Derivative, 'f', 3, 64)
	}
	if strings.TrimSpace(dom.Title) == "" {
		return hostRootIdentityProbe{
			URL:      strings.TrimSpace(rawPage.FinalURL),
			Metadata: metadata,
		}, nil
	}

	identityScore, reasons := discoverycore.ScoreStructuralURL(rawPage.FinalURL, false, nil)
	if identityScore <= 0 || !semantics.HostRootIdentity() {
		return hostRootIdentityProbe{
			URL:      strings.TrimSpace(rawPage.FinalURL),
			Title:    strings.TrimSpace(dom.Title),
			Metadata: metadata,
		}, nil
	}

	return hostRootIdentityProbe{
		URL:   strings.TrimSpace(rawPage.FinalURL),
		Title: strings.TrimSpace(dom.Title),
		Score: identityScore*0.45 + min(semantics.Goal*0.26+semantics.Origin*0.24, 0.35),
		Reasons: discoverycore.AppendUniqueReason(
			reasons,
			"host_root_identity_probe",
		),
		Metadata: metadata,
	}, nil
}

type hostRootSemanticScores struct {
	Goal       float64
	Origin     float64
	Derivative float64
}

func (scores hostRootSemanticScores) HostRootIdentity() bool {
	custodianRoot := scores.Goal >= hostRootSemanticMinSimilarity &&
		scores.Origin >= hostRootOriginMinSimilarity &&
		scores.Origin >= scores.Derivative+hostRootDerivativeMargin
	topicSpecificRoot := scores.Goal >= 0.40 &&
		scores.Origin >= 0.14 &&
		scores.Derivative <= scores.Goal+0.02
	return custodianRoot || topicSpecificRoot
}

func (s *Service) hostRootSemanticScores(ctx context.Context, goal, rootURL, title, rootContext string) hostRootSemanticScores {
	if s.semantic == nil || strings.TrimSpace(goal) == "" {
		return hostRootSemanticScores{}
	}
	text := discoverycore.JoinNonEmpty(title, rootContext, rootURL)
	if strings.TrimSpace(text) == "" {
		return hostRootSemanticScores{}
	}
	return hostRootSemanticScores{
		Goal:       s.scoreHostRootSemanticText(ctx, goal, rootURL, text),
		Origin:     s.scoreHostRootSemanticText(ctx, hostRootOriginProfile(), rootURL, text),
		Derivative: s.scoreHostRootSemanticText(ctx, hostRootDerivativeProfile(), rootURL, text),
	}
}

func (s *Service) scoreHostRootSemanticText(ctx context.Context, objective, rootURL, text string) float64 {
	scored, err := s.semantic.Score(ctx, objective, []intel.SemanticCandidate{{
		ID:   rootURL,
		Text: discoverycore.CompactSemanticText(text, 1200),
	}})
	if err != nil || len(scored) == 0 {
		return 0
	}
	best := 0.0
	for _, item := range scored {
		best = max(best, item.Similarity)
	}
	return best
}

func hostRootOriginProfile() string {
	return "Root surface of the responsible entity, project, institution, standard body, publisher, service, or product itself. Primary maintained identity, canonical owner presence, official home, source of record."
}

func hostRootDerivativeProfile() string {
	return "General secondary collection, tutorial site, encyclopedia, aggregator, community portal, directory, mirror, external explanation, or multi-entity catalogue about other projects and sources."
}

func (s *Service) selectIdentityReferenceCandidates(ctx context.Context, goal string, source DiscoverCandidate, refs []webdiscover.IdentityReferenceCandidate) []DiscoverCandidate {
	if len(refs) == 0 {
		return nil
	}
	baseLinks, relationByURL := webdiscover.IdentityBaseLinks(source.URL, refs)
	scored := discoverycore.ScoreStructuralCandidates("", "", baseLinks, nil)
	if len(scored) == 0 {
		return nil
	}
	goalSimilarity := s.scoreCandidateSetToGoal(ctx, goal, webdiscover.IdentitySemanticCandidates(source, scored))
	return webdiscover.IdentityDiscoverCandidates(source, scored, relationByURL, goalSimilarity)
}

func (s *Service) selectExpandedRecoveryCandidates(ctx context.Context, goal string, source DiscoverCandidate, expanded []DiscoverCandidate) []DiscoverCandidate {
	if len(expanded) == 0 {
		return nil
	}
	ordered := append([]DiscoverCandidate{}, expanded...)
	goalSimilarity := s.scoreCandidateSetToGoal(ctx, goal, expandedRecoverySemanticCandidates(source, ordered))
	applyExpandedRecoveryScores(source, ordered, goalSimilarity)
	discoverycore.SortCandidates(ordered)
	return selectExpandedRecoveryLeaders(ordered)
}

func expandedRecoverySemanticCandidates(source DiscoverCandidate, ordered []DiscoverCandidate) []intel.SemanticCandidate {
	semanticCandidates := make([]intel.SemanticCandidate, 0, len(ordered))
	for _, candidate := range ordered {
		semanticCandidates = append(semanticCandidates, intel.SemanticCandidate{
			ID: candidate.URL,
			Text: discoverycore.JoinNonEmpty(
				discoverycore.CompactSemanticText(source.Metadata["host_root_title"], 160),
				discoverycore.CompactSemanticText(source.Metadata["host_root_context"], 260),
				discoverycore.CompactSemanticText(source.Metadata["page_title"], 160),
				discoverycore.CompactSemanticText(source.Metadata["web_ir_context"], 320),
				discoverycore.CompactSemanticText(source.Label, 160),
				discoverycore.CompactSemanticText(candidate.Metadata["source_context"], 220),
				discoverycore.CompactSemanticText(candidate.Label, 160),
				candidate.Metadata["resource_class"],
			),
		})
	}
	return semanticCandidates
}

func applyExpandedRecoveryScores(source DiscoverCandidate, ordered []DiscoverCandidate, goalSimilarity map[string]float64) {
	sourceFamily, _ := webdiscover.CandidateFamily(source.URL)
	sourceDepth := discoverycore.URLPathDepth(source.URL)
	sourceSimilarity := semanticCandidateEvidence(source)
	sourceGrounded := sourceSimilarity >= 0.30 || candidateHasAnyReason(source, "semantic_root_identity_probe", "host_root_identity_probe", "semantic_evidence_probe")
	for i := range ordered {
		similarity := goalSimilarity[ordered[i].URL]
		family, familyOK := webdiscover.CandidateFamily(ordered[i].URL)
		sameFamily := familyOK && family != "" && family == sourceFamily
		candidateDepth := discoverycore.URLPathDepth(ordered[i].URL)
		if similarity > 0 {
			ordered[i].Score += similarity * 1.35
			ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "page_expand_semantic_grounding")
		}
		if sameFamily {
			switch {
			case candidateDepth > sourceDepth:
				ordered[i].Score += 0.42
				ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "same_family_child_recovery")
			case candidateDepth == sourceDepth:
				ordered[i].Score += 0.14
				ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "same_family_page_expand")
			default:
				ordered[i].Score -= 0.16
				ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "same_family_scope_regression")
			}
		}
		if familyOK && family != "" && family != sourceFamily {
			if similarity < 0.18 {
				ordered[i].Score -= 0.18
				ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "external_family_ungrounded")
			} else if sourceGrounded && similarity <= sourceSimilarity+0.08 {
				ordered[i].Score -= 0.22
				ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "external_family_context_only")
			} else {
				ordered[i].Score += 0.22 + min(max(0, similarity-sourceSimilarity)*0.90, 0.28)
				ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "external_family_recovery")
			}
		}
	}
}

func selectExpandedRecoveryLeaders(ordered []DiscoverCandidate) []DiscoverCandidate {
	out := make([]DiscoverCandidate, 0, 3)
	seenFamilies := map[string]struct{}{}
	for _, candidate := range ordered {
		family, _ := webdiscover.CandidateFamily(candidate.URL)
		if family != "" {
			if _, ok := seenFamilies[family]; ok {
				continue
			}
			seenFamilies[family] = struct{}{}
		}
		if candidateHasAnyReason(candidate, "external_family_context_only", "external_family_ungrounded") &&
			!candidateHasAnyReason(candidate, "same_family_child_recovery", "same_family_page_expand", "same_family_scope_regression") {
			continue
		}
		if candidateHasAnyReason(candidate, "same_family_scope_regression") {
			candidate.Score += 0.04
			candidate.Reason = discoverycore.AppendUniqueReason(candidate.Reason, "page_expand_scope_context")
		} else if candidateHasAnyReason(candidate, "same_family_child_recovery") {
			candidate.Score += 0.70
			candidate.Reason = discoverycore.AppendUniqueReason(candidate.Reason, "page_expand", "page_expand_child_context")
		} else {
			candidate.Score += 0.40
			candidate.Reason = discoverycore.AppendUniqueReason(candidate.Reason, "page_expand")
		}
		out = append(out, candidate)
		if len(out) >= 3 {
			break
		}
	}
	return out
}
