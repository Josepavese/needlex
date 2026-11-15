package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/platform"
)

// SessionFileEnv names the default navigation-session file. A command-line
// --session path and an explicit session_path argument take precedence.
const SessionFileEnv = "NEEDLEX_SESSION"

type sessionFile struct {
	Headers        []sessionHeader        `json:"headers,omitempty"`
	Cookies        []sessionCookie        `json:"cookies,omitempty"`
	BrowserProfile *sessionBrowserProfile `json:"browser_profile,omitempty"`
	Proxy          string                 `json:"proxy,omitempty"`
	TLS            sessionTLS             `json:"tls,omitempty"`
}

type sessionBrowserProfile struct {
	Path    string `json:"path"`
	Profile string `json:"profile,omitempty"`
}

type sessionHeader struct {
	Origin string `json:"origin"`
	Name   string `json:"name"`
	Value  string `json:"value"`
}

type sessionCookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	HostOnly *bool  `json:"host_only,omitempty"`
	Path     string `json:"path,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	HTTPOnly bool   `json:"http_only,omitempty"`
	SameSite string `json:"same_site,omitempty"`
	Expires  string `json:"expires,omitempty"`
}

type sessionTLS struct {
	CAFile             string `json:"ca_file,omitempty"`
	ClientCertFile     string `json:"client_cert_file,omitempty"`
	ClientKeyFile      string `json:"client_key_file,omitempty"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify,omitempty"`
}

// SessionPath resolves the command-line path, then NEEDLEX_SESSION, then an
// optional configured path. Empty means no session was configured.
func SessionPath(flagPath, configuredPath string) string {
	for _, candidate := range []string{flagPath, os.Getenv(SessionFileEnv), configuredPath} {
		if strings.TrimSpace(candidate) != "" {
			return strings.TrimSpace(candidate)
		}
	}
	return ""
}

// LoadSession reads a standalone navigation session. Values may be literal,
// env:NAME (resolved from the environment) or file:PATH (relative to the
// session file). Resolution happens here, before any transport is built, and no
// resolved secret is written to a config, trace or receipt.
func LoadSession(path string) (platform.SessionConfig, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return platform.SessionConfig{}, errors.New("session file path must not be empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return platform.SessionConfig{}, fmt.Errorf("read session file: %w", err)
	}
	var raw sessionFile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return platform.SessionConfig{}, fmt.Errorf("parse session file: %w", err)
	}
	baseDir := filepath.Dir(path)
	resolvePath := func(value string) string {
		value = strings.TrimSpace(value)
		if value == "" || filepath.IsAbs(value) {
			return value
		}
		return filepath.Join(baseDir, value)
	}
	out := platform.SessionConfig{ProxyURL: raw.Proxy, TLS: platform.SessionTLS{
		CAFile: resolvePath(raw.TLS.CAFile), ClientCertFile: resolvePath(raw.TLS.ClientCertFile),
		ClientKeyFile: resolvePath(raw.TLS.ClientKeyFile), InsecureSkipVerify: raw.TLS.InsecureSkipVerify,
	}}
	if raw.BrowserProfile != nil {
		profilePath := strings.TrimSpace(raw.BrowserProfile.Path)
		if profilePath == "" {
			return platform.SessionConfig{}, errors.New("browser_profile.path must not be empty")
		}
		if !filepath.IsAbs(profilePath) {
			profilePath = filepath.Join(baseDir, profilePath)
		}
		out.BrowserProfile = &platform.BrowserProfileConfig{Path: profilePath, Profile: raw.BrowserProfile.Profile}
	}
	if strings.TrimSpace(raw.Proxy) != "" {
		proxy, err := resolveSessionValue(raw.Proxy, baseDir)
		if err != nil {
			return platform.SessionConfig{}, fmt.Errorf("resolve session proxy: %w", err)
		}
		out.ProxyURL = proxy
	}
	for index, header := range raw.Headers {
		value, err := resolveSessionValue(header.Value, baseDir)
		if err != nil {
			return platform.SessionConfig{}, fmt.Errorf("resolve session header %d (%s): %w", index+1, header.Name, err)
		}
		out.Headers = append(out.Headers, platform.SessionHeader{Origin: header.Origin, Name: header.Name, Value: value})
	}
	for index, cookie := range raw.Cookies {
		value, err := resolveSessionValue(cookie.Value, baseDir)
		if err != nil {
			return platform.SessionConfig{}, fmt.Errorf("resolve session cookie %d (%s): %w", index+1, cookie.Name, err)
		}
		expires, err := parseSessionExpiry(cookie.Expires)
		if err != nil {
			return platform.SessionConfig{}, fmt.Errorf("session cookie %s: %w", cookie.Name, err)
		}
		hostOnly := !strings.HasPrefix(strings.TrimSpace(cookie.Domain), ".")
		if cookie.HostOnly != nil {
			hostOnly = *cookie.HostOnly
		}
		out.Cookies = append(out.Cookies, platform.SessionCookie{
			Name: cookie.Name, Value: value, Domain: cookie.Domain, HostOnly: hostOnly, Path: cookie.Path,
			Secure: cookie.Secure, HTTPOnly: cookie.HTTPOnly, SameSite: cookie.SameSite, Expires: expires,
		})
	}
	// Validate the shape here without importing browser cookies; the session is
	// materialized once when the command actually uses it.
	if err := platform.ValidateSessionConfig(out); err != nil {
		return platform.SessionConfig{}, fmt.Errorf("invalid session %s: %w", filepath.Base(path), err)
	}
	return out, nil
}

func parseSessionExpiry(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, errors.New("expires must be an RFC3339 timestamp")
	}
	return parsed, nil
}

// resolveSessionValue resolves the two secret indirections supported by session
// files. A value that starts with env: or file: can be made literal with the
// literal: prefix.
func resolveSessionValue(value, baseDir string) (string, error) {
	switch {
	case strings.HasPrefix(value, "literal:"):
		return strings.TrimPrefix(value, "literal:"), nil
	case strings.HasPrefix(value, "env:"):
		name := strings.TrimSpace(strings.TrimPrefix(value, "env:"))
		if name == "" {
			return "", errors.New("env reference needs a variable name")
		}
		resolved, ok := os.LookupEnv(name)
		if !ok {
			return "", fmt.Errorf("environment variable %s is not set", name)
		}
		if resolved == "" {
			return "", fmt.Errorf("environment variable %s is empty", name)
		}
		return singleLine(resolved, "environment variable "+name)
	case strings.HasPrefix(value, "file:"):
		path := strings.TrimSpace(strings.TrimPrefix(value, "file:"))
		if path == "" {
			return "", errors.New("file reference needs a path")
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(baseDir, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read secret file: %w", err)
		}
		return singleLine(string(data), "secret file")
	default:
		return value, nil
	}
}

func singleLine(value, source string) (string, error) {
	value = strings.TrimRight(value, "\r\n")
	if strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("%s contains an embedded line break", source)
	}
	return value, nil
}
