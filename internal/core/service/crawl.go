package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/josepavese/needlex/internal/core"
	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/pipeline"
)

type CrawlRequest struct {
	SeedURL        string
	Profile        string
	UserAgent      string
	MaxPages       int
	MaxDepth       int
	SameDomain     bool
	ForceLane      int
	PruningProfile string
	RenderHint     bool
}

type CrawlSummary struct {
	SeedURL         string `json:"seed_url"`
	PagesVisited    int    `json:"pages_visited"`
	MaxDepthReached int    `json:"max_depth_reached"`
	SameDomain      bool   `json:"same_domain"`
	ChunkCount      int    `json:"chunk_count"`
}

type CrawlResponse struct {
	Documents []core.Document `json:"documents"`
	Summary   CrawlSummary    `json:"summary"`
	Pages     []ReadResponse  `json:"pages"`
}

type linkCandidate = discoverycore.LinkCandidate

func (s *Service) Crawl(ctx context.Context, req CrawlRequest) (CrawlResponse, error) {
	profile, req, err := s.prepareCrawl(req)
	if err != nil {
		return CrawlResponse{}, err
	}
	state := crawlState{
		queue:     []crawlNode{{url: req.SeedURL, depth: 0}},
		visited:   map[string]struct{}{},
		pages:     make([]ReadResponse, 0, req.MaxPages),
		documents: make([]core.Document, 0, req.MaxPages),
	}
	for len(state.queue) > 0 && len(state.pages) < req.MaxPages {
		s.crawlNext(ctx, req, profile, &state)
	}
	resp := state.response(req)
	return resp, resp.Validate()
}

type crawlNode struct {
	url   string
	depth int
}

type crawlState struct {
	queue           []crawlNode
	visited         map[string]struct{}
	pages           []ReadResponse
	documents       []core.Document
	chunkCount      int
	maxDepthReached int
}

func (s *Service) prepareCrawl(req CrawlRequest) (string, CrawlRequest, error) {
	profile, err := resolveProfile(req.Profile)
	if err != nil {
		return "", CrawlRequest{}, err
	}
	if strings.TrimSpace(req.SeedURL) == "" {
		return "", CrawlRequest{}, fmt.Errorf("crawl request seed_url must not be empty")
	}
	if req.MaxPages <= 0 {
		req.MaxPages = s.cfg.Runtime.MaxPages
	}
	if req.MaxDepth <= 0 {
		req.MaxDepth = s.cfg.Runtime.MaxDepth
	}
	return profile, req, nil
}

func (s *Service) crawlNext(ctx context.Context, req CrawlRequest, profile string, state *crawlState) {
	node := state.queue[0]
	state.queue = state.queue[1:]
	if !state.visit(node) {
		return
	}
	readResp, ok := s.readCrawlNode(ctx, req, profile, node.url)
	if !ok {
		return
	}
	state.addPage(node, readResp)
	if node.depth >= req.MaxDepth {
		return
	}
	state.queue = append(state.queue, s.expandCrawlNode(ctx, req, node, readResp, state.visited)...)
}

func (s *Service) readCrawlNode(ctx context.Context, req CrawlRequest, profile, targetURL string) (ReadResponse, bool) {
	readResp, err := s.Read(ctx, ReadRequest{
		URL:            targetURL,
		Objective:      "crawl",
		Profile:        profile,
		UserAgent:      req.UserAgent,
		ForceLane:      req.ForceLane,
		PruningProfile: req.PruningProfile,
		RenderHint:     req.RenderHint,
	})
	return readResp, err == nil
}

func (s *Service) expandCrawlNode(ctx context.Context, req CrawlRequest, node crawlNode, readResp ReadResponse, visited map[string]struct{}) []crawlNode {
	rawPage, err := s.acquirer.Acquire(ctx, pipeline.AcquireInput{
		URL:          readResp.Document.FinalURL,
		Timeout:      time.Duration(s.cfg.Runtime.TimeoutMS) * time.Millisecond,
		MaxBytes:     s.cfg.Runtime.MaxBytes,
		UserAgent:    req.UserAgent,
		Profile:      s.cfg.Fetch.Profile,
		RetryProfile: s.cfg.Fetch.RetryProfile,
	})
	if err != nil {
		return nil
	}
	links := extractLinks(rawPage.HTML, rawPage.FinalURL, req.SameDomain)
	next := make([]crawlNode, 0, len(links))
	for _, targetURL := range links {
		if _, ok := visited[targetURL]; ok {
			continue
		}
		next = append(next, crawlNode{url: targetURL, depth: node.depth + 1})
	}
	return next
}

func (s *crawlState) visit(node crawlNode) bool {
	if _, ok := s.visited[node.url]; ok {
		return false
	}
	s.visited[node.url] = struct{}{}
	if node.depth > s.maxDepthReached {
		s.maxDepthReached = node.depth
	}
	return true
}

func (s *crawlState) addPage(node crawlNode, readResp ReadResponse) {
	_ = node
	s.pages = append(s.pages, readResp)
	s.documents = append(s.documents, readResp.Document)
	s.chunkCount += len(readResp.ResultPack.Chunks)
}

func (s crawlState) response(req CrawlRequest) CrawlResponse {
	return CrawlResponse{
		Documents: s.documents,
		Summary: CrawlSummary{
			SeedURL:         req.SeedURL,
			PagesVisited:    len(s.documents),
			MaxDepthReached: s.maxDepthReached,
			SameDomain:      req.SameDomain,
			ChunkCount:      s.chunkCount,
		},
		Pages: s.pages,
	}
}

func (r CrawlResponse) Validate() error {
	if strings.TrimSpace(r.Summary.SeedURL) == "" {
		return fmt.Errorf("crawl response summary.seed_url must not be empty")
	}
	if r.Summary.PagesVisited != len(r.Documents) {
		return fmt.Errorf("crawl response pages_visited does not match documents")
	}
	for i, document := range r.Documents {
		if err := document.Validate(); err != nil {
			return fmt.Errorf("crawl response documents[%d]: %w", i, err)
		}
	}
	for i, page := range r.Pages {
		if err := page.Validate(); err != nil {
			return fmt.Errorf("crawl response pages[%d]: %w", i, err)
		}
	}
	return nil
}

func extractLinks(rawHTML, baseURL string, sameDomain bool) []string {
	candidates := extractLinkCandidates(rawHTML, baseURL, sameDomain)
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate.URL)
	}
	return out
}

func extractLinkCandidates(rawHTML, baseURL string, sameDomain bool) []linkCandidate {
	root, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return nil
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	out := []linkCandidate{}
	seen := map[string]struct{}{}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && strings.EqualFold(node.Data, "a") {
			for _, attr := range node.Attr {
				if !strings.EqualFold(attr.Key, "href") || strings.TrimSpace(attr.Val) == "" {
					continue
				}
				ref, err := url.Parse(strings.TrimSpace(attr.Val))
				if err != nil {
					continue
				}
				resolved := base.ResolveReference(ref)
				if resolved.Scheme != "http" && resolved.Scheme != "https" {
					continue
				}
				if sameDomain && !sameHost(base, resolved) {
					continue
				}
				key := resolved.String()
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				out = append(out, linkCandidate{
					URL:   key,
					Label: nodeText(node),
				})
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return out
}

func sameHost(left, right *url.URL) bool {
	return strings.EqualFold(left.Hostname(), right.Hostname())
}

func nodeText(node *html.Node) string {
	var parts []string
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			text := strings.TrimSpace(current.Data)
			if text != "" {
				parts = append(parts, text)
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.Join(parts, " ")
}
