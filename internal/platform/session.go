package platform

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"
)

type SessionHeader struct {
	Origin string
	Name   string
	Value  string
}

type SessionCookie struct {
	Name     string
	Value    string
	Domain   string
	Path     string
	Secure   bool
	HTTPOnly bool
	SameSite string
	Expires  time.Time
}

type SessionTLS struct {
	CAFile             string
	ClientCertFile     string
	ClientKeyFile      string
	InsecureSkipVerify bool
}

type SessionConfig struct {
	Headers        []SessionHeader
	Cookies        []SessionCookie
	BrowserProfile *BrowserProfileConfig
	ProxyURL       string
	TLS            SessionTLS
}

// Session is the caller-owned navigation identity for one operation. It carries
// request credentials and transport settings without exposing them through
// formatting or serialization. A Session is safe for concurrent HTTP requests.
type Session struct {
	rules   []sessionHeaderRule
	cookies []SessionCookie
	jar     http.CookieJar
	proxy   *url.URL
	tls     *tls.Config
	managed map[string]struct{}
}

type sessionHeaderRule struct {
	origin string
	name   string
	value  string
}

// NewSession validates and materializes a navigation session. Hidden secrets
// are already resolved by the config owner; values are never logged here.
func NewSession(cfg SessionConfig) (*Session, error) {
	if cfg.BrowserProfile != nil {
		imported, err := ImportBrowserCookies(*cfg.BrowserProfile)
		if err != nil {
			return nil, err
		}
		cfg.Cookies = append(cfg.Cookies, imported.Cookies...)
	}
	session := &Session{managed: map[string]struct{}{}}
	for _, header := range cfg.Headers {
		origin, err := canonicalOrigin(header.Origin)
		if err != nil {
			return nil, fmt.Errorf("session header origin: %w", err)
		}
		name := http.CanonicalHeaderKey(strings.TrimSpace(header.Name))
		if name == "" || name == "Host" {
			return nil, fmt.Errorf("session header name %q is invalid", header.Name)
		}
		if strings.ContainsAny(header.Value, "\r\n") {
			return nil, fmt.Errorf("session header %s contains a line break", name)
		}
		session.rules = append(session.rules, sessionHeaderRule{origin: origin, name: name, value: header.Value})
		session.managed[name] = struct{}{}
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	for _, cookie := range cfg.Cookies {
		if err := validateSessionCookie(cookie); err != nil {
			return nil, err
		}
		host := strings.TrimPrefix(strings.TrimSpace(cookie.Domain), ".")
		scheme := "http"
		if cookie.Secure {
			scheme = "https"
		}
		jar.SetCookies(&url.URL{Scheme: scheme, Host: host}, []*http.Cookie{{
			Name: cookie.Name, Value: cookie.Value, Path: sessionCookiePath(cookie.Path),
			Domain: cookie.Domain, Secure: cookie.Secure, HttpOnly: cookie.HTTPOnly,
			SameSite: parseSameSite(cookie.SameSite), Expires: cookie.Expires,
		}})
		session.cookies = append(session.cookies, cookie)
	}
	session.jar = jar
	if strings.TrimSpace(cfg.ProxyURL) != "" {
		proxy, err := url.Parse(strings.TrimSpace(cfg.ProxyURL))
		if err != nil || proxy.Host == "" || (proxy.Scheme != "http" && proxy.Scheme != "https" && proxy.Scheme != "socks5") {
			return nil, errors.New("session proxy must be an absolute http, https or socks5 URL")
		}
		session.proxy = proxy
	}
	if sessionTLSConfigured(cfg.TLS) {
		tlsConfig, err := loadSessionTLS(cfg.TLS)
		if err != nil {
			return nil, err
		}
		session.tls = tlsConfig
	}
	return session, nil
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
	switch strings.ToLower(strings.TrimSpace(cookie.SameSite)) {
	case "", "default", "lax", "strict", "none":
	default:
		return fmt.Errorf("session cookie %q has an invalid SameSite", cookie.Name)
	}
	return nil
}

func sessionTLSConfigured(cfg SessionTLS) bool {
	return strings.TrimSpace(cfg.CAFile) != "" || strings.TrimSpace(cfg.ClientCertFile) != "" ||
		strings.TrimSpace(cfg.ClientKeyFile) != "" || cfg.InsecureSkipVerify
}

func loadSessionTLS(cfg SessionTLS) (*tls.Config, error) {
	out := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: cfg.InsecureSkipVerify} //nolint:gosec // explicit caller opt-in
	if cfg.CAFile != "" {
		pem, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read session CA: %w", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("load system roots: %w", err)
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("session CA file contains no certificates")
		}
		out.RootCAs = pool
	}
	if (cfg.ClientCertFile == "") != (cfg.ClientKeyFile == "") {
		return nil, errors.New("session client certificate and key must be provided together")
	}
	if cfg.ClientCertFile != "" {
		certificate, err := tls.LoadX509KeyPair(cfg.ClientCertFile, cfg.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load session client certificate: %w", err)
		}
		out.Certificates = []tls.Certificate{certificate}
	}
	return out, nil
}

