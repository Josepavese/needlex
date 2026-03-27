package pipeline

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultUserAgent = "needle-x/0.1"

type Acquirer struct {
	Client *http.Client
}

func (a Acquirer) Acquire(ctx context.Context, input AcquireInput) (RawPage, error) {
	if err := validateAcquireInput(input); err != nil {
		return RawPage{}, err
	}

	reqCtx := ctx
	cancel := func() {}
	if input.Timeout > 0 {
		reqCtx, cancel = context.WithTimeout(ctx, input.Timeout)
	}
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, input.URL, nil)
	if err != nil {
		return RawPage{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent(input.UserAgent))

	client := a.Client
	if client == nil {
		client = &http.Client{}
	}

	resp, err := client.Do(req)
	if err != nil {
		return RawPage{}, fmt.Errorf("fetch page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return RawPage{}, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !isAllowedContentType(contentType) {
		return RawPage{}, fmt.Errorf("unsupported content type %q", contentType)
	}

	htmlBody, err := readBounded(resp.Body, input.MaxBytes)
	if err != nil {
		return RawPage{}, err
	}

	return RawPage{
		URL:         input.URL,
		FinalURL:    resp.Request.URL.String(),
		StatusCode:  resp.StatusCode,
		ContentType: contentType,
		HTML:        htmlBody,
		FetchMode:   "http",
		FetchedAt:   time.Now().UTC(),
	}, nil
}

func validateAcquireInput(input AcquireInput) error {
	if strings.TrimSpace(input.URL) == "" {
		return fmt.Errorf("acquire input url must not be empty")
	}
	parsed, err := url.Parse(input.URL)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}
	if input.MaxBytes <= 0 {
		return fmt.Errorf("max bytes must be > 0")
	}
	return nil
}

func userAgent(candidate string) string {
	if strings.TrimSpace(candidate) == "" {
		return defaultUserAgent
	}
	return candidate
}

func isAllowedContentType(contentType string) bool {
	contentType = strings.ToLower(contentType)
	return strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/xhtml+xml")
}

func readBounded(body io.Reader, maxBytes int64) (string, error) {
	limited := io.LimitReader(body, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return "", fmt.Errorf("response exceeds max bytes budget")
	}
	return string(data), nil
}
