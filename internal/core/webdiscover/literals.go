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

func RefineCandidate(goal string, candidate discoverycore.Candidate, finalURL, pageTitle string, webIR core.WebIR, domainHints []string) discoverycore.Candidate {
	score, reasons := discoverycore.ScoreURL(goal, finalURL, discoverycore.JoinNonEmpty(pageTitle, candidate.Label), false, domainHints)
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
		score += 0.12
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

var literalURLPattern = regexp.MustCompile(`https?://[^\s"'<>` + "`" + `)]+`)

func ExtractLiteralURLCandidates(goal string, candidate discoverycore.Candidate, finalURL, rawHTML string, dom pipeline.SimplifiedDOM, domainHints []string) []discoverycore.Candidate {
	finalFamily, ok := CandidateFamily(finalURL)
	if !ok {
		return nil
	}
	sourceClass := discoverycore.ResourceClass(finalURL)
	literalLinks := sameFamilyLiteralLinks(finalFamily, candidate, rawHTML, dom, sourceClass)
	if len(literalLinks) == 0 {
		return nil
	}
	scored := discoverycore.ScoreCandidates(goal, "", discoverycore.JoinNonEmpty(dom.Title, candidate.Label), literalLinks, domainHints)
	return literalDiscoverCandidates(scored, candidate, dom, sourceClass)
}

func literalSearchTexts(rawHTML string, dom pipeline.SimplifiedDOM, sourceClass string) []string {
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

func sameFamilyLiteralLinks(finalFamily string, candidate discoverycore.Candidate, rawHTML string, dom pipeline.SimplifiedDOM, sourceClass string) []discoverycore.LinkCandidate {
	literalLinks := make([]discoverycore.LinkCandidate, 0, 4)
	seen := map[string]struct{}{}
	for _, text := range literalSearchTexts(rawHTML, dom, sourceClass) {
		for _, raw := range literalURLPattern.FindAllString(text, -1) {
			literalURL := trimLiteralURL(raw)
			if literalURL == "" {
				continue
			}
			if _, ok := seen[literalURL]; ok {
				continue
			}
			literalFamily, ok := CandidateFamily(literalURL)
			if !ok || literalFamily != finalFamily {
				continue
			}
			seen[literalURL] = struct{}{}
			literalLinks = append(literalLinks, discoverycore.LinkCandidate{
				URL:   literalURL,
				Label: discoverycore.JoinNonEmpty(dom.Title, candidate.Label),
			})
			if len(literalLinks) >= 4 {
				break
			}
		}
		if len(literalLinks) >= 4 {
			break
		}
	}
	return literalLinks
}

func literalDiscoverCandidates(scored []discoverycore.Candidate, candidate discoverycore.Candidate, dom pipeline.SimplifiedDOM, sourceClass string) []discoverycore.Candidate {
	out := make([]discoverycore.Candidate, 0, min(len(scored), 2))
	for _, item := range scored {
		resourceClass := discoverycore.ResourceClass(item.URL)
		if sourceClass == discoverycore.ResourceClassHTMLLike && resourceClass == discoverycore.ResourceClassMediaAsset {
			continue
		}
		boost := 1.10
		if discoverycore.URLPathDepth(item.URL) >= 3 {
			boost += 0.12
		}
		out = append(out, discoverycore.Candidate{
			URL:   item.URL,
			Label: discoverycore.FirstNonEmpty(item.Label, dom.Title, candidate.Label),
			Score: item.Score + boost,
			Reason: discoverycore.AppendUniqueReason(
				append([]string{}, item.Reason...),
				"literal_url_probe",
				"literal_url_same_family",
			),
			Metadata: discoverycore.MergeMetadata(candidate.Metadata, map[string]string{
				"literal_url_source": candidate.URL,
				"page_title":         strings.TrimSpace(dom.Title),
				"resource_class":     resourceClass,
			}),
		})
		if len(out) >= 2 {
			break
		}
	}
	return out
}

func trimLiteralURL(raw string) string {
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
