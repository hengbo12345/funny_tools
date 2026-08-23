package extension

import (
	mihomoCfg "github.com/metacubex/mihomo/config"

	"mihomo-sub-publisher/internal/config"
)

// Engine applies DSL mutations to Mihomo's RawConfig.
type Engine struct{}

// NewEngine creates a new Extension Engine.
func NewEngine() *Engine {
	return &Engine{}
}

// Apply executes all configured extensions in order on rawConfig.
func (e *Engine) Apply(raw *mihomoCfg.RawConfig, ext config.ExtensionConfig) error {
	if raw == nil {
		return nil
	}

	// 1. Proxies
	raw.Proxy = ApplyProxies(raw.Proxy, ext.Proxies)

	// 2. Proxy Groups
	raw.ProxyGroup = ApplyProxyGroups(raw.ProxyGroup, ext.ProxyGroups)

	// 3. Probe overrides on proxy groups
	raw.ProxyGroup = ApplyProbe(raw.ProxyGroup, ext.Probe)

	// 4. Rules
	raw.Rule = ApplyRules(raw.Rule, ext.Rules)

	// 5. DNS
	if err := ApplyDNS(&raw.DNS, ext.DNS); err != nil {
		return err
	}

	return nil
}
