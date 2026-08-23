package ruleproviders

// ProviderDefinition defines metadata for an external Rule Provider.
type ProviderDefinition struct {
	Name     string
	Behavior string
	URL      string
	Interval int
}

// ProviderGenerator generates map representation suitable for Mihomo rule-providers config.
func (p ProviderDefinition) ToMihomoMap(clientPathPrefix string) map[string]any {
	path := clientPathPrefix + p.Name + ".yaml"
	interval := p.Interval
	if interval <= 0 {
		interval = 86400 // default 24h
	}

	return map[string]any{
		"type":     "http",
		"behavior": p.Behavior,
		"url":      p.URL,
		"path":     path,
		"interval": interval,
	}
}
