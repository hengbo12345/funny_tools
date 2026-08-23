package mihomo

import (
	"fmt"
	"strings"

	mihomoCfg "github.com/metacubex/mihomo/config"
)

// ValidateRaw validates the structural integrity of RawConfig according to Mihomo and project rules.
func ValidateRaw(raw *mihomoCfg.RawConfig) error {
	if raw == nil {
		return fmt.Errorf("config is nil")
	}

	// 1. Validate Proxies
	proxyNames := make(map[string]struct{}, len(raw.Proxy))
	for i, p := range raw.Proxy {
		name, _ := p["name"].(string)
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("proxy[%d] has empty or missing 'name'", i)
		}
		if _, exists := proxyNames[name]; exists {
			return fmt.Errorf("duplicate proxy name: %q", name)
		}
		proxyNames[name] = struct{}{}

		pType, _ := p["type"].(string)
		if strings.TrimSpace(pType) == "" {
			return fmt.Errorf("proxy %q has empty or missing 'type'", name)
		}
	}

	// 2. Validate Proxy Groups
	groupNames := make(map[string]struct{}, len(raw.ProxyGroup))
	for i, g := range raw.ProxyGroup {
		name, _ := g["name"].(string)
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("proxy-group[%d] has empty or missing 'name'", i)
		}
		if _, exists := groupNames[name]; exists {
			return fmt.Errorf("duplicate proxy-group name: %q", name)
		}
		groupNames[name] = struct{}{}

		gType, _ := g["type"].(string)
		if strings.TrimSpace(gType) == "" {
			return fmt.Errorf("proxy-group %q has empty or missing 'type'", name)
		}
	}

	// 3. Validate Rule Providers
	for name, provider := range raw.RuleProvider {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("rule-provider has empty name")
		}
		pType, _ := provider["type"].(string)
		if pType == "" {
			return fmt.Errorf("rule-provider %q missing 'type'", name)
		}
		behavior, _ := provider["behavior"].(string)
		if behavior == "" {
			return fmt.Errorf("rule-provider %q missing 'behavior'", name)
		}
		pURL, _ := provider["url"].(string)
		if pURL == "" {
			return fmt.Errorf("rule-provider %q missing 'url'", name)
		}
	}

	// 4. Validate Rules and §22 RULE-SET references
	for i, ruleStr := range raw.Rule {
		ruleStr = strings.TrimSpace(ruleStr)
		if ruleStr == "" {
			return fmt.Errorf("rule[%d] cannot be empty", i)
		}
		parts := strings.Split(ruleStr, ",")
		if len(parts) < 2 {
			return fmt.Errorf("rule[%d] %q has invalid format (insufficient parts)", i, ruleStr)
		}

		ruleType := strings.ToUpper(strings.TrimSpace(parts[0]))
		if ruleType == "RULE-SET" {
			if len(parts) < 3 {
				return fmt.Errorf("RULE-SET rule[%d] %q must have format 'RULE-SET,provider_name,target'", i, ruleStr)
			}
			providerName := strings.TrimSpace(parts[1])
			if raw.RuleProvider == nil {
				return fmt.Errorf("rule[%d] references RULE-SET %q but no rule-providers are declared", i, providerName)
			}
			if _, exists := raw.RuleProvider[providerName]; !exists {
				return fmt.Errorf("rule[%d] references unknown rule-provider %q", i, providerName)
			}
		}
	}

	return nil
}
