package transport

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"strings"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/platform"
)

// sessionFlags is the CLI surface for one navigation session. The file is the
// persistent, origin-scoped form; the repeated flags are convenience overrides
// for the command target and are never echoed back by receipts or errors.
type sessionFlags struct {
	path               string
	headers            headerValues
	cookies            cookieValues
	proxy              string
	insecure           bool
	browserProfile     string
	browserProfileName string
}

type headerValues []string

func (h *headerValues) String() string { return strings.Join(*h, ", ") }
func (h *headerValues) Set(value string) error {
	*h = append(*h, value)
	return nil
}

type cookieValues []string

func (c *cookieValues) String() string { return strings.Join(*c, ", ") }
func (c *cookieValues) Set(value string) error {
	*c = append(*c, value)
	return nil
}

func (s *sessionFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&s.path, "session", "", "path to a navigation-session JSON file (default: NEEDLEX_SESSION)")
	fs.Var(&s.headers, "header", "request header NAME: VALUE for the target origin; repeat for multiple headers")
	fs.Var(&s.cookies, "cookie", "cookie NAME=VALUE for the target host; repeat for multiple cookies")
	fs.StringVar(&s.proxy, "proxy", "", "HTTP(S) or SOCKS5 proxy URL; requires network.policy=any")
	fs.BoolVar(&s.insecure, "insecure", false, "disable TLS certificate verification for this session (explicit opt-in)")
	fs.StringVar(&s.browserProfile, "browser-profile", "", "import cookies from a Chromium user-data or profile directory")
	fs.StringVar(&s.browserProfileName, "browser-profile-name", "Default", "profile name inside a Chromium user-data directory")
}

// load returns the target URL with userinfo removed and a session that applies
// exactly the caller-provided credentials. A nil session means no navigation
// credentials were supplied.
func (s *sessionFlags) load(cfg config.Config, targetURL string) (*platform.Session, string, error) {
	path := config.SessionPath(s.path, cfg.SessionFile)
	sessionConfig := platform.SessionConfig{}
	if path != "" {
		loaded, err := config.LoadSession(path)
		if err != nil {
			return nil, targetURL, err
		}
		sessionConfig = loaded
	}
	if err := s.applyTarget(&sessionConfig, targetURL); err != nil {
		return nil, targetURL, err
	}
	if s.proxy != "" {
		sessionConfig.ProxyURL = s.proxy
	}
	if s.insecure {
		sessionConfig.TLS.InsecureSkipVerify = true
	}
	if strings.TrimSpace(s.browserProfile) != "" {
		sessionConfig.BrowserProfile = &platform.BrowserProfileConfig{
			Path: strings.TrimSpace(s.browserProfile), Profile: strings.TrimSpace(s.browserProfileName),
		}
	}
	if !sessionConfigured(sessionConfig) {
		sanitized, _ := platform.SanitizeURL(targetURL)
		if sanitized == "" {
			return nil, targetURL, nil
		}
		return nil, sanitized, nil
	}
	session, err := platform.NewSession(sessionConfig)
	if err != nil {
		return nil, targetURL, err
	}
	sanitized, err := platform.SanitizeURL(targetURL)
	if err != nil {
		return nil, targetURL, err
	}
	return session, sanitized, nil
}

