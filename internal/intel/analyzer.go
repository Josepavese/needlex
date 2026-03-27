package intel

import (
	"fmt"
	"strings"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
)

const (
	ReasonAmbiguityTriggered = "NX_AMBIGUITY_TRIGGERED"
	ReasonCoverageTriggered  = "NX_COVERAGE_TRIGGERED"
	ReasonDomainForceLane    = "NX_DOMAIN_FORCE_LANE"
	ReasonExtractorTriggered = "NX_EXTRACTOR_TRIGGERED"
	ReasonFormatterTriggered = "NX_FORMATTER_TRIGGERED"
)

type Input struct {
	Fingerprint string
	Text        string
	HeadingPath []string
	Score       float64
	Confidence  float64
}

type Decision struct {
	Fingerprint      string
	Lane             int
	ReasonCode       string
	AmbiguityScore   float64
	CoverageLoss     float64
	RiskFlags        []string
	ModelInvocations []core.ModelInvocation
	TransformChain   []string
}

type Hints struct {
	ForceLane int
	Profile   string
}

type Summary struct {
	PageType        string
	Difficulty      string
	NoiseLevel      string
	MaxLane         int
	EscalationCount int
	Decisions       map[string]Decision
}

type Analyzer struct {
	cfg config.Config
}

func New(cfg config.Config) Analyzer {
	return Analyzer{cfg: cfg}
}

func (a Analyzer) Analyze(objective string, inputs []Input, hints Hints) Summary {
	decisions := make(map[string]Decision, len(inputs))
	maxLane := 0
	escalations := 0

	for _, input := range inputs {
		coverage := tokenCoverage(input, objective)
		coverageLoss := clamp(1-coverage, 0, 1)
		ambiguity := ambiguityScore(input, coverage)
		lane := 0
		reasonCode := ""
		riskFlags := buildRiskFlags(input, coverageLoss, ambiguity)
		modelInvocations := []core.ModelInvocation{}
		transformChain := []string{"intel:route:v1", "intel:judge:v1"}

		if a.cfg.Runtime.LaneMax >= 1 {
			switch {
			case ambiguity > a.cfg.Policy.ThresholdAmbiguity:
				lane = 1
				reasonCode = ReasonAmbiguityTriggered
			case coverageLoss > a.cfg.Policy.ThresholdCoverage:
				lane = 1
				reasonCode = ReasonCoverageTriggered
			}
		}
		if hints.ForceLane > lane {
			lane = min(hints.ForceLane, a.cfg.Runtime.LaneMax)
			if lane > 0 {
				reasonCode = ReasonDomainForceLane
			}
		}
		if lane < 2 && a.cfg.Runtime.LaneMax >= 2 {
			if ambiguity >= 0.85 || (coverageLoss >= 0.90 && input.Confidence < 0.70) {
				lane = 2
				reasonCode = ReasonExtractorTriggered
			}
		}
		if lane < 3 && a.cfg.Runtime.LaneMax >= 3 {
			if hints.Profile == core.ProfileTiny && (lane >= 2 || ambiguity >= 0.60) {
				lane = 3
				reasonCode = ReasonFormatterTriggered
			}
		}

		if lane > 0 {
			escalations++
			maxLane = max(maxLane, lane)
			modelInvocations = append(modelInvocations,
				core.ModelInvocation{
					Model:     a.cfg.Models.Router,
					Purpose:   "route_policy",
					TokensIn:  0,
					TokensOut: 0,
					LatencyMS: 0,
				},
				core.ModelInvocation{
					Model:     a.cfg.Models.Judge,
					Purpose:   "judge_policy",
					TokensIn:  0,
					TokensOut: 0,
					LatencyMS: 0,
				},
			)
		}
		if lane >= 2 {
			transformChain = append(transformChain, "intel:extract_slm:v1")
		}
		if lane >= 3 {
			transformChain = append(transformChain, "intel:formatter:v1")
		}

		decisions[input.Fingerprint] = Decision{
			Fingerprint:      input.Fingerprint,
			Lane:             lane,
			ReasonCode:       reasonCode,
			AmbiguityScore:   ambiguity,
			CoverageLoss:     coverageLoss,
			RiskFlags:        riskFlags,
			ModelInvocations: modelInvocations,
			TransformChain:   transformChain,
		}
	}

	return Summary{
		PageType:        classifyPageType(inputs),
		Difficulty:      classifyDifficulty(decisions),
		NoiseLevel:      classifyNoise(inputs),
		MaxLane:         maxLane,
		EscalationCount: escalations,
		Decisions:       decisions,
	}
}

