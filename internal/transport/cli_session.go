package transport

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/platform"
)

func writeSessionUsage(w io.Writer) {
	writeUsage(w, "needlex session login --profile DIR --url URL [--session PATH] [--browser PATH] [--profile-name Default] [--config PATH]")
}

func (r Runner) runSession(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		writeSessionUsage(stderr)
		return 2
	}
	if args[0] != "login" {
		writeSessionUsage(stderr)
		return 2
	}
	return r.runSessionLogin(args[1:], stdout, stderr)
}

func (r Runner) runSessionLogin(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("session login", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var profile, profileName, targetURL, sessionPath, browserPath, configPath string
	fs.StringVar(&profile, "profile", "", "Chromium user-data directory to use for the login")
	fs.StringVar(&profileName, "profile-name", "Default", "profile directory inside the user-data directory")
	fs.StringVar(&targetURL, "url", "", "login URL to open in the browser")
	fs.StringVar(&sessionPath, "session", "", "session JSON file to write (default: NEEDLEX_SESSION)")
	fs.StringVar(&browserPath, "browser", "", "non-headless Chromium executable")
	fs.StringVar(&configPath, "config", "", "PAL config supplying render.browser_path when --browser is omitted")
	if err := fs.Parse(normalizeArgs(fs, args)); err != nil {
		return 2
	}
	if fs.NArg() != 0 || strings.TrimSpace(profile) == "" || strings.TrimSpace(targetURL) == "" {
		writeSessionUsage(stderr)
		return 2
	}
	if parsed, err := platform.SanitizeURL(targetURL); err != nil || parsed == "" {
		fmt.Fprintln(stderr, "session login: --url must be an absolute HTTP(S) URL")
		return 2
	}
	profileName = strings.TrimSpace(profileName)
	if profileName == "" || strings.ContainsAny(profileName, `/\`) || profileName == "." || profileName == ".." {
		fmt.Fprintln(stderr, "session login: --profile-name is invalid")
		return 2
	}
	profile, err := filepath.Abs(strings.TrimSpace(profile))
	if err != nil {
		return r.reportCLIError(stderr, "session", err, nil)
	}
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return r.reportCLIError(stderr, "session", err, nil)
	}
	cfg, ok := r.loadConfigOrExit(configPath, stderr)
	if !ok {
		return 1
	}
	if strings.TrimSpace(browserPath) == "" {
		browserPath, err = platform.FindBrowserPath(cfg.Render.BrowserPath)
		if err != nil {
			return r.reportCLIError(stderr, "session", err, nil)
		}
	}

	output := config.SessionPath(strings.TrimSpace(sessionPath), cfg.SessionFile)
	if output == "" {
		fmt.Fprintln(stderr, "session login: --session or NEEDLEX_SESSION is required to save the reusable session")
		return 2
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return r.reportCLIError(stderr, "session", err, nil)
	}
	if _, err := os.Stat(output); err == nil {
		fmt.Fprintf(stderr, "session login: refusing to overwrite existing session file %s; choose a new --session path\n", output)
		return 1
	} else if !errors.Is(err, os.ErrNotExist) {
		return r.reportCLIError(stderr, "session", err, nil)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
	defer cancel()
	if err := platform.LaunchBrowserLogin(ctx, browserPath, profile, profileName, targetURL); err != nil {
		if errors.Is(err, platform.ErrBrowserLaunchFailed) {
			return r.reportCLIError(stderr, "session", err, nil)
		}
		// A browser window closed by the user normally exits non-zero; the
		// cookie import below is the useful result, so record but do not abort.
		fmt.Fprintf(stderr, "session login: browser exited: %v\n", err)
	}
	imported, err := platform.ImportBrowserCookies(platform.BrowserProfileConfig{Path: profile, Profile: profileName})
	if err != nil {
		return r.reportCLIError(stderr, "session", err, nil)
	}
	if len(imported.Cookies) == 0 {
		printImportNotes(stderr, imported.Notes)
		fmt.Fprintln(stderr, "session login: no cookies were imported; the session would not authenticate anything and was not written")
		return 1
	}
	if err := writeBrowserProfileSession(output, profile, profileName); err != nil {
		return r.reportCLIError(stderr, "session", err, nil)
	}
	fmt.Fprintf(stdout, "Session saved: %s\nBrowser profile: %s (profile %s)\nCookies imported: %d\n", output, profile, profileName, len(imported.Cookies))
	printImportNotes(stdout, imported.Notes)
	fmt.Fprintf(stdout, "Next: needlex read %s --session %s\n", sanitizedSessionLoginURL(targetURL), output)
	return 0
}

func writeBrowserProfileSession(path, profile, profileName string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	payload := map[string]any{
		"browser_profile": map[string]any{"path": profile, "profile": profileName},
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close() //nolint:errcheck
		return err
	}
	return file.Close()
}

func printImportNotes(w io.Writer, notes []string) {
	for index, note := range notes {
		if index == 8 {
			fmt.Fprintf(w, "Warning: %d additional skipped-cookie notes omitted\n", len(notes)-index)
			return
		}
		fmt.Fprintf(w, "Warning: %s\n", note)
	}
}

func sanitizedSessionLoginURL(raw string) string {
	sanitized, err := platform.SanitizeURL(raw)
	if err != nil || sanitized == "" {
		return "<url>"
	}
	if parsed, err := url.Parse(sanitized); err == nil && parsed.Host != "" {
		return parsed.Scheme + "://" + parsed.Host + "/"
	}
	return sanitized
}
