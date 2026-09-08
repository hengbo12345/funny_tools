package extension

import (
	"strings"

	"mihomo-sub-publisher/internal/config"
)

// ApplyProxyGroups applies prepend, append, replace, remove, and inject operations on proxy-groups list.
func ApplyProxyGroups(groups []map[string]any, ext config.ProxyGroupsExtension) []map[string]any {
	// Record upstream default (or first) proxy for each upstream group
	upstreamDefaults := extractGroupDefaultProxies(groups)

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

	// 5. Ensure upstream default (or first) proxy is preserved as much as possible
	preserveUpstreamDefaultProxies(result, upstreamDefaults)

	return result
}

func applyProxyGroupInject(group map[string]any, injects []config.ProxyGroupInject) {
	name, _ := group["name"].(string)
	rawProxies, exists := group["proxies"]
	if !exists || rawProxies == nil {
		return
	}

	var currentList []any
	if slice, ok := rawProxies.([]any); ok {
		currentList = make([]any, len(slice))
		copy(currentList, slice)
	} else if strSlice, ok := rawProxies.([]string); ok {
		currentList = make([]any, len(strSlice))
		for i, s := range strSlice {
			currentList[i] = s
		}
	} else {
		return
	}

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

func extractGroupDefaultProxies(groups []map[string]any) map[string]string {
	defaults := make(map[string]string, len(groups))
	for _, g := range groups {
		name, _ := g["name"].(string)
		if name == "" {
			continue
		}
		if def, ok := g["default"].(string); ok && strings.TrimSpace(def) != "" {
			defaults[name] = strings.TrimSpace(def)
			continue
		}
		rawProxies, exists := g["proxies"]
		if !exists || rawProxies == nil {
			continue
		}
		switch pList := rawProxies.(type) {
		case []any:
			if len(pList) > 0 {
				if s, ok := pList[0].(string); ok && strings.TrimSpace(s) != "" {
					defaults[name] = strings.TrimSpace(s)
				}
			}
		case []string:
			if len(pList) > 0 && strings.TrimSpace(pList[0]) != "" {
				defaults[name] = strings.TrimSpace(pList[0])
			}
		}
	}
	return defaults
}

func preserveUpstreamDefaultProxies(groups []map[string]any, upstreamDefaults map[string]string) {
	if len(upstreamDefaults) == 0 {
		return
	}

	for _, g := range groups {
		name, _ := g["name"].(string)
		origDefault, ok := upstreamDefaults[name]
		if !ok || origDefault == "" {
			continue
		}

		rawProxies, exists := g["proxies"]
		if !exists || rawProxies == nil {
			continue
		}

		switch pList := rawProxies.(type) {
		case []any:
			idx := -1
			for i, p := range pList {
				if s, ok := p.(string); ok && s == origDefault {
					idx = i
					break
				}
			}
			if idx > 0 {
				// Move original default proxy back to index 0
				newList := make([]any, 0, len(pList))
				newList = append(newList, pList[idx])
				newList = append(newList, pList[:idx]...)
				newList = append(newList, pList[idx+1:]...)
				g["proxies"] = newList
			}
		case []string:
			idx := -1
			for i, s := range pList {
				if s == origDefault {
					idx = i
					break
				}
			}
			if idx > 0 {
				newList := make([]string, 0, len(pList))
				newList = append(newList, pList[idx])
				newList = append(newList, pList[:idx]...)
				newList = append(newList, pList[idx+1:]...)
				g["proxies"] = newList
			}
		}

		if def, ok := g["default"].(string); ok && def != "" {
			g["default"] = origDefault
		}
	}
}
