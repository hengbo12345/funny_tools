package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultConfig returns a Config with default values populated.
func DefaultConfig() Config {
	return Config{
		Version: 1,
		Source: SourceConfig{
			Interval: 1 * time.Hour,
			Timeout:  30 * time.Second,
			Headers: map[string]string{
				"User-Agent": "clash.meta",
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
