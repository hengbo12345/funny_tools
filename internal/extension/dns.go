package extension

import (
	mihomoCfg "github.com/metacubex/mihomo/config"
	"gopkg.in/yaml.v3"

	"mihomo-sub-publisher/internal/config"
)

// ApplyDNS performs shallow merge of DNS override map into raw DNS config.
func ApplyDNS(rawDNS *mihomoCfg.RawDNS, ext config.DNSExtension) error {
	if len(ext.Override) == 0 {
		return nil
	}

	// 1. Marshal current rawDNS to YAML map
	data, err := yaml.Marshal(rawDNS)
	if err != nil {
		return err
	}

	var currentMap map[string]any
	if err := yaml.Unmarshal(data, &currentMap); err != nil {
		currentMap = make(map[string]any)
	}
	if currentMap == nil {
		currentMap = make(map[string]any)
	}

	// 2. Shallow merge override top-level keys
	for k, v := range ext.Override {
		currentMap[k] = v
	}

	// 3. Unmarshal back to RawDNS
	mergedData, err := yaml.Marshal(currentMap)
	if err != nil {
		return err
	}

	var newDNS mihomoCfg.RawDNS
	if err := yaml.Unmarshal(mergedData, &newDNS); err != nil {
		return err
	}

	*rawDNS = newDNS
	return nil
}
