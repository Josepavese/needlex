package semanticevidence

import (
	"context"
	"strings"
	"testing"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/intel"
)

type staticSemantic struct {
	scores          map[string]float64
	objectiveScores map[string]map[string]float64
	calls           int
	seen            []intel.SemanticCandidate
}

func (s *staticSemantic) Align(context.Context, string, []intel.SemanticCandidate) (intel.SemanticAlignment, error) {
	return intel.SemanticAlignment{}, nil
}

func (s *staticSemantic) Score(_ context.Context, objective string, candidates []intel.SemanticCandidate) ([]intel.SemanticScore, error) {
	s.calls++
	s.seen = append([]intel.SemanticCandidate{}, candidates...)
	out := make([]intel.SemanticScore, 0, len(candidates))
	scores := s.scores
	for prefix, objectiveScores := range s.objectiveScores {
		if strings.HasPrefix(objective, prefix) {
			scores = objectiveScores
			break
		}
	}
	for _, candidate := range candidates {
		out = append(out, intel.SemanticScore{ID: candidate.ID, Similarity: scores[candidate.ID]})
	}
	return out, nil
}

func TestRerankPromotesCandidateWithSemanticPageEvidence(t *testing.T) {
	semantic := &staticSemantic{scores: map[string]float64{
		"https://noise.example/":    0.05,
		"https://official.example/": 0.88,
	}}
	candidates := []discoverycore.Candidate{
		{URL: "https://noise.example/", Score: 1.20, Metadata: map[string]string{"page_title": "Unrelated evidence surface"}},
		{URL: "https://official.example/", Score: 0.90, Metadata: map[string]string{"page_title": "Official reference", "web_ir_context": "Authoritative evidence bundle"}},
	}

	got := Reranker{Semantic: semantic, Config: DefaultConfig()}.Rerank(context.Background(), "goal", candidates)
	if got[0].URL != "https://official.example/" {
		t.Fatalf("expected semantic evidence to promote official candidate, got %#v", got)
	}
	if !containsReason(got[0].Reason, ReasonSemanticEvidenceProbe) {
		t.Fatalf("expected semantic evidence reason, got %#v", got[0].Reason)
	}
	if got[0].Metadata["semantic_evidence_similarity"] == "" || got[0].Metadata["semantic_evidence_boost"] == "" {
		t.Fatalf("expected semantic evidence metadata, got %#v", got[0].Metadata)
	}
}

func TestRerankSkipsWhenCandidatesHaveNoPageEvidence(t *testing.T) {
	semantic := &staticSemantic{scores: map[string]float64{"https://b.example/": 0.99}}
	candidates := []discoverycore.Candidate{
		{URL: "https://a.example/", Score: 1.20},
		{URL: "https://b.example/", Score: 0.90},
	}

	got := Reranker{Semantic: semantic, Config: DefaultConfig()}.Rerank(context.Background(), "goal", candidates)
	if semantic.calls != 0 {
		t.Fatalf("expected no semantic call without page evidence, got %d", semantic.calls)
	}
	if got[0].URL != "https://a.example/" {
		t.Fatalf("expected original order without evidence, got %#v", got)
	}
}

func TestRerankCanChallengeLeaderWithoutEvidence(t *testing.T) {
	semantic := &staticSemantic{scores: map[string]float64{"https://official.example/": 0.90}}
	candidates := []discoverycore.Candidate{
		{URL: "https://noise.example/", Score: 1.20},
		{URL: "https://official.example/", Score: 0.90, Metadata: map[string]string{"web_ir_context": "Authoritative evidence bundle"}},
	}

	got := Reranker{Semantic: semantic, Config: DefaultConfig()}.Rerank(context.Background(), "goal", candidates)
	if got[0].URL != "https://official.example/" {
		t.Fatalf("expected evidenced runner-up to challenge leader without evidence, got %#v", got)
	}
	if semantic.calls != 4 {
		t.Fatalf("expected semantic evidence and authority calls, got %d", semantic.calls)
	}
}

func TestRerankPromotesSemanticOriginOverDerivativeSurface(t *testing.T) {
	semantic := &staticSemantic{
		scores: map[string]float64{
			"https://mirror.example/":   0.55,
			"https://official.example/": 0.54,
		},
		objectiveScores: map[string]map[string]float64{
			"Primary custodian": {
				"https://mirror.example/":   0.19,
				"https://official.example/": 0.42,
			},
			"Derivative secondary": {
				"https://mirror.example/":   0.39,
				"https://official.example/": 0.18,
			},
		},
	}
	candidates := []discoverycore.Candidate{
		{URL: "https://mirror.example/", Score: 1.20, Metadata: map[string]string{"page_title": "External tutorial summary"}},
		{URL: "https://official.example/", Score: 1.00, Metadata: map[string]string{"page_title": "Maintained official reference"}},
	}

	got := Reranker{Semantic: semantic, Config: DefaultConfig()}.Rerank(context.Background(), "goal", candidates)
	if got[0].URL != "https://official.example/" {
		t.Fatalf("expected semantic origin probe to promote official candidate, got %#v", got)
	}
	if !containsReason(got[0].Reason, ReasonSemanticOriginProbe) || !containsReason(got[1].Reason, ReasonSemanticDerivative) {
		t.Fatalf("expected origin and derivative reasons, got %#v / %#v", got[0].Reason, got[1].Reason)
	}
}

func TestEvidenceTextExcludesURLIdentitySurface(t *testing.T) {
	candidate := discoverycore.Candidate{
		URL: "https://literal.example/specific/path",
		Metadata: map[string]string{
			"page_title":        "Context title",
			"host_root_context": "Maintained root evidence",
			"web_ir_context":    "Meaningful page evidence",
		},
	}

	text := EvidenceText(candidate)
	if strings.Contains(text, "literal.example") || strings.Contains(text, "/specific/path") {
		t.Fatalf("expected evidence text to exclude URL identity surface, got %q", text)
	}
	if !strings.Contains(text, "Context title") || !strings.Contains(text, "Maintained root evidence") || !strings.Contains(text, "Meaningful page evidence") {
		t.Fatalf("expected page evidence text, got %q", text)
	}
}

func containsReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
