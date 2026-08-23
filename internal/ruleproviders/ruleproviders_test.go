package ruleproviders

import (
	"testing"
)

func TestDefaultRegistryClashRulesCN(t *testing.T) {
	reg := NewDefaultRegistry()
	defs, ok := reg.GetProviders("clash-rules-cn")
	if !ok {
		t.Fatalf("expected clash-rules-cn to be registered")
	}

	if len(defs) == 0 {
		t.Fatalf("expected non-empty providers for clash-rules-cn")
	}

	// Test ToMihomoMap
	def := defs[0]
	m := def.ToMihomoMap("./ruleset/")
	if m["type"] != "http" {
		t.Errorf("expected type http, got %v", m["type"])
	}
	if m["behavior"] != def.Behavior {
		t.Errorf("expected behavior %v, got %v", def.Behavior, m["behavior"])
	}
	if m["path"] != "./ruleset/"+def.Name+".yaml" {
		t.Errorf("expected path ./ruleset/%s.yaml, got %v", def.Name, m["path"])
	}
}
