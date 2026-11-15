package platform

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"
)

type SessionCookie struct {
	Name     string
	Value    string
	Domain   string
	HostOnly bool
	Path     string
	Secure   bool
	HTTPOnly bool
	SameSite string
	Expires  time.Time
}

func validateSessionCookie(cookie SessionCookie) error {
	if strings.TrimSpace(cookie.Name) == "" || strings.ContainsAny(cookie.Name, " \t\r\n;") {
		return errors.New("session cookie name is invalid")
	}
	if strings.TrimSpace(cookie.Domain) == "" {
		return fmt.Errorf("session cookie %q needs a domain", cookie.Name)
	}
	if strings.ContainsAny(cookie.Value, "\r\n") {
		return fmt.Errorf("session cookie %q contains a line break", cookie.Name)
	}
	domain := strings.TrimSuffix(strings.TrimSpace(cookie.Domain), ".")
	host := strings.TrimPrefix(domain, ".")
	if host == "" || strings.ContainsAny(host, " /\\") {
		return fmt.Errorf("session cookie %q has an invalid domain", cookie.Name)
	}
	if net.ParseIP(host) == nil && host != "localhost" {
		if suffix, _ := publicsuffix.PublicSuffix(host); suffix == host {
			return fmt.Errorf("session cookie %q must not target the public suffix %q", cookie.Name, host)
		}
	}
	switch strings.ToLower(strings.TrimSpace(cookie.SameSite)) {
	case "", "default", "lax", "strict", "none":
	default:
		return fmt.Errorf("session cookie %q has an invalid SameSite", cookie.Name)
	}
	return nil
}

func parseSameSite(value string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "lax":
		return http.SameSiteLaxMode
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteDefaultMode
	}
}

func sessionCookieKey(cookie SessionCookie) string {
	domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(cookie.Domain), "."))
	domain = strings.TrimPrefix(domain, ".")
	return cookie.Name + "\x00" + domain + "\x00" + sessionCookiePath(cookie.Path)
}

func sessionCookiePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || !strings.HasPrefix(path, "/") {
		return "/"
	}
	return path
}
