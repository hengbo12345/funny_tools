package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadValidConfig(t *testing.T) {
	yamlContent := `
version: 1

source:
  url: "https://example.com/subscription.yaml"
  interval: 2h
  timeout: 45s
  headers:
    User-Agent: "custom-ua"

extensions:
  proxies:
    prepend:
      - name: "DIRECT-LOCAL"
        type: direct
    append: []
    replace:
      - match: "old-proxy"
        value:
          name: "new-proxy"
          type: socks5
          server: "127.0.0.1"
          port: 1080
    remove:
      - "remove-me"

  proxy-groups:
    prepend:
      - name: "select-group"
        type: select
        proxies:
          - "DIRECT"
    append: []
    replace: []
    remove: []

  rules:
    prepend:
      - "DOMAIN-SUFFIX,google.com,PROXY"
    append:
      - "MATCH,DIRECT"
    replace:
      - match: "DOMAIN,bad.com,DIRECT"
        value: "DOMAIN,bad.com,REJECT"
    remove:
      - "DOMAIN,old.com,DIRECT"

  dns:
    override:
      enable: true
      enhanced-mode: fake-ip

  probe:
    override:
      url: "https://www.gstatic.com/generate_204"
      interval: 300

rules:
  providers:
    clash-rules-cn:
      enabled: true
      client-path-prefix: "./ruleset/"

server:
  listen: "0.0.0.0:8080"
  config-path: "/sub/{token}"
  shutdown-timeout: 5s

storage:
  cache-dir: "./test-cache"
  data-dir: "./test-data"

hot-reload:
  debounce: 30s
`
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Version != 1 {
		t.Errorf("expected version 1, got %d", cfg.Version)
	}
	if cfg.Source.URL != "https://example.com/subscription.yaml" {
		t.Errorf("unexpected source URL: %s", cfg.Source.URL)
	}
	if cfg.Source.Interval != 2*time.Hour {
		t.Errorf("expected interval 2h, got %v", cfg.Source.Interval)
	}
	if cfg.Source.Timeout != 45*time.Second {
		t.Errorf("expected timeout 45s, got %v", cfg.Source.Timeout)
	}
	if cfg.Server.Listen != "0.0.0.0:8080" {
		t.Errorf("expected server listen 0.0.0.0:8080, got %s", cfg.Server.Listen)
	}
	if cfg.Server.ConfigPath != "/sub/{token}" {
		t.Errorf("expected config path /sub/{token}, got %s", cfg.Server.ConfigPath)
	}
	if len(cfg.Extensions.Proxies.Prepend) != 1 {
		t.Errorf("expected 1 prepend proxy, got %d", len(cfg.Extensions.Proxies.Prepend))
	}
	if len(cfg.Extensions.Rules.Prepend) != 1 {
		t.Errorf("expected 1 prepend rule, got %d", len(cfg.Extensions.Rules.Prepend))
	}
}

func TestConfigValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		modify      func(c *Config)
		expectError bool
	}{
		{
			name: "empty source url",
			modify: func(c *Config) {
				c.Source.URL = ""
			},
			expectError: true,
		},
		{
			name: "invalid source url scheme",
			modify: func(c *Config) {
				c.Source.URL = "ftp://example.com"
			},
			expectError: true,
		},
		{
			name: "missing {token} in config-path",
			modify: func(c *Config) {
				c.Server.ConfigPath = "/config"
			},
			expectError: true,
		},
		{
			name: "empty server listen",
			modify: func(c *Config) {
				c.Server.Listen = ""
			},
			expectError: true,
		},
		{
			name: "zero version",
			modify: func(c *Config) {
				c.Version = 0
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Source.URL = "https://example.com/sub.yaml"
			tt.modify(&cfg)
			err := Validate(&cfg)
			if tt.expectError && err == nil {
				t.Errorf("expected error, got nil")
			} else if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func writeTempConfig(t *testing.T, yamlContent string) string {
	t.Helper()
	configFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configFile, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	return configFile
}

func TestLoadMigratesLegacyUserAgent(t *testing.T) {
	// 大小写不同的 header key（user-agent）也应被迁移
	configFile := writeTempConfig(t, `
version: 1
source:
  url: "https://example.com/sub.yaml"
  headers:
    user-agent: "mihomo-sub-publisher/1.0"
`)
	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got := cfg.Source.Headers["user-agent"]; got != DefaultClientUserAgent {
		t.Errorf("expected legacy UA migrated to %q, got %q", DefaultClientUserAgent, got)
	}
}

func TestLoadKeepsExplicitCustomUserAgent(t *testing.T) {
	configFile := writeTempConfig(t, `
version: 1
source:
  url: "https://example.com/sub.yaml"
  headers:
    User-Agent: "my-custom-ua/2.0"
`)
	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got := cfg.Source.Headers["User-Agent"]; got != "my-custom-ua/2.0" {
		t.Errorf("expected explicit UA preserved, got %q", got)
	}
}
