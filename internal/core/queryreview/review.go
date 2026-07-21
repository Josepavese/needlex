package queryreview

import (
	"sort"
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
)

const challengerLimit = 2

type Plan struct {
	SelectedURL   string
	CandidateURLs []string
	Diagnostics   []Diagnostic
}

type Challenger struct {
	URL        string
	Diagnostic Diagnostic
	PriorMerit float64
}

type PreReadFallback struct {
	URL                string
	Diagnostic         Diagnostic
	SelectedDiagnostic Diagnostic
}

type Score struct {
	Goal         float64
	Entity       float64
	Origin       float64
	Derivative   float64
	SourceMerit  float64
	ContentChars int
	ChunkCount   int
}

func (score Score) Empty() bool {
	return score.Goal <= 0 && score.Entity <= 0 && score.Origin <= 0 && score.Derivative <= 0
}

func SourceMerit(goal, entity, origin, derivative float64) float64 {
	return goal*0.25 + entity*0.55 + origin*0.75 - derivative*0.55
}

func SelectedNeedsReview(diag Diagnostic, score Score) bool {
	if selectedHasStrongSourceEvidence(diag, score) {
		return false
	}
	if score.Derivative >= 0.12 && score.Derivative > score.Origin+0.04 {
		return true
	}
	if score.Goal >= 0.25 && score.Entity > 0 && score.Entity < score.Goal-0.08 {
		return true
	}
	origin, derivative := Origin(diag), Derivative(diag)
	if derivative >= 0.12 && derivative > origin+0.02 {
		return true
	}
	if diag.SemanticFamilyIntentTopic >= 0.25 && diag.SemanticFamilyIntentTopic > diag.SemanticFamilyIntentIdentity+0.04 && HasAnyReason(diag, "semantic_topic_without_identity_penalty") {
		return true
	}
	if HasAnyReason(diag, "semantic_derivative_surface_penalty") && (derivative > origin || score.Derivative >= score.Origin) {
		return true
	}
	if score.ContentChars > 0 && score.ContentChars <= 260 && HasAnyReason(diag, "semantic_derivative_surface_penalty", "semantic_topic_without_identity_penalty") {
		return true
	}
	return score.SourceMerit < 0.15 && HasAnyReason(diag, "semantic_derivative_surface_penalty", "semantic_topic_without_identity_penalty")
}

func selectedHasStrongSourceEvidence(diag Diagnostic, score Score) bool {
	origin, derivative := Origin(diag), Derivative(diag)
	if HasAnyReason(diag, "semantic_provenance_identity", "semantic_family_evidence_mass", "identity_reference", "candidate_identity_alignment", "semantic_custodian_alignment") &&
		origin >= derivative && score.Origin >= score.Derivative-0.03 {
		return true
	}
	if diag.SemanticFamilyIntentProvenance >= 2 && score.Origin >= score.Derivative {
		return true
	}
	return (diag.SemanticFamilyEvidence >= 0.25 || diag.SemanticFamilyEvidenceStrong >= 2) &&
		diag.SemanticFamilyIntentProvenance >= 2 && score.Goal >= 0.35 && score.Entity >= score.Derivative+0.10
}

func ChallengerBeatsSelected(selectedDiag, challengerDiag Diagnostic, selected, challenger Score) bool {
	if selected.Empty() || challenger.Empty() || challenger.Goal+0.10 < selected.Goal && challenger.Goal < 0.35 {
		return false
	}
	comparison := newChallengerComparison(selectedDiag, challengerDiag, selected, challenger)
	return comparison.clearMeritWin() || comparison.trapRecovery() ||
		comparison.familyRecovery() || comparison.priorRecovery()
}

type challengerComparison struct {
	selectedTrap       bool
	entityGrounded     bool
	sourceDominant     bool
	sourceGain         float64
	entityGain         float64
	derivativeRelief   float64
	meritGain          float64
	familyIdentityGain float64
	familyMeritGain    float64
	priorGain          float64
	selected           Score
	challenger         Score
}

