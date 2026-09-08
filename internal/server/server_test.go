package server

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mihomo-sub-publisher/internal/config"
	"mihomo-sub-publisher/internal/generator"
	"mihomo-sub-publisher/internal/token"
)

func setupTestServer(t *testing.T) (*Server, *token.Store, *generator.SnapshotManager) {
	tmpDir := t.TempDir()
	tokensFile := filepath.Join(tmpDir, "tokens.yaml")
	stateFile := filepath.Join(tmpDir, "token-state.json")

	tokensYAML := `
version: 1
tokens:
  - name: valid-user
    token: "token-valid"
    limit: 5

  - name: exhausted-user
    token: "token-exhausted"
    limit: 0

  - name: expired-user
    token: "token-expired"
    expires_at: "2020-01-01T00:00:00Z"
    limit: 10
`
	if err := os.WriteFile(tokensFile, []byte(tokensYAML), 0600); err != nil {
		t.Fatalf("failed to write tokens: %v", err)
	}

	tokenStore := token.NewStore(tokensFile, stateFile)
	if err := tokenStore.Load(); err != nil {
		t.Fatalf("tokenStore.Load failed: %v", err)
	}

	snapMgr := generator.NewSnapshotManager()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Listen:          "127.0.0.1:0", // random port
			ConfigPath:      "/config/{token}",
			ShutdownTimeout: 5 * time.Second,
		},
	}

	srv := NewServer(cfg, tokenStore, snapMgr)
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	return srv, tokenStore, snapMgr
}

func TestServerHealthAndStatus(t *testing.T) {
	srv, _, snapMgr := setupTestServer(t)
	defer srv.Shutdown(context.Background())

	baseURL := "http://" + srv.Addr()

	// 1. Health
	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		t.Fatalf("GET /health failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// 2. Status without snapshot
	resp, err = http.Get(baseURL + "/status")
	if err != nil {
		t.Fatalf("GET /status failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "NO_SNAPSHOT") {
		t.Errorf("expected NO_SNAPSHOT in status body, got: %s", string(body))
	}

	// 3. Status with snapshot
	snapMgr.Swap(&generator.Snapshot{
		Version:         1,
		SourceUpdatedAt: time.Now(),
		GeneratedAt:     time.Now(),
		SHA256:          "sha123",
	})
	resp, err = http.Get(baseURL + "/status")
	if err != nil {
		t.Fatalf("GET /status failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "ready") {
		t.Errorf("expected ready in status body, got: %s", string(body))
	}
}

func getWithUA(url, ua string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	return http.DefaultClient.Do(req)
}

func TestServerConfigDownloadAndQuota(t *testing.T) {
	srv, tokenStore, snapMgr := setupTestServer(t)
	defer srv.Shutdown(context.Background())

	baseURL := "http://" + srv.Addr()
	clashUA := "clash.meta"

	// 1. No snapshot available -> 503
	resp, err := getWithUA(baseURL+"/config/token-valid", clashUA)
	if err != nil {
		t.Fatalf("GET /config/token-valid failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}

	// Supply Snapshot with subscription headers
	snapMgr.Swap(&generator.Snapshot{
		Version:         1,
		SourceUpdatedAt: time.Now(),
		GeneratedAt:     time.Now(),
		SHA256:          "sha123",
		Headers: map[string]string{
			"Subscription-Userinfo":   "upload=100; download=200; total=1000; expire=1800000000",
			"Profile-Update-Interval": "24",
			"Content-Disposition":     "attachment;filename*=UTF-8''TEST",
		},
		Content: []byte("mixed-port: 7890\n"),
	})

	// 2. Valid token -> 200 OK and quota -1
	item, _ := tokenStore.Authorize("token-valid")
	initialRemaining := item.Remaining.Load()

	resp, err = getWithUA(baseURL+"/config/token-valid", clashUA)
	if err != nil {
		t.Fatalf("GET /config/token-valid failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/yaml" {
		t.Errorf("expected application/yaml, got %s", ct)
	}
	if ui := resp.Header.Get("Subscription-Userinfo"); ui != "upload=100; download=200; total=1000; expire=1800000000" {
		t.Errorf("expected Subscription-Userinfo header, got %s", ui)
	}
	if interval := resp.Header.Get("Profile-Update-Interval"); interval != "24" {
		t.Errorf("expected Profile-Update-Interval 24, got %s", interval)
	}
	if disp := resp.Header.Get("Content-Disposition"); disp != "attachment;filename*=UTF-8''TEST" {
		t.Errorf("expected Content-Disposition header, got %s", disp)
	}

	newRemaining := item.Remaining.Load()
	if newRemaining != initialRemaining-1 {
		t.Errorf("expected quota to decrement from %d to %d, got %d", initialRemaining, initialRemaining-1, newRemaining)
	}

	// 3. Non-existent token -> 404
	resp, err = getWithUA(baseURL+"/config/non-existent-token", clashUA)
	if err != nil {
		t.Fatalf("GET /config/non-existent failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}

	// 4. Exhausted token -> 403
	resp, err = getWithUA(baseURL+"/config/token-exhausted", clashUA)
	if err != nil {
		t.Fatalf("GET /config/token-exhausted failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}

	// 5. Expired token -> 403
	resp, err = getWithUA(baseURL+"/config/token-expired", clashUA)
	if err != nil {
		t.Fatalf("GET /config/token-expired failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

func TestServerUserAgentAndHeaderValidation(t *testing.T) {
	srv, tokenStore, snapMgr := setupTestServer(t)
	defer srv.Shutdown(context.Background())

	snapMgr.Swap(&generator.Snapshot{
		Version:         1,
		SourceUpdatedAt: time.Now(),
		GeneratedAt:     time.Now(),
		SHA256:          "sha123",
		Content:         []byte("mixed-port: 7890\n"),
	})

	baseURL := "http://" + srv.Addr()

	// 1. Non-clash User-Agents should return 404 Not Found
	nonClashUAs := []string{
		"",
		"curl/7.88.1",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
		"python-requests/2.31.0",
		"Wget/1.21.4",
		"Go-http-client/1.1",
	}

	for _, ua := range nonClashUAs {
		resp, err := getWithUA(baseURL+"/config/token-valid", ua)
		if err != nil {
			t.Fatalf("request failed for UA %q: %v", ua, err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("UA %q expected 404 Not Found, got %d", ua, resp.StatusCode)
		}
		resp.Body.Close()
	}

	// 2. Clash-compatible User-Agents should succeed (200 OK)
	item, _ := tokenStore.Authorize("token-valid")
	item.Remaining.Store(100)

	clashUAs := []string{
		"clash.meta",
		"ClashforWindows/0.20.39",
		"clash-verge/v1.7.7",
		"ClashX/1.118.0",
		"Mihomo/1.19.0",
		"Stash/2.6.0",
	}

	for _, ua := range clashUAs {
		resp, err := getWithUA(baseURL+"/config/token-valid", ua)
		if err != nil {
			t.Fatalf("request failed for UA %q: %v", ua, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("UA %q expected 200 OK, got %d", ua, resp.StatusCode)
		}
		resp.Body.Close()
	}
}
