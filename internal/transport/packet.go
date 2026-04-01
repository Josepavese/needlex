package transport

import (
	"strings"

	"github.com/josepavese/needlex/internal/core"
	coreservice "github.com/josepavese/needlex/internal/core/service"
)

type compactChunk struct {
	Text           string   `json:"text"`
	HeadingPath    []string `json:"heading_path,omitempty"`
	SourceURL      string   `json:"source_url,omitempty"`
	SourceSelector string   `json:"source_selector,omitempty"`
	ProofRef       string   `json:"proof_ref,omitempty"`
}

type compactCandidate struct {
	URL    string   `json:"url"`
	Label  string   `json:"label,omitempty"`
	Reason []string `json:"reason,omitempty"`
}

type compactWebIRSummary struct {
	NodeCount         int     `json:"node_count"`
	HeadingRatio      float64 `json:"heading_ratio,omitempty"`
	ShortTextRatio    float64 `json:"short_text_ratio,omitempty"`
	EmbeddedNodeCount int     `json:"embedded_node_count,omitempty"`
	SubstrateClass    string  `json:"substrate_class,omitempty"`
}

type compactSignals struct {
	Confidence     float64 `json:"confidence,omitempty"`
	SubstrateClass string  `json:"substrate_class,omitempty"`
}

type compactUncertainty struct {
	Level   string   `json:"level"`
	Reasons []string `json:"reasons,omitempty"`
}

type compactReadOutput struct {
	Kind         string              `json:"kind"`
	URL          string              `json:"url"`
	Title        string              `json:"title,omitempty"`
	Summary      string              `json:"summary,omitempty"`
	Uncertainty  compactUncertainty  `json:"uncertainty"`
	Profile      string              `json:"profile,omitempty"`
	TraceID      string              `json:"trace_id,omitempty"`
	Outline      []string            `json:"outline,omitempty"`
	Chunks       []compactChunk      `json:"chunks"`
	Links        []string            `json:"links,omitempty"`
	Signals      compactSignals      `json:"signals,omitempty"`
	WebIRSummary compactWebIRSummary `json:"web_ir_summary"`
	CostReport   core.CostReport     `json:"cost_report"`
}

type compactQueryOutput struct {
	Kind         string              `json:"kind"`
	Goal         string              `json:"goal"`
	SeedURL      string              `json:"seed_url,omitempty"`
	SelectedURL  string              `json:"selected_url"`
	Summary      string              `json:"summary,omitempty"`
	Uncertainty  compactUncertainty  `json:"uncertainty"`
	SelectionWhy []string            `json:"selection_why,omitempty"`
	Provider     string              `json:"provider,omitempty"`
	Profile      string              `json:"profile,omitempty"`
	TraceID      string              `json:"trace_id,omitempty"`
	Outline      []string            `json:"outline,omitempty"`
	Chunks       []compactChunk      `json:"chunks"`
	Candidates   []compactCandidate  `json:"candidates,omitempty"`
	Signals      compactSignals      `json:"signals,omitempty"`
	WebIRSummary compactWebIRSummary `json:"web_ir_summary"`
	CostReport   core.CostReport     `json:"cost_report"`
}

type compactCrawlDocument struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

type compactCrawlOutput struct {
	Kind       string                   `json:"kind"`
	Summary    coreservice.CrawlSummary `json:"summary"`
	Documents  []compactCrawlDocument   `json:"documents"`
	StoredRuns int                      `json:"stored_runs"`
}

