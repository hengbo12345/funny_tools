package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"mihomo-sub-publisher/internal/logging"
)

const (
	// DefaultClientUserAgent is the default User-Agent sent to upstream subscriptions.
	DefaultClientUserAgent = "clash.meta"
	// LegacyClientUserAgent is the pre-1.2 default, migrated once at config load time.
	LegacyClientUserAgent = "mihomo-sub-publisher/1.0"
)

// DefaultAllowedUserAgents are the wildcard UA patterns accepted on gated
// endpoints (/config, /status) when server.allowed-user-agents is not configured.
// (A var, not a const: Go constants cannot be slices.)
var DefaultAllowedUserAgents = []string{"*clash*", "*mihomo*", "*stash*"}

// DefaultConfig returns a Config with default values populated.
func DefaultConfig() Config {
	return Config{
		Version: 1,
		Source: SourceConfig{
			Interval: 1 * time.Hour,
			Timeout:  30 * time.Second,
			Headers: map[string]string{
				"User-Agent": DefaultClientUserAgent,
			},
		},
		Server: ServerConfig{
			Listen:          "127.0.0.1:8080",
			ConfigPath:      "/config/{token}",
			ShutdownTimeout: 10 * time.Second,
		},
		Storage: StorageConfig{
			CacheDir: "./cache",
			DataDir:  "./data",
		},
		HotReload: HotReloadConfig{
			Debounce: 1 * time.Minute,
		},
		Rules: RulesConfig{
			Providers: map[string]ProviderConfig{
				"clash-rules-cn": {
					Enabled:          true,
					ClientPathPrefix: "./ruleset/",
				},
			},
		},
	}
}

// Load reads and parses a YAML configuration file.
func Load(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", filePath, err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", filePath, err)
	}

	// One-time migration: rewrite the legacy default User-Agent (header names are
	// case-insensitive, so match any spelling of the key). This runs at load time
	// only — an explicit, non-legacy UA is always preserved verbatim.
	for k, v := range cfg.Source.Headers {
		if strings.EqualFold(k, "User-Agent") && v == LegacyClientUserAgent {
			cfg.Source.Headers[k] = DefaultClientUserAgent
			logging.Logger().Info("config: migrated legacy default User-Agent",
				"from", LegacyClientUserAgent, "to", DefaultClientUserAgent)
		}
	}

	// Apply default provider client path prefixes if not set
	for name, provider := range cfg.Rules.Providers {
		if provider.ClientPathPrefix == "" {
			provider.ClientPathPrefix = "./ruleset/"
			cfg.Rules.Providers[name] = provider
		}
	}

	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("config validation error: %w", err)
	}

	return &cfg, nil
}
