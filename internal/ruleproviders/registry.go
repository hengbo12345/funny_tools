package ruleproviders

import (
	"sync"
)

// Registry manages known Rule Provider groups and their definitions.
type Registry struct {
	mu     sync.RWMutex
	groups map[string][]ProviderDefinition
}

// NewDefaultRegistry creates a registry pre-populated with built-in rule providers.
func NewDefaultRegistry() *Registry {
	r := &Registry{
		groups: make(map[string][]ProviderDefinition),
	}
	r.Register("clash-rules-cn", ClashRulesCNProviders())
	return r
}

// Register adds or replaces a provider group.
func (r *Registry) Register(groupName string, defs []ProviderDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.groups[groupName] = defs
}

// GetProviders returns provider definitions for a given group.
func (r *Registry) GetProviders(groupName string) ([]ProviderDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs, exists := r.groups[groupName]
	return defs, exists
}
