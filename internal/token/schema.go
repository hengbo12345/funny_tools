package token

import (
	"time"
)

// TokensConfig represents tokens.yaml configuration.
type TokensConfig struct {
	Version int               `yaml:"version"`
	Tokens  []TokenDefinition `yaml:"tokens"`
}

// TokenDefinition represents a single token definition in tokens.yaml.
type TokenDefinition struct {
	Name      string    `yaml:"name"`
	Token     string    `yaml:"token"`
	ExpiresAt time.Time `yaml:"expires_at"`
	Limit     int64     `yaml:"limit"`
}

// TokenState represents token-state.json.
type TokenState struct {
	Version int                         `json:"version"`
	Tokens  map[string]*TokenStateEntry `json:"tokens"`
}

// TokenStateEntry stores the remaining download count.
type TokenStateEntry struct {
	Remaining int64 `json:"remaining"`
}

// ResetRequest represents the content of token-reset-requests.json.
type ResetRequest struct {
	TriggeredAt string `json:"triggered_at"`
}