func newChallengerComparison(selectedDiag, challengerDiag Diagnostic, selected, challenger Score) challengerComparison {
	return challengerComparison{
		selectedTrap:       SelectedNeedsReview(selectedDiag, selected),
		entityGrounded:     challenger.Entity >= selected.Entity+0.05 || challenger.Entity >= 0.35,
		sourceDominant:     challenger.Origin >= 0.10 && challenger.Origin >= challenger.Derivative+0.035,
		sourceGain:         challenger.Origin - selected.Origin,
		entityGain:         challenger.Entity - selected.Entity,
		derivativeRelief:   selected.Derivative - challenger.Derivative,
		meritGain:          challenger.SourceMerit - selected.SourceMerit,
		familyIdentityGain: challengerDiag.SemanticFamilyIntentIdentity - selectedDiag.SemanticFamilyIntentIdentity,
		familyMeritGain:    challengerDiag.SemanticFamilyIntentMerit - selectedDiag.SemanticFamilyIntentMerit,
		priorGain:          Prior(challengerDiag) - Prior(selectedDiag),
		selected:           selected,
		challenger:         challenger,
	}
}

func (comparison challengerComparison) clearMeritWin() bool {
	return comparison.entityGrounded && comparison.sourceDominant && comparison.meritGain >= 0.06
}

func (comparison challengerComparison) trapRecovery() bool {
	materialGain := comparison.entityGain >= 0.05 || comparison.sourceGain >= 0.04 ||
		comparison.derivativeRelief >= 0.04 || comparison.familyIdentityGain >= 0.02
	return comparison.selectedTrap && comparison.entityGrounded && comparison.sourceDominant && materialGain
}

func (comparison challengerComparison) familyRecovery() bool {
	return comparison.selectedTrap && comparison.entityGrounded && comparison.familyIdentityGain >= 0.025 &&
		comparison.familyMeritGain >= 0.005 && comparison.challenger.Origin >= comparison.selected.Origin-0.02 &&
		comparison.challenger.Derivative <= comparison.selected.Derivative+0.02
}

func (comparison challengerComparison) priorRecovery() bool {
	return comparison.priorGain >= 0.10 && comparison.entityGrounded &&
		comparison.sourceDominant && comparison.sourceGain >= 0.02
}

func PreReadFallbackCandidate(seedURL, discoveryMode, webMode string, plan Plan, candidates []discoverycore.Candidate) (PreReadFallback, bool) {
	if strings.TrimSpace(seedURL) != "" || discoveryMode != webMode || strings.TrimSpace(plan.SelectedURL) == "" || len(plan.CandidateURLs) < 2 || len(candidates) == 0 {
		return PreReadFallback{}, false
	}
	selected, ok := DiagnosticForURL(plan, candidates, plan.SelectedURL)
	if !ok || !preReadSelectedRecoverable(selected) {
		return PreReadFallback{}, false
	}
	for _, challenger := range Challengers(plan, candidates, plan.SelectedURL, selected) {
		if PreReadChallengerBeatsSelected(selected, challenger.Diagnostic) {
			return PreReadFallback{URL: challenger.URL, Diagnostic: challenger.Diagnostic, SelectedDiagnostic: selected}, true
		}
	}
	return PreReadFallback{}, false
}

func preReadSelectedRecoverable(diag Diagnostic) bool {
	origin, derivative := Origin(diag), Derivative(diag)
	if derivative >= 0.12 && derivative > origin+0.03 {
		return true
	}
	return diag.SemanticFamilyIntentTopic >= 0.25 &&
		diag.SemanticFamilyIntentTopic > diag.SemanticFamilyIntentIdentity+0.06 &&
		HasAnyReason(diag, "semantic_topic_without_identity_penalty", "semantic_derivative_surface_penalty")
}

func PreReadChallengerBeatsSelected(selected, challenger Diagnostic) bool {
	comparison := newPreReadComparison(selected, challenger)
	return comparison.strongFamilyWin() || comparison.familyIdentityWin() ||
		comparison.selectedTrapRecovery() || comparison.provenanceRecovery() ||
		comparison.derivativeRecovery() || comparison.rootIdentityRecovery() ||
		comparison.priorRecovery()
}

type preReadComparison struct {
	selected                       Diagnostic
	challenger                     Diagnostic
	priorGain                      float64
	sourceRoleNotWorse             bool
	redundantWithoutFamilyEvidence bool
}

