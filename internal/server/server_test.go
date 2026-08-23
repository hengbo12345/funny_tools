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

func TestServerConfigDownloadAndQuota(t *testing.T) {
	srv, tokenStore, snapMgr := setupTestServer(t)
	defer srv.Shutdown(context.Background())

	baseURL := "http://" + srv.Addr()

	// 1. No snapshot available -> 503
	resp, err := http.Get(baseURL + "/config/token-valid")
	if err != nil {
		t.Fatalf("GET /config/token-valid failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}

	// Supply Snapshot
	snapMgr.Swap(&generator.Snapshot{
		Version:         1,
		SourceUpdatedAt: time.Now(),
		GeneratedAt:     time.Now(),
		SHA256:          "sha123",
		Content:         []byte("mixed-port: 7890\n"),
	})

	// 2. Valid token -> 200 OK and quota -1
	item, _ := tokenStore.Authorize("token-valid")
	initialRemaining := item.Remaining.Load()

	resp, err = http.Get(baseURL + "/config/token-valid")
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

	newRemaining := item.Remaining.Load()
	if newRemaining != initialRemaining-1 {
		t.Errorf("expected quota to decrement from %d to %d, got %d", initialRemaining, initialRemaining-1, newRemaining)
	}

	// 3. Non-existent token -> 404
	resp, err = http.Get(baseURL + "/config/non-existent-token")
	if err != nil {
		t.Fatalf("GET /config/non-existent failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}

	// 4. Exhausted token -> 403
	resp, err = http.Get(baseURL + "/config/token-exhausted")
	if err != nil {
		t.Fatalf("GET /config/token-exhausted failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}

	// 5. Expired token -> 403
	resp, err = http.Get(baseURL + "/config/token-expired")
	if err != nil {
		t.Fatalf("GET /config/token-expired failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}
