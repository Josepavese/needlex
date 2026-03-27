package service

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/html"

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

type DiscoverWebRequest struct {
	Goal          string
	SeedURL       string
	UserAgent     string
	MaxCandidates int
}

type DiscoverWebResponse struct {
	SeedURL      string              `json:"seed_url"`
	Provider     string              `json:"provider"`
	SelectedURL  string              `json:"selected_url"`
	DiscoveryURL string              `json:"discovery_url"`
	Candidates   []DiscoverCandidate `json:"candidates"`
}

const defaultWebDiscoverBaseURL = "https://html.duckduckgo.com/html/"

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

func (s *Service) DiscoverWeb(ctx context.Context, req DiscoverWebRequest) (DiscoverWebResponse, error) {
	if strings.TrimSpace(req.Goal) == "" {
		return DiscoverWebResponse{}, fmt.Errorf("discover web request goal must not be empty")
	}
	if req.MaxCandidates <= 0 {
		req.MaxCandidates = 5
	}

	baseURL := s.webDiscoverBaseURL
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultWebDiscoverBaseURL
	}
	searchURL, err := webSearchURL(baseURL, req.Goal)
	if err != nil {
		return DiscoverWebResponse{}, err
	}

	rawPage, err := s.acquirer.Acquire(ctx, pipeline.AcquireInput{
		URL:       searchURL,
		Timeout:   time.Duration(s.cfg.Runtime.TimeoutMS) * time.Millisecond,
		MaxBytes:  s.cfg.Runtime.MaxBytes,
		UserAgent: effectiveUserAgent(req.UserAgent, true),
	})
	if err != nil {
		return DiscoverWebResponse{}, err
	}

	results := extractSearchResults(rawPage.HTML, rawPage.FinalURL)
	candidates := scoreDiscoveryCandidates(req.Goal, req.SeedURL, "", results)
	filtered := make([]DiscoverCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.URL == "" {
			continue
		}
		filtered = append(filtered, candidate)
		if len(filtered) == req.MaxCandidates {
			break
		}
	}
	if len(filtered) == 0 {
		return DiscoverWebResponse{}, fmt.Errorf("discover web returned no candidates")
	}

	return DiscoverWebResponse{
		SeedURL:      req.SeedURL,
		Provider:     discoverProviderName(baseURL),
		SelectedURL:  filtered[0].URL,
		DiscoveryURL: rawPage.FinalURL,
		Candidates:   filtered,
	}, nil
}

func scoreDiscoveryCandidates(goal, seedURL, seedLabel string, links []linkCandidate) []DiscoverCandidate {
	out := make([]DiscoverCandidate, 0, len(links)+1)
	seen := map[string]struct{}{}

	if strings.TrimSpace(seedURL) != "" {
		seedScore, seedReason := discoveryScore(goal, seedURL, seedLabel, true)
		out = append(out, DiscoverCandidate{
			URL:    seedURL,
			Label:  strings.TrimSpace(seedLabel),
			Score:  seedScore,
			Reason: seedReason,
		})
		seen[seedURL] = struct{}{}
	}

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

func webSearchURL(baseURL, goal string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse web discover base url: %w", err)
	}
	query := parsed.Query()
	query.Set("q", goal)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func discoverProviderName(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Hostname() == "" {
		return "web_search_bootstrap"
	}
	return parsed.Hostname()
}

func extractSearchResults(rawHTML, baseURL string) []linkCandidate {
	root, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return nil
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	var out []linkCandidate
	seen := map[string]struct{}{}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && strings.EqualFold(node.Data, "a") {
			href := attrValue(node, "href")
			label := nodeText(node)
			resolved, ok := resolveSearchResultURL(base, href)
			if ok && label != "" {
				if _, exists := seen[resolved]; !exists {
					seen[resolved] = struct{}{}
					out = append(out, linkCandidate{URL: resolved, Label: label})
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return out
}

func resolveSearchResultURL(base *url.URL, href string) (string, bool) {
	if strings.TrimSpace(href) == "" {
		return "", false
	}
	ref, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return "", false
	}
	resolved := base.ResolveReference(ref)
	if uddg := resolved.Query().Get("uddg"); uddg != "" {
		decoded, err := url.QueryUnescape(uddg)
		if err == nil {
			resolved, err = url.Parse(decoded)
			if err != nil {
				return "", false
			}
		}
	}
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return "", false
	}
	if base.Host != "" && strings.EqualFold(resolved.Host, base.Host) {
		return "", false
	}
	return resolved.String(), true
}

func attrValue(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}
	return ""
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
