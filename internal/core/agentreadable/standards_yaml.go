package agentreadable

import (
	"encoding/xml"
	"net/url"
	"path"
	"strings"
)

func normalizeYAMLValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			out[key] = normalizeYAMLValue(child)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			if text, ok := key.(string); ok {
				out[text] = normalizeYAMLValue(child)
			}
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, child := range typed {
			out = append(out, normalizeYAMLValue(child))
		}
		return out
	default:
		return value
	}
}

func sitemapLocations(body string) []string {
	locations := []string{}
	decoder := xml.NewDecoder(strings.NewReader(body))
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := token.(xml.StartElement)
		if !ok || !strings.EqualFold(start.Name.Local, "loc") {
			continue
		}
		var value string
		if err := decoder.DecodeElement(&value, &start); err == nil {
			value = strings.TrimSpace(value)
			if value != "" {
				locations = append(locations, value)
			}
		}
	}
	if len(locations) > 0 {
		return uniqueStrings(locations)
	}
	for _, raw := range absoluteURLPattern.FindAllString(body, -1) {
		locations = append(locations, strings.TrimSpace(raw))
	}
	return uniqueStrings(locations)
}

func collectCatalogCandidates(base *url.URL, value any, relHint string, out *[]Candidate) {
	switch typed := value.(type) {
	case map[string]any:
		rel := firstCatalogString(typed, "rel", "relation")
		if rel == "" {
			rel = relHint
		}
		contentType := firstCatalogString(typed, "type", "media_type", "mediaType")
		for _, key := range []string{"href", "url", "uri"} {
			if raw := firstCatalogString(typed, key); raw != "" {
				if candidate, ok := candidateFromCatalogLink(base, raw, rel, contentType); ok {
					*out = append(*out, candidate)
				}
			}
		}
		for key, child := range typed {
			collectCatalogCandidates(base, child, relationHintFromKey(key, rel), out)
		}
	case []any:
		for _, child := range typed {
			collectCatalogCandidates(base, child, relHint, out)
		}
	case string:
		if candidate, ok := candidateFromCatalogLink(base, typed, relHint, ""); ok {
			*out = append(*out, candidate)
		}
	}
}

func candidateFromCatalogLink(base *url.URL, rawHref, rel, contentType string) (Candidate, bool) {
	resolved := resolveURL(base, rawHref)
	if resolved == "" || !sameOrigin(base.String(), resolved) {
		return Candidate{}, false
	}
	rel = strings.ToLower(strings.TrimSpace(rel))
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	switch {
	case strings.Contains(rel, "api-catalog") || isAPICatalogURL(resolved):
		return Candidate{URL: resolved, Kind: KindAPICatalog, DeclaredBy: "api_catalog", Priority: 50}, true
	case strings.Contains(rel, "service-desc") || strings.Contains(rel, "service-meta") || strings.Contains(rel, "describedby") || looksServiceDescriptionReference(resolved, contentType):
		return Candidate{URL: resolved, Kind: KindServiceDescription, DeclaredBy: "api_catalog", Priority: 56}, true
	case strings.Contains(rel, "service-doc") && hasExtensionURL(resolved, ".md", ".mdx", ".markdown"):
		return Candidate{URL: resolved, Kind: KindMarkdownVariant, DeclaredBy: "api_catalog", Priority: 58}, true
	case strings.Contains(rel, "alternate") && hasExtensionURL(resolved, ".md", ".mdx", ".markdown"):
		return Candidate{URL: resolved, Kind: KindMarkdownVariant, DeclaredBy: "api_catalog", Priority: 58}, true
	default:
		return candidateFromProtocolURL(resolved, "api_catalog")
	}
}