func compactReadResponse(resp coreservice.ReadResponse) compactReadOutput {
	selectedChunks := compactChunkSelection(resp.AgentContext.Chunks)
	return compactReadOutput{
		Kind:         "page_read",
		URL:          firstNonEmptyTrimmed(resp.Document.FinalURL, resp.Document.URL),
		Title:        cleanDisplayString(resp.Document.Title),
		Summary:      deriveSummary(resp.AgentContext.Title, selectedChunks),
		Uncertainty:  deriveReadUncertainty(selectedChunks, resp.WebIR),
		Profile:      resp.ResultPack.Profile,
		TraceID:      resp.Trace.TraceID,
		Outline:      cleanDisplayPath(resp.ResultPack.Outline),
		Chunks:       compactChunks(selectedChunks),
		Links:        append([]string{}, resp.ResultPack.Links...),
		Signals:      compactSignals{Confidence: topChunkConfidence(selectedChunks), SubstrateClass: resp.WebIR.Signals.SubstrateClass},
		WebIRSummary: compactWebIR(resp.WebIR),
		CostReport:   resp.ResultPack.CostReport,
	}
}

func compactQueryResponse(resp coreservice.QueryResponse) compactQueryOutput {
	selectedChunks := compactChunkSelection(resp.AgentContext.Chunks)
	return compactQueryOutput{
		Kind:         "goal_query",
		Goal:         resp.Plan.Goal,
		SeedURL:      resp.Plan.SeedURL,
		SelectedURL:  firstNonEmptyTrimmed(resp.Document.FinalURL, resp.Plan.SelectedURL),
		Summary:      deriveSummary(resp.AgentContext.Title, selectedChunks),
		Uncertainty:  deriveQueryUncertainty(selectedChunks, resp.AgentContext.Candidates, resp.Plan.SelectedURL, resp.WebIR),
		SelectionWhy: topCandidateReasons(resp.Plan.SelectedURL, resp.AgentContext.Candidates),
		Provider:     resp.Plan.DiscoveryProvider,
		Profile:      resp.ResultPack.Profile,
		TraceID:      resp.TraceID,
		Outline:      cleanDisplayPath(resp.ResultPack.Outline),
		Chunks:       compactChunks(selectedChunks),
		Candidates:   compactCandidates(resp.AgentContext.Candidates),
		Signals:      compactSignals{Confidence: topChunkConfidence(selectedChunks), SubstrateClass: resp.WebIR.Signals.SubstrateClass},
		WebIRSummary: compactWebIR(resp.WebIR),
		CostReport:   resp.CostReport,
	}
}

func compactCrawlResponse(resp coreservice.CrawlResponse, artifacts crawlArtifacts) compactCrawlOutput {
	documents := make([]compactCrawlDocument, 0, len(resp.Documents))
	for _, doc := range resp.Documents {
		documents = append(documents, compactCrawlDocument{
			URL:   firstNonEmptyTrimmed(doc.FinalURL, doc.URL),
			Title: strings.TrimSpace(doc.Title),
		})
	}
	return compactCrawlOutput{
		Kind:       "bounded_crawl",
		Summary:    resp.Summary,
		Documents:  documents,
		StoredRuns: artifacts.StoredRuns,
	}
}

func compactChunks(chunks []coreservice.AgentChunk) []compactChunk {
	out := make([]compactChunk, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, compactChunk{
			Text:           cleanDisplayString(chunk.Text),
			HeadingPath:    cleanDisplayPath(chunk.HeadingPath),
			SourceURL:      chunk.SourceURL,
			SourceSelector: chunk.SourceSelector,
			ProofRef:       chunk.ProofRef,
		})
	}
	return out
}

func compactCandidates(candidates []coreservice.AgentCandidate) []compactCandidate {
	out := make([]compactCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, compactCandidate{
			URL:    candidate.URL,
			Label:  candidate.Label,
			Reason: append([]string{}, candidate.Reason...),
		})
	}
	return out
}

func compactWebIR(webIR core.WebIR) compactWebIRSummary {
	return compactWebIRSummary{
		NodeCount:         webIR.NodeCount,
		HeadingRatio:      webIR.Signals.HeadingRatio,
		ShortTextRatio:    webIR.Signals.ShortTextRatio,
		EmbeddedNodeCount: webIR.Signals.EmbeddedNodeCount,
		SubstrateClass:    webIR.Signals.SubstrateClass,
	}
}
