package extension

import (
	"strings"

	"mihomo-sub-publisher/internal/config"
)

// ApplyRules applies prepend, append, replace, remove, and exact deduplication on rules list.
func ApplyRules(rules []string, ext config.RulesExtension) []string {
	var combined []string

	// 1. Prepend
	for _, r := range ext.Prepend {
		trimmed := strings.TrimSpace(r)
		if trimmed != "" {
			combined = append(combined, trimmed)
		}
	}

	// 2. Process source rules (Replace and Remove)
	removeSet := make(map[string]struct{}, len(ext.Remove))
	for _, r := range ext.Remove {
		removeSet[strings.TrimSpace(r)] = struct{}{}
	}

	replaceMap := make(map[string]string, len(ext.Replace))
	for _, r := range ext.Replace {
		replaceMap[strings.TrimSpace(r.Match)] = strings.TrimSpace(r.Value)
	}

	for _, r := range rules {
		trimmed := strings.TrimSpace(r)
		if trimmed == "" {
			continue
		}

		// Check remove
		if _, shouldRemove := removeSet[trimmed]; shouldRemove {
			continue
		}

		// Check replace
		if newVal, shouldReplace := replaceMap[trimmed]; shouldReplace {
			combined = append(combined, newVal)
		} else {
			combined = append(combined, trimmed)
		}
	}

	// 3. Append
	for _, r := range ext.Append {
		trimmed := strings.TrimSpace(r)
		if trimmed != "" {
			combined = append(combined, trimmed)
		}
	}

	// 4. Stable exact deduplication (preserve first occurrence)
	seen := make(map[string]struct{}, len(combined))
	result := make([]string, 0, len(combined))
	for _, r := range combined {
		if _, exists := seen[r]; !exists {
			seen[r] = struct{}{}
			result = append(result, r)
		}
	}

	return result
}

// EnsureRuleProxyGroups checks rule targets in ext.Rules (prepend, replace, append).
// If a target proxy-group does not exist (and is not a built-in target or proxy node),
// it creates the proxy-group with type "select" whose proxies are the union of
// custom proxies (from ext.Proxies) and the proxies of the first proxy-group.
func EnsureRuleProxyGroups(
	groups []map[string]any,
	rulesExt config.RulesExtension,
	proxiesExt config.ProxiesExtension,
	proxies []map[string]any,
) []map[string]any {
	existingGroups := make(map[string]struct{}, len(groups))
	for _, g := range groups {
		if name, ok := g["name"].(string); ok && strings.TrimSpace(name) != "" {
			existingGroups[strings.TrimSpace(name)] = struct{}{}
		}
	}

	existingProxies := make(map[string]struct{}, len(proxies))
	for _, p := range proxies {
		if name, ok := p["name"].(string); ok && strings.TrimSpace(name) != "" {
			existingProxies[strings.TrimSpace(name)] = struct{}{}
		}
	}

	customProxies := getCustomProxyNames(proxiesExt, existingProxies)
	firstGroupProxies := getFirstGroupProxies(groups)
	combinedProxies := buildCombinedProxies(customProxies, firstGroupProxies)

	// Ensure the proxy group never has an empty proxies list (Clash client requirement)
	if len(combinedProxies) == 0 {
		combinedProxies = []any{"DIRECT"}
	}

	var candidateRules []string
	candidateRules = append(candidateRules, rulesExt.Prepend...)
	for _, r := range rulesExt.Replace {
		candidateRules = append(candidateRules, r.Value)
	}
	candidateRules = append(candidateRules, rulesExt.Append...)

	for _, ruleStr := range candidateRules {
		trimmed := strings.TrimSpace(ruleStr)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}

		target := extractRuleTarget(trimmed)
		target = strings.Trim(strings.TrimSpace(target), "\"'")
		if target == "" {
			continue
		}

		if isBuiltinTarget(target) {
			continue
		}

		if _, isProxy := existingProxies[target]; isProxy {
			continue
		}

		if _, exists := existingGroups[target]; exists {
			continue
		}

		groupProxies := make([]any, len(combinedProxies))
		copy(groupProxies, combinedProxies)

		newGroup := map[string]any{
			"name":    target,
			"type":    "select",
			"proxies": groupProxies,
		}

		groups = append(groups, newGroup)
		existingGroups[target] = struct{}{}
	}

	return groups
}

func extractRuleTarget(ruleStr string) string {
	parts := strings.Split(ruleStr, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if len(parts) < 2 {
		return ""
	}

	tp := strings.ToUpper(parts[0])
	switch tp {
	case "MATCH":
		return parts[1]
	case "NOT", "OR", "AND", "SUB-RULE", "DOMAIN-REGEX", "PROCESS-NAME-REGEX", "PROCESS-PATH-REGEX":
		// Target is the last element
		return parts[len(parts)-1]
	default:
		if len(parts) >= 3 {
			return parts[2]
		}
		return ""
	}
}

func getCustomProxyNames(ext config.ProxiesExtension, existingProxies map[string]struct{}) []string {
	removeSet := make(map[string]struct{}, len(ext.Remove))
	for _, name := range ext.Remove {
		removeSet[strings.TrimSpace(name)] = struct{}{}
	}

	seen := make(map[string]struct{})
	var custom []string

	add := func(rawName string) {
		name := strings.Trim(strings.TrimSpace(rawName), "\"'")
		if name == "" {
			return
		}
		if _, removed := removeSet[name]; removed {
			return
		}
		// If existingProxies is provided, ensure the proxy actually exists in the config
		if len(existingProxies) > 0 {
			if _, exists := existingProxies[name]; !exists {
				return
			}
		}
		if _, exists := seen[name]; !exists {
			seen[name] = struct{}{}
			custom = append(custom, name)
		}
	}

	for _, p := range ext.Prepend {
		if name, ok := p["name"].(string); ok {
			add(name)
		}
	}
	for _, p := range ext.Append {
		if name, ok := p["name"].(string); ok {
			add(name)
		}
	}
	for _, r := range ext.Replace {
		if name, ok := r.Value["name"].(string); ok {
			add(name)
		}
	}

	return custom
}

func getFirstGroupProxies(groups []map[string]any) []string {
	if len(groups) == 0 {
		return nil
	}
	rawProxies, exists := groups[0]["proxies"]
	if !exists || rawProxies == nil {
		return nil
	}
	list, ok := proxyListValue(rawProxies)
	if !ok {
		return nil
	}
	var res []string
	for _, item := range list {
		if s, ok := item.(string); ok {
			trimmed := strings.TrimSpace(s)
			if trimmed != "" {
				res = append(res, trimmed)
			}
		}
	}
	return res
}

func buildCombinedProxies(customProxies []string, firstGroupProxies []string) []any {
	seen := make(map[string]struct{}, len(customProxies)+len(firstGroupProxies))
	combined := make([]any, 0, len(customProxies)+len(firstGroupProxies))

	for _, name := range customProxies {
		if _, exists := seen[name]; !exists {
			seen[name] = struct{}{}
			combined = append(combined, name)
		}
	}

	for _, name := range firstGroupProxies {
		if _, exists := seen[name]; !exists {
			seen[name] = struct{}{}
			combined = append(combined, name)
		}
	}

	return combined
}

func isBuiltinTarget(target string) bool {
	switch strings.ToUpper(strings.TrimSpace(target)) {
	case "DIRECT", "REJECT", "REJECT-DROP", "PASS", "PASS-RULE", "COMPATIBLE":
		return true
	default:
		return false
	}
}
