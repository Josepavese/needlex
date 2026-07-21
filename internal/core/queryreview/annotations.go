package queryreview

import (
	"strconv"
	"strings"

	"github.com/josepavese/needlex/internal/core/queryplan"
)

func AnnotateFallback(compiler *queryplan.QueryCompiler, previousURL, selectedURL string, previousDiag, selectedDiag Diagnostic, previousScore, selectedScore Score) {
	compiler.Decisions = append(compiler.Decisions, queryplan.QueryPlanDecision{
		Stage:      "select.post_read_semantic_fallback",
		Choice:     selectedURL,
		ReasonCode: queryplan.QueryPlanReasonSelection,
		Metadata: map[string]string{
			"previous_selected_url":                    strings.TrimSpace(previousURL),
			"validation_surface":                       "post_read_embedding_source_role",
			"previous_goal_similarity":                 formatScore(previousScore.Goal),
			"previous_entity_similarity":               formatScore(previousScore.Entity),
			"previous_origin_similarity":               formatScore(previousScore.Origin),
			"previous_derivative_similarity":           formatScore(previousScore.Derivative),
			"previous_source_merit":                    formatScore(previousScore.SourceMerit),
			"selected_goal_similarity":                 formatScore(selectedScore.Goal),
			"selected_entity_similarity":               formatScore(selectedScore.Entity),
			"selected_origin_similarity":               formatScore(selectedScore.Origin),
			"selected_derivative_similarity":           formatScore(selectedScore.Derivative),
			"selected_source_merit":                    formatScore(selectedScore.SourceMerit),
			"previous_semantic_family_intent_identity": formatScore(previousDiag.SemanticFamilyIntentIdentity),
			"previous_semantic_family_intent_merit":    formatScore(previousDiag.SemanticFamilyIntentMerit),
			"selected_semantic_family_intent_identity": formatScore(selectedDiag.SemanticFamilyIntentIdentity),
			"selected_semantic_family_intent_merit":    formatScore(selectedDiag.SemanticFamilyIntentMerit),
			"previous_semantic_diagnostic_prior":       formatScore(Prior(previousDiag)),
			"selected_semantic_diagnostic_prior":       formatScore(Prior(selectedDiag)),
		},
	})
}

func AnnotatePreReadFallback(compiler *queryplan.QueryCompiler, previousURL, selectedURL string, previousDiag, selectedDiag Diagnostic) {
	compiler.Decisions = append(compiler.Decisions, queryplan.QueryPlanDecision{
		Stage:      "select.pre_read_semantic_fallback",
		Choice:     strings.TrimSpace(selectedURL),
		ReasonCode: queryplan.QueryPlanReasonSelection,
		Metadata: map[string]string{
			"previous_selected_url":                    strings.TrimSpace(previousURL),
			"validation_surface":                       "pre_read_embedding_family_evidence",
			"previous_semantic_diagnostic_prior":       formatScore(Prior(previousDiag)),
			"selected_semantic_diagnostic_prior":       formatScore(Prior(selectedDiag)),
			"previous_semantic_family_intent_identity": formatScore(previousDiag.SemanticFamilyIntentIdentity),
			"previous_semantic_family_intent_merit":    formatScore(previousDiag.SemanticFamilyIntentMerit),
			"selected_semantic_family_intent_identity": formatScore(selectedDiag.SemanticFamilyIntentIdentity),
			"selected_semantic_family_intent_merit":    formatScore(selectedDiag.SemanticFamilyIntentMerit),
			"selected_semantic_family_evidence":        formatScore(selectedDiag.SemanticFamilyEvidence),
			"selected_semantic_family_provenance":      strconv.Itoa(selectedDiag.SemanticFamilyIntentProvenance),
		},
	})
}

func AnnotateReview(compiler *queryplan.QueryCompiler, selectedURL string, selectedDiag Diagnostic, selectedScore Score, challengerCount int, bestChallengerURL string, bestChallengerDiag Diagnostic, bestChallengerScore Score, outcome string) {
	metadata := map[string]string{
		"validation_surface":                    "post_read_embedding_source_role",
		"outcome":                               strings.TrimSpace(outcome),
		"challenger_count":                      strconv.Itoa(challengerCount),
		"selected_goal_similarity":              formatScore(selectedScore.Goal),
		"selected_entity_similarity":            formatScore(selectedScore.Entity),
		"selected_origin_similarity":            formatScore(selectedScore.Origin),
		"selected_derivative_similarity":        formatScore(selectedScore.Derivative),
		"selected_source_merit":                 formatScore(selectedScore.SourceMerit),
		"selected_content_chars":                strconv.Itoa(selectedScore.ContentChars),
		"selected_chunk_count":                  strconv.Itoa(selectedScore.ChunkCount),
		"selected_semantic_family_intent_merit": formatScore(selectedDiag.SemanticFamilyIntentMerit),
		"selected_semantic_diagnostic_prior":    formatScore(Prior(selectedDiag)),
	}
	if strings.TrimSpace(bestChallengerURL) != "" {
		metadata["best_challenger_url"] = strings.TrimSpace(bestChallengerURL)
		metadata["best_challenger_goal_similarity"] = formatScore(bestChallengerScore.Goal)
		metadata["best_challenger_entity_similarity"] = formatScore(bestChallengerScore.Entity)
		metadata["best_challenger_origin_similarity"] = formatScore(bestChallengerScore.Origin)
		metadata["best_challenger_derivative_similarity"] = formatScore(bestChallengerScore.Derivative)
		metadata["best_challenger_source_merit"] = formatScore(bestChallengerScore.SourceMerit)
		metadata["best_challenger_semantic_family_intent_merit"] = formatScore(bestChallengerDiag.SemanticFamilyIntentMerit)
		metadata["best_challenger_semantic_diagnostic_prior"] = formatScore(Prior(bestChallengerDiag))
	}
	compiler.Decisions = append(compiler.Decisions, queryplan.QueryPlanDecision{
		Stage:      "select.post_read_semantic_review",
		Choice:     strings.TrimSpace(selectedURL),
		ReasonCode: queryplan.QueryPlanReasonSelection,
		Metadata:   metadata,
	})
}

func formatScore(value float64) string {
	return strconv.FormatFloat(value, 'f', 3, 64)
}