func newPreReadComparison(selected, challenger Diagnostic) preReadComparison {
	return preReadComparison{
		selected:                       selected,
		challenger:                     challenger,
		priorGain:                      Prior(challenger) - Prior(selected),
		sourceRoleNotWorse:             Origin(challenger) >= Origin(selected)-0.01 && Derivative(challenger) <= Derivative(selected)+0.02,
		redundantWithoutFamilyEvidence: HasAnyReason(challenger, "candidate_cluster_redundant") && challenger.SemanticFamilyEvidence < 0.25 && challenger.SemanticFamilyEvidenceStrong < 2,
	}
}

func (comparison preReadComparison) strongFamilyWin() bool {
	challenger := comparison.challenger
	strongEvidence := challenger.SemanticFamilyEvidence >= 0.25 || challenger.SemanticFamilyEvidenceStrong >= 2 ||
		challenger.SemanticFamilyIntentProvenance >= 2 && !comparison.redundantWithoutFamilyEvidence
	return comparison.sourceRoleNotWorse && strongEvidence
}

func (comparison preReadComparison) familyIdentityWin() bool {
	return !comparison.redundantWithoutFamilyEvidence && comparison.sourceRoleNotWorse &&
		comparison.challenger.SemanticFamilyIntentIdentity >= comparison.selected.SemanticFamilyIntentIdentity+0.025 &&
		comparison.challenger.SemanticFamilyIntentMerit >= comparison.selected.SemanticFamilyIntentMerit+0.005
}

func (comparison preReadComparison) selectedTrapRecovery() bool {
	return !comparison.redundantWithoutFamilyEvidence && comparison.sourceRoleNotWorse &&
		comparison.challenger.SemanticFamilyIntentIdentity >= comparison.selected.SemanticFamilyIntentIdentity+0.03 &&
		comparison.challenger.SemanticFamilyIntentMerit >= comparison.selected.SemanticFamilyIntentMerit-0.05 &&
		HasAnyReason(comparison.selected, "semantic_topic_without_identity_penalty", "semantic_derivative_surface_penalty")
}

func (comparison preReadComparison) provenanceRecovery() bool {
	return HasAnyReason(comparison.challenger, "semantic_provenance_identity") &&
		!HasAnyReason(comparison.selected, "semantic_provenance_identity") &&
		comparison.challenger.SemanticFamilyIntentIdentity >= comparison.selected.SemanticFamilyIntentIdentity+0.05 &&
		comparison.challenger.SemanticFamilyIntentMerit >= comparison.selected.SemanticFamilyIntentMerit+0.05 &&
		Derivative(comparison.challenger) <= Derivative(comparison.selected)+0.06
}

func (comparison preReadComparison) derivativeRecovery() bool {
	return comparison.challenger.SemanticFamilyIntentMerit >= comparison.selected.SemanticFamilyIntentMerit+0.04 &&
		Derivative(comparison.challenger) <= Derivative(comparison.selected)-0.03
}

func (comparison preReadComparison) rootIdentityRecovery() bool {
	return Derivative(comparison.challenger) <= Derivative(comparison.selected)-0.04 &&
		comparison.challenger.SemanticFamilyIntentIdentity >= comparison.selected.SemanticFamilyIntentIdentity-0.04 &&
		HasAnyReason(comparison.challenger, "host_root_identity_probe", "semantic_root_identity_probe")
}

func (comparison preReadComparison) priorRecovery() bool {
	sourceRecovery := Origin(comparison.challenger) >= Origin(comparison.selected)+0.05 &&
		Derivative(comparison.challenger) <= Derivative(comparison.selected)+0.03
	identityRecovery := Identity(comparison.challenger) >= Identity(comparison.selected)+0.04 &&
		comparison.challenger.SemanticFamilyIntentMerit >= comparison.selected.SemanticFamilyIntentMerit
	return comparison.priorGain >= 0.40 && (sourceRecovery || identityRecovery)
}

func Challengers(plan Plan, candidates []discoverycore.Candidate, selectedURL string, selected Diagnostic) []Challenger {
	selectedPrior := Prior(selected)
	out := make([]Challenger, 0, challengerLimit)
	for _, candidateURL := range plan.CandidateURLs {
		candidateURL = strings.TrimSpace(candidateURL)
		if candidateURL == "" || sameURL(candidateURL, selectedURL) {
			continue
		}
		diag, ok := DiagnosticForURL(plan, candidates, candidateURL)
		if !ok || !HasSemantic(diag) {
			continue
		}
		prior := Prior(diag)
		if challengerViable(selected, diag, selectedPrior, prior) {
			out = append(out, Challenger{URL: candidateURL, Diagnostic: diag, PriorMerit: prior})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].PriorMerit == out[j].PriorMerit {
			return out[i].URL < out[j].URL
		}
		return out[i].PriorMerit > out[j].PriorMerit
	})
	if len(out) > challengerLimit {
		out = out[:challengerLimit]
	}
	return out
}

