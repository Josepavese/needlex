package pipeline

import "strings"

func inferSubstrateClass(page RawPage) string {
	if sourceKind := strings.ToLower(strings.TrimSpace(page.SourceKind)); sourceKind != "" {
		switch {
		case strings.Contains(sourceKind, "render"):
			return "rendered_html"
		case strings.Contains(sourceKind, "markdown") || strings.Contains(sourceKind, "llms"):
			return "agent_markdown"
		}
	}
	haystack := strings.ToLower(strings.TrimSpace(page.HTML))
	if haystack == "" {
		return "generic_content"
	}
	if containsAny(haystack,
		"__next_f",
		"self.__next_f",
		"__nuxt__",
		"__apollo_state__",
		"<app-root",
		"<app-shell",
		"data-reactroot",
		"id=\"__next_data__\"",
		"id=\"__next\"",
		"id=\"__nuxt\"",
	) {
		return "client_rendered_app"
	}
	if containsAny(haystack,
		"wp-content",
		"wp-json",
		"wp-includes",
		"et_pb",
		"elementor",
		"swiper",
		"gsap",
	) {
		return "theme_heavy_wordpress"
	}
	return "generic_content"
}

func containsAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}