func (s *sessionFlags) applyTarget(sessionConfig *platform.SessionConfig, targetURL string) error {
	if strings.TrimSpace(targetURL) == "" {
		if len(s.headers) > 0 || len(s.cookies) > 0 {
			return errors.New("--header and --cookie require a target URL origin")
		}
		return nil
	}
	parsedTarget, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil || parsedTarget.Hostname() == "" || (parsedTarget.Scheme != "http" && parsedTarget.Scheme != "https") {
		if len(s.headers) > 0 || len(s.cookies) > 0 {
			return errors.New("--header and --cookie require an absolute HTTP(S) target URL")
		}
		return nil
	}
	sanitized, rule, ok := platform.SplitURLCredentials(targetURL)
	if ok {
		sessionConfig.Headers = append(sessionConfig.Headers, rule)
	}
	parsed, err := url.Parse(strings.TrimSpace(sanitized))
	if err != nil || parsed.Hostname() == "" {
		return errors.New("target URL is not absolute")
	}
	origin := platform.Origin(parsed)
	if origin == "" {
		return errors.New("target URL has no valid origin")
	}
	for _, raw := range s.headers {
		name, value, ok := strings.Cut(raw, ":")
		if !ok || strings.TrimSpace(name) == "" {
			return errors.New("--header requires NAME: VALUE")
		}
		sessionConfig.Headers = append(sessionConfig.Headers, platform.SessionHeader{
			Origin: origin, Name: strings.TrimSpace(name), Value: strings.TrimSpace(value),
		})
	}
	for _, raw := range s.cookies {
		name, value, ok := strings.Cut(raw, "=")
		if !ok || strings.TrimSpace(name) == "" {
			return errors.New("--cookie requires NAME=VALUE")
		}
		sessionConfig.Cookies = append(sessionConfig.Cookies, platform.SessionCookie{
			Name: strings.TrimSpace(name), Value: value, Domain: parsed.Hostname(),
			Path: "/", Secure: parsed.Scheme == "https",
		})
	}
	return nil
}

// sessionFromMCPArgs accepts a session file plus a minimal inline credential
// surface. Inline headers are scoped to the target origin; inline cookies are
// scoped to the target host.
func sessionFromMCPArgs(args map[string]any, cfg config.Config, targetURL string) (*platform.Session, string, error) {
	path := config.SessionPath(stringArg(args, "session_path"), cfg.SessionFile)
	sessionConfig := platform.SessionConfig{}
	if path != "" {
		loaded, err := config.LoadSession(path)
		if err != nil {
			return nil, targetURL, err
		}
		sessionConfig = loaded
	}
	if proxy := stringArg(args, "proxy"); proxy != "" {
		sessionConfig.ProxyURL = proxy
	}
	profile, err := mcpBrowserProfileArg(args)
	if err != nil {
		return nil, targetURL, err
	}
	if profile != nil {
		sessionConfig.BrowserProfile = profile
	}
	if boolArg(args, "insecure_tls") {
		sessionConfig.TLS.InsecureSkipVerify = true
	}
	if err := applyMCPTargetSession(&sessionConfig, args, targetURL); err != nil {
		return nil, targetURL, err
	}
	if !sessionConfigured(sessionConfig) {
		sanitized, _ := platform.SanitizeURL(targetURL)
		if sanitized == "" {
			return nil, targetURL, nil
		}
		return nil, sanitized, nil
	}
	session, err := platform.NewSession(sessionConfig)
	if err != nil {
		return nil, targetURL, err
	}
	sanitized, err := platform.SanitizeURL(targetURL)
	if err != nil {
		return nil, targetURL, err
	}
	return session, sanitized, nil
}

func sessionConfigured(cfg platform.SessionConfig) bool {
	return len(cfg.Headers) > 0 || len(cfg.Cookies) > 0 || cfg.BrowserProfile != nil ||
		cfg.ProxyURL != "" || cfg.TLS.InsecureSkipVerify || cfg.TLS.CAFile != "" || cfg.TLS.ClientCertFile != ""
}

func mcpBrowserProfileArg(args map[string]any) (*platform.BrowserProfileConfig, error) {
	value, ok := args["browser_profile"]
	if !ok || value == nil {
		return nil, nil
	}
	raw, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("browser_profile must be an object with path and optional profile")
	}
	path := stringArg(raw, "path")
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("browser_profile.path must not be empty")
	}
	return &platform.BrowserProfileConfig{Path: path, Profile: stringArg(raw, "profile")}, nil
}

