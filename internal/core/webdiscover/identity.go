package webdiscover

import (
	"net/url"
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/intel"
	"golang.org/x/net/html"
)

type IdentityReferenceCandidate struct {
	URL      string
	Label    string
	Relation string
}

const (
	identityExternalMinSimilarity = 0.20
)

func ExtractIdentityReferenceCandidates(rawHTML, baseURL, label string) []IdentityReferenceCandidate {
	root, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return nil
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}
	out := make([]IdentityReferenceCandidate, 0, 6)
	seen := map[string]struct{}{}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch strings.ToLower(strings.TrimSpace(node.Data)) {
			case "link":
				rel := strings.ToLower(strings.TrimSpace(htmlAttr(node, "rel")))
				if rel == "canonical" || strings.Contains(rel, "alternate") {
					if href := resolveReferenceURL(base, htmlAttr(node, "href")); href != "" {
						if _, ok := seen[href]; !ok {
							seen[href] = struct{}{}
							relation := "alternate"
							if rel == "canonical" {
								relation = "canonical"
							}
							out = append(out, IdentityReferenceCandidate{URL: href, Label: strings.TrimSpace(label), Relation: relation})
						}
					}
				}
			case "meta":
				property := strings.ToLower(strings.TrimSpace(htmlAttr(node, "property")))
				if property == "og:url" {
					if href := resolveReferenceURL(base, htmlAttr(node, "content")); href != "" {
						if _, ok := seen[href]; !ok {
							seen[href] = struct{}{}
							out = append(out, IdentityReferenceCandidate{URL: href, Label: strings.TrimSpace(label), Relation: "og_url"})
						}
					}
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return out
}

func IdentityBaseLinks(sourceURL string, refs []IdentityReferenceCandidate) ([]discoverycore.LinkCandidate, map[string]string) {
	baseLinks := make([]discoverycore.LinkCandidate, 0, len(refs))
	relationByURL := make(map[string]string, len(refs))
	cleanSourceURL := strings.TrimSpace(sourceURL)
	for _, ref := range refs {
		if discoverycore.SameCanonicalURL(ref.URL, cleanSourceURL) {
			continue
		}
		baseLinks = append(baseLinks, discoverycore.LinkCandidate{URL: ref.URL, Label: ref.Label})
		relationByURL[ref.URL] = ref.Relation
	}
	return baseLinks, relationByURL
}

func IdentitySemanticCandidates(source discoverycore.Candidate, scored []discoverycore.Candidate) []intel.SemanticCandidate {
	semanticCandidates := make([]intel.SemanticCandidate, 0, len(scored))
	for _, candidate := range scored {
		semanticCandidates = append(semanticCandidates, intel.SemanticCandidate{
			ID: candidate.URL,
			Text: discoverycore.JoinNonEmpty(
				source.Metadata["host_root_title"],
				source.Metadata["page_title"],
				source.Label,
				candidate.Metadata["source_context"],
				candidate.Label,
				discoverycore.URLIdentityText(candidate.URL),
				candidate.Metadata["resource_class"],
			),
		})
	}
	return semanticCandidates
}

func IdentityDiscoverCandidates(source discoverycore.Candidate, scored []discoverycore.Candidate, relationByURL map[string]string, goalSimilarity map[string]float64) []discoverycore.Candidate {
	sourceFamily, _ := CandidateFamily(source.URL)
	out := make([]discoverycore.Candidate, 0, 2)
	for _, candidate := range scored {
		similarity := goalSimilarity[candidate.URL]
		relation := discoverycore.FirstNonEmpty(relationByURL[candidate.URL], "unknown")
		family, familyOK := CandidateFamily(candidate.URL)
		externalFamily := familyOK && family != "" && sourceFamily != "" && family != sourceFamily
		if externalFamily && relation != "canonical" && similarity < identityExternalMinSimilarity {
			continue
		}
		boost := identityReferenceBoost(relation, similarity, externalFamily)
		if boost == 0 {
			continue
		}
		reasons := discoverycore.AppendUniqueReason(candidate.Reason,
			"identity_reference",
			"identity_reference_"+relation,
		)
		if externalFamily {
			reasons = discoverycore.AppendUniqueReason(reasons, "external_family_recovery")
		}
		out = append(out, discoverycore.Candidate{
			URL:    candidate.URL,
			Label:  discoverycore.FirstNonEmpty(candidate.Label, source.Label, candidate.URL),
			Score:  candidate.Score + boost,
			Reason: reasons,
			Metadata: discoverycore.MergeMetadata(source.Metadata, map[string]string{
				"identity_reference_source": source.URL,
				"identity_reference_kind":   relation,
				"identity_reference_scope":  identityReferenceScope(externalFamily),
				"resource_class":            discoverycore.ResourceClass(candidate.URL),
			}),
		})
		if len(out) >= 2 {
			break
		}
	}
	return out
}

func identityReferenceBoost(relation string, similarity float64, externalFamily bool) float64 {
	if externalFamily {
		boost := 0.26 + similarity*1.05
		switch relation {
		case "canonical":
			boost += 1.12
		case "og_url":
			boost += 0.22
		case "alternate":
			boost += 0.16
		}
		return boost
	}
	boost := 0.08 + similarity*0.24
	switch relation {
	case "canonical":
		boost += 0.12
	case "og_url":
		boost += 0.08
	case "alternate":
		boost += 0.06
	}
	return boost
}

func identityReferenceScope(externalFamily bool) string {
	if externalFamily {
		return "external_family"
	}
	return "same_family"
}

func resolveReferenceURL(base *url.URL, raw string) string {
	ref, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	resolved := base.ResolveReference(ref)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return ""
	}
	return resolved.String()
}

func htmlAttr(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(strings.TrimSpace(attr.Key), key) {
			return strings.TrimSpace(attr.Val)
		}
	}
	return ""
}
