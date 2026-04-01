package pipeline

import "strings"

func inferSubstrateClass(rawHTML string) string {
	haystack := strings.ToLower(strings.TrimSpace(rawHTML))
	if haystack == "" {
		return "generic_content"
	}
	if containsAny(haystack,
		"window._a2s",
		"__next_data__",
		"__nuxt__",
		"__apollo_state__",
		"<app-root",
		"data-reactroot",
	) {
		return "embedded_app_payload"
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
