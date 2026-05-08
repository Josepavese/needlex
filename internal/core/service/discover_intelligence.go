package service

import (
	"context"
	"strings"

	"github.com/josepavese/needlex/internal/core/candidateintel"
	"github.com/josepavese/needlex/internal/intel"
)

func (s *Service) applyCandidateIntelligence(ctx context.Context, goal string, candidates []DiscoverCandidate) []DiscoverCandidate {
	return candidateintel.Apply(ctx, s.semantic, goal, candidates)
}

func (s *Service) scoreCandidateSetToGoal(ctx context.Context, goal string, candidates []intel.SemanticCandidate) map[string]float64 {
	if len(candidates) == 0 || strings.TrimSpace(goal) == "" {
		return nil
	}
	scored, err := s.semantic.Score(ctx, goal, candidates)
	if err != nil || len(scored) == 0 {
		return nil
	}
	out := make(map[string]float64, len(scored))
	for _, item := range scored {
		out[item.ID] = max(item.Similarity, 0)
	}
	return out
}