func challengerViable(selected, challenger Diagnostic, selectedPrior, challengerPrior float64) bool {
	return challengerPrior >= selectedPrior+0.03 ||
		challenger.SemanticFamilyIntentIdentity >= selected.SemanticFamilyIntentIdentity+0.02 && challenger.SemanticFamilyIntentMerit >= selected.SemanticFamilyIntentMerit-0.03 ||
		Origin(challenger) >= Origin(selected)+0.04 && Derivative(challenger) <= Derivative(selected)+0.04 ||
		challenger.SemanticFamilyEvidence >= selected.SemanticFamilyEvidence+0.05 ||
		challenger.SemanticProvenanceIdentity >= selected.SemanticProvenanceIdentity+0.05
}

func DiagnosticForURL(plan Plan, candidates []discoverycore.Candidate, rawURL string) (Diagnostic, bool) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return Diagnostic{}, false
	}
	for _, diag := range plan.Diagnostics {
		if sameURL(diag.URL, rawURL) {
			return diag, true
		}
	}
	for _, candidate := range candidates {
		if sameURL(candidate.URL, rawURL) {
			return FromCandidate(candidate), true
		}
	}
	return Diagnostic{URL: rawURL}, false
}

func HasSemantic(diag Diagnostic) bool {
	return Identity(diag) > 0 || Topic(diag) > 0 || Origin(diag) > 0 || Derivative(diag) > 0 ||
		diag.SemanticFamilyIntentMerit > 0 || diag.SemanticFamilyEvidence > 0 || diag.LateInteractionScore > 0
}

func Identity(diag Diagnostic) float64 {
	return max(diag.SemanticFamilyIntentIdentity, diag.SemanticProvenanceIdentity, diag.SemanticOriginAlignment, diag.SemanticOriginSimilarity)
}

func Topic(diag Diagnostic) float64 {
	return max(diag.SemanticFamilyIntentTopic, diag.SemanticProvenanceTopic, diag.SemanticEvidenceSimilarity, diag.LateInteractionScore)
}

func Origin(diag Diagnostic) float64 {
	return max(diag.SemanticFamilyIntentOrigin, diag.SemanticOriginAlignment, diag.SemanticOriginSimilarity, diag.SemanticProvenanceIdentity)
}

func Derivative(diag Diagnostic) float64 {
	return max(diag.SemanticFamilyIntentDerivative, diag.SemanticDerivativeAlignment, diag.SemanticDerivativeSimilarity, diag.SemanticCommunitySimilarity)
}

func Prior(diag Diagnostic) float64 {
	evidence := diag.SemanticFamilyEvidence + min(float64(diag.SemanticFamilyEvidenceStrong), 3)*0.04 +
		min(float64(diag.SemanticFamilyProvenance), 2)*0.05 + min(float64(diag.SemanticFamilyIntentProvenance), 2)*0.04
	return diag.SemanticFamilyIntentMerit*1.25 + Identity(diag) + Origin(diag)*0.75 + evidence*0.70 + Topic(diag)*0.18 +
		diag.LateInteractionScore*0.12 + diag.SemanticEvidenceBoost*0.20 + diag.SemanticAuthorityBoost*0.18 -
		Derivative(diag)*0.70 - diag.SemanticProvenancePenalty*0.50 - diag.SemanticAuthorityPenalty*0.30 - diag.SemanticCommunityPenalty*0.20
}

func HasAnyReason(diag Diagnostic, reasons ...string) bool {
	for _, existing := range diag.Reasons {
		for _, wanted := range reasons {
			if strings.TrimSpace(existing) == wanted {
				return true
			}
		}
	}
	return false
}

func sameURL(left, right string) bool {
	left, right = strings.TrimSpace(left), strings.TrimSpace(right)
	if left == "" || right == "" {
		return left == right
	}
	return left == right || discoverycore.SameCanonicalURL(left, right)
}
