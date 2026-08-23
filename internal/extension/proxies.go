package extension

import (
	"mihomo-sub-publisher/internal/config"
)

// ApplyProxies applies prepend, append, replace, and remove operations on proxies list.
func ApplyProxies(proxies []map[string]any, ext config.ProxiesExtension) []map[string]any {
	result := make([]map[string]any, 0, len(proxies)+len(ext.Prepend)+len(ext.Append))

	// 1. Prepend
	for _, p := range ext.Prepend {
		result = append(result, cloneMap(p))
	}

	// 2. Process source proxies (Replace and Remove)
	removeSet := make(map[string]struct{}, len(ext.Remove))
	for _, name := range ext.Remove {
		removeSet[name] = struct{}{}
	}

	replaceMap := make(map[string]map[string]any, len(ext.Replace))
	for _, r := range ext.Replace {
		replaceMap[r.Match] = r.Value
	}

	for _, p := range proxies {
		name, _ := p["name"].(string)

		// Check remove
		if _, shouldRemove := removeSet[name]; shouldRemove {
			continue
		}

		// Check replace
		if newVal, shouldReplace := replaceMap[name]; shouldReplace {
			result = append(result, cloneMap(newVal))
		} else {
			result = append(result, cloneMap(p))
		}
	}

	// 3. Append
	for _, p := range ext.Append {
		result = append(result, cloneMap(p))
	}

	return result
}

func cloneMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		if subMap, ok := v.(map[string]any); ok {
			dst[k] = cloneMap(subMap)
		} else if subSlice, ok := v.([]any); ok {
			dst[k] = cloneSlice(subSlice)
		} else {
			dst[k] = v
		}
	}
	return dst
}

func cloneSlice(src []any) []any {
	if src == nil {
		return nil
	}
	dst := make([]any, len(src))
	for i, v := range src {
		if subMap, ok := v.(map[string]any); ok {
			dst[i] = cloneMap(subMap)
		} else if subSlice, ok := v.([]any); ok {
			dst[i] = cloneSlice(subSlice)
		} else {
			dst[i] = v
		}
	}
	return dst
}
