package extension

import (
	"strings"

	"mihomo-sub-publisher/internal/config"
)

// ApplyProxyGroups applies prepend, append, replace, remove, and inject operations on proxy-groups list.
func ApplyProxyGroups(groups []map[string]any, ext config.ProxyGroupsExtension) []map[string]any {
	result := make([]map[string]any, 0, len(groups)+len(ext.Prepend)+len(ext.Append))

	// 1. Prepend
	for _, g := range ext.Prepend {
		result = append(result, cloneMap(g))
	}

	// 2. Process source groups (Replace and Remove)
	removeSet := make(map[string]struct{}, len(ext.Remove))
	for _, name := range ext.Remove {
		removeSet[name] = struct{}{}
	}

	replaceMap := make(map[string]map[string]any, len(ext.Replace))
	for _, r := range ext.Replace {
		replaceMap[r.Match] = r.Value
	}

	for _, g := range groups {
		name, _ := g["name"].(string)

		// Check remove
		if _, shouldRemove := removeSet[name]; shouldRemove {
			continue
		}

		// Check replace
		if newVal, shouldReplace := replaceMap[name]; shouldReplace {
			result = append(result, cloneMap(newVal))
		} else {
			result = append(result, cloneMap(g))
		}
	}

	// 3. Append
	for _, g := range ext.Append {
		result = append(result, cloneMap(g))
	}

	// 4. Inject proxies into target groups
	if len(ext.Inject) > 0 {
		for _, g := range result {
			applyProxyGroupInject(g, ext.Inject)
		}
	}

	// 5. Optionally keep the upstream default (first) proxy at index 0.
	// Replaced groups are skipped: a full replacement is an explicit new ordering.
	if ext.PreserveUpstreamDefaults() {
		preserveUpstreamDefaultProxies(result, groups, replaceMap)
	}

	return result
}

// proxyListValue normalizes a group's "proxies" value into []any
// ([]string entries are widened). ok is false when the key is missing,
// nil, or of an unsupported type.
func proxyListValue(raw any) (list []any, ok bool) {
	switch l := raw.(type) {
	case []any:
		return l, true
	case []string:
		out := make([]any, len(l))
		for i, s := range l {
			out[i] = s
		}
		return out, true
	}
	return nil, false
}

func applyProxyGroupInject(group map[string]any, injects []config.ProxyGroupInject) {
	name, _ := group["name"].(string)
	rawProxies, exists := group["proxies"]
	if !exists || rawProxies == nil {
		return
	}

	src, ok := proxyListValue(rawProxies)
	if !ok {
		return
	}
	currentList := make([]any, len(src))
	copy(currentList, src)

	seen := make(map[string]struct{}, len(currentList))
	for _, p := range currentList {
		if s, ok := p.(string); ok {
			seen[s] = struct{}{}
		}
	}

	for _, inj := range injects {
		if inj.Target != "*" && inj.Target != name {
			continue
		}

		// 1. Prepend proxies
		var toPrepend []any
		for _, pName := range inj.PrependProxies {
			if _, already := seen[pName]; !already {
				seen[pName] = struct{}{}
				toPrepend = append(toPrepend, pName)
			}
		}
		if len(toPrepend) > 0 {
			currentList = append(toPrepend, currentList...)
		}

		// 2. Append proxies
		for _, pName := range inj.AppendProxies {
			if _, already := seen[pName]; !already {
				seen[pName] = struct{}{}
				currentList = append(currentList, pName)
			}
		}
	}

	group["proxies"] = currentList
}

// preserveUpstreamDefaultProxies moves each upstream group's original first
// proxy back to index 0 after mutations. Groups present in skip (replaced
// groups) are left untouched.
func preserveUpstreamDefaultProxies(result []map[string]any, upstreamGroups []map[string]any, skip map[string]map[string]any) {
	defaults := make(map[string]string, len(upstreamGroups))
	for _, g := range upstreamGroups {
		name, _ := g["name"].(string)
		if name == "" {
			continue
		}
		pList, ok := proxyListValue(g["proxies"])
		if !ok || len(pList) == 0 {
			continue
		}
		if s, ok := pList[0].(string); ok && strings.TrimSpace(s) != "" {
			defaults[name] = strings.TrimSpace(s)
		}
	}
	if len(defaults) == 0 {
		return
	}

	for _, g := range result {
		name, _ := g["name"].(string)
		origDefault, ok := defaults[name]
		if !ok || origDefault == "" {
			continue
		}
		if _, replaced := skip[name]; replaced {
			continue
		}

		pList, ok := proxyListValue(g["proxies"])
		if !ok {
			continue
		}
		for i, p := range pList {
			if s, ok := p.(string); ok && s == origDefault && i > 0 {
				// Move the upstream default proxy back to index 0,
				// shifting the others right in place (relative order kept).
				copy(pList[1:i+1], pList[0:i])
				pList[0] = p
				g["proxies"] = pList
				break
			}
		}
	}
}
