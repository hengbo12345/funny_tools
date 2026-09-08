package generator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"mihomo-sub-publisher/internal/config"
	"mihomo-sub-publisher/internal/extension"
	"mihomo-sub-publisher/internal/logging"
	"mihomo-sub-publisher/internal/mihomo"
	"mihomo-sub-publisher/internal/ruleproviders"
	"mihomo-sub-publisher/internal/source"
	"mihomo-sub-publisher/internal/storage"
)

// Pipeline coordinates the end-to-end generation of Mihomo subscription configs.
type Pipeline struct {
	mu          sync.Mutex
	cfg         *config.Config
	fetcher     *source.Fetcher
	registry    *ruleproviders.Registry
	extEngine   *extension.Engine
	snapshotMgr *SnapshotManager
}

// NewPipeline creates a new generation pipeline.
func NewPipeline(
	cfg *config.Config,
	fetcher *source.Fetcher,
	registry *ruleproviders.Registry,
	extEngine *extension.Engine,
	snapshotMgr *SnapshotManager,
) *Pipeline {
	return &Pipeline{
		cfg:         cfg,
		fetcher:     fetcher,
		registry:    registry,
		extEngine:   extEngine,
		snapshotMgr: snapshotMgr,
	}
}

// Run executes the generation pipeline synchronously under a mutex lock.
func (p *Pipeline) Run(ctx context.Context, inputResult *source.FetchResult) (*Snapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	logger := logging.Logger()

	// 1. Fetch source subscription if not provided
	fetchRes := inputResult
	if fetchRes == nil {
		var err error
		fetchRes, err = p.fetcher.Fetch(ctx)
		if err != nil {
			logger.Error("generation.failure", "stage", "fetch", "error", err)
			return nil, fmt.Errorf("fetch failed: %w", err)
		}
	}

	// 2. Compute fingerprint
	configHash, err := ComputeConfigHash(p.cfg)
	if err != nil {
		logger.Error("generation.failure", "stage", "config_hash", "error", err)
		return nil, fmt.Errorf("config hash failed: %w", err)
	}

	fingerprint := ComputeFingerprint(fetchRes.SHA256, configHash, GeneratorVersion, MihomoVersion)

	currentSnap := p.snapshotMgr.Get()
	if currentSnap != nil && currentSnap.Fingerprint == fingerprint {
		logger.Info("generation skipped: fingerprint unchanged", "fingerprint", fingerprint)
		if !maps.Equal(currentSnap.Headers, fetchRes.Headers) {
			// updatedSnap is a copy of currentSnap with updated headers; Content byte slice is immutable.
			updatedSnap := *currentSnap
			updatedSnap.Headers = fetchRes.Headers
			updatedSnap.SourceUpdatedAt = fetchRes.UpdatedAt
			p.snapshotMgr.Swap(&updatedSnap)

			p.saveMetadata(p.cfg.Storage.CacheDir, updatedSnap.ToMetadata())
			return &updatedSnap, nil
		}
		return currentSnap, nil
	}

	// 3. First Mihomo Raw Parse & Validation
	rawConfig, err := mihomo.UnmarshalRaw(fetchRes.Content)
	if err != nil {
		logger.Error("config.parse.failure", "stage", "source_raw_parse", "error", err)
		return nil, fmt.Errorf("source unmarshal failed: %w", err)
	}

	if err := mihomo.ValidateRaw(rawConfig); err != nil {
		logger.Warn("source validation warning", "error", err)
		// Proceed as extensions might fix incomplete source rules
	}

	// 4. Apply Extension DSL
	if err := p.extEngine.Apply(rawConfig, p.cfg.Extensions); err != nil {
		logger.Error("extension.apply.failure", "error", err)
		return nil, fmt.Errorf("extension apply failed: %w", err)
	}
	logger.Info("extension.apply.success")

	// 5. Inject Rule Provider Declarations
	if rawConfig.RuleProvider == nil {
		rawConfig.RuleProvider = make(map[string]map[string]any)
	}

	for groupName, pCfg := range p.cfg.Rules.Providers {
		if !pCfg.Enabled {
			continue
		}

		defs, found := p.registry.GetProviders(groupName)
		if !found {
			// If not in registry, check if custom URL was provided in config
			if pCfg.URL != "" {
				rawConfig.RuleProvider[groupName] = map[string]any{
					"type":     "http",
					"behavior": "domain",
					"url":      pCfg.URL,
					"path":     pCfg.ClientPathPrefix + groupName + ".yaml",
					"interval": 86400,
				}
			} else {
				logger.Warn("unknown rule provider group", "name", groupName)
			}
			continue
		}

		for _, def := range defs {
			rawConfig.RuleProvider[def.Name] = def.ToMihomoMap(pCfg.ClientPathPrefix)
		}
	}

	// Ensure any source rule-providers also have a default path if not specified
	for name, pMap := range rawConfig.RuleProvider {
		if path, ok := pMap["path"].(string); !ok || path == "" {
			pMap["path"] = "./ruleset/" + name + ".yaml"
		}
	}

	// 6. Serialize raw configuration to YAML
	yamlBytes, err := yaml.Marshal(rawConfig)
	if err != nil {
		logger.Error("generation.failure", "stage", "serialization", "error", err)
		return nil, fmt.Errorf("yaml marshal failed: %w", err)
	}

	// 7. Prefix YAML header with metadata comments
	generatedAt := time.Now()
	finalYAML := FormatYAMLWithComment(yamlBytes, fetchRes.UpdatedAt, generatedAt, fetchRes.SHA256)

	// 8. Second Mihomo Parse & Final Validate
	finalRaw, err := mihomo.UnmarshalRaw(finalYAML)
	if err != nil {
		logger.Error("generation.failure", "stage", "final_parse", "error", err)
		return nil, fmt.Errorf("final parse failed: %w", err)
	}

	if err := mihomo.ValidateRaw(finalRaw); err != nil {
		logger.Error("generation.failure", "stage", "final_validation", "error", err)
		return nil, fmt.Errorf("final validation failed: %w", err)
	}

	// 9. Compute final content hash
	finalHash := sha256.Sum256(finalYAML)
	finalSHAStr := hex.EncodeToString(finalHash[:])

	var newVersion uint64 = 1
	if currentSnap != nil {
		newVersion = currentSnap.Version + 1
	}

	newSnap := &Snapshot{
		Metadata: Metadata{
			Version:         newVersion,
			SourceUpdatedAt: fetchRes.UpdatedAt,
			GeneratedAt:     generatedAt,
			SHA256:          finalSHAStr,
			SourceSHA256:    fetchRes.SHA256,
			Fingerprint:     fingerprint,
			Headers:         fetchRes.Headers,
		},
		Content: finalYAML,
	}

	// 10. Write cache files to disk atomically
	cacheDir := p.cfg.Storage.CacheDir
	if err := storage.EnsureDir(cacheDir, storage.DefaultDirPerm); err != nil {
		logger.Error("failed to create cache dir", "dir", cacheDir, "error", err)
	} else {
		if err := storage.AtomicWriteFile(filepath.Join(cacheDir, "source.yaml"), fetchRes.Content, storage.DefaultFilePerm); err != nil {
			logger.Warn("failed to write source.yaml to cache", "error", err)
		}
		if err := storage.AtomicWriteFile(filepath.Join(cacheDir, "generated.yaml"), finalYAML, storage.DefaultFilePerm); err != nil {
			logger.Warn("failed to write generated.yaml to cache", "error", err)
		}
		p.saveMetadata(cacheDir, newSnap.ToMetadata())
	}

	// 11. Atomic swap in memory
	p.snapshotMgr.Swap(newSnap)
	logger.Info("generation.success", "version", newSnap.Version, "sha256", newSnap.SHA256)
	return newSnap, nil
}

func (p *Pipeline) saveMetadata(cacheDir string, meta Metadata) {
	logger := logging.Logger()
	metaJSON, err := meta.ToJSON()
	if err != nil {
		logger.Warn("failed to serialize metadata JSON", "error", err)
		return
	}
	if err := storage.AtomicWriteFile(filepath.Join(cacheDir, "generated.yaml.meta.json"), metaJSON, storage.DefaultFilePerm); err != nil {
		logger.Warn("failed to write generated.yaml.meta.json", "error", err)
	}
}
