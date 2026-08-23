package generator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"

	"mihomo-sub-publisher/internal/config"
)

var (
	// GeneratorVersion can be set during build with -ldflags "-X mihomo-sub-publisher/internal/generator.GeneratorVersion=..."
	GeneratorVersion = "1.1.0"
	// MihomoVersion records the Mihomo library version
	MihomoVersion = "1.19.30"
)

// Snapshot represents an immutable generated configuration in memory.
type Snapshot struct {
	Version         uint64    `json:"version"`
	SourceUpdatedAt time.Time `json:"source_updated_at"`
	GeneratedAt     time.Time `json:"generated_at"`
	SHA256          string    `json:"sha256"`
	SourceSHA256    string    `json:"source_sha256"`
	Fingerprint     string    `json:"fingerprint"`
	Content         []byte    `json:"-"`
}

// Metadata represents generated.yaml.meta.json on disk.
type Metadata struct {
	Version         uint64    `json:"version"`
	Fingerprint     string    `json:"fingerprint"`
	SourceSHA256    string    `json:"source_sha256"`
	SHA256          string    `json:"sha256"`
	GeneratedAt     time.Time `json:"generated_at"`
	SourceUpdatedAt time.Time `json:"source_updated_at"`
}

// ComputeConfigHash calculates a stable SHA256 over generation-relevant config fields.
func ComputeConfigHash(cfg *config.Config) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("config is nil")
	}

	relevant := struct {
		Source     config.SourceConfig
		Extensions config.ExtensionConfig
		Rules      config.RulesConfig
	}{
		Source:     cfg.Source,
		Extensions: cfg.Extensions,
		Rules:      cfg.Rules,
	}

	data, err := yaml.Marshal(relevant)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config for hash: %w", err)
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// ComputeFingerprint calculates the overall generation fingerprint.
func ComputeFingerprint(sourceSHA256, configHash, genVersion, mihomoVer string) string {
	combined := fmt.Sprintf("%s:%s:%s:%s", sourceSHA256, configHash, genVersion, mihomoVer)
	hash := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(hash[:])
}

// FormatYAMLWithComment prepends metadata comments to the generated YAML.
func FormatYAMLWithComment(yamlBytes []byte, sourceUpdatedAt, generatedAt time.Time, sourceSHA string) []byte {
	header := fmt.Sprintf("# source-updated-at: %s\n# generated-at: %s\n# source-sha256: %s\n\n",
		sourceUpdatedAt.UTC().Format(time.RFC3339),
		generatedAt.UTC().Format(time.RFC3339),
		sourceSHA,
	)

	result := make([]byte, 0, len(header)+len(yamlBytes))
	result = append(result, []byte(header)...)
	result = append(result, yamlBytes...)
	return result
}

// SaveMetadata serializes Metadata to JSON.
func (m *Metadata) ToJSON() ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}
