package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/proof"
)

const (
	QueryDiscoveryOff      = "off"
	QueryDiscoverySameSite = "same_site_links"
)

type QueryRequest struct {
	Goal           string
	SeedURL        string
	Profile        string
	UserAgent      string
	ForceLane      int
	DiscoveryMode  string
	PruningProfile string
	RenderHint     bool
}

type QueryPlan struct {
	Goal          string      `json:"goal"`
	SeedURL       string      `json:"seed_url"`
	Profile       string      `json:"profile"`
	Budget        core.Budget `json:"budget"`
	LaneMax       int         `json:"lane_max"`
	DiscoveryMode string      `json:"discovery_mode,omitempty"`
	SelectedURL   string      `json:"selected_url,omitempty"`
	CandidateURLs []string    `json:"candidate_urls,omitempty"`
	DomainHints   []string    `json:"domain_hints,omitempty"`
}

type QueryResponse struct {
	Plan         QueryPlan           `json:"plan"`
	Document     core.Document       `json:"document"`
	ResultPack   core.ResultPack     `json:"result_pack"`
	ProofRefs    []string            `json:"proof_refs"`
	ProofRecords []proof.ProofRecord `json:"proof_records"`
	Trace        proof.RunTrace      `json:"trace"`
	TraceID      string              `json:"trace_id"`
	CostReport   core.CostReport     `json:"cost_report"`
}

func (s *Service) Query(ctx context.Context, req QueryRequest) (QueryResponse, error) {
	profile, err := resolveProfile(req.Profile)
	if err != nil {
		return QueryResponse{}, err
	}
	if req.SeedURL == "" {
		return QueryResponse{}, fmt.Errorf("query request seed_url must not be empty")
	}
	if req.Goal == "" {
		return QueryResponse{}, fmt.Errorf("query request goal must not be empty")
	}
	discoveryMode, err := resolveDiscoveryMode(req.DiscoveryMode)
	if err != nil {
		return QueryResponse{}, err
	}

	plan := QueryPlan{
		Goal:    req.Goal,
		SeedURL: req.SeedURL,
		Profile: profile,
		Budget: core.Budget{
			MaxTokens:    s.cfg.Budget.MaxTokens,
			MaxLatencyMS: s.cfg.Budget.MaxLatencyMS,
			MaxPages:     s.cfg.Runtime.MaxPages,
			MaxDepth:     s.cfg.Runtime.MaxDepth,
			MaxBytes:     s.cfg.Runtime.MaxBytes,
		},
		LaneMax:       s.cfg.Runtime.LaneMax,
		DiscoveryMode: discoveryMode,
	}

	selectedURL := req.SeedURL
	candidateURLs := []string{req.SeedURL}
	if discoveryMode == QueryDiscoverySameSite {
		discovery, err := s.Discover(ctx, DiscoverRequest{
			Goal:          req.Goal,
			SeedURL:       req.SeedURL,
			UserAgent:     req.UserAgent,
			SameDomain:    true,
			MaxCandidates: min(5, s.cfg.Runtime.MaxPages),
		})
		if err != nil {
			return QueryResponse{}, err
		}
		selectedURL = discovery.SelectedURL
		candidateURLs = discoveryURLs(discovery.Candidates)
	}
	plan.SelectedURL = selectedURL
	plan.CandidateURLs = candidateURLs

	readResp, err := s.Read(ctx, ReadRequest{
		URL:            selectedURL,
		Objective:      req.Goal,
		Profile:        profile,
		UserAgent:      req.UserAgent,
		ForceLane:      req.ForceLane,
		PruningProfile: req.PruningProfile,
		RenderHint:     req.RenderHint,
	})
	if err != nil {
		return QueryResponse{}, err
	}
	readResp.ResultPack.Query = req.Goal

	resp := QueryResponse{
		Plan:         plan,
		Document:     readResp.Document,
		ResultPack:   readResp.ResultPack,
		ProofRefs:    append([]string{}, readResp.ResultPack.ProofRefs...),
		ProofRecords: append([]proof.ProofRecord{}, readResp.ProofRecords...),
		Trace:        readResp.Trace,
		TraceID:      readResp.Trace.TraceID,
		CostReport:   readResp.ResultPack.CostReport,
	}
	return resp, resp.Validate()
}

func (r QueryResponse) Validate() error {
	if r.Plan.Goal == "" {
		return fmt.Errorf("query response plan.goal must not be empty")
	}
	if r.Plan.SeedURL == "" {
		return fmt.Errorf("query response plan.seed_url must not be empty")
	}
	if r.Plan.SelectedURL == "" {
		return fmt.Errorf("query response plan.selected_url must not be empty")
	}
	if err := r.Document.Validate(); err != nil {
		return err
	}
	if err := r.ResultPack.Validate(); err != nil {
		return err
	}
	for i, record := range r.ProofRecords {
		if err := record.Validate(); err != nil {
			return fmt.Errorf("query response proof_records[%d]: %w", i, err)
		}
	}
	if err := r.Trace.Validate(); err != nil {
		return err
	}
	if r.TraceID == "" {
		return fmt.Errorf("query response trace_id must not be empty")
	}
	if err := r.CostReport.Validate(); err != nil {
		return err
	}
	return nil
}

func discoveryURLs(candidates []DiscoverCandidate) []string {
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate.URL)
	}
	return out
}

func resolveDiscoveryMode(mode string) (string, error) {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		return QueryDiscoverySameSite, nil
	}
	switch mode {
	case QueryDiscoveryOff, QueryDiscoverySameSite:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported query discovery mode %q", mode)
	}
}
