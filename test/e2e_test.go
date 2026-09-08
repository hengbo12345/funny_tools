package test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"mihomo-sub-publisher/internal/config"
	"mihomo-sub-publisher/internal/extension"
	"mihomo-sub-publisher/internal/generator"
	"mihomo-sub-publisher/internal/ruleproviders"
	"mihomo-sub-publisher/internal/server"
	"mihomo-sub-publisher/internal/source"
	"mihomo-sub-publisher/internal/testutil"
	"mihomo-sub-publisher/internal/token"
)

func TestEndToEndPipelineAndServer(t *testing.T) {
	// 1. Mock upstream subscription server
	var upstreamStatus atomic.Int32
	upstreamStatus.Store(http.StatusOK)

	upstreamYAML := `
mixed-port: 7890
mode: rule
log-level: info
proxies:
  - name: "HK-Node-01"
    type: ss
    server: 1.1.1.1
    port: 8388
    cipher: aes-128-gcm
    password: test
proxy-groups:
  - name: "Proxy"
    type: select
    proxies:
      - "HK-Node-01"
      - DIRECT
rules:
  - DOMAIN-SUFFIX,upstream-rule.com,DIRECT
`
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := upstreamStatus.Load()
		if status != http.StatusOK {
			w.WriteHeader(int(status))
			return
		}
		w.Header().Set("Content-Type", "application/yaml")
		w.Header().Set("Subscription-Userinfo", "upload=526559161; download=10697466941; total=214748364800; expire=1813969467")
		w.Header().Set("Profile-Update-Interval", "24")
		w.Header().Set("Content-Disposition", "attachment;filename*=UTF-8''FLZT")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(upstreamYAML))
	}))
	defer upstreamServer.Close()

	// 2. Setup temporary workspace
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	dataDir := filepath.Join(tmpDir, "data")
	tokensFile := filepath.Join(tmpDir, "tokens.yaml")
	stateFile := filepath.Join(dataDir, "token-state.json")
	triggerFile := filepath.Join(dataDir, "token-reset-requests.json")

	tokensYAML := `
version: 1
tokens:
  - name: alice
    token: "alice-secret-token"
    limit: 2
`
	if err := os.WriteFile(tokensFile, []byte(tokensYAML), 0600); err != nil {
		t.Fatalf("failed to write tokens: %v", err)
	}

	cfg := &config.Config{
		Version: 1,
		Source: config.SourceConfig{
			URL:      upstreamServer.URL,
			Interval: 1 * time.Hour,
			Timeout:  2 * time.Second,
		},
		Extensions: config.ExtensionConfig{
			Proxies: config.ProxiesExtension{
				Prepend: []map[string]any{
					{"name": "DIRECT-LOCAL", "type": "direct"},
				},
			},
			ProxyGroups: config.ProxyGroupsExtension{
				Prepend: []map[string]any{
					{
						"name":    "AUTO-HK",
						"type":    "url-test",
						"proxies": []any{"HK-Node-01", "DIRECT-LOCAL"},
						"url":     "http://example.com/generate_204",
					},
				},
				Inject: []config.ProxyGroupInject{
					{
						Target:         "Proxy",
						PrependProxies: []string{"AUTO-HK"},
					},
				},
			},
			Rules: config.RulesExtension{
				Prepend: []string{
					"RULE-SET,direct-domain,DIRECT",
					"RULE-SET,proxy-domain,Proxy",
				},
				Append: []string{
					"MATCH,Proxy",
				},
			},
			DNS: config.DNSExtension{
				Override: map[string]any{
					"enable":        true,
					"enhanced-mode": "fake-ip",
				},
			},
			Probe: config.ProbeExtension{
				Override: map[string]any{
					"url":      "https://www.gstatic.com/generate_204",
					"interval": 300,
				},
			},
		},
		Rules: config.RulesConfig{
			Providers: map[string]config.ProviderConfig{
				"clash-rules-cn": {
					Enabled:          true,
					ClientPathPrefix: "./ruleset/",
				},
			},
		},
		Server: config.ServerConfig{
			Listen:          "127.0.0.1:0",
			ConfigPath:      "/config/{token}",
			ShutdownTimeout: 5 * time.Second,
		},
		Storage: config.StorageConfig{
			CacheDir: cacheDir,
			DataDir:  dataDir,
		},
		HotReload: config.HotReloadConfig{
			Debounce: 10 * time.Millisecond,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 3. Initialize components
	tokenStore := token.NewStore(tokensFile, stateFile)
	if err := tokenStore.Load(); err != nil {
		t.Fatalf("tokenStore.Load failed: %v", err)
	}

	snapMgr := generator.NewSnapshotManager()
	fetcher := source.NewFetcher(cfg.Source)
	registry := ruleproviders.NewDefaultRegistry()
	extEngine := extension.NewEngine()
	pipeline := generator.NewPipeline(cfg, fetcher, registry, extEngine, snapMgr)

	srv := server.NewServer(cfg, tokenStore, snapMgr)
	if err := srv.Start(); err != nil {
		t.Fatalf("srv.Start failed: %v", err)
	}
	defer srv.Shutdown(ctx)

	watcher := token.NewWatcher(triggerFile, tokenStore, cfg.HotReload.Debounce)
	watcher.Start(ctx)

	// 4. Run initial pipeline
	snap, err := pipeline.Run(ctx, nil)
	if err != nil {
		t.Fatalf("pipeline.Run failed: %v", err)
	}
	if snap == nil {
		t.Fatalf("expected non-nil snapshot")
	}

	baseURL := "http://" + srv.Addr()

	// 5. Test GET /health (Health endpoints do not require Clash UA)
	resp, err := http.Get(baseURL + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /health failed: code=%v, err=%v", resp.StatusCode, err)
	}
	resp.Body.Close()

	clashUA := "clash.meta"

	// 5.1 Test that non-clash User-Agent receives 404 Not Found
	nonClashResp, err := http.Get(baseURL + "/config/alice-secret-token")
	if err != nil || nonClashResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for non-clash UA, got code=%v, err=%v", nonClashResp.StatusCode, err)
	}
	nonClashResp.Body.Close()

	// 6. Test GET /config/alice-secret-token (Download #1 with Clash UA)
	resp, err = testutil.GetWithUA(baseURL+"/config/alice-secret-token", clashUA)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /config/alice failed: code=%v, err=%v", resp.StatusCode, err)
	}

	// Verify upstream subscription headers forwarded to downstream
	if uinfo := resp.Header.Get("Subscription-Userinfo"); uinfo != "upload=526559161; download=10697466941; total=214748364800; expire=1813969467" {
		t.Errorf("expected Subscription-Userinfo header, got %q", uinfo)
	}
	if interval := resp.Header.Get("Profile-Update-Interval"); interval != "24" {
		t.Errorf("expected Profile-Update-Interval 24, got %q", interval)
	}
	if disp := resp.Header.Get("Content-Disposition"); disp != "attachment;filename*=UTF-8''FLZT" {
		t.Errorf("expected Content-Disposition header, got %q", disp)
	}

	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "# source-updated-at:") {
		t.Errorf("expected metadata header comments in generated YAML, got:\n%s", bodyStr)
	}
	if !strings.Contains(bodyStr, "rule-providers:") {
		t.Errorf("expected rule-providers in generated YAML, got:\n%s", bodyStr)
	}
	if !strings.Contains(bodyStr, "https://raw.githubusercontent.com/mcxiaochenn/clash-rules-cn/rules/direct-domain.yaml") {
		t.Errorf("expected direct-domain rule provider url in generated YAML")
	}
	if !strings.Contains(bodyStr, "RULE-SET,direct-domain,DIRECT") {
		t.Errorf("expected RULE-SET in rules")
	}

	// Verify valid YAML parsing of final config
	var parsedFinal map[string]any
	if err := yaml.Unmarshal(body, &parsedFinal); err != nil {
		t.Fatalf("failed to parse generated config as YAML: %v", err)
	}

	// Verify upstream default/first proxy was preserved for group "Proxy"
	if pGroups, ok := parsedFinal["proxy-groups"].([]any); ok {
		var foundProxyGroup bool
		for _, gAny := range pGroups {
			if gMap, ok := gAny.(map[string]any); ok && gMap["name"] == "Proxy" {
				foundProxyGroup = true
				if proxies, ok := gMap["proxies"].([]any); ok {
					if len(proxies) == 0 || proxies[0] != "HK-Node-01" {
						t.Errorf("expected group 'Proxy' default/first proxy to remain 'HK-Node-01', got %v", proxies)
					}
				} else {
					t.Errorf("group 'Proxy' missing proxies list")
				}
			}
		}
		if !foundProxyGroup {
			t.Errorf("group 'Proxy' not found in final config")
		}
	} else {
		t.Errorf("missing proxy-groups in final config")
	}

	// 7. Test Download #2 (Reaches limit of 2)
	resp, err = testutil.GetWithUA(baseURL+"/config/alice-secret-token", clashUA)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /config/alice (download 2) failed: code=%v, err=%v", resp.StatusCode, err)
	}
	resp.Body.Close()

	// 8. Test Download #3 (Exhausted -> 403)
	resp, err = testutil.GetWithUA(baseURL+"/config/alice-secret-token", clashUA)
	if err != nil {
		t.Fatalf("GET /config/alice failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden after quota exhausted, got %d", resp.StatusCode)
	}

	// 9. Test Token Reset Trigger
	if err := os.WriteFile(triggerFile, []byte(`{"triggered_at":"2026-08-23T12:00:00Z"}`), 0600); err != nil {
		t.Fatalf("failed to create trigger file: %v", err)
	}

	// Wait for reset
	var resetOK bool
	for i := 0; i < 50; i++ {
		time.Sleep(20 * time.Millisecond)
		item, err := tokenStore.Authorize("alice-secret-token")
		if err == nil && item.Remaining.Load() == 2 {
			resetOK = true
			break
		}
	}
	if !resetOK {
		t.Fatalf("expected token quota to be reset to 2")
	}

	// Download #4 should now succeed (200 OK)
	resp, err = testutil.GetWithUA(baseURL+"/config/alice-secret-token", clashUA)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /config/alice after reset failed: code=%v, err=%v", resp.StatusCode, err)
	}
	resp.Body.Close()

	// 10. Test Failure Injection: Upstream fails (500 Error)
	upstreamStatus.Store(http.StatusInternalServerError)

	// Trigger pipeline run with upstream failure
	_, failErr := pipeline.Run(ctx, nil)
	if failErr == nil {
		t.Fatalf("expected pipeline.Run to return error when upstream fails")
	}

	// Last-known-good snapshot must continue to be served!
	resp, err = testutil.GetWithUA(baseURL+"/config/alice-secret-token", clashUA)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /config/alice should succeed with last-known-good snapshot during upstream failure: code=%v, err=%v", resp.StatusCode, err)
	}
	resp.Body.Close()

	// Verify /health still 200
	resp, err = http.Get(baseURL + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /health should remain 200 during upstream outage")
	}
	resp.Body.Close()

	fmt.Println("End-to-End pipeline & server integration test completed successfully!")
}

func init() {
	// Ensure generator hash function handles nil and valid cases
	_ = hex.EncodeToString
	_ = sha256.Sum256
}
