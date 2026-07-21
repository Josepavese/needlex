package service

import (
	"context"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/core/queryreview"
)

const (
	queryPostReadSemanticMinRemaining      = 500 * time.Millisecond
	queryPostReadSemanticScoreMinRemaining = 150 * time.Millisecond
)

func (s *Service) maybePostReadSemanticFallback(ctx context.Context, req QueryRequest, profile, discoveryMode string, plan *QueryPlan, candidates []DiscoverCandidate, selectedResp ReadResponse) (ReadResponse, bool) {
	if !s.queryPostReadSemanticEnabled(req, discoveryMode, plan, candidates) {
		return ReadResponse{}, false
	}
	selectedURL := strings.TrimSpace(plan.SelectedURL)
	selectedDiag, ok := queryreview.DiagnosticForURL(queryReviewPlan(plan), candidates, selectedURL)
	if !ok || !queryreview.HasSemantic(selectedDiag) {
		return ReadResponse{}, false
	}
	if !seedlessQueryHasTimeLeft(ctx, queryPostReadSemanticMinRemaining) {
		return ReadResponse{}, false
	}
	selectedScore := s.scoreQueryPostReadSemantic(ctx, req.Goal, selectedResp)
	if selectedScore.Empty() || !queryreview.SelectedNeedsReview(selectedDiag, selectedScore) {
		return ReadResponse{}, false
	}

	challengers := queryreview.Challengers(queryReviewPlan(plan), candidates, selectedURL, selectedDiag)
	bestRejectedURL := ""
	bestRejectedDiag := queryreview.Diagnostic{}
	bestRejectedScore := queryreview.Score{}
	for _, challenger := range challengers {
		if !seedlessQueryHasTimeLeft(ctx, queryPostReadSemanticMinRemaining) {
			break
		}
		nextResp, err := s.Read(ctx, s.prepareQuerySelectedReadRequest(req, profile, discoveryMode, challenger.URL))
		if err != nil {
			continue
		}
		nextScore := s.scoreQueryPostReadSemantic(ctx, req.Goal, nextResp)
		if nextScore.Empty() {
			continue
		}
		if bestRejectedURL == "" || nextScore.SourceMerit > bestRejectedScore.SourceMerit {
			bestRejectedURL = challenger.URL
			bestRejectedDiag = challenger.Diagnostic
			bestRejectedScore = nextScore
		}
		if !queryreview.ChallengerBeatsSelected(selectedDiag, challenger.Diagnostic, selectedScore, nextScore) {
			continue
		}
		previous := plan.SelectedURL
		plan.SelectedURL = challenger.URL
		queryreview.AnnotateFallback(&plan.Compiler, previous, challenger.URL, selectedDiag, challenger.Diagnostic, selectedScore, nextScore)
		return nextResp, true
	}
	queryreview.AnnotateReview(&plan.Compiler, selectedURL, selectedDiag, selectedScore, len(challengers), bestRejectedURL, bestRejectedDiag, bestRejectedScore, "kept_selected")
	return ReadResponse{}, false
}

func (s *Service) queryPostReadSemanticEnabled(req QueryRequest, discoveryMode string, plan *QueryPlan, candidates []DiscoverCandidate) bool {
	return s.semantic != nil &&
		strings.TrimSpace(req.SeedURL) == "" &&
		discoveryMode == QueryDiscoveryWeb &&
		plan != nil &&
		strings.TrimSpace(plan.SelectedURL) != "" &&
		len(plan.CandidateURLs) > 1 &&
		len(candidates) > 0
}

func (s *Service) scoreQueryPostReadSemantic(ctx context.Context, goal string, resp ReadResponse) queryreview.Score {
	text := queryPostReadSemanticText(resp)
	url := discovery.FirstNonEmpty(resp.Document.FinalURL, resp.Document.URL)
	if s.semantic == nil || strings.TrimSpace(goal) == "" || strings.TrimSpace(text) == "" || strings.TrimSpace(url) == "" {
		return queryreview.Score{}
	}
	score := queryreview.Score{}
	score.ContentChars = len([]rune(text))
	score.ChunkCount = len(resp.ResultPack.Chunks)
	if seedlessQueryHasTimeLeft(ctx, queryPostReadSemanticScoreMinRemaining) {
		score.Goal = s.scoreHostRootSemanticText(ctx, goal, url, text)
	}
	if seedlessQueryHasTimeLeft(ctx, queryPostReadSemanticScoreMinRemaining) {
		score.Entity = s.scoreHostRootSemanticText(ctx, queryPostReadEntityIdentityProfile(goal), url, text)
	}
	if seedlessQueryHasTimeLeft(ctx, queryPostReadSemanticScoreMinRemaining) {
		score.Origin = s.scoreHostRootSemanticText(ctx, hostRootOriginProfile(), url, text)
	}
	if seedlessQueryHasTimeLeft(ctx, queryPostReadSemanticScoreMinRemaining) {
		score.Derivative = s.scoreHostRootSemanticText(ctx, hostRootDerivativeProfile(), url, text)
	}
	score.SourceMerit = queryreview.SourceMerit(score.Goal, score.Entity, score.Origin, score.Derivative)
	return score
}

func queryPostReadSemanticText(resp ReadResponse) string {
	parts := []string{
		discovery.CompactSemanticText(resp.Document.Title, 180),
		discovery.CompactSemanticText(strings.Join(resp.ResultPack.Outline, " "), 320),
	}
	for i, chunk := range resp.ResultPack.Chunks {
		if i >= 5 {
			break
		}
		parts = append(parts,
			discovery.CompactSemanticText(strings.Join(chunk.HeadingPath, " "), 180),
			discovery.CompactSemanticText(chunk.Text, 520),
		)
	}
	if len(resp.ResultPack.Chunks) == 0 {
		for i, node := range resp.WebIR.Nodes {
			if i >= 8 {
				break
			}
			parts = append(parts, discovery.CompactSemanticText(node.Text, 320))
		}
	}
	parts = append(parts, discovery.CompactSemanticText(discovery.FirstNonEmpty(resp.Document.FinalURL, resp.Document.URL), 220))
	return discovery.CompactSemanticText(discovery.JoinNonEmpty(parts...), 2400)
}

func queryPostReadEntityIdentityProfile(goal string) string {
	return discovery.JoinNonEmpty(
		"Requested source-owner identity and maintained entity for this retrieval goal.",
		"Prefer the primary project, organization, product, standard body, publisher, or service named by the goal; not topic-adjacent services, integrations, tutorials, mirrors, catalogues, or third-party providers.",
		strings.TrimSpace(goal),
	)
}