func applyMCPTargetSession(sessionConfig *platform.SessionConfig, args map[string]any, targetURL string) error {
	headers, err := stringMapArg(args, "headers")
	if err != nil {
		return err
	}
	cookies, err := mcpCookiesArg(args, "cookies")
	if err != nil {
		return err
	}
	if (len(headers) > 0 || len(cookies) > 0) && strings.TrimSpace(targetURL) == "" {
		return errors.New("headers and cookies require a target URL origin")
	}
	if strings.TrimSpace(targetURL) != "" {
		sanitized, rule, ok := platform.SplitURLCredentials(targetURL)
		if ok {
			sessionConfig.Headers = append(sessionConfig.Headers, rule)
		}
		parsed, err := url.Parse(strings.TrimSpace(sanitized))
		if err != nil || platform.Origin(parsed) == "" {
			return errors.New("target URL has no valid origin")
		}
		origin := platform.Origin(parsed)
		for name, value := range headers {
			sessionConfig.Headers = append(sessionConfig.Headers, platform.SessionHeader{
				Origin: origin, Name: name, Value: value,
			})
		}
		for _, cookie := range cookies {
			if cookie.Domain == "" {
				cookie.Domain = parsed.Hostname()
			}
			sessionConfig.Cookies = append(sessionConfig.Cookies, cookie)
		}
	}
	return nil
}

func stringMapArg(args map[string]any, key string) (map[string]string, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return nil, nil
	}
	raw, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object of header names to string values", key)
	}
	out := make(map[string]string, len(raw))
	for name, item := range raw {
		text, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s.%s must be a string", key, name)
		}
		out[name] = text
	}
	return out, nil
}

func mcpCookiesArg(args map[string]any, key string) ([]platform.SessionCookie, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return nil, nil
	}
	raw, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", key)
	}
	out := make([]platform.SessionCookie, 0, len(raw))
	for index, item := range raw {
		object, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be an object", key, index)
		}
		cookie := platform.SessionCookie{
			Name: stringArg(object, "name"), Value: stringArg(object, "value"),
			Domain: stringArg(object, "domain"), Path: stringArg(object, "path"),
			Secure: boolArg(object, "secure"), HTTPOnly: boolArg(object, "http_only"),
			SameSite: stringArg(object, "same_site"),
		}
		if cookie.Name == "" && cookie.Value == "" {
			return nil, fmt.Errorf("%s[%d] needs name and value", key, index)
		}
		out = append(out, cookie)
	}
	return out, nil
}

func sessionToolSchema() map[string]any {
	return map[string]any{
		"session_path": map[string]any{"type": "string", "description": "Optional path to a navigation-session JSON file. Values may use env:NAME or file:PATH references; otherwise set NEEDLEX_SESSION."},
		"headers": map[string]any{
			"type": "object", "additionalProperties": map[string]any{"type": "string"},
			"description": "Optional request headers scoped to the target origin, for example {\"Authorization\":\"Bearer ...\"}. Never echoed in output.",
		},
		"cookies": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"name", "value"},
				"properties": map[string]any{
					"name": map[string]any{"type": "string"}, "value": map[string]any{"type": "string"},
					"domain":    map[string]any{"type": "string", "description": "Cookie domain; defaults to the target host."},
					"path":      map[string]any{"type": "string", "default": "/"},
					"secure":    map[string]any{"type": "boolean"},
					"http_only": map[string]any{"type": "boolean"},
					"same_site": map[string]any{"type": "string", "enum": []string{"lax", "strict", "none"}},
				},
			},
			"description": "Optional cookies for the target host. Domain scoping is enforced by the HTTP client and the browser.",
		},
		"browser_profile": map[string]any{
			"type": "object", "additionalProperties": false, "required": []string{"path"},
			"properties": map[string]any{
				"path":    map[string]any{"type": "string", "description": "Chromium user-data directory or profile directory."},
				"profile": map[string]any{"type": "string", "description": "Profile inside a user-data directory; defaults to Default."},
			},
			"description": "Import cookies from a local Chromium browser profile. The profile is read server-side.",
		},
		"proxy":        map[string]any{"type": "string", "description": "Optional HTTP(S) or SOCKS5 proxy; requires network.policy=any."},
		"insecure_tls": map[string]any{"type": "boolean", "description": "Explicitly disable TLS certificate verification for this operation."},
	}
}

func withSessionToolSchema(properties map[string]any) map[string]any {
	for key, value := range sessionToolSchema() {
		properties[key] = value
	}
	return properties
}
