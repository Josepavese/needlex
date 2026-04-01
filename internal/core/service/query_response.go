package service

import (
	"fmt"

	"github.com/josepavese/needlex/internal/core/queryplan"
	"github.com/josepavese/needlex/internal/proof"
)

func finalizeQueryResponse(plan QueryPlan, baseCompiler QueryCompiler, candidates []DiscoverCandidate, readResp ReadResponse) (QueryResponse, error) {
	readResp.ResultPack.Query = plan.Goal
	resp := QueryResponse{
		Plan:         plan,
		Document:     readResp.Document,
		WebIR:        readResp.WebIR,
		ResultPack:   readResp.ResultPack,
		AgentContext: buildAgentContext(readResp.Document, readResp.ResultPack, readResp.ProofRecords, candidates),
		ProofRefs:    append([]string{}, readResp.ResultPack.ProofRefs...),
		ProofRecords: append([]proof.ProofRecord{}, readResp.ProofRecords...),
		Trace:        readResp.Trace,
		TraceID:      readResp.Trace.TraceID,
		CostReport:   readResp.ResultPack.CostReport,
	}
	resp.Plan.Compiler = queryplan.AnnotateQueryCompilerWithWebIR(
		resp.Plan.Compiler,
		resp.WebIR.NodeCount,
		resp.WebIR.Signals.EmbeddedNodeCount,
		resp.WebIR.Signals.HeadingRatio,
		resp.WebIR.Signals.ShortTextRatio,
	)
	resp.Plan.Compiler = queryplan.AnnotateQueryCompilerWithExecution(resp.Plan.Compiler, resp.Plan.SelectedURL, resp.Document.FinalURL, maxLane(resp.ResultPack.CostReport.LanePath))
	resp.Plan.Compiler = queryplan.AnnotateQueryCompilerWithBudgetOutcome(resp.Plan.Compiler, resp.Plan.Budget.MaxLatencyMS, resp.ResultPack.CostReport.LatencyMS, resp.Plan.LaneMax, maxLane(resp.ResultPack.CostReport.LanePath))
	escalations, budgetWarnings, runtimeErrors := proof.EffectCounts(resp.Trace)
	resp.Plan.Compiler = queryplan.AnnotateQueryCompilerWithRuntimeEffects(resp.Plan.Compiler, escalations, budgetWarnings, runtimeErrors)
	resp.Plan.Compiler = queryplan.AnnotateQueryCompilerWithExecutionBoundary(resp.Plan.Compiler)
	resp.Plan.Compiler = queryplan.AnnotateQueryCompilerWithPlanDiff(baseCompiler, resp.Plan.Compiler)
	return resp, resp.Validate()
}

func (r QueryResponse) Validate() error {
	if r.Plan.Goal == "" {
		return fmt.Errorf("query response plan.goal must not be empty")
	}
	if r.Plan.SelectedURL == "" {
		return fmt.Errorf("query response plan.selected_url must not be empty")
	}
	if err := r.Plan.Compiler.Validate(); err != nil {
		return fmt.Errorf("query response plan.compiler: %w", err)
	}
	if err := r.Document.Validate(); err != nil {
		return err
	}
	if err := r.WebIR.Validate(); err != nil {
		return err
	}
	if err := r.ResultPack.Validate(); err != nil {
		return err
	}
	for i, record := range r.ProofRecords {
		if err := record.Validate(); err != nil {
			return fmt.Errorf("query response proof_records[%d]: %w", i, err)
		}
	}
	if err := r.Trace.Validate(); err != nil {
		return err
	}
	if r.TraceID == "" {
		return fmt.Errorf("query response trace_id must not be empty")
	}
	if err := r.CostReport.Validate(); err != nil {
		return err
	}
	return nil
}
