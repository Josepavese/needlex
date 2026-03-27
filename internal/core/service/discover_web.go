package service

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/josepavese/needlex/internal/pipeline"
)

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
const webProbeLimit = 3

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
	candidates = s.expandAndRerankWebCandidates(ctx, req.Goal, req.UserAgent, candidates, req.MaxCandidates)
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

func (s *Service) expandAndRerankWebCandidates(ctx context.Context, goal, userAgent string, candidates []DiscoverCandidate, maxCandidates int) []DiscoverCandidate {
	if len(candidates) == 0 {
		return nil
	}
	probeCount := min(len(candidates), min(maxCandidates, webProbeLimit))
	if probeCount <= 0 {
		probeCount = min(len(candidates), webProbeLimit)
	}

	merged := append([]DiscoverCandidate{}, candidates...)
	for _, candidate := range candidates[:probeCount] {
		probed, err := s.probeWebCandidate(ctx, goal, userAgent, candidate)
		if err != nil {
			continue
		}
		merged = mergeDiscoverCandidate(merged, probed)
	}

	slices.SortStableFunc(merged, func(left, right DiscoverCandidate) int {
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
	return merged
}

func (s *Service) probeWebCandidate(ctx context.Context, goal, userAgent string, candidate DiscoverCandidate) ([]DiscoverCandidate, error) {
	rawPage, err := s.acquirer.Acquire(ctx, pipeline.AcquireInput{
		URL:       candidate.URL,
		Timeout:   time.Duration(s.cfg.Runtime.TimeoutMS) * time.Millisecond,
		MaxBytes:  s.cfg.Runtime.MaxBytes,
		UserAgent: effectiveUserAgent(userAgent, true),
	})
	if err != nil {
		return nil, err
	}

	dom, err := s.reducer.Reduce(rawPage)
	if err != nil {
		return nil, err
	}

	refined := refineWebCandidate(goal, candidate, rawPage.FinalURL, dom.Title)
	out := []DiscoverCandidate{refined}

	expanded := extractLinkCandidates(rawPage.HTML, rawPage.FinalURL, true)
	expandedScored := scoreDiscoveryCandidates(goal, "", "", expanded)
	if len(expandedScored) > 0 {
		best := expandedScored[0]
		best.Score += 0.40
		best.Reason = appendUniqueReason(best.Reason, "page_expand")
		out = append(out, best)
	}
	return out, nil
}

func refineWebCandidate(goal string, candidate DiscoverCandidate, finalURL, pageTitle string) DiscoverCandidate {
	score, reasons := discoveryScore(goal, finalURL, joinNonEmpty(pageTitle, candidate.Label), false)
	if strings.TrimSpace(pageTitle) != "" {
		score += 0.35
		reasons = append(reasons, "page_title_probe")
	}
	if strings.TrimSpace(finalURL) != "" && finalURL != candidate.URL {
		reasons = append(reasons, "redirect_resolved")
	}
	return DiscoverCandidate{
		URL:    finalURL,
		Label:  firstNonEmpty(pageTitle, candidate.Label),
		Score:  max(score, candidate.Score),
		Reason: appendUniqueReason(append([]string{}, candidate.Reason...), reasons...),
	}
}

func mergeDiscoverCandidate(existing []DiscoverCandidate, incoming []DiscoverCandidate) []DiscoverCandidate {
	out := append([]DiscoverCandidate{}, existing...)
	for _, candidate := range incoming {
		if strings.TrimSpace(candidate.URL) == "" {
			continue
		}
		replaced := false
		for i := range out {
			if out[i].URL != candidate.URL {
				continue
			}
			if candidate.Score > out[i].Score {
				out[i].Score = candidate.Score
			}
			out[i].Label = firstNonEmpty(candidate.Label, out[i].Label)
			out[i].Reason = appendUniqueReason(out[i].Reason, candidate.Reason...)
			replaced = true
			break
		}
		if !replaced {
			out = append(out, candidate)
		}
	}
	return out
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
