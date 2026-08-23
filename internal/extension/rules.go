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
