package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSessionResolvesEnvAndFileReferences(t *testing.T) {
	t.Setenv("NEEDLEX_TEST_TOKEN", "Bearer env-secret")
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "cookie.txt")
	if err := os.WriteFile(secretPath, []byte("file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sessionPath := filepath.Join(dir, "session.json")
	content := `{
  "headers": [{"origin": "https://api.example.com", "name": "Authorization", "value": "env:NEEDLEX_TEST_TOKEN"}],
  "cookies": [{"name": "sid", "value": "file:cookie.txt", "domain": ".example.com", "secure": true}],
  "proxy": "http://127.0.0.1:3128"
}`
	if err := os.WriteFile(sessionPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSession(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Headers) != 1 || loaded.Headers[0].Value != "Bearer env-secret" {
		t.Fatalf("headers=%+v", loaded.Headers)
	}
	if len(loaded.Cookies) != 1 || loaded.Cookies[0].Value != "file-secret" {
		t.Fatalf("cookies=%+v", loaded.Cookies)
	}
	if loaded.ProxyURL != "http://127.0.0.1:3128" {
		t.Fatalf("proxy=%q", loaded.ProxyURL)
	}
}

func TestLoadSessionRejectsUnknownFieldsAndMissingEnv(t *testing.T) {
	dir := t.TempDir()
	unknown := filepath.Join(dir, "unknown.json")
	if err := os.WriteFile(unknown, []byte(`{"header": []}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSession(unknown); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unexpected error: %v", err)
	}
	missing := filepath.Join(dir, "missing.json")
	if err := os.WriteFile(missing, []byte(`{"headers":[{"origin":"https://example.com","name":"Authorization","value":"env:NEEDLEX_TEST_MISSING"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSession(missing); err == nil || !strings.Contains(err.Error(), "NEEDLEX_TEST_MISSING") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSessionPathPrecedence(t *testing.T) {
	t.Setenv(SessionFileEnv, "env-session.json")
	if got := SessionPath("flag-session.json", "config-session.json"); got != "flag-session.json" {
		t.Fatalf("flag precedence: %q", got)
	}
	if got := SessionPath("", "config-session.json"); got != "env-session.json" {
		t.Fatalf("env precedence: %q", got)
	}
	t.Setenv(SessionFileEnv, "")
	if got := SessionPath("", "config-session.json"); got != "config-session.json" {
		t.Fatalf("config fallback: %q", got)
	}
}

func TestLoadSessionResolvesTLSPathsRelativeToFile(t *testing.T) {
	dir := t.TempDir()
	systemCA, err := os.ReadFile("/etc/ssl/certs/ca-certificates.crt")
	if err != nil {
		t.Skipf("system CA bundle unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ca.pem"), systemCA, 0o600); err != nil {
		t.Fatal(err)
	}
	sessionPath := filepath.Join(dir, "session.json")
	if err := os.WriteFile(sessionPath, []byte(`{"tls":{"ca_file":"ca.pem"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSession(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(loaded.TLS.CAFile) || filepath.Dir(loaded.TLS.CAFile) != dir {
		t.Fatalf("ca path=%q, want absolute under %s", loaded.TLS.CAFile, dir)
	}
}

func TestLoadSessionHostOnlyDefaultsFromDomain(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "session.json")
	content := `{"cookies":[{"name":"host","value":"v","domain":"app.example.com"},{"name":"domain","value":"v","domain":".example.com"}]}`
	if err := os.WriteFile(sessionPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSession(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Cookies[0].HostOnly || loaded.Cookies[1].HostOnly {
		t.Fatalf("host-only defaults=%+v", loaded.Cookies)
	}
}
