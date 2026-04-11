package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	req "github.com/imroc/req/v3"
)

const (
	defaultUserAgent        = "needle-x/0.1"
	defaultBrowserUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0 Safari/537.36"
)

type Acquirer struct {
	Client *http.Client
}

type pacingState struct {
	mu       sync.Mutex
	lastSeen map[string]time.Time
}

var globalPacingState = &pacingState{lastSeen: map[string]time.Time{}}

func (a Acquirer) Acquire(ctx context.Context, input AcquireInput) (RawPage, error) {
	if err := validateAcquireInput(input); err != nil {
		return RawPage{}, err
	}

	page, err := a.acquireAttempt(ctx, input, input.Timeout, normalizeFetchProfile(input.Profile))
	if err == nil {
		return page, nil
	}
	if retryProfile := normalizeRetryProfile(input.RetryProfile); shouldRetryOnBlocked(err, retryProfile) {
		retrySleep, waitErr := sleepWithJitter(ctx, input.BlockedRetryBackoff, input.BlockedRetryJitter)
		if waitErr != nil {
			return RawPage{}, waitErr
		}
		page, retryErr := a.acquireAttempt(ctx, input, input.Timeout, retryProfile)
		if retryErr == nil {
			page.RetryCount = 1
			page.RetryReason = "blocked_status"
			page.RetrySleepMS = retrySleep.Milliseconds()
			return page, nil
		}
		err = retryErr
	}
	if !shouldRetryOnTimeout(ctx, err, input.Timeout) {
		return RawPage{}, err
	}

	retryTimeout := expandedTimeout(input.Timeout)
	retrySleep, waitErr := sleepWithJitter(ctx, input.TimeoutRetryBackoff, input.TimeoutRetryJitter)
	if waitErr != nil {
		return RawPage{}, waitErr
	}
	page, retryErr := a.acquireAttempt(ctx, input, retryTimeout, normalizeFetchProfile(input.Profile))
	if retryErr == nil {
		page.RetryCount = 1
		page.RetryReason = "timeout"
		page.RetrySleepMS = retrySleep.Milliseconds()
		return page, nil
	}
	return RawPage{}, retryErr
}

func (a Acquirer) acquireAttempt(ctx context.Context, input AcquireInput, timeout time.Duration, profile string) (RawPage, error) {
	attemptInput := input
	attemptInput.Timeout = timeout
	attemptInput.Profile = profile
	pacingDelay, err := applyPerHostPacing(ctx, attemptInput.URL, attemptInput.PerHostMinGap, attemptInput.PerHostJitter)
	if err != nil {
		return RawPage{}, err
	}
	if !supportsBrowserImpersonation(attemptInput.URL) {
		page, err := a.acquireWithHTTP(ctx, attemptInput, profile)
		if err == nil {
			page.HostPacingMS = pacingDelay.Milliseconds()
		}
		return page, err
	}
	switch profile {
	case "browser_like", "hardened":
		page, err := a.acquireWithReq(ctx, attemptInput, profile)
		if err == nil {
			page.HostPacingMS = pacingDelay.Milliseconds()
			return page, nil
		}
		if shouldFallbackToHTTP(err) {
			page, httpErr := a.acquireWithHTTP(ctx, attemptInput, profile)
			if httpErr == nil {
				page.HostPacingMS = pacingDelay.Milliseconds()
			}
			return page, httpErr
		}
		return RawPage{}, err
	default:
		page, err := a.acquireWithHTTP(ctx, attemptInput, profile)
		if err == nil {
			page.HostPacingMS = pacingDelay.Milliseconds()
		}
		return page, err
	}
}

func supportsBrowserImpersonation(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Scheme, "https")
}