// HTTPClient returns a client that applies the session to every request and
// redirect hop. With a non-nil egress, request credentials still pass the public
// policy; a custom proxy is rejected until it can be chained with that policy.
func (s *Session) HTTPClient(base *http.Client, egress *Egress) (*http.Client, error) {
	if s == nil {
		if egress != nil {
			return egress.HTTPClient(base), nil
		}
		if base == nil {
			return &http.Client{}, nil
		}
		return base, nil
	}
	if s.proxy != nil && egress != nil {
		return nil, errors.New("session proxy requires network.policy=any; public egress cannot inspect proxied destinations")
	}
	client := &http.Client{}
	if base != nil {
		client = &http.Client{
			Transport:     base.Transport,
			CheckRedirect: base.CheckRedirect,
			Jar:           base.Jar,
			Timeout:       base.Timeout,
		}
	}
	transport := defaultHTTPTransport()
	if existing, ok := client.Transport.(*http.Transport); ok && existing != nil {
		transport = existing.Clone()
	}
	if s.tls != nil {
		transport.TLSClientConfig = s.tls.Clone()
	}
	if s.proxy != nil {
		transport.Proxy = http.ProxyURL(s.proxy)
	}
	client.Transport = transport
	if egress != nil {
		client = egress.HTTPClient(client)
	}
	client.Transport = sessionTransport{session: s, base: client.Transport}
	if client.Jar == nil {
		client.Jar = s.jar
	}
	return client, nil
}

func defaultHTTPTransport() *http.Transport {
	if transport, ok := http.DefaultTransport.(*http.Transport); ok {
		return transport.Clone()
	}
	return &http.Transport{}
}

type sessionTransport struct {
	session *Session
	base    http.RoundTripper
}

func (t sessionTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	t.session.StripManagedHeaders(clone.Header)
	t.session.ApplyHeaders(clone.Header, clone.URL)
	return t.base.RoundTrip(clone)
}

// ApplyHeaders adds rules matching the request origin. Callers that reuse a
// request across redirects must first StripManagedHeaders.
func (s *Session) ApplyHeaders(headers http.Header, target *url.URL) {
	if s == nil || headers == nil || target == nil {
		return
	}
	origin, err := originFromURL(target)
	if err != nil {
		return
	}
	for _, rule := range s.rules {
		if rule.origin == origin {
			headers.Set(rule.name, rule.value)
		}
	}
}

// StripManagedHeaders removes every header name controlled by the session. It is
// what prevents net/http's redirect header copy from leaking credentials to a
// different origin.
func (s *Session) StripManagedHeaders(headers http.Header) {
	if s == nil || headers == nil {
		return
	}
	for name := range s.managed {
		headers.Del(name)
	}
}

// HasHeaders reports whether the session carries any request header rule.
func (s *Session) HasHeaders() bool {
	return s != nil && len(s.rules) > 0
}

// HeadersFor is the req/v3-compatible view of the session rules for one URL.
func (s *Session) HeadersFor(rawURL string) map[string]string {
	if s == nil {
		return nil
	}
	target, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil
	}
	headers := http.Header{}
	s.ApplyHeaders(headers, target)
	out := make(map[string]string, len(headers))
	for name, values := range headers {
		if len(values) > 0 {
			out[name] = values[0]
		}
	}
	return out
}

