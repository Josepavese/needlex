package intel

import (
	"fmt"
	"strings"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
)

const (
	ReasonAmbiguityTriggered   = "NX_AMBIGUITY_TRIGGERED"
	ReasonCoverageTriggered    = "NX_COVERAGE_TRIGGERED"
	ReasonDomainForceLane      = "NX_DOMAIN_FORCE_LANE"
	ReasonEmbeddedWorthiness   = "NX_EMBEDDED_WORTHINESS_TRIGGERED"
	ReasonExtractorTriggered   = "NX_EXTRACTOR_TRIGGERED"
	ReasonFormatterTriggered   = "NX_FORMATTER_TRIGGERED"
	ReasonDriftTriggered       = "NX_DRIFT_TRIGGERED"
	ReasonGraphTriggered       = "NX_GRAPH_TRIGGERED"
	ReasonCompressionTriggered = "NX_COMPRESSION_TRIGGERED"
)

type Input struct {
	Fingerprint      string
	Text             string
	HeadingPath      []string
	ContextAlignment float64
	Score            float64
	Confidence       float64
}

type Decision struct {
	Fingerprint      string
	Lane             int
	ReasonCode       string
	AmbiguityScore   float64
	CoverageLoss     float64
	RiskFlags        []string
	TaskRoutes       []TaskRoute
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

func (a Analyzer) Analyze(inputs []Input, hints Hints) Summary {
	decisions, maxLane, escalations := a.analyzeInputs(inputs, hints)
	return Summary{
		PageType:        classifyPageType(inputs),
		Difficulty:      classifyDifficulty(decisions),
		NoiseLevel:      classifyNoise(inputs),
		MaxLane:         maxLane,
		EscalationCount: escalations,
		Decisions:       decisions,
	}
}

func (a Analyzer) analyzeInputs(inputs []Input, hints Hints) (map[string]Decision, int, int) {
	decisions := make(map[string]Decision, len(inputs))
	maxLane := 0
	escalations := 0

	for _, input := range inputs {
		decision := a.decisionForInput(input, hints)
		if decision.Lane > 0 {
			escalations++
			maxLane = max(maxLane, decision.Lane)
		}
		decisions[input.Fingerprint] = decision
	}
	return decisions, maxLane, escalations
}

func (a Analyzer) decisionForInput(input Input, hints Hints) Decision {
	coverage := clamp(input.ContextAlignment, 0, 1)
	coverageLoss := clamp(1-coverage, 0, 1)
	ambiguity := ambiguityScore(input, coverage)
	lane, reasonCode := a.resolveLane(ambiguity, coverageLoss, input.Confidence, hints)
	return Decision{
		Fingerprint:      input.Fingerprint,
		Lane:             lane,
		ReasonCode:       reasonCode,
		AmbiguityScore:   ambiguity,
		CoverageLoss:     coverageLoss,
		RiskFlags:        buildRiskFlags(input, coverageLoss, ambiguity),
		ModelInvocations: routeInvocations(a.cfg, lane),
		TransformChain:   transformChainForLane(lane),
	}
}

func (a Analyzer) resolveLane(ambiguity, coverageLoss, confidence float64, hints Hints) (int, string) {
	lane, reasonCode := baseLaneDecision(a.cfg, ambiguity, coverageLoss)
	if hints.ForceLane > lane {
		lane = min(hints.ForceLane, a.cfg.Runtime.LaneMax)
		if lane > 0 {
			reasonCode = ReasonDomainForceLane
		}
	}
	if lane < 2 && a.cfg.Runtime.LaneMax >= 2 && shouldExtract(ambiguity, coverageLoss, confidence) {
		lane, reasonCode = 2, ReasonExtractorTriggered
	}
	if lane < 3 && a.cfg.Runtime.LaneMax >= 3 && shouldFormat(hints.Profile, lane, ambiguity) {
		lane, reasonCode = 3, ReasonFormatterTriggered
	}
	return lane, reasonCode
}

func baseLaneDecision(cfg config.Config, ambiguity, coverageLoss float64) (int, string) {
	if cfg.Runtime.LaneMax < 1 {
		return 0, ""
	}
	switch {
	case ambiguity > cfg.Policy.ThresholdAmbiguity:
		return 1, ReasonAmbiguityTriggered
	case coverageLoss > cfg.Policy.ThresholdCoverage:
		return 1, ReasonCoverageTriggered
	default:
		return 0, ""
	}
}

func shouldExtract(ambiguity, coverageLoss, confidence float64) bool {
	return ambiguity >= 0.85 || (coverageLoss >= 0.90 && confidence < 0.70)
}

func shouldFormat(profile string, lane int, ambiguity float64) bool {
	return profile == core.ProfileTiny && (lane >= 2 || ambiguity >= 0.60)
}

func routeInvocations(cfg config.Config, lane int) []core.ModelInvocation {
	if lane <= 0 {
		return nil
	}
	return []core.ModelInvocation{
		{Model: cfg.Models.Router, Purpose: "route_policy"},
		{Model: cfg.Models.Judge, Purpose: "judge_policy"},
	}
}

func transformChainForLane(lane int) []string {
	chain := []string{"intel:route:v1", "intel:judge:v1"}
	if lane >= 2 {
		chain = append(chain, "intel:extract_slm:v1")
	}
	if lane >= 3 {
		chain = append(chain, "intel:formatter:v1")
	}
	return chain
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
	taskNames := make([]string, 0, len(d.TaskRoutes))
	for _, route := range d.TaskRoutes {
		taskNames = append(taskNames, route.Task)
	}
	out := map[string]string{
		"fingerprint":      d.Fingerprint,
		"lane":             fmt.Sprintf("%d", d.Lane),
		"reason_code":      d.ReasonCode,
		"ambiguity_score":  fmt.Sprintf("%.2f", d.AmbiguityScore),
		"coverage_loss":    fmt.Sprintf("%.2f", d.CoverageLoss),
		"risk_flag_count":  fmt.Sprintf("%d", len(d.RiskFlags)),
		"task_route_count": fmt.Sprintf("%d", len(d.TaskRoutes)),
		"invocation_count": fmt.Sprintf("%d", len(d.ModelInvocations)),
	}
	if len(taskNames) > 0 {
		out["task_names"] = strings.Join(taskNames, ",")
	}
	return out
}
