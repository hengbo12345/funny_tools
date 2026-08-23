package token

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"

	"mihomo-sub-publisher/internal/logging"
	"mihomo-sub-publisher/internal/storage"
)

var (
	ErrTokenNotFound = errors.New("not_found")
	ErrTokenExpired  = errors.New("forbidden")
	ErrTokenExhausted = errors.New("forbidden")
)

// TokenItem stores definition and runtime quota for a token.
type TokenItem struct {
	Definition TokenDefinition
	Remaining  atomic.Int64
}

// Store manages in-memory token state and quota checking.
type Store struct {
	mu             sync.RWMutex
	tokensFilePath string
	stateFilePath  string
	items          map[string]*TokenItem
}

// NewStore creates a new token store.
func NewStore(tokensFilePath, stateFilePath string) *Store {
	return &Store{
		tokensFilePath: tokensFilePath,
		stateFilePath:  stateFilePath,
		items:          make(map[string]*TokenItem),
	}
}

// Load loads tokens.yaml and restores remaining quota from token-state.json.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.loadInternal(false)
}

// ResetAllAndReload reloads tokens.yaml, resets remaining=limit for all tokens, and culls deleted tokens.
func (s *Store) ResetAllAndReload() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.loadInternal(true)
}

func (s *Store) loadInternal(resetAll bool) error {
	logger := logging.Logger()

	// 1. Read and parse tokens.yaml
	data, err := os.ReadFile(s.tokensFilePath)
	if err != nil {
		return fmt.Errorf("failed to read tokens file %s: %w", s.tokensFilePath, err)
	}

	var cfg TokensConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse tokens yaml: %w", err)
	}

	// Validate tokens uniqueness and non-empty values
	seen := make(map[string]struct{}, len(cfg.Tokens))
	for i, t := range cfg.Tokens {
		if t.Token == "" {
			return fmt.Errorf("token[%d].token cannot be empty", i)
		}
		if _, exists := seen[t.Token]; exists {
			return fmt.Errorf("duplicate token detected: %s", t.Name)
		}
		seen[t.Token] = struct{}{}
	}

	// 2. Read existing state if available (only if not resetting all)
	var state TokenState
	stateMap := make(map[string]int64)
	if !resetAll {
		if stateData, err := os.ReadFile(s.stateFilePath); err == nil {
			if err := json.Unmarshal(stateData, &state); err == nil && state.Tokens != nil {
				for k, v := range state.Tokens {
					if v != nil {
						stateMap[k] = v.Remaining
					}
				}
			}
		}
	}

	// 3. Build new items
	newItems := make(map[string]*TokenItem, len(cfg.Tokens))
	for _, def := range cfg.Tokens {
		item := &TokenItem{
			Definition: def,
		}

		if resetAll {
			// Full reset
			item.Remaining.Store(def.Limit)
		} else {
			if remaining, ok := stateMap[def.Token]; ok {
				item.Remaining.Store(remaining)
			} else {
				// New token initialized to limit
				item.Remaining.Store(def.Limit)
			}
		}

		newItems[def.Token] = item
	}

	// 4. Update in-memory state
	s.items = newItems

	// 5. If resetting or initializing, flush state to disk immediately
	if err := s.saveStateInternal(); err != nil {
		logger.Error("failed to flush token state to disk", "error", err)
	}

	logger.Info("token store loaded successfully", "count", len(s.items), "resetAll", resetAll)
	return nil
}

// Authorize checks if a token is valid, active, and has remaining quota.
func (s *Store) Authorize(tokenStr string) (*TokenItem, error) {
	s.mu.RLock()
	item, exists := s.items[tokenStr]
	s.mu.RUnlock()

	if !exists {
		return nil, ErrTokenNotFound
	}

	now := time.Now()
	if !item.Definition.ExpiresAt.IsZero() && now.After(item.Definition.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	remaining := item.Remaining.Load()
	if remaining <= 0 {
		return nil, ErrTokenExhausted
	}

	return item, nil
}

// Deduct decrements the token remaining quota by 1 (write-after deduction).
func (s *Store) Deduct(item *TokenItem) {
	if item != nil {
		item.Remaining.Add(-1)
	}
}

// SaveState flushes the current in-memory token state to disk.
func (s *Store) SaveState() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.saveStateInternal()
}

func (s *Store) saveStateInternal() error {
	state := TokenState{
		Version: 1,
		Tokens:  make(map[string]*TokenStateEntry, len(s.items)),
	}

	for k, item := range s.items {
		state.Tokens[k] = &TokenStateEntry{
			Remaining: item.Remaining.Load(),
		}
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal token state: %w", err)
	}

	return storage.AtomicWriteFile(s.stateFilePath, data, storage.DefaultFilePerm)
}
