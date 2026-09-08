package source

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"mihomo-sub-publisher/internal/config"
)

func TestFetcherSuccess(t *testing.T) {
	expectedBody := "mixed-port: 7890\nproxies:\n  - name: test\n    type: direct\n"
	expectedHash := sha256.Sum256([]byte(expectedBody))
	expectedSHA := hex.EncodeToString(expectedHash[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "test-agent" {
			t.Errorf("unexpected User-Agent: %s", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "application/yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(expectedBody))
	}))
	defer server.Close()

	cfg := config.SourceConfig{
		URL:      server.URL,
		Timeout:  2 * time.Second,
		Interval: 1 * time.Hour,
		Headers: map[string]string{
			"User-Agent": "test-agent",
		},
	}

	fetcher := NewFetcher(cfg)
	res, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if string(res.Content) != expectedBody {
		t.Errorf("got body %q, want %q", string(res.Content), expectedBody)
	}
	if res.SHA256 != expectedSHA {
		t.Errorf("got sha256 %s, want %s", res.SHA256, expectedSHA)
	}
}

func TestFetcherGzipDecompression(t *testing.T) {
	rawContent := "mixed-port: 7890\nproxies: []\n"
	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	_, _ = gw.Write([]byte(rawContent))
	_ = gw.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(gzBuf.Bytes())
	}))
	defer server.Close()

	cfg := config.SourceConfig{
		URL:     server.URL,
		Timeout: 2 * time.Second,
	}

	fetcher := NewFetcher(cfg)
	res, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if string(res.Content) != rawContent {
		t.Errorf("decompression failed: got %q, want %q", string(res.Content), rawContent)
	}
}

func TestFetcherRetryOn5xx(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := attempts.Add(1)
		if count < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success-after-retry"))
	}))
	defer server.Close()

	cfg := config.SourceConfig{
		URL:     server.URL,
		Timeout: 2 * time.Second,
	}

	fetcher := NewFetcher(cfg)
	res, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch should succeed after retry, got: %v", err)
	}

	if string(res.Content) != "success-after-retry" {
		t.Fatalf("unexpected content: %s", string(res.Content))
	}
	if attempts.Load() != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts.Load())
	}
}

func TestFetcherDefaultClashUAAndSubscriptionHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "clash.meta" {
			t.Errorf("expected default User-Agent clash.meta, got %s", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "text/yaml; charset=UTF-8")
		w.Header().Set("Subscription-Userinfo", "upload=526559161; download=10697466941; total=214748364800; expire=1813969467")
		w.Header().Set("Profile-Update-Interval", "24")
		w.Header().Set("Content-Disposition", "attachment;filename*=UTF-8''FLZT")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("mixed-port: 7890\nproxies: []\n"))
	}))
	defer server.Close()

	// No User-Agent header specified in cfg
	cfg := config.SourceConfig{
		URL:     server.URL,
		Timeout: 2 * time.Second,
	}

	fetcher := NewFetcher(cfg)
	res, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if res.Headers == nil {
		t.Fatalf("expected non-nil res.Headers")
	}
	if userInfo := res.Headers["Subscription-Userinfo"]; userInfo != "upload=526559161; download=10697466941; total=214748364800; expire=1813969467" {
		t.Errorf("unexpected Subscription-Userinfo: %s", userInfo)
	}
	if interval := res.Headers["Profile-Update-Interval"]; interval != "24" {
		t.Errorf("unexpected Profile-Update-Interval: %s", interval)
	}
	if disp := res.Headers["Content-Disposition"]; disp != "attachment;filename*=UTF-8''FLZT" {
		t.Errorf("unexpected Content-Disposition: %s", disp)
	}
}
