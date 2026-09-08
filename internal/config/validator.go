package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Validate validates the configuration values.
func Validate(cfg *Config) error {
	if cfg == nil {
		return errors.New("config cannot be nil")
	}

	if cfg.Version <= 0 {
		return errors.New("version must be greater than 0")
	}

	// Validate Source
	if cfg.Source.URL == "" {
		return errors.New("source.url cannot be empty")
	}
	parsedURL, err := url.ParseRequestURI(cfg.Source.URL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return fmt.Errorf("source.url must be a valid http/https URL: %q", cfg.Source.URL)
	}

	if cfg.Source.Interval < 1*time.Second {
		return fmt.Errorf("source.interval must be at least 1s, got %v", cfg.Source.Interval)
	}
	if cfg.Source.Timeout < 1*time.Second {
		return fmt.Errorf("source.timeout must be at least 1s, got %v", cfg.Source.Timeout)
	}

	// Validate Extensions
	for i, r := range cfg.Extensions.Proxies.Replace {
		if r.Match == "" {
			return fmt.Errorf("extensions.proxies.replace[%d].match cannot be empty", i)
		}
		if len(r.Value) == 0 {
			return fmt.Errorf("extensions.proxies.replace[%d].value cannot be empty", i)
		}
	}

	for i, r := range cfg.Extensions.ProxyGroups.Replace {
		if r.Match == "" {
			return fmt.Errorf("extensions.proxy-groups.replace[%d].match cannot be empty", i)
		}
		if len(r.Value) == 0 {
			return fmt.Errorf("extensions.proxy-groups.replace[%d].value cannot be empty", i)
		}
	}

	for i, r := range cfg.Extensions.Rules.Replace {
		if r.Match == "" {
			return fmt.Errorf("extensions.rules.replace[%d].match cannot be empty", i)
		}
		if r.Value == "" {
			return fmt.Errorf("extensions.rules.replace[%d].value cannot be empty", i)
		}
	}

	// Validate Server
	if strings.TrimSpace(cfg.Server.Listen) == "" {
		return errors.New("server.listen cannot be empty")
	}
	if !strings.Contains(cfg.Server.ConfigPath, "{token}") {
		return fmt.Errorf("server.config-path must contain '{token}', got %q", cfg.Server.ConfigPath)
	}
	if cfg.Server.ShutdownTimeout <= 0 {
		return fmt.Errorf("server.shutdown-timeout must be positive, got %v", cfg.Server.ShutdownTimeout)
	}
	for i, ua := range cfg.Server.AllowedUserAgents {
		if strings.TrimSpace(ua) == "" {
			return fmt.Errorf("server.allowed-user-agents[%d] cannot be empty or whitespace", i)
		}
	}
	for key, val := range cfg.Server.RequiredHeaders {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("server.required-headers key cannot be empty or whitespace")
		}
		// "" and "*" both mean "header must be present, value unconstrained";
		// anything else is an exact (case-insensitive) match — whitespace-only is a typo.
		if val != "" && val != "*" && strings.TrimSpace(val) == "" {
			return fmt.Errorf("server.required-headers[%q] value cannot be whitespace-only (use \"*\" to require presence only)", key)
		}
	}

	// Validate Storage
	if strings.TrimSpace(cfg.Storage.CacheDir) == "" {
		return errors.New("storage.cache-dir cannot be empty")
	}
	if strings.TrimSpace(cfg.Storage.DataDir) == "" {
		return errors.New("storage.data-dir cannot be empty")
	}

	// Validate HotReload
	if cfg.HotReload.Debounce < 0 {
		return fmt.Errorf("hot-reload.debounce cannot be negative, got %v", cfg.HotReload.Debounce)
	}

	return nil
}
