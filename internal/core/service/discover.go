package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/pipeline"
)

type DiscoverRequest struct {
	Goal          string
	SeedURL       string
	UserAgent     string
	SameDomain    bool
	MaxCandidates int
	DomainHints   []string
}

type DiscoverCandidate = discoverycore.Candidate

type DiscoverResponse struct {
	SeedURL      string              `json:"seed_url"`
	SelectedURL  string              `json:"selected_url"`
	DiscoveryURL string              `json:"discovery_url"`
	Candidates   []DiscoverCandidate `json:"candidates"`
}

func (s *Service) Discover(ctx context.Context, req DiscoverRequest) (DiscoverResponse, error) {
	req = normalizeDiscoverRequest(req)
	if err := validateDiscoverRequest(req); err != nil {
		return DiscoverResponse{}, err
	}
	rawPage, dom, err := s.acquireDiscoverPage(ctx, req.SeedURL, req.UserAgent)
	if err != nil {
		return DiscoverResponse{}, err
	}
	candidates := discoverycore.NewSet(discoverycore.ScoreCandidates(req.Goal, rawPage.FinalURL, dom.Title, extractLinkCandidates(rawPage.HTML, rawPage.FinalURL, req.SameDomain), req.DomainHints))
	candidates = discoverycore.NewSet(s.semanticRerankDiscoverCandidates(ctx, req.Goal, candidates.Sorted()))
	selected := candidates.SelectedURL(rawPage.FinalURL)
	return DiscoverResponse{SeedURL: req.SeedURL, SelectedURL: selected, DiscoveryURL: rawPage.FinalURL, Candidates: candidates.Limited(req.MaxCandidates)}, nil
}

func normalizeDiscoverRequest(req DiscoverRequest) DiscoverRequest {
	if req.MaxCandidates <= 0 {
		req.MaxCandidates = 5
	}
	return req
}

func validateDiscoverRequest(req DiscoverRequest) error {
	switch {
	case strings.TrimSpace(req.SeedURL) == "":
		return fmt.Errorf("discover request seed_url must not be empty")
	case strings.TrimSpace(req.Goal) == "":
		return fmt.Errorf("discover request goal must not be empty")
	default:
		return nil
	}
}

func (s *Service) acquireDiscoverPage(ctx context.Context, rawURL, userAgent string) (pipeline.RawPage, pipeline.SimplifiedDOM, error) {
	rawPage, err := s.acquirer.Acquire(ctx, pipeline.AcquireInput{
		URL:          rawURL,
		Timeout:      time.Duration(s.cfg.Runtime.TimeoutMS) * time.Millisecond,
		MaxBytes:     s.cfg.Runtime.MaxBytes,
		UserAgent:    userAgent,
		Profile:      s.cfg.Fetch.Profile,
		RetryProfile: s.cfg.Fetch.RetryProfile,
	})
	if err != nil {
		return pipeline.RawPage{}, pipeline.SimplifiedDOM{}, err
	}
	dom, err := s.reducer.Reduce(rawPage)
	if err != nil {
		return pipeline.RawPage{}, pipeline.SimplifiedDOM{}, err
	}
	return rawPage, dom, nil
}

func (s *Service) semanticRerankDiscoverCandidates(ctx context.Context, goal string, candidates []DiscoverCandidate) []DiscoverCandidate {
	if strings.TrimSpace(goal) == "" || len(candidates) == 0 {
		return candidates
	}
	semanticCandidates := make([]intel.SemanticCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		text := discoverycore.JoinNonEmpty(
			candidate.Metadata["page_title"],
			strings.TrimSpace(candidate.Label),
			discoverycore.URLTokenText(candidate.URL),
		)
		semanticCandidates = append(semanticCandidates, intel.SemanticCandidate{
			ID:   candidate.URL,
			Text: text,
		})
	}
	scored, err := s.semantic.Score(ctx, goal, semanticCandidates)
	if err != nil || len(scored) == 0 {
		return candidates
	}
	byURL := make(map[string]float64, len(scored))
	for _, item := range scored {
		byURL[item.ID] = item.Similarity
	}
	out := append([]DiscoverCandidate{}, candidates...)
	for i := range out {
		if similarity, ok := byURL[out[i].URL]; ok {
			out[i].Score += similarity * 3
			out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, "semantic_goal_alignment")
		}
	}
	discoverycore.SortCandidates(out)
	return out
}
