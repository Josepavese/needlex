package service

import (
	"strings"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/proof"
)

type AgentChunk struct {
	ID               string   `json:"id"`
	Text             string   `json:"text"`
	HeadingPath      []string `json:"heading_path,omitempty"`
	ContextAlignment float64  `json:"context_alignment,omitempty"`
	Score            float64  `json:"score"`
	Confidence       float64  `json:"confidence"`
	SourceURL        string   `json:"source_url,omitempty"`
	SourceTitle      string   `json:"source_title,omitempty"`
	SourceSelector   string   `json:"source_selector,omitempty"`
	ProofRef         string   `json:"proof_ref,omitempty"`
}

type AgentCandidate struct {
	URL    string   `json:"url"`
	Label  string   `json:"label,omitempty"`
	Score  float64  `json:"score"`
	Reason []string `json:"reason,omitempty"`
}

type AgentContext struct {
	URL           string           `json:"url"`
	Title         string           `json:"title,omitempty"`
	Links         []string         `json:"links,omitempty"`
	CandidateURLs []string         `json:"candidate_urls,omitempty"`
	Candidates    []AgentCandidate `json:"candidates,omitempty"`
	Chunks        []AgentChunk     `json:"chunks"`
}

func buildAgentContext(document core.Document, pack core.ResultPack, proofs []proof.ProofRecord, candidates []DiscoverCandidate) AgentContext {
	proofByChunk := make(map[string]proof.ProofRecord, len(proofs))
	for _, record := range proofs {
		proofByChunk[record.Proof.ChunkID] = record
	}
	sourceByChunk := make(map[string]core.SourceRef, len(pack.Chunks))
	for _, source := range pack.Sources {
		for _, chunkID := range source.ChunkIDs {
			sourceByChunk[chunkID] = source
		}
	}
	chunks := make([]AgentChunk, 0, len(pack.Chunks))
	for _, chunk := range pack.Chunks {
		record := proofByChunk[chunk.ID]
		source := sourceByChunk[chunk.ID]
		chunks = append(chunks, AgentChunk{
			ID:               chunk.ID,
			Text:             chunk.Text,
			HeadingPath:      append([]string{}, chunk.HeadingPath...),
			ContextAlignment: chunk.ContextAlignment,
			Score:            chunk.Score,
			Confidence:       chunk.Confidence,
			SourceURL:        source.URL,
			SourceTitle:      source.Title,
			SourceSelector:   record.Proof.SourceSpan.Selector,
			ProofRef:         record.ID,
		})
	}
	agentCandidates := make([]AgentCandidate, 0, len(candidates))
	candidateURLs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidateURLs = append(candidateURLs, candidate.URL)
		agentCandidates = append(agentCandidates, AgentCandidate{
			URL:    candidate.URL,
			Label:  candidate.Label,
			Score:  candidate.Score,
			Reason: append([]string{}, candidate.Reason...),
		})
	}
	return AgentContext{
		URL:           firstNonEmptyString(document.FinalURL, document.URL),
		Title:         strings.TrimSpace(document.Title),
		Links:         append([]string{}, pack.Links...),
		CandidateURLs: candidateURLs,
		Candidates:    agentCandidates,
		Chunks:        chunks,
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
