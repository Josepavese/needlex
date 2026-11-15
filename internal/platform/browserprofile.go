package platform

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// BrowserProfileConfig identifies a Chromium-family profile whose cookie store
// may be imported into a navigation session. Path is either a user-data
// directory or a profile directory; Profile defaults to "Default".
type BrowserProfileConfig struct {
	Path    string
	Profile string
}

// BrowserCookieImport is the result of reading a browser cookie store. Notes
// describe cookies that were skipped rather than silently discarded.
type BrowserCookieImport struct {
	Cookies []SessionCookie
	Notes   []string
}

const chromeCookieEpochOffsetMicros = int64(11644473600000000)

// ImportBrowserCookies copies the Chromium cookie database to a private
// temporary directory before reading it, so a running browser cannot change the
// result underneath the import and the profile itself is never modified.
func ImportBrowserCookies(cfg BrowserProfileConfig) (BrowserCookieImport, error) {
	cookiePath, err := browserCookiePath(cfg)
	if err != nil {
		return BrowserCookieImport{}, err
	}
	temp, err := os.MkdirTemp("", "needlex-cookie-import-")
	if err != nil {
		return BrowserCookieImport{}, err
	}
	defer os.RemoveAll(temp) //nolint:errcheck
	for _, name := range []string{"Cookies", "Cookies-wal", "Cookies-shm", "Cookies-journal"} {
		source := filepath.Join(filepath.Dir(cookiePath), name)
		if err := copyBrowserFile(source, filepath.Join(temp, name)); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return BrowserCookieImport{}, err
		}
	}
	dbPath := filepath.Join(temp, "Cookies")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return BrowserCookieImport{}, fmt.Errorf("open browser cookie store: %w", err)
	}
	defer db.Close() //nolint:errcheck
	rows, err := db.Query(`SELECT host_key, name, value, encrypted_value, path, expires_utc, is_secure, is_httponly, samesite FROM cookies ORDER BY host_key, name, path`)
	if err != nil {
		return BrowserCookieImport{}, fmt.Errorf("read browser cookies: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var result BrowserCookieImport
	for rows.Next() {
		if len(result.Cookies) >= 4096 {
			result.Notes = append(result.Notes, "cookie import capped at 4096 entries")
			break
		}
		var (
			host, name, path string
			value, encrypted []byte
			expires          sql.NullInt64
			sameSite         any
			secure, httpOnly bool
		)
		if err := rows.Scan(&host, &name, &value, &encrypted, &path, &expires, &secure, &httpOnly, &sameSite); err != nil {
			return BrowserCookieImport{}, fmt.Errorf("scan browser cookie: %w", err)
		}
		cookieValue := string(value)
		if cookieValue == "" && len(encrypted) > 0 {
			decoded, err := decryptChromiumCookie(encrypted, host)
			if err != nil {
				result.Notes = append(result.Notes, fmt.Sprintf("cookie %q on %s skipped: %v", name, host, err))
				continue
			}
			cookieValue = decoded
		}
		if strings.TrimSpace(host) == "" || strings.TrimSpace(name) == "" {
			continue
		}
		cookie := SessionCookie{
			Name: name, Value: cookieValue, Domain: host,
			HostOnly: !strings.HasPrefix(host, "."), Path: path,
			Secure: secure, HTTPOnly: httpOnly, SameSite: chromeSameSite(fmt.Sprint(sameSite)),
		}
		if expires.Valid && expires.Int64 > 0 {
			unixMicros := expires.Int64 - chromeCookieEpochOffsetMicros
			cookie.Expires = time.Unix(0, unixMicros*int64(time.Microsecond)).UTC()
		}
		result.Cookies = append(result.Cookies, cookie)
	}
	if err := rows.Err(); err != nil {
		return BrowserCookieImport{}, fmt.Errorf("iterate browser cookies: %w", err)
	}
	return result, nil
}

func browserCookiePath(cfg BrowserProfileConfig) (string, error) {
	path := strings.TrimSpace(cfg.Path)
	if path == "" {
		return "", errors.New("browser profile path must not be empty")
	}
	path = filepath.Clean(path)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path, nil
	}
	profile := strings.TrimSpace(cfg.Profile)
	if profile == "" {
		profile = "Default"
	}
	if profile == "." || profile == ".." || strings.ContainsAny(profile, `/\\`) {
		return "", errors.New("browser profile name must be a single directory name")
	}
	candidates := []string{
		filepath.Join(path, "Cookies"),
		filepath.Join(path, "Network", "Cookies"),
		filepath.Join(path, profile, "Cookies"),
		filepath.Join(path, profile, "Network", "Cookies"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("browser profile %s has no readable Cookies database", path)
}

func copyBrowserFile(source, destination string) error {
	from, err := os.Open(source)
	if err != nil {
		return err
	}
	defer from.Close() //nolint:errcheck
	to, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(to, from)
	closeErr := to.Close()
	return errors.Join(copyErr, closeErr)
}

func chromeSameSite(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "-1":
		return ""
	case "0", "none":
		return "none"
	case "1", "lax":
		return "lax"
	case "2", "strict":
		return "strict"
	default:
		return ""
	}
}

// decryptChromiumCookie implements the legacy Linux "v10"/"v11" AES-CBC format
// used by Chromium builds that store the cookie key as "peanuts" (for example
// Chrome with --password-store=basic). Keychain/DPAPI/app-bound formats are
// reported as unsupported instead of producing a bogus value.
func decryptChromiumCookie(encrypted []byte, host string) (string, error) {
	if len(encrypted) < 3 {
		return "", errors.New("encrypted value is too short")
	}
	version := string(encrypted[:3])
	switch version {
	case "v10", "v11":
	default:
		return "", fmt.Errorf("unsupported cookie encryption %q", version)
	}
	key, err := pbkdf2.Key(sha1.New, "peanuts", []byte("saltysalt"), 1, 16)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	payload := encrypted[3:]
	if len(payload) == 0 || len(payload)%aes.BlockSize != 0 {
		return "", errors.New("encrypted cookie has an invalid block length")
	}
	plain := make([]byte, len(payload))
	cipher.NewCBCDecrypter(block, []byte("                ")).CryptBlocks(plain, payload)
	plain, err = unpadPKCS7(plain)
	if err != nil {
		return "", err
	}
	// Chrome 130+ with cookie DB version >= 24 prefixes the plaintext with
	// SHA-256(host_key). Strip it only when it matches, so older profiles keep
	// their literal value.
	if len(plain) >= sha256.Size {
		sum := sha256.Sum256([]byte(host))
		if bytes.Equal(plain[:sha256.Size], sum[:]) {
			plain = plain[sha256.Size:]
		}
	}
	return string(plain), nil
}

func unpadPKCS7(value []byte) ([]byte, error) {
	if len(value) == 0 {
		return nil, errors.New("empty decrypted cookie")
	}
	padding := int(value[len(value)-1])
	if padding < 1 || padding > aes.BlockSize || padding > len(value) {
		return nil, errors.New("decrypted cookie has invalid padding")
	}
	for _, b := range value[len(value)-padding:] {
		if int(b) != padding {
			return nil, errors.New("decrypted cookie has invalid padding")
		}
	}
	return value[:len(value)-padding], nil
}
