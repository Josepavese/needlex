package sourceresolution

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/core/agentreadable"
	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/core/webirbuilder"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/pipeline"
	"github.com/josepavese/needlex/internal/proof"
	"github.com/josepavese/needlex/internal/rendering"
)

const (
	AutoRenderDeadlineTimeout      = 8 * time.Second
	AutoRenderDeadlineMinRemaining = 10 * time.Second

	AgentReadableProbeTimeout         = 2 * time.Second
	agentReadableProbeMinRemaining    = 750 * time.Millisecond
	agentReadableConventionalLimit    = 6
	agentReadableExpandedLimit        = 4
	agentReadableConventionalSitemaps = 2
)

type Request struct {
	Objective, UserAgent, PruningProfile string
	FetchProfile, FetchRetryProfile      string
	RenderHint                           bool
	RenderMode, AgentReadableMode        string
}

type Resolver struct {
	Config   config.Config
	Acquirer pipeline.Acquirer
	Reducer  pipeline.Reducer
	Renderer rendering.Renderer
	Semantic intel.SemanticAligner
	Now      func() time.Time
	Reduce   func(*proof.Recorder, pipeline.RawPage, string) (pipeline.SimplifiedDOM, error)
}

func (r Resolver) now() time.Time {
	if r.Now == nil {
		return time.Now()
	}
	return r.Now()
}

func (r Resolver) ResolveAgentReadable(ctx context.Context, recorder *proof.Recorder, req Request, rawPage pipeline.RawPage) (pipeline.RawPage, error) {
	const stage = "agent_readable"
	mode := normalizeAgentReadableMode(req.AgentReadableMode)
	if mode == "off" || !r.Config.Agent.ReadableEnabled || !pipeline.IsHTMLLikeRawPage(rawPage) {
		return rawPage, nil
	}
	if err := recorder.StageStarted(stage, rawPage, r.now().UTC()); err != nil {
		return pipeline.RawPage{}, err
	}
	maxCandidates := agentReadableCandidateLimit(r.Config.Agent.MaxCandidates)
	allCandidates := agentreadable.Discover(rawPage, 0)
	declaredCandidates := agentReadableCandidateBatch(declaredAgentReadableCandidates(allCandidates), maxCandidates)
	probeConventional := mode == "auto" && shouldProbeConventionalAgentSources(rawPage, req, r.Reducer) && agentReadableHasProbeTime(ctx)
	candidates := append([]agentreadable.Candidate{}, declaredCandidates...)
	metadata := map[string]string{
		"candidate_count": fmt.Sprintf("%d", len(candidates)),
		"selected":        "false",
		"expanded":        fmt.Sprintf("%t", probeConventional),
	}
	if !probeConventional {
		selection, ok := r.tryAgentReadableCandidates(ctx, req, rawPage, declaredCandidates, recorder)
		if !ok {
			if err := recorder.StageCompleted(stage, rawPage, len(candidates), metadata, r.now().UTC()); err != nil {
				return pipeline.RawPage{}, err
			}
			return rawPage, nil
		}
		markSelectedAgentReadable(metadata, selection)
		if err := recorder.StageCompleted(stage, selection.Page, 1, metadata, r.now().UTC()); err != nil {
			return pipeline.RawPage{}, err
		}
		return selection.Page, nil
	}

	accepted := r.collectAgentReadableCandidates(ctx, req, rawPage, declaredCandidates, recorder)
	if probeConventional {
		robotsPolicy, robotsLoaded := r.fetchAgentReadableRobotsPolicy(ctx, req, rawPage)
		conventional := agentReadableCandidateDifference(allCandidates, declaredCandidates)
		conventional, conventionallyDisallowed := filterCandidatesByRobots(conventional, robotsPolicy, robotsLoaded, pipeline.EffectiveUserAgent(req.UserAgent, req.RenderHint))
		conventional = agentReadableBalancedConventionalBatch(conventional, agentReadableConventionalCandidateLimit(maxCandidates))
		candidates = append(candidates, conventional...)
		metadata["robots_policy"] = fmt.Sprintf("%t", robotsLoaded)
		metadata["robots_disallowed_count"] = fmt.Sprintf("%d", conventionallyDisallowed)
		metadata["candidate_count"] = fmt.Sprintf("%d", len(candidates))
		accepted = append(accepted, r.collectAgentReadableCandidates(ctx, req, rawPage, conventional, recorder)...)
		expanded := r.expandAgentReadableCandidates(ctx, req, rawPage, candidates, robotsPolicy, robotsLoaded, agentReadableExpandedCandidateLimit(maxCandidates))
		extra := agentReadableCandidateBatch(agentReadableCandidateDifference(expanded, candidates), agentReadableExpandedCandidateLimit(maxCandidates))
		candidates = append(candidates, extra...)
		metadata["candidate_count"] = fmt.Sprintf("%d", len(candidates))
		metadata["expanded_candidate_count"] = fmt.Sprintf("%d", len(extra))
		accepted = append(accepted, r.collectAgentReadableCandidates(ctx, req, rawPage, extra, recorder)...)
	}
	if selection, ok := r.selectAgentReadableCandidate(ctx, req, accepted); ok {
		markSelectedAgentReadable(metadata, selection)
		if err := recorder.StageCompleted(stage, selection.Page, len(candidates), metadata, r.now().UTC()); err != nil {
			return pipeline.RawPage{}, err
		}
		return selection.Page, nil
	}
	if err := recorder.StageCompleted(stage, rawPage, len(candidates), metadata, r.now().UTC()); err != nil {
		return pipeline.RawPage{}, err
	}
	return rawPage, nil
}

