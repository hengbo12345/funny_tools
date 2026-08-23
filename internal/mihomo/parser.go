package mihomo

import (
	"fmt"

	mihomoCfg "github.com/metacubex/mihomo/config"
)

// UnmarshalRaw parses raw YAML into Mihomo's RawConfig structure.
func UnmarshalRaw(buf []byte) (*mihomoCfg.RawConfig, error) {
	raw, err := mihomoCfg.UnmarshalRawConfig(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal raw mihomo config: %w", err)
	}
	if raw == nil {
		return nil, fmt.Errorf("parsed mihomo config is nil")
	}
	return raw, nil
}
