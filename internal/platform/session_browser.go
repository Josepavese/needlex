package platform

import (
	"net/url"
	"strings"
)

// BrowserCookies returns the session cookies that apply to rawURL for CDP
// Network.setCookies. It seeds only the requested origin: injecting the whole
// profile would expose unrelated cookies to arbitrary subresources and redirect
// targets in the rendered page. Host-only cookies are matched exactly; domain
// cookies use browser suffix matching.
func (s *Session) BrowserCookies(rawURL string) []map[string]any {
	if s == nil || strings.TrimSpace(rawURL) == "" {
		return nil
	}
	target, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || target.Hostname() == "" {
		return nil
	}
	host := strings.ToLower(target.Hostname())
	scheme := strings.ToLower(target.Scheme)
	type cookieKey struct {
		name, domain, path string
		hostOnly           bool
	}
	seen := make(map[cookieKey]bool, len(s.cookies))
	out := make([]map[string]any, 0, len(s.cookies))
	for i := len(s.cookies) - 1; i >= 0; i-- {
		cookie := s.cookies[i]
		domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(cookie.Domain), "."))
		matchHost := strings.TrimPrefix(domain, ".")
		if cookie.HostOnly {
			if matchHost != host {
				continue
			}
		} else if matchHost != host && !strings.HasSuffix(host, "."+matchHost) {
			continue
		}
		path := sessionCookiePath(cookie.Path)
		key := cookieKey{name: cookie.Name, domain: domain, path: path, hostOnly: cookie.HostOnly}
		if seen[key] {
			continue
		}
		seen[key] = true
		value := map[string]any{
			"name": cookie.Name, "value": cookie.Value, "path": path,
			"secure": cookie.Secure, "httpOnly": cookie.HTTPOnly,
		}
		if cookie.HostOnly {
			cookieScheme := scheme
			if cookie.Secure {
				cookieScheme = "https"
			}
			value["url"] = (&url.URL{Scheme: cookieScheme, Host: matchHost, Path: path}).String()
		} else {
			value["domain"] = cookie.Domain
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
