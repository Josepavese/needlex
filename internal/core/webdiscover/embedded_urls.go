package webdiscover

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/josepavese/needlex/internal/core"
	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/pipeline"
)

func RefineCandidate(_ string, candidate discoverycore.Candidate, finalURL, pageTitle string, webIR core.WebIR, domainHints []string) discoverycore.Candidate {
	score, reasons := discoverycore.ScoreStructuralURL(finalURL, false, domainHints)
	resourceClass := discoverycore.ResourceClass(finalURL)
	if strings.TrimSpace(pageTitle) != "" {
		score += 0.35
		reasons = append(reasons, "page_title_probe")
	}
	if webIR.NodeCount > 0 {
		score += 0.10
		reasons = append(reasons, "web_ir_probe")
	}
	if webIR.Signals.EmbeddedNodeCount > 0 {
		reasons = append(reasons, "web_ir_embedded")
	}
	if strings.TrimSpace(finalURL) != "" && finalURL != candidate.URL {
		reasons = append(reasons, "redirect_resolved")
	}
	metadata := discoverycore.MergeMetadata(candidate.Metadata, IRMetadata(webIR))
	if metadata == nil {
		metadata = map[string]string{}
	}
	if strings.TrimSpace(pageTitle) != "" {
		metadata["page_title"] = strings.TrimSpace(pageTitle)
	}
	if context := IRContext(webIR, 1200); context != "" {
		metadata["web_ir_context"] = context
	}
	if host, ok := discoverycore.Hostname(finalURL); ok {
		metadata["final_host"] = host
	}
	metadata["resource_class"] = resourceClass
	return discoverycore.Candidate{
		URL:      finalURL,
		Label:    discoverycore.FirstNonEmpty(pageTitle, candidate.Label),
		Score:    max(score, candidate.Score),
		Reason:   discoverycore.AppendUniqueReason(append([]string{}, candidate.Reason...), reasons...),
		Metadata: metadata,
	}
}

var embeddedURLPattern = regexp.MustCompile(`https?://[^\s"'<>` + "`" + `)]+`)

func ExtractEmbeddedURLCandidates(_ string, candidate discoverycore.Candidate, finalURL, rawHTML string, dom pipeline.SimplifiedDOM, domainHints []string) []discoverycore.Candidate {
	finalFamily, ok := CandidateFamily(finalURL)
	if !ok {
		return nil
	}
	sourceClass := discoverycore.ResourceClass(finalURL)
	embeddedLinks := sameFamilyEmbeddedLinks(finalFamily, candidate, rawHTML, dom, sourceClass)
	if len(embeddedLinks) == 0 {
		return nil
	}
	scored := discoverycore.ScoreStructuralCandidates("", "", embeddedLinks, domainHints)
	return embeddedURLDiscoverCandidates(scored, candidate, dom, sourceClass)
}

func embeddedURLSearchTexts(rawHTML string, dom pipeline.SimplifiedDOM, sourceClass string) []string {
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
	return texts
}

func sameFamilyEmbeddedLinks(finalFamily string, candidate discoverycore.Candidate, rawHTML string, dom pipeline.SimplifiedDOM, sourceClass string) []discoverycore.LinkCandidate {
	embeddedLinks := make([]discoverycore.LinkCandidate, 0, 4)
	seen := map[string]struct{}{}
	for _, text := range embeddedURLSearchTexts(rawHTML, dom, sourceClass) {
		for _, raw := range embeddedURLPattern.FindAllString(text, -1) {
			embeddedURL := trimEmbeddedURL(raw)
			if embeddedURL == "" {
				continue
			}
			if _, ok := seen[embeddedURL]; ok {
				continue
			}
			embeddedFamily, ok := CandidateFamily(embeddedURL)
			if !ok || embeddedFamily != finalFamily {
				continue
			}
			seen[embeddedURL] = struct{}{}
			embeddedLinks = append(embeddedLinks, discoverycore.LinkCandidate{
				URL:   embeddedURL,
				Label: discoverycore.JoinNonEmpty(dom.Title, candidate.Label),
			})
			if len(embeddedLinks) >= 4 {
				break
			}
		}
		if len(embeddedLinks) >= 4 {
			break
		}
	}
	return embeddedLinks
}