func agentReadableCandidateLimit(configured int) int {
	if configured > 0 {
		return configured
	}
	return 16
}

func normalizeAgentReadableMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "auto":
		return "auto"
	case "declared", "off":
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return "auto"
	}
}

func agentReadableConventionalCandidateLimit(maxCandidates int) int {
	if maxCandidates <= 0 {
		return agentReadableConventionalLimit
	}
	return min(maxCandidates, agentReadableConventionalLimit)
}

func agentReadableExpandedCandidateLimit(maxCandidates int) int {
	if maxCandidates <= 0 {
		return agentReadableExpandedLimit
	}
	return min(maxCandidates, agentReadableExpandedLimit)
}

func shouldProbeConventionalAgentSources(rawPage pipeline.RawPage, req Request, reducer pipeline.Reducer) bool {
	dom, err := reducer.ReduceProfile(rawPage, req.PruningProfile)
	if err != nil {
		return false
	}
	webIR := webirbuilder.Build(webirbuilder.EnsureMinimum(dom))
	if webIR.Signals.SubstrateClass == "client_rendered_app" {
		return true
	}
	if !agentReadableSurfaceHint(rawPage) {
		return false
	}
	for _, reason := range core.WebIRUtilityReasons(webIR) {
		switch reason {
		case "low_node_count", "low_reduced_chars", "navigation_like_surface", "client_rendered_app_surface":
			return true
		}
	}
	return false
}

func agentReadableSurfaceHint(rawPage pipeline.RawPage) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawPage.FinalURL))
	if err != nil {
		return false
	}
	cleanPath := path.Clean("/" + strings.TrimSpace(parsed.Path))
	if cleanPath != "/" && strings.Count(strings.Trim(cleanPath, "/"), "/") >= 1 {
		return true
	}
	return false
}

func declaredAgentReadableCandidates(candidates []agentreadable.Candidate) []agentreadable.Candidate {
	out := make([]agentreadable.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		switch candidate.DeclaredBy {
		case "link_header", "html_link":
			out = append(out, candidate)
		}
	}
	return out
}

type agentReadableSelection struct {
	Page          pipeline.RawPage
	Candidate     agentreadable.Candidate
	SemanticScore float64
}

func (r Resolver) tryAgentReadableCandidates(ctx context.Context, req Request, rawPage pipeline.RawPage, candidates []agentreadable.Candidate, recorder *proof.Recorder) (agentReadableSelection, bool) {
	accepted := r.collectAgentReadableCandidates(ctx, req, rawPage, candidates, recorder)
	return r.selectAgentReadableCandidate(ctx, req, accepted)
}

func (r Resolver) collectAgentReadableCandidates(ctx context.Context, req Request, rawPage pipeline.RawPage, candidates []agentreadable.Candidate, recorder *proof.Recorder) []agentReadableSelection {
	accepted := make([]agentReadableSelection, 0, len(candidates))
	for _, candidate := range candidates {
		if !agentReadableHasProbeTime(ctx) {
			break
		}
		page, selectedCandidate, ok := r.tryAgentReadableCandidate(ctx, req, rawPage, candidate, recorder)
		if ok {
			accepted = append(accepted, agentReadableSelection{Page: page, Candidate: selectedCandidate})
		}
	}
	return accepted
}

func markSelectedAgentReadable(metadata map[string]string, selection agentReadableSelection) {
	metadata["selected"] = "true"
	metadata["selected_url"] = selection.Page.FinalURL
	metadata["selected_kind"] = selection.Page.SourceKind
	metadata["selected_declared_by"] = selection.Candidate.DeclaredBy
	if selection.SemanticScore > 0 {
		metadata["selected_semantic_score"] = fmt.Sprintf("%.4f", selection.SemanticScore)
	}
}

func (r Resolver) selectAgentReadableCandidate(ctx context.Context, req Request, accepted []agentReadableSelection) (agentReadableSelection, bool) {
	if len(accepted) == 0 {
		return agentReadableSelection{}, false
	}
	if len(accepted) == 1 {
		return accepted[0], true
	}
	scores := r.scoreAgentReadableSelections(ctx, req, accepted)
	best := 0
	for index := range accepted {
		accepted[index].SemanticScore = scores[agentReadableSelectionID(index)]
		if agentReadableSelectionBetter(accepted[index], accepted[best]) {
			best = index
		}
	}
	return accepted[best], true
}

func (r Resolver) scoreAgentReadableSelections(ctx context.Context, req Request, accepted []agentReadableSelection) map[string]float64 {
	if r.Semantic == nil || strings.TrimSpace(req.Objective) == "" {
		return nil
	}
	candidates := make([]intel.SemanticCandidate, 0, len(accepted))
	for index, selection := range accepted {
		text := strings.TrimSpace(strings.Join([]string{
			selection.Candidate.Kind,
			selection.Candidate.DeclaredBy,
			selection.Page.FinalURL,
			discoverycore.CompactSemanticText(selection.Page.HTML, 1600),
		}, "\n"))
		if text == "" {
			continue
		}
		candidates = append(candidates, intel.SemanticCandidate{ID: agentReadableSelectionID(index), Text: text})
	}
	scored, err := r.Semantic.Score(ctx, req.Objective, candidates)
	if err != nil {
		return nil
	}
	out := map[string]float64{}
	for _, score := range scored {
		out[score.ID] = score.Similarity
	}
	return out
}