// StripHeadersForRedirect removes managed credentials from a redirected request
// and re-applies only rules that match its destination origin.
func (s *Session) StripHeadersForRedirect(request *http.Request) {
	if s == nil || request == nil {
		return
	}
	s.StripManagedHeaders(request.Header)
	s.ApplyHeaders(request.Header, request.URL)
}

func (s *Session) Proxy() *url.URL {
	if s == nil || s.proxy == nil {
		return nil
	}
	copy := *s.proxy
	return &copy
}

func (s *Session) TLSConfig() *tls.Config {
	if s == nil || s.tls == nil {
		return nil
	}
	return s.tls.Clone()
}

func (s *Session) CookieJar() http.CookieJar {
	if s == nil {
		return nil
	}
	return s.jar
}

// BrowserCookies returns domain-matching cookies for CDP Network.setCookies.
// The browser still enforces domain, path and Secure scoping for subresources.
func (s *Session) BrowserCookies(rawURL string) []map[string]any {
	if s == nil {
		return nil
	}
	target, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || target.Hostname() == "" {
		return nil
	}
	host := strings.ToLower(target.Hostname())
	out := []map[string]any{}
	for _, cookie := range s.cookies {
		domain := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(cookie.Domain)), ".")
		if domain != host && !strings.HasSuffix(host, "."+domain) {
			continue
		}
		value := map[string]any{
			"name": cookie.Name, "value": cookie.Value,
			"domain": cookie.Domain, "path": sessionCookiePath(cookie.Path),
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

func sessionCookiePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || !strings.HasPrefix(path, "/") {
		return "/"
	}
	return path
}

func basicAuthToken(username, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}

func canonicalOrigin(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("parse origin: %w", err)
	}
	return canonicalOriginURL(parsed)
}

func canonicalOriginURL(parsed *url.URL) (string, error) {
	if parsed != nil && (parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/")) {
		return "", errors.New("origin must contain only scheme, host and optional port")
	}
	return originFromURL(parsed)
}

func originFromURL(parsed *url.URL) (string, error) {
	if parsed == nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("origin must be an absolute http or https origin")
	}
	host := strings.ToLower(parsed.Hostname())
	if port := parsed.Port(); port != "" && !((parsed.Scheme == "http" && port == "80") || (parsed.Scheme == "https" && port == "443")) {
		host = host + ":" + port
	}
	return parsed.Scheme + "://" + host, nil
}

// Origin returns the canonical scheme://host[:port] identity for a URL.
func Origin(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	origin, err := originFromURL(parsed)
	if err != nil {
		return ""
	}
	return origin
}

// SanitizeURL removes URL-embedded credentials and fragments for display and
// storage while leaving the transport target intact when it has no credentials.
func SanitizeURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		return "", errors.New("URL must be absolute")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("URL must use http or https")
	}
	parsed.User = nil
	parsed.Fragment, parsed.RawFragment = "", ""
	return parsed.String(), nil
}

// SplitURLCredentials extracts userinfo as a Basic authorization rule for the
// URL origin and returns a sanitized URL. The secret never enters URL fields.
func SplitURLCredentials(raw string) (sanitized string, rule SessionHeader, ok bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.User == nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		return raw, SessionHeader{}, false
	}
	origin := Origin(parsed)
	if origin == "" {
		return raw, SessionHeader{}, false
	}
	username := parsed.User.Username()
	password, _ := parsed.User.Password()
	parsed.User = nil
	parsed.Fragment, parsed.RawFragment = "", ""
	return parsed.String(), SessionHeader{Origin: origin, Name: "Authorization", Value: "Basic " + basicAuthToken(username, password)}, true
}

type sessionContextKey struct{}

// WithSession attaches a caller-owned navigation session to one operation.
func WithSession(ctx context.Context, session *Session) context.Context {
	if session == nil {
		return ctx
	}
	return context.WithValue(ctx, sessionContextKey{}, session)
}

// SessionFromContext returns the operation's navigation session, if any.
func SessionFromContext(ctx context.Context) *Session {
	if ctx == nil {
		return nil
	}
	session, _ := ctx.Value(sessionContextKey{}).(*Session)
	return session
}
