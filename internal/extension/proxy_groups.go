package extension

import (
	"mihomo-sub-publisher/internal/config"
)

// ApplyProxyGroups applies prepend, append, replace, and remove operations on proxy-groups list.
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

	return result
}
