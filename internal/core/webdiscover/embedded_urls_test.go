package webdiscover

import (
	"testing"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/pipeline"
)

func TestExtractEmbeddedURLCandidatesKeepsSameDepthTransportReferenceSubordinate(t *testing.T) {
	source := discoverycore.Candidate{
		URL:   "https://code.example/project/repository",
		Label: "Repository",
	}
	dom := pipeline.SimplifiedDOM{
		Title: "Repository",
		Nodes: []pipeline.SimplifiedNode{{Text: "Clone from https://code.example/project/repository.git"}},
	}
	got := ExtractEmbeddedURLCandidates("", source, source.URL, "", dom, nil)
	if len(got) != 1 {
		t.Fatalf("expected embedded candidate, got %#v", got)
	}
	if got[0].Score >= 1.0 {
		t.Fatalf("expected same-depth transport reference to remain subordinate, score=%f candidate=%#v", got[0].Score, got[0])
	}
	if !hasReason(got[0].Reason, "embedded_url_untyped_resource") {
		t.Fatalf("expected untyped resource reason, got %#v", got[0].Reason)
	}
}

func TestExtractEmbeddedURLCandidatesBoostsDeepSameFamilyResource(t *testing.T) {
	source := discoverycore.Candidate{
		URL:   "https://docs.example/guide",
		Label: "Guide",
	}
	dom := pipeline.SimplifiedDOM{
		Title: "Guide",
		Nodes: []pipeline.SimplifiedNode{{Text: "Use https://docs.example/api/native/v1/resource for machine access."}},
	}
	got := ExtractEmbeddedURLCandidates("", source, source.URL, "", dom, nil)
	if len(got) != 1 {
		t.Fatalf("expected embedded candidate, got %#v", got)
	}
	if got[0].Score <= 1.2 {
		t.Fatalf("expected deep same-family resource to receive contextual boost, score=%f candidate=%#v", got[0].Score, got[0])
	}
	if !hasReason(got[0].Reason, "embedded_url_deep_resource") {
		t.Fatalf("expected deep resource reason, got %#v", got[0].Reason)
	}
}

func hasReason(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