func embeddedURLDiscoverCandidates(scored []discoverycore.Candidate, candidate discoverycore.Candidate, dom pipeline.SimplifiedDOM, sourceClass string) []discoverycore.Candidate {
	out := make([]discoverycore.Candidate, 0, min(len(scored), 2))
	for _, item := range scored {
		resourceClass := discoverycore.ResourceClass(item.URL)
		if sourceClass == discoverycore.ResourceClassHTMLLike && resourceClass == discoverycore.ResourceClassMediaAsset {
			continue
		}
		boost, boostReasons := embeddedURLContextBoost(candidate.URL, item.URL, sourceClass, resourceClass)
		out = append(out, discoverycore.Candidate{
			URL:   item.URL,
			Label: discoverycore.FirstNonEmpty(item.Label, dom.Title, candidate.Label),
			Score: item.Score + boost,
			Reason: discoverycore.AppendUniqueReason(
				append([]string{}, item.Reason...),
				append([]string{"embedded_url_provenance", "embedded_url_same_family"}, boostReasons...)...,
			),
			Metadata: discoverycore.MergeMetadata(candidate.Metadata, map[string]string{
				"embedded_url_source": candidate.URL,
				"page_title":          strings.TrimSpace(dom.Title),
				"resource_class":      resourceClass,
			}),
		})
		if len(out) >= 2 {
			break
		}
	}
	return out
}

func embeddedURLContextBoost(sourceURL, embeddedURL, sourceClass, resourceClass string) (float64, []string) {
	boost := 0.34
	reasons := []string{"embedded_url_contextual_evidence"}
	depthDelta := discoverycore.URLPathDepth(embeddedURL) - discoverycore.URLPathDepth(sourceURL)
	if depthDelta >= 2 {
		boost += 1.10
		reasons = append(reasons, "embedded_url_deep_resource")
	}
	if sourceClass != discoverycore.ResourceClassHTMLLike {
		boost += 0.34
		reasons = append(reasons, "embedded_url_non_html_source")
	}
	switch resourceClass {
	case discoverycore.ResourceClassHTMLLike:
		boost += 0.04
	case discoverycore.ResourceClassStructured, discoverycore.ResourceClassTextAsset, discoverycore.ResourceClassDocumentFile:
		boost += 0.02
	case discoverycore.ResourceClassMediaAsset, discoverycore.ResourceClassArchiveFile:
		boost -= 0.12
	case discoverycore.ResourceClassUnknown:
		boost -= 0.10
		reasons = append(reasons, "embedded_url_untyped_resource")
	}
	return boost, reasons
}

func trimEmbeddedURL(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.TrimRight(value, ".,;:)]}\"'")
	parsed, err := url.Parse(value)
	if err != nil || strings.TrimSpace(parsed.Scheme) == "" || strings.TrimSpace(parsed.Host) == "" {
		return ""
	}
	return parsed.String()
}

func IRMetadata(webIR core.WebIR) map[string]string {
	if webIR.NodeCount <= 0 {
		return nil
	}
	return map[string]string{
		"web_ir_node_count":          strconv.Itoa(webIR.NodeCount),
		"web_ir_embedded_node_count": strconv.Itoa(webIR.Signals.EmbeddedNodeCount),
		"web_ir_heading_ratio":       strconv.FormatFloat(webIR.Signals.HeadingRatio, 'f', 3, 64),
		"web_ir_short_text_ratio":    strconv.FormatFloat(webIR.Signals.ShortTextRatio, 'f', 3, 64),
	}
}

func IRContext(webIR core.WebIR, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	parts := make([]string, 0, min(len(webIR.Nodes)+1, 13))
	if title := strings.TrimSpace(webIR.Title); title != "" {
		parts = append(parts, title)
	}
	for _, node := range webIR.Nodes {
		text := strings.TrimSpace(node.Text)
		if text == "" {
			continue
		}
		parts = append(parts, text)
		if len(parts) >= 13 {
			break
		}
	}
	return compactIRContext(strings.Join(parts, "\n"), maxChars)
}

func compactIRContext(text string, maxChars int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if len(text) <= maxChars {
		return text
	}
	return strings.TrimSpace(text[:maxChars])
}
