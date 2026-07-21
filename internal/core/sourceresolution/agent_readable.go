package sourceresolution

import (
	"context"
	"fmt"
	"strings"

	"github.com/josepavese/needlex/internal/core/agentreadable"
	"github.com/josepavese/needlex/internal/core/fetchpolicy"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/pipeline"
	"github.com/josepavese/needlex/internal/proof"
)

func agentReadableSelectionID(index int) string {
	return fmt.Sprintf("agent_readable_%d", index)
}

func agentReadableSelectionBetter(left, right agentReadableSelection) bool {
	if left.SemanticScore > 0 || right.SemanticScore > 0 {
		if left.SemanticScore != right.SemanticScore {
			return left.SemanticScore > right.SemanticScore
		}
	}
	if left.Candidate.Priority != right.Candidate.Priority {
		return left.Candidate.Priority < right.Candidate.Priority
	}
	return left.Candidate.URL < right.Candidate.URL
}

func agentReadableCandidateBatch(candidates []agentreadable.Candidate, maxCandidates int) []agentreadable.Candidate {
	if maxCandidates <= 0 || len(candidates) <= maxCandidates {
		return candidates
	}
	return candidates[:maxCandidates]
}

func agentReadableBalancedConventionalBatch(candidates []agentreadable.Candidate, maxCandidates int) []agentreadable.Candidate {
	selected := agentReadableCandidateBatch(candidates, maxCandidates)
	if maxCandidates <= 0 || len(candidates) <= maxCandidates {
		return selected
	}
	selected = ensureAgentReadableKindInBatch(selected, candidates, agentreadable.KindAPICatalog)
	selected = ensureAgentReadableKindInBatch(selected, candidates, agentreadable.KindServiceDescription)
	return agentreadable.NormalizeCandidates(selected, 0)
}

func ensureAgentReadableKindInBatch(selected, candidates []agentreadable.Candidate, kind string) []agentreadable.Candidate {
	if agentReadableBatchContainsKind(selected, kind) {
		return selected
	}
	candidate, ok := firstAgentReadableKind(candidates, kind)
	if !ok {
		return selected
	}
	replace := replaceableAgentReadableBatchIndex(selected)
	if replace < 0 {
		return selected
	}
	out := append([]agentreadable.Candidate{}, selected...)
	out[replace] = candidate
	return out
}

func agentReadableBatchContainsKind(candidates []agentreadable.Candidate, kind string) bool {
	for _, candidate := range candidates {
		if candidate.Kind == kind {
			return true
		}
	}
	return false
}

func firstAgentReadableKind(candidates []agentreadable.Candidate, kind string) (agentreadable.Candidate, bool) {
	for _, candidate := range candidates {
		if candidate.Kind == kind {
			return candidate, true
		}
	}
	return agentreadable.Candidate{}, false
}

func replaceableAgentReadableBatchIndex(candidates []agentreadable.Candidate) int {
	for index := len(candidates) - 1; index >= 0; index-- {
		switch candidates[index].Kind {
		case agentreadable.KindAPICatalog, agentreadable.KindServiceDescription:
			continue
		default:
			return index
		}
	}
	return -1
}

