package semanticcalibrate

import (
	"strconv"
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
)

const ReasonSemanticCalibration = "trace_semantic_calibration"

type FeatureWeight struct {
	Name   string
	Weight float64
}

type Model struct {
	ID       string
	Features []FeatureWeight
	MaxBoost float64
}

func DefaultModel() Model {
	return Model{
		ID:       "semantic-calibrator-v1",
		MaxBoost: 0.18,
		Features: []FeatureWeight{
			{Name: "late_interaction_score", Weight: 0.16},
			{Name: "late_interaction_confidence", Weight: 0.10},
			{Name: "semantic_evidence_similarity", Weight: 0.04},
			{Name: "semantic_origin_similarity", Weight: 0.08},
			{Name: "semantic_authority_boost", Weight: 0.20},
			{Name: "semantic_authority_penalty", Weight: -0.42},
			{Name: "semantic_community_penalty", Weight: -0.30},
			{Name: "semantic_origin_alignment", Weight: 0.18},
			{Name: "semantic_derivative_alignment", Weight: -0.20},
			{Name: "cluster_coherence", Weight: 0.08},
			{Name: "cluster_centrality", Weight: 0.08},
			{Name: "semantic_role_intent", Weight: 0.06},
			{Name: "semantic_quorum_provider_count", Weight: 0.025},
		},
	}
}

func Apply(candidates []discoverycore.Candidate, model Model) []discoverycore.Candidate {
	if len(candidates) < 2 {
		return candidates
	}
	if model.MaxBoost <= 0 {
		model.MaxBoost = DefaultModel().MaxBoost
	}
	if strings.TrimSpace(model.ID) == "" {
		model.ID = DefaultModel().ID
	}
	out := append([]discoverycore.Candidate{}, candidates...)
	for i := range out {
		score := calibrationScore(out[i].Metadata, model)
		if score == 0 {
			continue
		}
		boost := clamp(score, -model.MaxBoost, model.MaxBoost)
		out[i].Score += boost
		out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, ReasonSemanticCalibration)
		out[i].Metadata = discoverycore.MergeMetadata(out[i].Metadata, map[string]string{
			"semantic_calibration_model": model.ID,
			"semantic_calibration_score": formatFloat(score),
			"semantic_calibration_boost": formatFloat(boost),
		})
	}
	discoverycore.SortCandidates(out)
	return out
}

func calibrationScore(meta map[string]string, model Model) float64 {
	score := 0.0
	for _, feature := range model.Features {
		value := parseFeature(meta, feature.Name)
		if value == 0 {
			continue
		}
		score += value * feature.Weight
	}
	return score
}

func parseFeature(meta map[string]string, name string) float64 {
	value := parseFloat(meta[name])
	if strings.HasSuffix(name, "_count") && value > 0 {
		return min(value/4, 1)
	}
	return value
}

func parseFloat(raw string) float64 {
	value, _ := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	return value
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 3, 64)
}

func clamp(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
