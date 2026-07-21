package rendering

import (
	"net/url"
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
)

func normalizeRenderNetworkScalarValue(value string) string {
	value = normalizeRenderNetworkText(value)
	if sanitized, ok := sanitizeRenderNetworkURLValue(value); ok {
		return sanitized
	}
	return value
}

func renderNetworkValueUseful(value string) bool {
	value = strings.TrimSpace(value)
	if len([]rune(value)) < 3 {
		return false
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return renderNetworkURLUseful(value)
	}
	if strings.Count(lower, "/") > 4 && (strings.Contains(lower, ".jpg") || strings.Contains(lower, ".png") || strings.Contains(lower, ".webp")) {
		return false
	}
	if renderNetworkLooksOpaqueIdentifier(value) {
		return false
	}
	return true
}

func sanitizeRenderNetworkURLValue(value string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || strings.TrimSpace(parsed.Scheme) == "" || strings.TrimSpace(parsed.Hostname()) == "" {
		return value, false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return value, false
	}
	parsed.Fragment = ""
	if renderNetworkQueryLooksSensitive(parsed.RawQuery) || len(parsed.RawQuery) > 160 {
		parsed.RawQuery = ""
	}
	return parsed.String(), true
}

func renderNetworkURLUseful(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || strings.TrimSpace(parsed.Hostname()) == "" {
		return false
	}
	switch discoverycore.ResourceClass(parsed.String()) {
	case discoverycore.ResourceClassMediaAsset, discoverycore.ResourceClassArchiveFile:
		return false
	default:
		return true
	}
}

func renderNetworkQueryLooksSensitive(rawQuery string) bool {
	lower := strings.ToLower(strings.TrimSpace(rawQuery))
	if lower == "" {
		return false
	}
	for _, marker := range []string{"token", "secret", "signature", "sig=", "key=", "api_key", "apikey", "auth", "session", "jwt", "password", "credential"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func renderNetworkKeyUseful(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	switch {
	case lower == "id", lower == "_id", lower == "uuid", lower == "url", lower == "href", lower == "src":
		return false
	case strings.Contains(lower, "token"), strings.Contains(lower, "secret"), strings.Contains(lower, "password"):
		return false
	case strings.HasSuffix(lower, "_url"), strings.HasSuffix(lower, "url"), strings.HasSuffix(lower, "_id"):
		return false
	default:
		return true
	}
}

func renderNetworkLooksOpaqueIdentifier(value string) bool {
	if strings.ContainsAny(value, " \t\r\n") || len(value) < 28 {
		return false
	}
	alphaNum := 0
	other := 0
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			alphaNum++
		case r == '-' || r == '_' || r == '.':
		default:
			other++
		}
	}
	return other == 0 && alphaNum >= 24
}