func candidateFromProtocolURL(rawURL, declaredBy string) (Candidate, bool) {
	lower := strings.ToLower(strings.TrimSpace(rawURL))
	switch {
	case strings.HasSuffix(lower, "/llms-full.txt"):
		return Candidate{URL: rawURL, Kind: KindLLMSFull, DeclaredBy: declaredBy, Priority: 36}, true
	case strings.HasSuffix(lower, "/llms.txt"):
		return Candidate{URL: rawURL, Kind: KindLLMSIndex, DeclaredBy: declaredBy, Priority: 32}, true
	case hasExtensionURL(rawURL, ".md", ".mdx", ".markdown"):
		return Candidate{URL: rawURL, Kind: KindMarkdownVariant, DeclaredBy: declaredBy, Priority: 45}, true
	case isAPICatalogURL(rawURL):
		return Candidate{URL: rawURL, Kind: KindAPICatalog, DeclaredBy: declaredBy, Priority: 84}, true
	case isServiceDescriptionURL(rawURL):
		return Candidate{URL: rawURL, Kind: KindServiceDescription, DeclaredBy: declaredBy, Priority: 92}, true
	default:
		return Candidate{}, false
	}
}

func sitemapPathPriority(target *url.URL, candidateURL string) int {
	if target == nil || strings.TrimSpace(target.Path) == "" {
		return 30
	}
	candidate, err := url.Parse(candidateURL)
	if err != nil {
		return 30
	}
	if samePathStem(target.Path, candidate.Path) {
		return 0
	}
	if strings.EqualFold(path.Dir(path.Clean(target.Path)), path.Dir(path.Clean(candidate.Path))) {
		return 10
	}
	return 30
}

func samePathStem(left, right string) bool {
	leftStem := strings.TrimSuffix(path.Clean(left), path.Ext(left))
	rightStem := strings.TrimSuffix(path.Clean(right), path.Ext(right))
	return strings.EqualFold(leftStem, rightStem)
}

func relationHintFromKey(key, fallback string) string {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch normalized {
	case "api-catalog", "service-desc", "service-doc", "service-meta", "describedby", "alternate":
		return normalized
	case "href", "url", "uri", "type", "media_type", "mediatype", "rel", "relation":
		return fallback
	default:
		if strings.Contains(normalized, "service-desc") || strings.Contains(normalized, "service-doc") || strings.Contains(normalized, "service-meta") || strings.Contains(normalized, "api-catalog") {
			return normalized
		}
		return fallback
	}
}

func firstCatalogString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			return strings.TrimSpace(typed)
		case []any:
			parts := []string{}
			for _, item := range typed {
				if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
					parts = append(parts, strings.TrimSpace(text))
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, " ")
			}
		}
	}
	return ""
}

func looksServiceDescriptionReference(rawURL, contentType string) bool {
	lower := strings.ToLower(rawURL + " " + contentType)
	return strings.Contains(lower, "openapi") ||
		strings.Contains(lower, "swagger") ||
		strings.Contains(lower, "asyncapi") ||
		strings.Contains(lower, "application/openapi") ||
		strings.Contains(lower, "application/vnd.oai.openapi")
}

func isServiceDescriptionURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	lowerPath := strings.ToLower(parsed.Path)
	if !(strings.HasSuffix(lowerPath, ".json") || strings.HasSuffix(lowerPath, ".yaml") || strings.HasSuffix(lowerPath, ".yml")) {
		return false
	}
	return strings.Contains(lowerPath, "openapi") ||
		strings.Contains(lowerPath, "swagger") ||
		strings.Contains(lowerPath, "asyncapi")
}

func isAPICatalogURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	return strings.EqualFold(path.Clean(parsed.Path), "/.well-known/api-catalog")
}

func looksLikeServiceDescription(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return (strings.Contains(lower, `"openapi"`) || strings.Contains(lower, "openapi:") || strings.Contains(lower, `"swagger"`) || strings.Contains(lower, "swagger:") || strings.Contains(lower, `"asyncapi"`) || strings.Contains(lower, "asyncapi:")) &&
		(strings.Contains(lower, `"info"`) || strings.Contains(lower, "info:")) &&
		(strings.Contains(lower, `"paths"`) || strings.Contains(lower, "paths:") || strings.Contains(lower, `"channels"`) || strings.Contains(lower, "channels:"))
}

func looksLikeAPICatalog(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(lower, "linkset") ||
		strings.Contains(lower, "api-catalog") ||
		strings.Contains(lower, "service-desc") ||
		strings.Contains(lower, "openapi")
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}