func (a Acquirer) acquireWithHTTP(ctx context.Context, input AcquireInput, profile string) (RawPage, error) {
	reqCtx, cancel := requestContext(ctx, input.Timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(reqCtx, http.MethodGet, input.URL, nil)
	if err != nil {
		return RawPage{}, fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("User-Agent", userAgent(input.UserAgent, profile))

	client := a.Client
	if client == nil {
		client = &http.Client{}
	}

	resp, err := client.Do(request)
	if err != nil {
		return RawPage{}, fmt.Errorf("fetch page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return RawPage{}, statusError(resp.StatusCode)
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
		URL:          input.URL,
		FinalURL:     resp.Request.URL.String(),
		StatusCode:   resp.StatusCode,
		ContentType:  contentType,
		HTML:         htmlBody,
		FetchMode:    "http",
		FetchProfile: profile,
		FetchedAt:    time.Now().UTC(),
	}, nil
}

func (a Acquirer) acquireWithReq(ctx context.Context, input AcquireInput, profile string) (RawPage, error) {
	reqCtx, cancel := requestContext(ctx, input.Timeout)
	defer cancel()

	client := req.C().
		SetTimeout(input.Timeout).
		SetUserAgent(userAgent(input.UserAgent, profile)).
		EnableForceHTTP2()
	if profile == "hardened" {
		client = client.ImpersonateChrome().SetTLSFingerprintChrome()
	} else {
		client = client.ImpersonateChrome()
	}

	resp, err := client.R().
		SetContext(reqCtx).
		Get(input.URL)
	if err != nil {
		return RawPage{}, fmt.Errorf("fetch page: %w", err)
	}

	if resp.GetStatusCode() < 200 || resp.GetStatusCode() >= 300 {
		return RawPage{}, statusError(resp.GetStatusCode())
	}

	contentType := resp.GetContentType()
	if !isAllowedContentType(contentType) {
		return RawPage{}, fmt.Errorf("unsupported content type %q", contentType)
	}

	body, err := resp.ToBytes()
	if err != nil {
		return RawPage{}, fmt.Errorf("read body: %w", err)
	}
	if int64(len(body)) > input.MaxBytes {
		return RawPage{}, fmt.Errorf("response exceeds max bytes budget")
	}

	finalURL := input.URL
	if resp.Request != nil && resp.Request.RawURL != "" {
		finalURL = resp.Request.RawURL
	}
	if finalURL == "" && resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}

	return RawPage{
		URL:          input.URL,
		FinalURL:     finalURL,
		StatusCode:   resp.GetStatusCode(),
		ContentType:  contentType,
		HTML:         string(body),
		FetchMode:    "req",
		FetchProfile: profile,
		FetchedAt:    time.Now().UTC(),
	}, nil
}

func requestContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func sleepWithJitter(ctx context.Context, base, jitter time.Duration) (time.Duration, error) {
	delay := base
	if jitter > 0 {
		delay += time.Duration(rand.Int63n(int64(jitter) + 1))
	}
	if delay <= 0 {
		return 0, nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-timer.C:
		return delay, nil
	}
}

func applyPerHostPacing(ctx context.Context, rawURL string, minGap, jitter time.Duration) (time.Duration, error) {
	if minGap <= 0 {
		return 0, nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || strings.TrimSpace(parsed.Hostname()) == "" {
		return 0, nil
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	targetGap := minGap
	if jitter > 0 {
		targetGap += time.Duration(rand.Int63n(int64(jitter) + 1))
	}
	globalPacingState.mu.Lock()
	lastSeen := globalPacingState.lastSeen[host]
	now := time.Now()
	delay := time.Duration(0)
	if !lastSeen.IsZero() {
		nextAllowed := lastSeen.Add(targetGap)
		if nextAllowed.After(now) {
			delay = nextAllowed.Sub(now)
		}
	}
	globalPacingState.lastSeen[host] = now.Add(delay)
	globalPacingState.mu.Unlock()
	if delay <= 0 {
		return 0, nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-timer.C:
		return delay, nil
	}
}

func shouldRetryOnBlocked(err error, retryProfile string) bool {
	if strings.TrimSpace(retryProfile) == "" {
		return false
	}
	var statusErr *httpStatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	switch statusErr.StatusCode {
	case http.StatusForbidden, http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return true
	default:
		return false
	}
}

func shouldRetryOnTimeout(ctx context.Context, err error, timeout time.Duration) bool {
	if timeout <= 0 {
		return false
	}
	if ctx.Err() != nil {
		return false
	}
	return errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(err.Error()), "deadline exceeded")
}

func shouldFallbackToHTTP(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unexpected alpn protocol") ||
		strings.Contains(message, `want "h2"`) ||
		strings.Contains(message, "http2:") ||
		strings.Contains(message, "tls: handshake failure")
}

func expandedTimeout(current time.Duration) time.Duration {
	next := current + current/2
	maxTimeout := 12 * time.Second
	if next > maxTimeout {
		return maxTimeout
	}
	return next
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

func normalizeFetchProfile(candidate string) string {
	switch strings.TrimSpace(candidate) {
	case "", "browser_like":
		return "browser_like"
	case "standard", "hardened":
		return strings.TrimSpace(candidate)
	default:
		return "browser_like"
	}
}

func normalizeRetryProfile(candidate string) string {
	switch strings.TrimSpace(candidate) {
	case "", "hardened":
		return "hardened"
	case "standard", "browser_like":
		return strings.TrimSpace(candidate)
	default:
		return ""
	}
}

func userAgent(candidate, profile string) string {
	if strings.TrimSpace(candidate) != "" {
		return candidate
	}
	if profile == "standard" {
		return defaultUserAgent
	}
	return defaultBrowserUserAgent
}

func isAllowedContentType(contentType string) bool {
	contentType = strings.ToLower(contentType)
	return strings.Contains(contentType, "text/html") ||
		strings.Contains(contentType, "application/xhtml+xml") ||
		strings.Contains(contentType, "text/plain") ||
		strings.Contains(contentType, "text/markdown") ||
		strings.Contains(contentType, "application/json") ||
		strings.Contains(contentType, "application/xml") ||
		strings.Contains(contentType, "text/xml")
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

type httpStatusError struct {
	StatusCode int
}

func statusError(code int) error {
	return &httpStatusError{StatusCode: code}
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("unexpected status code %d", e.StatusCode)
}
