package discovery

import (
	"net/url"
	"path"
	"strings"
)

func CanonicalURLKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return strings.TrimRight(strings.ToLower(raw), "/")
	}
	host := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(parsed.Hostname())), "www.")
	if host == "" {
		return strings.TrimRight(strings.ToLower(raw), "/")
	}
	cleanPath := canonicalPathKey(parsed)
	return host + cleanPath
}

func SameCanonicalURL(left, right string) bool {
	return CanonicalURLKey(left) == CanonicalURLKey(right)
}

func IsCanonicalHomeURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return canonicalPathKey(parsed) == "/"
}

func canonicalPathKey(parsed *url.URL) string {
	cleanPath := strings.TrimSpace(parsed.EscapedPath())
	if cleanPath == "" || cleanPath == "/" {
		return "/"
	}
	cleanPath = path.Clean("/" + strings.Trim(cleanPath, "/"))
	if cleanPath == "." || cleanPath == "" {
		return "/"
	}
	base := strings.ToLower(path.Base(cleanPath))
	switch base {
	case "index", "index.html", "index.htm", "index.xhtml":
		parent := path.Dir(cleanPath)
		if parent == "." || parent == "" {
			return "/"
		}
		return parent
	default:
		return strings.TrimRight(cleanPath, "/")
	}
}
