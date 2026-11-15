package platform

import (
	"net/url"
	"strings"
)

// BrowserCookies returns cookies for CDP Network.setCookies. An empty URL
// returns every session cookie so a rendered redirect or an authenticated
// subresource on another session domain can use its own domain-scoped cookie;
// the browser still enforces domain, path and Secure scoping. Cookies are
// deduplicated last-first so explicit session cookies override profile values.
func (s *Session) BrowserCookies(rawURL string) []map[string]any {
	if s == nil {
		return nil
	}
	host := ""
	if strings.TrimSpace(rawURL) != "" {
		target, err := url.Parse(strings.TrimSpace(rawURL))
		if err != nil || target.Hostname() == "" {
			return nil
		}
		host = strings.ToLower(target.Hostname())
	}
	type cookieKey struct{ name, domain, path string }
	seen := make(map[cookieKey]bool, len(s.cookies))
	out := make([]map[string]any, 0, len(s.cookies))
	for i := len(s.cookies) - 1; i >= 0; i-- {
		cookie := s.cookies[i]
		domain := strings.ToLower(strings.TrimSpace(cookie.Domain))
		if host != "" {
			match := strings.TrimPrefix(domain, ".")
			if match != host && !strings.HasSuffix(host, "."+match) {
				continue
			}
		}
		path := sessionCookiePath(cookie.Path)
		key := cookieKey{name: cookie.Name, domain: domain, path: path}
		if seen[key] {
			continue
		}
		seen[key] = true
		value := map[string]any{
			"name": cookie.Name, "value": cookie.Value,
			"domain": cookie.Domain, "path": path,
			"secure": cookie.Secure, "httpOnly": cookie.HTTPOnly,
		}
		if cookie.Expires.IsZero() {
			value["expires"] = -1
		} else {
			value["expires"] = float64(cookie.Expires.Unix())
		}
		if sameSite := cdpSameSite(cookie.SameSite); sameSite != "" {
			value["sameSite"] = sameSite
		}
		out = append(out, value)
	}
	return out
}

func cdpSameSite(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "lax":
		return "Lax"
	case "strict":
		return "Strict"
	case "none":
		return "None"
	default:
		return ""
	}
}
