package config

import (
	"time"
)

// Config represents the application configuration (config.yaml).
type Config struct {
	Version    int              `yaml:"version"`
	Source     SourceConfig     `yaml:"source"`
	Extensions ExtensionConfig  `yaml:"extensions"`
	Rules      RulesConfig      `yaml:"rules"`
	Server     ServerConfig     `yaml:"server"`
	Storage    StorageConfig    `yaml:"storage"`
	HotReload  HotReloadConfig  `yaml:"hot-reload"`
}

// SourceConfig defines the upstream subscription settings.
type SourceConfig struct {
	URL      string            `yaml:"url"`
	Interval time.Duration     `yaml:"interval"`
	Timeout  time.Duration     `yaml:"timeout"`
	Headers  map[string]string `yaml:"headers"`
}

// ExtensionConfig defines configuration mutations.
type ExtensionConfig struct {
	Proxies     ProxiesExtension     `yaml:"proxies"`
	ProxyGroups ProxyGroupsExtension `yaml:"proxy-groups"`
	Rules       RulesExtension       `yaml:"rules"`
	DNS         DNSExtension         `yaml:"dns"`
	Probe       ProbeExtension       `yaml:"probe"`
}

// ProxiesExtension defines modifications to proxies list.
type ProxiesExtension struct {
	Prepend []map[string]any `yaml:"prepend"`
	Append  []map[string]any `yaml:"append"`
	Replace []ProxyReplace   `yaml:"replace"`
	Remove  []string         `yaml:"remove"`
}

// ProxyReplace defines replacement for a proxy matching name.
type ProxyReplace struct {
	Match string         `yaml:"match"`
	Value map[string]any `yaml:"value"`
}

// ProxyGroupsExtension defines modifications to proxy-groups list.
type ProxyGroupsExtension struct {
	Prepend []map[string]any    `yaml:"prepend"`
	Append  []map[string]any    `yaml:"append"`
	Replace []ProxyGroupReplace `yaml:"replace"`
	Remove  []string            `yaml:"remove"`
	Inject  []ProxyGroupInject  `yaml:"inject"`
}

// ProxyGroupInject defines injection of proxy names into existing proxy-groups.
type ProxyGroupInject struct {
	Target         string   `yaml:"target"`
	PrependProxies []string `yaml:"prepend-proxies"`
	AppendProxies  []string `yaml:"append-proxies"`
}

// ProxyGroupReplace defines replacement for a proxy group matching name.
type ProxyGroupReplace struct {
	Match string         `yaml:"match"`
	Value map[string]any `yaml:"value"`
}

// RulesExtension defines modifications to rules list.
type RulesExtension struct {
	Prepend []string      `yaml:"prepend"`
	Append  []string      `yaml:"append"`
	Replace []RuleReplace `yaml:"replace"`
	Remove  []string      `yaml:"remove"`
}

// RuleReplace defines replacement for an exact rule string match.
type RuleReplace struct {
	Match string `yaml:"match"`
	Value string `yaml:"value"`
}

// DNSExtension defines DNS override configuration.
type DNSExtension struct {
	Override map[string]any `yaml:"override"`
}

// ProbeExtension defines probe health-check override configuration.
type ProbeExtension struct {
	Override map[string]any `yaml:"override"`
}

// RulesConfig defines rule providers settings.
type RulesConfig struct {
	Providers map[string]ProviderConfig `yaml:"providers"`
}

// ProviderConfig configures a rule provider.
type ProviderConfig struct {
	Enabled          bool   `yaml:"enabled"`
	ClientPathPrefix string `yaml:"client-path-prefix"`
	URL              string `yaml:"url,omitempty"`
}

// ServerConfig defines the HTTP server settings.
type ServerConfig struct {
	Listen            string            `yaml:"listen"`
	ConfigPath        string            `yaml:"config-path"`
	ShutdownTimeout   time.Duration     `yaml:"shutdown-timeout"`
	AllowedUserAgents []string          `yaml:"allowed-user-agents,omitempty"`
	RequiredHeaders   map[string]string `yaml:"required-headers,omitempty"`
}

// StorageConfig defines data and cache directory paths.
type StorageConfig struct {
	CacheDir string `yaml:"cache-dir"`
	DataDir  string `yaml:"data-dir"`
}

// HotReloadConfig defines hot-reload behavior for trigger files.
type HotReloadConfig struct {
	Debounce time.Duration `yaml:"debounce"`
}