func tokenCoverage(input Input, objective string) float64 {
	tokens := objectiveTokens(objective)
	if len(tokens) == 0 {
		return 1
	}

	haystack := strings.ToLower(strings.Join(input.HeadingPath, " ") + " " + input.Text)
	matches := 0
	for _, token := range tokens {
		if strings.Contains(haystack, token) {
			matches++
		}
	}
	return clamp(float64(matches)/float64(len(tokens)), 0, 1)
}

func ambiguityScore(input Input, coverage float64) float64 {
	score := 0.0
	if input.Confidence < 0.78 {
		score += 0.30
	}
	if coverage < 0.5 {
		score += 0.28
	}
	if len(input.HeadingPath) == 0 {
		score += 0.10
	}
	if len(strings.TrimSpace(input.Text)) < 48 {
		score += 0.12
	}
	if input.Score < 0.75 {
		score += 0.10
	}
	return clamp(score, 0, 1)
}

func buildRiskFlags(input Input, coverageLoss, ambiguity float64) []string {
	flags := []string{}
	if coverageLoss > 0.50 {
		flags = append(flags, "coverage_gap")
	}
	if ambiguity > 0.50 {
		flags = append(flags, "high_ambiguity")
	}
	if len(input.HeadingPath) == 0 {
		flags = append(flags, "missing_heading_context")
	}
	if len(strings.TrimSpace(input.Text)) < 48 {
		flags = append(flags, "short_segment")
	}
	return flags
}

func classifyPageType(inputs []Input) string {
	joined := ""
	for _, input := range inputs {
		joined += " " + strings.Join(input.HeadingPath, " ") + " " + input.Text
	}
	haystack := strings.ToLower(joined)
	switch {
	case strings.Contains(haystack, "forum") || strings.Contains(haystack, "thread"):
		return "forum"
	case strings.Contains(haystack, "api") || strings.Contains(haystack, "docs"):
		return "docs"
	default:
		return "article"
	}
}

func classifyDifficulty(decisions map[string]Decision) string {
	if len(decisions) == 0 {
		return "low"
	}
	total := 0.0
	for _, decision := range decisions {
		total += decision.AmbiguityScore
	}
	avg := total / float64(len(decisions))
	switch {
	case avg >= 0.55:
		return "high"
	case avg >= 0.30:
		return "medium"
	default:
		return "low"
	}
}

func classifyNoise(inputs []Input) string {
	if len(inputs) == 0 {
		return "low"
	}
	shortOrUntitled := 0
	for _, input := range inputs {
		if len(input.HeadingPath) == 0 || len(strings.TrimSpace(input.Text)) < 48 {
			shortOrUntitled++
		}
	}
	ratio := float64(shortOrUntitled) / float64(len(inputs))
	switch {
	case ratio >= 0.5:
		return "high"
	case ratio >= 0.25:
		return "medium"
	default:
		return "low"
	}
}

func objectiveTokens(input string) []string {
	fields := strings.Fields(strings.ToLower(input))
	out := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, field := range fields {
		token := strings.Trim(field, ".,:;!?()[]{}\"'")
		if len(token) < 3 {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	return out
}

func clamp(value, lower, upper float64) float64 {
	switch {
	case value < lower:
		return lower
	case value > upper:
		return upper
	default:
		return value
	}
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func (d Decision) Metadata() map[string]string {
	return map[string]string{
		"fingerprint":      d.Fingerprint,
		"lane":             fmt.Sprintf("%d", d.Lane),
		"reason_code":      d.ReasonCode,
		"ambiguity_score":  fmt.Sprintf("%.2f", d.AmbiguityScore),
		"coverage_loss":    fmt.Sprintf("%.2f", d.CoverageLoss),
		"risk_flag_count":  fmt.Sprintf("%d", len(d.RiskFlags)),
		"invocation_count": fmt.Sprintf("%d", len(d.ModelInvocations)),
	}
}
