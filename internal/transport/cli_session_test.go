package transport

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/platform"

	_ "modernc.org/sqlite"
)

func TestSessionLoginWritesReusableBrowserProfile(t *testing.T) {
	profile := t.TempDir()
	profileDir := filepath.Join(profile, "Default")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(profileDir, "Cookies"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE cookies (host_key TEXT, name TEXT, value TEXT, encrypted_value BLOB, path TEXT, expires_utc INTEGER, is_secure INTEGER, is_httponly INTEGER, samesite INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO cookies VALUES ('.example.com','sid','login-cookie','','/',0,1,1,1)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "session.json")
	runner := Runner{loadConfig: config.Load, storeRoot: t.TempDir()}
	var stdout, stderr bytes.Buffer
	code := runner.Run([]string{
		"session", "login",
		"--profile", profile,
		"--url", "https://example.com/private",
		"--browser", "/bin/true",
		"--session", out,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("login failed: %d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("session mode=%o", info.Mode().Perm())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		BrowserProfile struct {
			Path    string `json:"path"`
			Profile string `json:"profile"`
		} `json:"browser_profile"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.BrowserProfile.Path != profile || payload.BrowserProfile.Profile != "Default" {
		t.Fatalf("profile=%+v", payload.BrowserProfile)
	}
	loaded, err := config.LoadSession(out)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.BrowserProfile == nil {
		t.Fatalf("loaded session lost browser profile: %+v", loaded)
	}
	session, err := platform.NewSession(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if len(session.BrowserCookies("https://example.com/")) == 0 {
		t.Fatal("loaded session did not import profile cookies")
	}
	if stdout.String() == "" {
		t.Fatal("expected login summary")
	}
}

func TestSessionLoginFailedBrowserLaunchDoesNotWriteSession(t *testing.T) {
	profile := t.TempDir()
	profileDir := filepath.Join(profile, "Default")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(profileDir, "Cookies"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE cookies (host_key TEXT, name TEXT, value TEXT, encrypted_value BLOB, path TEXT, expires_utc INTEGER, is_secure INTEGER, is_httponly INTEGER, samesite INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO cookies VALUES ('.example.com','sid','login-cookie','','/',0,1,1,1)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "session.json")
	runner := Runner{loadConfig: config.Load, storeRoot: t.TempDir()}
	var stdout, stderr bytes.Buffer
	code := runner.Run([]string{
		"session", "login",
		"--profile", profile,
		"--url", "https://example.com/",
		"--browser", filepath.Join(t.TempDir(), "missing-browser"),
		"--session", out,
	}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("login unexpectedly succeeded: stdout=%q", stdout.String())
	}
	if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("session file written after failed launch: %v", err)
	}
}
