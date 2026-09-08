package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mihomo-sub-publisher/internal/config"
	"mihomo-sub-publisher/internal/extension"
	"mihomo-sub-publisher/internal/ruleproviders"
	"mihomo-sub-publisher/internal/source"
)

func TestPipelineSuccessAndFingerprintSkip(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	dataDir := filepath.Join(tmpDir, "data")

	cfg := &config.Config{
		Version: 1,
		Source: config.SourceConfig{
			URL:      "http://example.com/sub.yaml",
			Interval: 1 * time.Hour,
			Timeout:  10 * time.Second,
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
						"name":    "AUTO",
						"type":    "url-test",
						"proxies": []any{"DIRECT-LOCAL", "DIRECT"},
					},
				},
			},
			Rules: config.RulesExtension{
				Prepend: []string{
					"RULE-SET,direct-domain,DIRECT",
				},
				Append: []string{
					"MATCH,DIRECT",
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
		Storage: config.StorageConfig{
			CacheDir: cacheDir,
			DataDir:  dataDir,
		},
	}

	sourceContent := `
mixed-port: 7890
mode: rule
proxies:
  - name: "HK-01"
    type: direct
proxy-groups:
  - name: "Proxy"
    type: select
    proxies:
      - "HK-01"
      - DIRECT
rules:
  - DOMAIN-SUFFIX,google.com,Proxy
`
	fetchRes := &source.FetchResult{
		Content:   []byte(sourceContent),
		SHA256:    "dummy-sha-12345",
		UpdatedAt: time.Now(),
		Headers: map[string]string{
			"Subscription-Userinfo":   "upload=1; download=2; total=10; expire=1800000000",
			"Profile-Update-Interval": "24",
		},
	}

	registry := ruleproviders.NewDefaultRegistry()
	extEngine := extension.NewEngine()
	snapMgr := NewSnapshotManager()

	pipeline := NewPipeline(cfg, nil, registry, extEngine, snapMgr)

	// 1. First Run
	snap, err := pipeline.Run(context.Background(), fetchRes)
	if err != nil {
		t.Fatalf("Pipeline.Run failed: %v", err)
	}

	if snap.Version != 1 {
		t.Errorf("expected version 1, got %d", snap.Version)
	}
	if snap.Headers["Subscription-Userinfo"] != "upload=1; download=2; total=10; expire=1800000000" {
		t.Errorf("expected subscription userinfo in snapshot, got %v", snap.Headers)
	}

	// Verify disk cache
	genFile := filepath.Join(cacheDir, "generated.yaml")
	if _, err := os.Stat(genFile); err != nil {
		t.Fatalf("expected generated.yaml to exist: %v", err)
	}

	// 2. Second Run with same input should skip generation and return same snapshot
	snap2, err := pipeline.Run(context.Background(), fetchRes)
	if err != nil {
		t.Fatalf("Pipeline.Run (second) failed: %v", err)
	}
	if snap2.Version != snap.Version {
		t.Errorf("expected skipped generation version %d, got %d", snap.Version, snap2.Version)
	}

	// 2.1 Third Run with same content hash but updated headers should update headers
	fetchResUpdatedHeaders := &source.FetchResult{
		Content:   []byte(sourceContent),
		SHA256:    "dummy-sha-12345",
		UpdatedAt: time.Now(),
		Headers: map[string]string{
			"Subscription-Userinfo":   "upload=100; download=200; total=1000; expire=1800000000",
			"Profile-Update-Interval": "12",
		},
	}
	snap3, err := pipeline.Run(context.Background(), fetchResUpdatedHeaders)
	if err != nil {
		t.Fatalf("Pipeline.Run (header update) failed: %v", err)
	}
	if snap3.Headers["Subscription-Userinfo"] != "upload=100; download=200; total=1000; expire=1800000000" {
		t.Errorf("expected updated headers in snapshot, got %v", snap3.Headers)
	}

	// 3. Restore Snapshot from disk in a fresh manager
	freshMgr := NewSnapshotManager()
	restoredSnap, err := freshMgr.Restore(cacheDir)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}
	if restoredSnap.Version != snap3.Version {
		t.Errorf("restored version %d != original %d", restoredSnap.Version, snap3.Version)
	}
	if restoredSnap.Headers["Subscription-Userinfo"] != "upload=100; download=200; total=1000; expire=1800000000" {
		t.Errorf("restored headers missing or incorrect: %v", restoredSnap.Headers)
	}
}
