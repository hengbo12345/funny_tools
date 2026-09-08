package source

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mihomo-sub-publisher/internal/config"
	"mihomo-sub-publisher/internal/logging"
)

// FetchResult contains the downloaded content and its metadata.
type FetchResult struct {
	Content   []byte
	SHA256    string
	UpdatedAt time.Time
	Headers   map[string]string
}

// Fetcher handles downloading upstream subscription configuration with retries.
type Fetcher struct {
	client *http.Client
	cfg    config.SourceConfig
}

// NewFetcher creates a new subscription Fetcher.
func NewFetcher(cfg config.SourceConfig) *Fetcher {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &Fetcher{
		cfg: cfg,
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("stopped after 10 redirects")
				}
				return nil
			},
		},
	}
}

// Fetch downloads the subscription configuration, applying exponential backoff on retries.
func (f *Fetcher) Fetch(ctx context.Context) (*FetchResult, error) {
	logger := logging.Logger()
	maxRetries := 3
	backoff := 1 * time.Second

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		logger.Info("source.fetch.start", "attempt", attempt, "url", f.cfg.URL)

		result, err := f.doFetch(ctx)
		if err == nil {
			logger.Info("source.fetch.success", "sha256", result.SHA256, "bytes", len(result.Content))
			return result, nil
		}

		lastErr = err
		logger.Warn("source.fetch.failure", "attempt", attempt, "error", err)

		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
				backoff *= 2
			}
		}
	}

	return nil, fmt.Errorf("source fetch failed after %d attempts: %w", maxRetries, lastErr)
}

func (f *Fetcher) doFetch(ctx context.Context) (*FetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.cfg.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Apply headers
	req.Header.Set("Accept-Encoding", "gzip")
	hasUA := false
	for k, v := range f.cfg.Headers {
		req.Header.Set(k, v)
		if strings.EqualFold(k, "User-Agent") && strings.TrimSpace(v) != "" {
			hasUA = true
		}
	}

	// Default to a Clash-compatible User-Agent if none set or if default placeholder
	currentUA := req.Header.Get("User-Agent")
	if !hasUA || currentUA == "" || currentUA == "mihomo-sub-publisher/1.0" {
		req.Header.Set("User-Agent", "clash.meta")
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected http status: %s", resp.Status)
	}

	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if len(content) == 0 {
		return nil, fmt.Errorf("empty subscription response body")
	}

	// Extract subscription metadata headers (subscription-userinfo, profile-update-interval, content-disposition, etc.)
	headers := make(map[string]string)
	for k, values := range resp.Header {
		if len(values) == 0 {
			continue
		}
		lower := strings.ToLower(k)
		if lower == "subscription-userinfo" ||
			lower == "profile-update-interval" ||
			lower == "content-disposition" ||
			lower == "profile-title" ||
			lower == "profile-web-page-url" ||
			lower == "support-url" ||
			strings.HasPrefix(lower, "subscription-") ||
			strings.HasPrefix(lower, "profile-") {
			headers[http.CanonicalHeaderKey(k)] = values[0]
		}
	}

	hash := sha256.Sum256(content)
	shaStr := hex.EncodeToString(hash[:])

	return &FetchResult{
		Content:   content,
		SHA256:    shaStr,
		UpdatedAt: time.Now(),
		Headers:   headers,
	}, nil
}