func agentReadableCandidateDifference(candidates, existing []agentreadable.Candidate) []agentreadable.Candidate {
	seen := map[string]struct{}{}
	for _, candidate := range existing {
		seen[agentReadableCandidateKey(candidate)] = struct{}{}
	}
	out := make([]agentreadable.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		key := agentReadableCandidateKey(candidate)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func agentReadableCandidateKey(candidate agentreadable.Candidate) string {
	return strings.ToLower(strings.TrimSpace(candidate.Kind + "\x00" + candidate.URL + "\x00" + candidate.Accept))
}

func (r Resolver) tryAgentReadableCandidate(ctx context.Context, req Request, original pipeline.RawPage, candidate agentreadable.Candidate, recorder *proof.Recorder) (pipeline.RawPage, agentreadable.Candidate, bool) {
	return r.tryAgentReadableCandidateDepth(ctx, req, original, candidate, recorder, 0)
}

func (r Resolver) tryAgentReadableCandidateDepth(ctx context.Context, req Request, original pipeline.RawPage, candidate agentreadable.Candidate, recorder *proof.Recorder, depth int) (pipeline.RawPage, agentreadable.Candidate, bool) {
	candidateCtx, cancel, ok := agentReadableProbeContext(ctx)
	if !ok {
		return pipeline.RawPage{}, agentreadable.Candidate{}, false
	}
	defer cancel()
	page, err := r.Acquirer.Acquire(candidateCtx, fetchpolicy.Input(r.Config, candidate.URL, pipeline.EffectiveUserAgent(req.UserAgent, req.RenderHint), req.FetchProfile, req.FetchRetryProfile, agentreadable.RequestAccept(candidate)))
	if err != nil {
		recorder.Error("agent_readable", "NX_AGENT_READABLE_FETCH_FAILED", err.Error(), map[string]string{
			"candidate_url":  candidate.URL,
			"candidate_kind": candidate.Kind,
		}, r.now().UTC())
		return pipeline.RawPage{}, agentreadable.Candidate{}, false
	}
	page.SourceKind = candidate.Kind
	page.SourceReason = candidate.DeclaredBy
	page.SourceFrom = original.FinalURL
	if candidate.Kind == agentreadable.KindAPICatalog && depth < 2 && agentReadableHasProbeTime(ctx) {
		for _, linked := range agentreadable.CandidatesFromAPICatalog(page.FinalURL, page.HTML, agentReadableCandidateLimit(r.Config.Agent.MaxCandidates)) {
			if strings.EqualFold(strings.TrimSpace(linked.URL), strings.TrimSpace(page.FinalURL)) {
				continue
			}
			if !agentReadableHasProbeTime(ctx) {
				break
			}
			linkedPage, linkedCandidate, ok := r.tryAgentReadableCandidateDepth(ctx, req, original, linked, recorder, depth+1)
			if ok {
				return linkedPage, linkedCandidate, true
			}
		}
	}
	if candidate.Kind == agentreadable.KindLLMSIndex && depth < 2 && agentReadableHasProbeTime(ctx) {
		if linked := r.bestLLMSMarkdownCandidate(ctx, req, original.FinalURL, page.FinalURL, page.HTML, candidate.Priority); linked.URL != "" {
			linkedPage, linkedCandidate, ok := r.tryAgentReadableCandidateDepth(ctx, req, original, linked, recorder, depth+1)
			if ok {
				return linkedPage, linkedCandidate, true
			}
		}
	}
	if agentreadable.IsAgentReadablePage(page) {
		return page, candidate, true
	}
	return pipeline.RawPage{}, agentreadable.Candidate{}, false
}

func (r Resolver) bestLLMSMarkdownCandidate(ctx context.Context, req Request, targetURL, indexURL, markdown string, priority int) agentreadable.Candidate {
	links := agentreadable.MarkdownLinkDetails(indexURL, markdown)
	if len(links) == 0 {
		return agentreadable.Candidate{}
	}
	if r.Semantic != nil && strings.TrimSpace(req.Objective) != "" {
		candidates := make([]intel.SemanticCandidate, 0, len(links))
		for index, link := range links {
			candidates = append(candidates, intel.SemanticCandidate{
				ID:   fmt.Sprintf("llms_link_%d", index),
				Text: strings.TrimSpace(link.Label + "\n" + link.URL),
			})
		}
		scored, err := r.Semantic.Score(ctx, req.Objective, candidates)
		if err == nil && len(scored) > 0 {
			bestIndex := -1
			bestScore := 0.0
			for _, score := range scored {
				var index int
				if _, err := fmt.Sscanf(score.ID, "llms_link_%d", &index); err != nil || index < 0 || index >= len(links) {
					continue
				}
				if bestIndex < 0 || score.Similarity > bestScore {
					bestIndex = index
					bestScore = score.Similarity
				}
			}
			if bestIndex >= 0 && bestScore > 0 {
				return agentreadable.Candidate{
					URL:        links[bestIndex].URL,
					Kind:       agentreadable.KindMarkdownVariant,
					DeclaredBy: "llms_txt",
					Priority:   priority,
				}
			}
		}
	}
	if linked := agentreadable.BestLinkedMarkdownFor(targetURL, indexURL, markdown); linked != "" {
		return agentreadable.Candidate{
			URL:        linked,
			Kind:       agentreadable.KindMarkdownVariant,
			DeclaredBy: "llms_txt",
			Priority:   priority,
		}
	}
	return agentreadable.Candidate{
		URL:        links[0].URL,
		Kind:       agentreadable.KindMarkdownVariant,
		DeclaredBy: "llms_txt",
		Priority:   priority,
	}
}

func (r Resolver) expandAgentReadableCandidates(ctx context.Context, req Request, rawPage pipeline.RawPage, candidates []agentreadable.Candidate, robotsPolicy agentreadable.RobotsPolicy, robotsLoaded bool, maxCandidates int) []agentreadable.Candidate {
	baseURL := strings.TrimSpace(rawPage.FinalURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(rawPage.URL)
	}
	out := append([]agentreadable.Candidate{}, candidates...)
	sitemapURLs := []string{}
	if robotsLoaded {
		sitemapURLs = append(sitemapURLs, robotsPolicy.Sitemaps...)
	}
	if len(sitemapURLs) == 0 {
		sitemapURLs = append(sitemapURLs, agentreadable.ConventionalSitemapURLs(baseURL)...)
	}
	for index, sitemapURL := range sitemapURLs {
		if index >= agentReadableConventionalSitemaps || !agentReadableHasProbeTime(ctx) {
			break
		}
		page, ok := r.fetchAgentReadableAuxiliary(ctx, req, sitemapURL, "application/xml, text/xml;q=0.9, text/plain;q=0.6, */*;q=0.1")
		if !ok {
			continue
		}
		sitemapCandidates := agentreadable.CandidatesFromSitemap(baseURL, page.FinalURL, page.HTML, 0)
		sitemapCandidates, _ = filterCandidatesByRobots(sitemapCandidates, robotsPolicy, robotsLoaded, pipeline.EffectiveUserAgent(req.UserAgent, req.RenderHint))
		out = append(out, agentReadableCandidateBatch(sitemapCandidates, maxCandidates)...)
	}
	return agentreadable.NormalizeCandidates(out, 0)
}

func (r Resolver) fetchAgentReadableRobotsPolicy(ctx context.Context, req Request, rawPage pipeline.RawPage) (agentreadable.RobotsPolicy, bool) {
	baseURL := strings.TrimSpace(rawPage.FinalURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(rawPage.URL)
	}
	robotsURL := agentreadable.RootRobotsURL(baseURL)
	if robotsURL == "" {
		return agentreadable.RobotsPolicy{}, false
	}
	page, ok := r.fetchAgentReadableAuxiliary(ctx, req, robotsURL, "text/plain, */*;q=0.1")
	if !ok {
		return agentreadable.RobotsPolicy{}, false
	}
	return agentreadable.ParseRobots(baseURL, page.HTML), true
}
