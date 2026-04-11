package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/josepavese/needlex/internal/intel"
)

type queryRewriteResult struct {
	SearchQueries   []string `json:"search_queries"`
	CanonicalEntity string   `json:"canonical_entity"`
	LocalityHints   []string `json:"locality_hints"`
	CategoryHints   []string `json:"category_hints"`
	Confidence      float64  `json:"confidence"`
}

func (s *Service) maybeRewriteSearchQueries(ctx context.Context, req QueryRequest, discoveryMode string) ([]string, queryRewriteResult, bool) {
	if strings.TrimSpace(req.SeedURL) != "" || discoveryMode != QueryDiscoveryWeb {
		return nil, queryRewriteResult{}, false
	}
	modelReq := intel.ModelRequest{
		Task:            intel.TaskQueryRewrite,
		ModelClass:      intel.ModelClassMicroSolver,
		MaxInputTokens:  512,
		MaxOutputTokens: 192,
		TimeoutMS:       max(500, s.cfg.Models.MicroTimeoutMS),
		SchemaName:      "query_rewrite.v1",
		Input: map[string]any{
			"goal":         strings.TrimSpace(req.Goal),
			"domain_hints": req.DomainHints,
		},
	}
	resp, err := s.runtime.Run(ctx, modelReq)
	if err != nil {
		return nil, queryRewriteResult{}, false
	}
	var out queryRewriteResult
	if err := json.Unmarshal([]byte(resp.OutputJSON), &out); err != nil {
		return nil, queryRewriteResult{}, false
	}
	out.CanonicalEntity = strings.TrimSpace(out.CanonicalEntity)
	if out.CanonicalEntity == "" {
		return nil, queryRewriteResult{}, false
	}
	queries := normalizeRewriteQueries(out.SearchQueries, strings.TrimSpace(out.CanonicalEntity), strings.TrimSpace(req.Goal))
	if len(queries) < 2 {
		return nil, queryRewriteResult{}, false
	}
	out.SearchQueries = queries
	return queries, out, true
}

func normalizeRewriteQueries(queries []string, canonicalEntity, fallback string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, min(len(queries)+1, 3))
	for _, query := range append([]string{fallback}, queries...) {
		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}
		key := strings.ToLower(query)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, query)
		if len(out) == 3 {
			break
		}
	}
	if len(out) == 0 && strings.TrimSpace(canonicalEntity) != "" {
		out = append(out, strings.TrimSpace(canonicalEntity))
	}
	return out
}
