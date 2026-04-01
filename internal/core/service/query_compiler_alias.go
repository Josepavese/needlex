package service

import "github.com/josepavese/needlex/internal/core/queryplan"

type (
	QueryCompiler     = queryplan.QueryCompiler
	QueryPlanDecision = queryplan.QueryPlanDecision
)

const (
	QueryCompilerVersion = queryplan.QueryCompilerVersion

	QueryPlanReasonSeedPresent            = queryplan.QueryPlanReasonSeedPresent
	QueryPlanReasonSeedMissing            = queryplan.QueryPlanReasonSeedMissing
	QueryPlanReasonDefaultMode            = queryplan.QueryPlanReasonDefaultMode
	QueryPlanReasonUserMode               = queryplan.QueryPlanReasonUserMode
	QueryPlanReasonSeedlessDefaultWeb     = queryplan.QueryPlanReasonSeedlessDefaultWeb
	QueryPlanReasonBudgetApplied          = queryplan.QueryPlanReasonBudgetApplied
	QueryPlanReasonSelection              = queryplan.QueryPlanReasonSelection
	QueryPlanReasonWebIR                  = queryplan.QueryPlanReasonWebIR
	QueryPlanReasonWebIRSelection         = queryplan.QueryPlanReasonWebIRSelection
	QueryPlanReasonDomainHintEvidence     = queryplan.QueryPlanReasonDomainHintEvidence
	QueryPlanReasonGraphEvidence          = queryplan.QueryPlanReasonGraphEvidence
	QueryPlanReasonWebBootstrapFallback   = queryplan.QueryPlanReasonWebBootstrapFallback
	QueryPlanReasonLowCandidateSetRisk    = queryplan.QueryPlanReasonLowCandidateSetRisk
	QueryPlanReasonAmbiguousSelectionRisk = queryplan.QueryPlanReasonAmbiguousSelectionRisk
	QueryPlanReasonStableRegionBias       = queryplan.QueryPlanReasonStableRegionBias
	QueryPlanReasonNoveltyBias            = queryplan.QueryPlanReasonNoveltyBias
	QueryPlanReasonDeltaRisk              = queryplan.QueryPlanReasonDeltaRisk
	QueryPlanReasonQualityLatencyMode     = queryplan.QueryPlanReasonQualityLatencyMode
	QueryPlanReasonLanePolicy             = queryplan.QueryPlanReasonLanePolicy
	QueryPlanReasonExecutionAligned       = queryplan.QueryPlanReasonExecutionAligned
	QueryPlanReasonExecutionDrift         = queryplan.QueryPlanReasonExecutionDrift
	QueryPlanReasonPlanDiffObserved       = queryplan.QueryPlanReasonPlanDiffObserved
	QueryPlanReasonRuntimeEffectsClean    = queryplan.QueryPlanReasonRuntimeEffectsClean
	QueryPlanReasonRuntimeEffectsDetected = queryplan.QueryPlanReasonRuntimeEffectsDetected
	QueryPlanReasonIntentBoundary         = queryplan.QueryPlanReasonIntentBoundary
	QueryPlanReasonExecutionBoundary      = queryplan.QueryPlanReasonExecutionBoundary
	QueryPlanReasonBudgetOutcomeOK        = queryplan.QueryPlanReasonBudgetOutcomeOK
	QueryPlanReasonBudgetOutcomeExceeded  = queryplan.QueryPlanReasonBudgetOutcomeExceeded
)
