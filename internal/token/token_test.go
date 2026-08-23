package token

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTokenStoreAuthorizationAndQuota(t *testing.T) {
	tmpDir := t.TempDir()
	tokensFile := filepath.Join(tmpDir, "tokens.yaml")
	stateFile := filepath.Join(tmpDir, "token-state.json")

	tokensYAML := `
version: 1
tokens:
  - name: active-user
    token: "token-active-123"
    expires_at: "2099-12-31T23:59:59Z"
    limit: 2

  - name: expired-user
    token: "token-expired-456"
    expires_at: "2020-01-01T00:00:00Z"
    limit: 100
`
	if err := os.WriteFile(tokensFile, []byte(tokensYAML), 0600); err != nil {
		t.Fatalf("failed to write tokens.yaml: %v", err)
	}

	store := NewStore(tokensFile, stateFile)
	if err := store.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 1. Authorize valid token
	item, err := store.Authorize("token-active-123")
	if err != nil {
		t.Fatalf("expected valid token authorization, got: %v", err)
	}
	if item.Remaining.Load() != 2 {
		t.Fatalf("expected remaining 2, got %d", item.Remaining.Load())
	}

	// Deduct 1
	store.Deduct(item)
	if item.Remaining.Load() != 1 {
		t.Fatalf("expected remaining 1 after deduct, got %d", item.Remaining.Load())
	}

	// Deduct 2nd
	store.Deduct(item)
	if item.Remaining.Load() != 0 {
		t.Fatalf("expected remaining 0 after deduct, got %d", item.Remaining.Load())
	}

	// 2. Authorize exhausted token
	_, err = store.Authorize("token-active-123")
	if err != ErrTokenExhausted {
		t.Fatalf("expected ErrTokenExhausted, got: %v", err)
	}

	// 3. Authorize expired token
	_, err = store.Authorize("token-expired-456")
	if err != ErrTokenExpired {
		t.Fatalf("expected ErrTokenExpired, got: %v", err)
	}

	// 4. Authorize non-existent token
	_, err = store.Authorize("unknown-token")
	if err != ErrTokenNotFound {
		t.Fatalf("expected ErrTokenNotFound, got: %v", err)
	}
}

func TestTokenResetAndCulling(t *testing.T) {
	tmpDir := t.TempDir()
	tokensFile := filepath.Join(tmpDir, "tokens.yaml")
	stateFile := filepath.Join(tmpDir, "token-state.json")

	tokensV1 := `
version: 1
tokens:
  - name: user-1
    token: "token-1"
    limit: 10
  - name: user-2
    token: "token-2"
    limit: 5
`
	if err := os.WriteFile(tokensFile, []byte(tokensV1), 0600); err != nil {
		t.Fatalf("failed to write tokens.yaml: %v", err)
	}

	store := NewStore(tokensFile, stateFile)
	if err := store.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Consume some quota
	item1, _ := store.Authorize("token-1")
	store.Deduct(item1)
	store.Deduct(item1) // remaining 8
	if item1.Remaining.Load() != 8 {
		t.Fatalf("expected remaining 8, got %d", item1.Remaining.Load())
	}

	// Update tokens.yaml: remove token-2, add token-3, change token-1 limit to 20
	tokensV2 := `
version: 1
tokens:
  - name: user-1
    token: "token-1"
    limit: 20
  - name: user-3
    token: "token-3"
    limit: 30
`
	if err := os.WriteFile(tokensFile, []byte(tokensV2), 0600); err != nil {
		t.Fatalf("failed to write tokens.yaml: %v", err)
	}

	// Reset all and reload
	if err := store.ResetAllAndReload(); err != nil {
		t.Fatalf("ResetAllAndReload failed: %v", err)
	}

	// token-1 should have limit reset to 20
	item1, err := store.Authorize("token-1")
	if err != nil || item1.Remaining.Load() != 20 {
		t.Fatalf("expected token-1 remaining 20, got %v, err: %v", item1.Remaining.Load(), err)
	}

	// token-2 should be culled (deleted)
	_, err = store.Authorize("token-2")
	if err != ErrTokenNotFound {
		t.Fatalf("expected token-2 to be culled, got: %v", err)
	}

	// token-3 should exist with limit 30
	item3, err := store.Authorize("token-3")
	if err != nil || item3.Remaining.Load() != 30 {
		t.Fatalf("expected token-3 remaining 30, got %v, err: %v", item3.Remaining.Load(), err)
	}
}

func TestWatcherTrigger(t *testing.T) {
	tmpDir := t.TempDir()
	tokensFile := filepath.Join(tmpDir, "tokens.yaml")
	stateFile := filepath.Join(tmpDir, "token-state.json")
	triggerFile := filepath.Join(tmpDir, "token-reset-requests.json")

	tokensYAML := `
version: 1
tokens:
  - name: test-user
    token: "token-test"
    limit: 10
`
	if err := os.WriteFile(tokensFile, []byte(tokensYAML), 0600); err != nil {
		t.Fatalf("failed to write tokens.yaml: %v", err)
	}

	store := NewStore(tokensFile, stateFile)
	if err := store.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	item, _ := store.Authorize("token-test")
	store.Deduct(item) // remaining 9
	if item.Remaining.Load() != 9 {
		t.Fatalf("expected remaining 9, got %d", item.Remaining.Load())
	}

	// Start watcher with short debounce for test (10ms)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watcher := NewWatcher(triggerFile, store, 10*time.Millisecond)
	watcher.Start(ctx)

	// Create trigger file
	if err := os.WriteFile(triggerFile, []byte(`{"triggered_at":"2026-08-23T10:00:00Z"}`), 0600); err != nil {
		t.Fatalf("failed to write trigger file: %v", err)
	}

	// Wait for reset to be processed and trigger file removed
	var resetSuccess bool
	for i := 0; i < 50; i++ {
		time.Sleep(20 * time.Millisecond)
		item, _ = store.Authorize("token-test")
		if item.Remaining.Load() == 10 {
			resetSuccess = true
			break
		}
	}

	if !resetSuccess {
		t.Fatalf("expected token quota to be reset to 10 by watcher")
	}

	// Verify trigger file was removed
	if _, err := os.Stat(triggerFile); !os.IsNotExist(err) {
		t.Fatalf("expected trigger file to be removed after processing")
	}
}
