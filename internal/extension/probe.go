package extension

import (
	"strings"

	"mihomo-sub-publisher/internal/config"
)

// ApplyProbe applies probe health-check overrides to url-test, fallback, and load-balance proxy groups.
func ApplyProbe(groups []map[string]any, ext config.ProbeExtension) []map[string]any {
	if len(ext.Override) == 0 {
		return groups
	}

	result := make([]map[string]any, len(groups))
	for i, g := range groups {
		newGroup := cloneMap(g)
		gType, _ := newGroup["type"].(string)
		lowerType := strings.ToLower(strings.TrimSpace(gType))

		if lowerType == "url-test" || lowerType == "fallback" || lowerType == "load-balance" {
			if u, ok := ext.Override["url"]; ok {
				newGroup["url"] = u
			}
			if interval, ok := ext.Override["interval"]; ok {
				newGroup["interval"] = interval
			}
			if timeout, ok := ext.Override["timeout"]; ok {
				newGroup["timeout"] = timeout
			}
		}

		result[i] = newGroup
	}

	return result
}
