package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/pipeline"
)

type CrawlRequest struct {
	SeedURL    string
	Profile    string
	UserAgent  string
	MaxPages   int
	MaxDepth   int
	SameDomain bool
	ForceLane  int
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

func (s *Service) Crawl(ctx context.Context, req CrawlRequest) (CrawlResponse, error) {
	profile, err := resolveProfile(req.Profile)
	if err != nil {
		return CrawlResponse{}, err
	}
	if strings.TrimSpace(req.SeedURL) == "" {
		return CrawlResponse{}, fmt.Errorf("crawl request seed_url must not be empty")
	}
	if req.MaxPages <= 0 {
		req.MaxPages = s.cfg.Runtime.MaxPages
	}
	if req.MaxDepth <= 0 {
		req.MaxDepth = s.cfg.Runtime.MaxDepth
	}

	type crawlNode struct {
		url   string
		depth int
	}

	queue := []crawlNode{{url: req.SeedURL, depth: 0}}
	visited := map[string]struct{}{}
	pages := make([]ReadResponse, 0, req.MaxPages)
	documents := make([]core.Document, 0, req.MaxPages)
	chunkCount := 0
	maxDepthReached := 0

	for len(queue) > 0 && len(pages) < req.MaxPages {
		node := queue[0]
		queue = queue[1:]

		if _, ok := visited[node.url]; ok {
			continue
		}
		visited[node.url] = struct{}{}
		if node.depth > maxDepthReached {
			maxDepthReached = node.depth
		}

		readResp, err := s.Read(ctx, ReadRequest{
			URL:       node.url,
			Objective: "crawl",
			Profile:   profile,
			UserAgent: req.UserAgent,
			ForceLane: req.ForceLane,
		})
		if err != nil {
			continue
		}

		pages = append(pages, readResp)
		documents = append(documents, readResp.Document)
		chunkCount += len(readResp.ResultPack.Chunks)

		if node.depth >= req.MaxDepth {
			continue
		}

		rawPage, err := s.acquirer.Acquire(ctx, pipeline.AcquireInput{
			URL:       readResp.Document.FinalURL,
			Timeout:   time.Duration(s.cfg.Runtime.TimeoutMS) * time.Millisecond,
			MaxBytes:  s.cfg.Runtime.MaxBytes,
			UserAgent: req.UserAgent,
		})
		if err != nil {
			continue
		}

		links := extractLinks(rawPage.HTML, rawPage.FinalURL, req.SameDomain)
		for _, next := range links {
			if _, ok := visited[next]; ok {
				continue
			}
			queue = append(queue, crawlNode{url: next, depth: node.depth + 1})
		}
	}

	resp := CrawlResponse{
		Documents: documents,
		Summary: CrawlSummary{
			SeedURL:         req.SeedURL,
			PagesVisited:    len(documents),
			MaxDepthReached: maxDepthReached,
			SameDomain:      req.SameDomain,
			ChunkCount:      chunkCount,
		},
		Pages: pages,
	}
	return resp, resp.Validate()
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
	root, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return nil
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	out := []string{}
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
				out = append(out, key)
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
