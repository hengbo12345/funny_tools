package generator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"mihomo-sub-publisher/internal/logging"
	"mihomo-sub-publisher/internal/mihomo"
)

// SnapshotManager manages the atomic in-memory snapshot and on-disk recovery.
type SnapshotManager struct {
	current atomic.Pointer[Snapshot]
}

// NewSnapshotManager creates a new snapshot manager.
func NewSnapshotManager() *SnapshotManager {
	return &SnapshotManager{}
}

// Get returns the current active snapshot (immutable).
func (m *SnapshotManager) Get() *Snapshot {
	return m.current.Load()
}

// Swap atomically replaces the current snapshot.
func (m *SnapshotManager) Swap(newSnap *Snapshot) {
	m.current.Store(newSnap)
}

// Restore attempts to restore and validate last-known-good snapshot from disk.
func (m *SnapshotManager) Restore(cacheDir string) (*Snapshot, error) {
	logger := logging.Logger()

	genFile := filepath.Join(cacheDir, "generated.yaml")
	metaFile := filepath.Join(cacheDir, "generated.yaml.meta.json")

	content, err := os.ReadFile(genFile)
	if err != nil {
		return nil, fmt.Errorf("no generated.yaml found in %s: %w", cacheDir, err)
	}

	// Validate content with Mihomo parser
	raw, err := mihomo.UnmarshalRaw(content)
	if err != nil {
		return nil, fmt.Errorf("generated.yaml failed raw parse: %w", err)
	}
	if err := mihomo.ValidateRaw(raw); err != nil {
		return nil, fmt.Errorf("generated.yaml failed structural validation: %w", err)
	}

	// Read metadata if present
	var meta Metadata
	if metaData, err := os.ReadFile(metaFile); err == nil {
		_ = json.Unmarshal(metaData, &meta)
	}

	hash := sha256.Sum256(content)
	shaStr := hex.EncodeToString(hash[:])

	snap := &Snapshot{
		Version:         meta.Version,
		SourceUpdatedAt: meta.SourceUpdatedAt,
		GeneratedAt:     meta.GeneratedAt,
		SHA256:          shaStr,
		SourceSHA256:    meta.SourceSHA256,
		Fingerprint:     meta.Fingerprint,
		Headers:         meta.Headers,
		Content:         content,
	}

	m.Swap(snap)
	logger.Info("restored last-known-good snapshot from disk", "version", snap.Version, "sha256", snap.SHA256)
	return snap, nil
}
