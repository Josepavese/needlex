package service

import (
	"context"
	"encoding/json"
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/core/fetchpolicy"
	"github.com/josepavese/needlex/internal/core/webdiscover"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/pipeline"
)

func (s *Service) maybePromoteEndpointCandidate(ctx context.Context, goal, userAgent string, domainHints []string, candidates []DiscoverCandidate) []DiscoverCandidate {
	backend := strings.TrimSpace(s.cfg.Models.Backend)
	if len(candidates) == 0 || backend != intel.BackendOpenAICompatible {
		return candidates
	}
	pageInputs, allowed := s.collectEndpointPageInputs(ctx, goal, userAgent, candidates)
	if len(pageInputs) == 0 {
		return candidates
	}
	out, ok := s.runEndpointExtractor(ctx, goal, domainHints, pageInputs)
	selectedPage, found := allowed[out.SelectedURL]
	if !ok || !found || out.Confidence < 0.55 {
		return candidates
	}
	return webdiscover.PromoteEndpointCandidate(candidates, selectedPage, out)
}

func (s *Service) collectEndpointPageInputs(ctx context.Context, goal, userAgent string, candidates []DiscoverCandidate) ([]webdiscover.EndpointPageInput, map[string]webdiscover.EndpointPageInput) {
	pageInputs := make([]webdiscover.EndpointPageInput, 0, min(len(candidates), 4))
	allowed := map[string]webdiscover.EndpointPageInput{}
	for _, candidate := range s.orderEndpointCandidates(ctx, goal, candidates, min(len(candidates), 4)) {
		rawPage, err := s.acquirer.Acquire(ctx, fetchpolicy.Input(s.cfg, candidate.URL, pipeline.EffectiveUserAgent(userAgent, true), "", "", ""))
		if err != nil {
			continue
		}
		dom, err := s.reducer.Reduce(rawPage)
		if err != nil {
			continue
		}
		embeddedURLs := webdiscover.EmbeddedURLsForPage(rawPage.FinalURL, rawPage.HTML, dom)
		if len(embeddedURLs) == 0 {
			continue
		}
		input := webdiscover.EndpointPageInput{
			PageURL:      strings.TrimSpace(rawPage.FinalURL),
			PageTitle:    strings.TrimSpace(dom.Title),
			EmbeddedURLs: embeddedURLs,
		}
		pageInputs = append(pageInputs, input)
		for _, embeddedURL := range embeddedURLs {
			allowed[embeddedURL] = input
		}
	}
	return pageInputs, allowed
}

func (s *Service) runEndpointExtractor(ctx context.Context, goal string, domainHints []string, pageInputs []webdiscover.EndpointPageInput) (webdiscover.EndpointExtractorResult, bool) {
	modelReq := intel.ModelRequest{
		Task:            intel.TaskEndpointExtract,
		ModelClass:      intel.ModelClassMicroSolver,
		MaxInputTokens:  1200,
		MaxOutputTokens: 180,
		TimeoutMS:       max(600, s.cfg.Models.MicroTimeoutMS),
		SchemaName:      "endpoint_extract.v1",
		Input: map[string]any{
			"goal":            strings.TrimSpace(goal),
			"domain_hints":    domainHints,
			"candidate_pages": pageInputs,
		},
	}
	resp, err := s.runtime.Run(ctx, modelReq)
	if err != nil {
		return webdiscover.EndpointExtractorResult{}, false
	}
	var out webdiscover.EndpointExtractorResult
	if err := json.Unmarshal([]byte(resp.OutputJSON), &out); err != nil {
		return webdiscover.EndpointExtractorResult{}, false
	}
	out.SelectedURL = strings.TrimSpace(out.SelectedURL)
	return out, true
}

func (s *Service) orderEndpointCandidates(ctx context.Context, goal string, candidates []DiscoverCandidate, limit int) []DiscoverCandidate {
	if len(candidates) == 0 || limit <= 0 {
		return nil
	}
	window := min(len(candidates), max(limit*2, limit))
	ordered := append([]DiscoverCandidate{}, candidates[:window]...)
	semanticCandidates := make([]intel.SemanticCandidate, 0, len(ordered))
	for _, candidate := range ordered {
		semanticCandidates = append(semanticCandidates, intel.SemanticCandidate{
			ID: candidate.URL,
			Text: discoverycore.JoinNonEmpty(
				candidate.Metadata["source_context"],
				candidate.Metadata["page_title"],
				candidate.Label,
				candidate.Metadata["resource_class"],
			),
		})
	}
	byURL := map[string]float64{}
	if len(semanticCandidates) > 0 {
		if scored, err := s.semantic.Score(ctx, goal, semanticCandidates); err == nil {
			for _, item := range scored {
				byURL[item.ID] = item.Similarity
			}
		}
	}
	for i := range ordered {
		resourceClass := discoverycore.ResourceClass(ordered[i].URL)
		switch resourceClass {
		case discoverycore.ResourceClassHTMLLike:
			ordered[i].Score += 0.08
			ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "endpoint_resource_class_html_like")
		case discoverycore.ResourceClassStructured:
			ordered[i].Score += 0.05
			ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "endpoint_resource_class_structured")
		case discoverycore.ResourceClassMediaAsset, discoverycore.ResourceClassArchiveFile:
			ordered[i].Score -= 0.08
			ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "endpoint_resource_class_penalty")
		}
		if similarity, ok := byURL[ordered[i].URL]; ok && similarity > 0 {
			ordered[i].Score += similarity * 1.2
			ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "endpoint_probe_semantic_alignment")
		}
		ordered[i].Metadata = discoverycore.MergeMetadata(ordered[i].Metadata, map[string]string{
			"resource_class": resourceClass,
		})
	}
	discoverycore.SortCandidates(ordered)
	return ordered[:min(len(ordered), limit)]
}
