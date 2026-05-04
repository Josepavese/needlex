package webdiscover

import (
	"net/url"
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/pipeline"
)

type EndpointExtractorResult struct {
	SelectedURL     string  `json:"selected_url"`
	EvidencePageURL string  `json:"evidence_page_url"`
	Kind            string  `json:"kind"`
	Confidence      float64 `json:"confidence"`
}

type EndpointPageInput struct {
	PageURL      string   `json:"page_url"`
	PageTitle    string   `json:"page_title"`
	EmbeddedURLs []string `json:"embedded_urls"`
}

func PromoteEndpointCandidate(candidates []discoverycore.Candidate, selectedPage EndpointPageInput, out EndpointExtractorResult) []discoverycore.Candidate {
	boosted := append([]discoverycore.Candidate{}, candidates...)
	boosted = append(boosted, discoverycore.Candidate{
		URL:   out.SelectedURL,
		Label: discoverycore.FirstNonEmpty(strings.TrimSpace(selectedPage.PageTitle), out.SelectedURL),
		Score: 4.50 + out.Confidence,
		Reason: discoverycore.AppendUniqueReason(nil,
			"endpoint_extract_llm",
			"embedded_url_provenance",
		),
		Metadata: map[string]string{
			"endpoint_extract_kind":          strings.TrimSpace(out.Kind),
			"endpoint_extract_evidence_page": strings.TrimSpace(selectedPage.PageURL),
		},
	})
	return discoverycore.NewSet(boosted).Sorted()
}

func EmbeddedURLsForPage(finalURL, rawHTML string, dom pipeline.SimplifiedDOM) []string {
	family, ok := CandidateFamily(finalURL)
	if !ok {
		return nil
	}
	sourceClass := discoverycore.ResourceClass(finalURL)
	texts := make([]string, 0, len(dom.Nodes)+2)
	if sourceClass != discoverycore.ResourceClassHTMLLike {
		if trimmed := strings.TrimSpace(rawHTML); trimmed != "" {
			texts = append(texts, trimmed)
		}
	}
	if trimmed := strings.TrimSpace(dom.Title); trimmed != "" {
		texts = append(texts, trimmed)
	}
	for _, node := range dom.Nodes {
		if trimmed := strings.TrimSpace(node.Text); trimmed != "" {
			texts = append(texts, trimmed)
		}
	}
	out := make([]string, 0, 8)
	seen := map[string]struct{}{}
	for _, text := range texts {
		for _, raw := range embeddedURLPattern.FindAllString(text, -1) {
			embeddedURL := trimEmbeddedURL(raw)
			if embeddedURL == "" {
				continue
			}
			if _, ok := seen[embeddedURL]; ok {
				continue
			}
			embeddedFamily, ok := CandidateFamily(embeddedURL)
			if !ok || embeddedFamily != family {
				continue
			}
			seen[embeddedURL] = struct{}{}
			out = append(out, embeddedURL)
			if len(out) >= 8 {
				return out
			}
		}
	}
	return out
}

func HostRootURL(rawURL string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || strings.TrimSpace(parsed.Scheme) == "" || strings.TrimSpace(parsed.Host) == "" {
		return "", false
	}
	return (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host, Path: "/"}).String(), true
}
