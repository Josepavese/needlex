package service

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/pipeline"
)

type DiscoverRequest struct {
	Goal          string
	SeedURL       string
	UserAgent     string
	SameDomain    bool
	MaxCandidates int
}

type DiscoverCandidate struct {
	URL    string   `json:"url"`
	Label  string   `json:"label,omitempty"`
	Score  float64  `json:"score"`
	Reason []string `json:"reason,omitempty"`
}

type DiscoverResponse struct {
	SeedURL      string              `json:"seed_url"`
	SelectedURL  string              `json:"selected_url"`
	DiscoveryURL string              `json:"discovery_url"`
	Candidates   []DiscoverCandidate `json:"candidates"`
}

func (s *Service) Discover(ctx context.Context, req DiscoverRequest) (DiscoverResponse, error) {
	if strings.TrimSpace(req.SeedURL) == "" {
		return DiscoverResponse{}, fmt.Errorf("discover request seed_url must not be empty")
	}
	if strings.TrimSpace(req.Goal) == "" {
		return DiscoverResponse{}, fmt.Errorf("discover request goal must not be empty")
	}
	if req.MaxCandidates <= 0 {
		req.MaxCandidates = 5
	}

	rawPage, err := s.acquirer.Acquire(ctx, pipeline.AcquireInput{
		URL:       req.SeedURL,
		Timeout:   time.Duration(s.cfg.Runtime.TimeoutMS) * time.Millisecond,
		MaxBytes:  s.cfg.Runtime.MaxBytes,
		UserAgent: req.UserAgent,
	})
	if err != nil {
		return DiscoverResponse{}, err
	}

	dom, err := s.reducer.Reduce(rawPage)
	if err != nil {
		return DiscoverResponse{}, err
	}

	candidates := scoreDiscoveryCandidates(req.Goal, rawPage.FinalURL, dom.Title, extractLinkCandidates(rawPage.HTML, rawPage.FinalURL, req.SameDomain))
	limit := min(len(candidates), req.MaxCandidates)
	candidates = append([]DiscoverCandidate{}, candidates[:limit]...)

	selectedURL := rawPage.FinalURL
	if len(candidates) > 0 {
		selectedURL = candidates[0].URL
	}

	return DiscoverResponse{
		SeedURL:      req.SeedURL,
		SelectedURL:  selectedURL,
		DiscoveryURL: rawPage.FinalURL,
		Candidates:   candidates,
	}, nil
}

func scoreDiscoveryCandidates(goal, seedURL, seedLabel string, links []linkCandidate) []DiscoverCandidate {
	out := make([]DiscoverCandidate, 0, len(links)+1)
	seen := map[string]struct{}{}

	seedScore, seedReason := discoveryScore(goal, seedURL, seedLabel, true)
	out = append(out, DiscoverCandidate{
		URL:    seedURL,
		Label:  strings.TrimSpace(seedLabel),
		Score:  seedScore,
		Reason: seedReason,
	})
	seen[seedURL] = struct{}{}

	for _, link := range links {
		if _, ok := seen[link.URL]; ok {
			continue
		}
		seen[link.URL] = struct{}{}
		score, reason := discoveryScore(goal, link.URL, link.Label, false)
		out = append(out, DiscoverCandidate{
			URL:    link.URL,
			Label:  strings.TrimSpace(link.Label),
			Score:  score,
			Reason: reason,
		})
	}

	slices.SortStableFunc(out, func(left, right DiscoverCandidate) int {
		switch {
		case left.Score > right.Score:
			return -1
		case left.Score < right.Score:
			return 1
		case left.URL < right.URL:
			return -1
		case left.URL > right.URL:
			return 1
		default:
			return 0
		}
	})
	return out
}

func discoveryScore(goal, rawURL, label string, isSeed bool) (float64, []string) {
	reasons := []string{}
	score := 0.0

	goalTokens := uniqueTokens(goal)
	labelTokens := uniqueTokens(label)
	urlTokens := uniqueTokens(urlTokenText(rawURL))

	labelMatches := tokenOverlap(goalTokens, labelTokens)
	urlMatches := tokenOverlap(goalTokens, urlTokens)

	if labelMatches > 0 {
		score += float64(labelMatches) * 1.5
		reasons = append(reasons, "label_match")
	}
	if urlMatches > 0 {
		score += float64(urlMatches)
		reasons = append(reasons, "url_match")
	}
	if isSeed {
		score += 0.35
		reasons = append(reasons, "seed_fallback")
	}
	if strings.Contains(strings.ToLower(rawURL), "docs") || strings.Contains(strings.ToLower(rawURL), "guide") {
		score += 0.20
		reasons = append(reasons, "path_hint")
	}

	return score, reasons
}

func uniqueTokens(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	out := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, field := range fields {
		if len(field) < 2 {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	return out
}

func tokenOverlap(left, right []string) int {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	set := make(map[string]struct{}, len(right))
	for _, token := range right {
		set[token] = struct{}{}
	}
	matches := 0
	for _, token := range left {
		if _, ok := set[token]; ok {
			matches++
		}
	}
	return matches
}

func urlTokenText(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return strings.Join([]string{parsed.Hostname(), parsed.Path, path.Base(parsed.Path)}, " ")
}
