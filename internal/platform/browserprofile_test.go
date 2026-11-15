package platform

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestImportBrowserCookiesAndUseThem(t *testing.T) {
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
	encrypted := encryptTestCookieForHost(t, "example.com", "encrypted-secret")
	if _, err := db.Exec(`INSERT INTO cookies VALUES ('.example.com','sid','plain','', '/',0,1,1,1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO cookies VALUES ('example.com','token','',?,'/',0,0,0,2)`, encrypted); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO cookies VALUES ('127.0.0.1','local','plain','', '/',0,0,0,0)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	imported, err := ImportBrowserCookies(BrowserProfileConfig{Path: profile})
	if err != nil {
		t.Fatal(err)
	}
	if len(imported.Cookies) != 3 {
		t.Fatalf("imported=%d notes=%v", len(imported.Cookies), imported.Notes)
	}
	values := map[string]string{}
	for _, cookie := range imported.Cookies {
		values[cookie.Name] = cookie.Value
	}
	if values["sid"] != "plain" || values["token"] != "encrypted-secret" {
		t.Fatalf("values=%v", values)
	}
	session, err := NewSession(SessionConfig{BrowserProfile: &BrowserProfileConfig{Path: profile}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Cookie") == "" || !strings.Contains(req.Header.Get("Cookie"), "local=plain") {
			http.Error(w, "missing cookie", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client, err := session.HTTPClient(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close() //nolint:errcheck
	if response.StatusCode != http.StatusOK {
		t.Fatalf("cookie-authenticated request status=%d", response.StatusCode)
	}
}

func encryptTestCookieForHost(t *testing.T, host, value string) []byte {
	t.Helper()
	sum := sha256.Sum256([]byte(host))
	plain := append(sum[:], []byte(value)...)
	return encryptTestPlaintext(t, plain)
}

func encryptTestPlaintext(t *testing.T, plain []byte) []byte {
	t.Helper()
	key, err := pbkdf2.Key(sha1.New, "peanuts", []byte("saltysalt"), 1, 16)
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	padding := aes.BlockSize - len(plain)%aes.BlockSize
	plain = append(append([]byte{}, plain...), []byte(strings.Repeat(string(rune(padding)), padding))...)
	encrypted := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, []byte("                ")).CryptBlocks(encrypted, plain)
	return append([]byte("v10"), encrypted...)
}

func TestChromeSameSiteUnspecifiedIsNotNone(t *testing.T) {
	if got := chromeSameSite("-1"); got != "" {
		t.Fatalf("undefined SameSite mapped to %q, want empty", got)
	}
	if got := chromeSameSite("0"); got != "none" {
		t.Fatalf("no-restriction SameSite mapped to %q, want none", got)
	}
}

func TestNewSessionExplicitCookieOverridesProfile(t *testing.T) {
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
	if _, err := db.Exec(`INSERT INTO cookies VALUES ('127.0.0.1','sid','profile-value','','/',0,0,0,0)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	session, err := NewSession(SessionConfig{
		BrowserProfile: &BrowserProfileConfig{Path: profile},
		Cookies: []SessionCookie{{
			Name: "sid", Value: "explicit-value", Domain: "127.0.0.1", Path: "/",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if cookie, err := req.Cookie("sid"); err != nil || cookie.Value != "explicit-value" {
			t.Errorf("cookie=%v err=%v", cookie, err)
		}
	}))
	defer server.Close()
	client, err := session.HTTPClient(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
}

func TestNewSessionRejectsEmptyBrowserProfile(t *testing.T) {
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
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSession(SessionConfig{BrowserProfile: &BrowserProfileConfig{Path: profile}}); err == nil {
		t.Fatal("empty browser profile produced a session")
	}
}

func TestNewSessionAllowsEmptyProfileWithExplicitCookie(t *testing.T) {
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
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSession(SessionConfig{
		BrowserProfile: &BrowserProfileConfig{Path: profile},
		Cookies:        []SessionCookie{{Name: "sid", Value: "explicit", Domain: "127.0.0.1", Path: "/"}},
	}); err != nil {
		t.Fatalf("explicit cookie should not be rejected with an empty profile: %v", err)
	}
}
