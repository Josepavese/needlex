package service

import "github.com/josepavese/needlex/internal/core/queryplan"

type (
	QueryCompiler     = queryplan.QueryCompiler
	QueryPlanDecision = queryplan.QueryPlanDecision
)

const (
	QueryCompilerVersion = queryplan.QueryCompilerVersion

	QueryPlanReasonSeedPresent          = queryplan.QueryPlanReasonSeedPresent
	QueryPlanReasonExperimentalWebOptIn = queryplan.QueryPlanReasonExperimentalWebOptIn
	QueryPlanReasonSelection            = queryplan.QueryPlanReasonSelection
	QueryPlanReasonWebIR                = queryplan.QueryPlanReasonWebIR
	QueryPlanReasonWebIRSelection       = queryplan.QueryPlanReasonWebIRSelection
	QueryPlanReasonGraphEvidence        = queryplan.QueryPlanReasonGraphEvidence
	QueryPlanReasonWebBootstrapFallback = queryplan.QueryPlanReasonWebBootstrapFallback
	QueryPlanReasonLowCandidateSetRisk  = queryplan.QueryPlanReasonLowCandidateSetRisk
	QueryPlanReasonStableRegionBias     = queryplan.QueryPlanReasonStableRegionBias
	QueryPlanReasonNoveltyBias          = queryplan.QueryPlanReasonNoveltyBias
	QueryPlanReasonDeltaRisk            = queryplan.QueryPlanReasonDeltaRisk
	QueryPlanReasonQualityLatencyMode   = queryplan.QueryPlanReasonQualityLatencyMode
	QueryPlanReasonLanePolicy           = queryplan.QueryPlanReasonLanePolicy
	QueryPlanReasonExecutionAligned     = queryplan.QueryPlanReasonExecutionAligned
	QueryPlanReasonExecutionDrift       = queryplan.QueryPlanReasonExecutionDrift
	QueryPlanReasonPlanDiffObserved     = queryplan.QueryPlanReasonPlanDiffObserved

	QueryPlanReasonRuntimeEffectsClean    = queryplan.QueryPlanReasonRuntimeEffectsClean
	QueryPlanReasonRuntimeEffectsDetected = queryplan.QueryPlanReasonRuntimeEffectsDetected
	QueryPlanReasonIntentBoundary         = queryplan.QueryPlanReasonIntentBoundary
	QueryPlanReasonExecutionBoundary      = queryplan.QueryPlanReasonExecutionBoundary
	QueryPlanReasonBudgetOutcomeOK        = queryplan.QueryPlanReasonBudgetOutcomeOK
	QueryPlanReasonBudgetOutcomeExceeded  = queryplan.QueryPlanReasonBudgetOutcomeExceeded
)
