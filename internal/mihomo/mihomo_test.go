package mihomo

import (
	"testing"
)

func TestUnmarshalAndValidateSuccess(t *testing.T) {
	yamlData := `
mixed-port: 7890
mode: rule
log-level: info

proxies:
  - name: "HK-01"
    type: ss
    server: 1.2.3.4
    port: 8388
    cipher: aes-128-gcm
    password: test

proxy-groups:
  - name: "Proxy"
    type: select
    proxies:
      - "HK-01"
      - DIRECT

rule-providers:
  direct-domain:
    type: http
    behavior: domain
    url: "https://example.com/rules/direct.yaml"
    path: "./ruleset/direct.yaml"

rules:
  - RULE-SET,direct-domain,DIRECT
  - DOMAIN-SUFFIX,google.com,Proxy
  - MATCH,DIRECT
`
	raw, err := UnmarshalRaw([]byte(yamlData))
	if err != nil {
		t.Fatalf("UnmarshalRaw failed: %v", err)
	}

	if err := ValidateRaw(raw); err != nil {
		t.Fatalf("ValidateRaw failed: %v", err)
	}

	if len(raw.Proxy) != 1 {
		t.Errorf("expected 1 proxy, got %d", len(raw.Proxy))
	}
	if len(raw.ProxyGroup) != 1 {
		t.Errorf("expected 1 proxy-group, got %d", len(raw.ProxyGroup))
	}
	if len(raw.Rule) != 3 {
		t.Errorf("expected 3 rules, got %d", len(raw.Rule))
	}
}

func TestValidateUnknownRuleSet(t *testing.T) {
	yamlData := `
mixed-port: 7890
proxies:
  - name: "HK-01"
    type: direct

proxy-groups:
  - name: "Proxy"
    type: select
    proxies:
      - DIRECT

rules:
  - RULE-SET,non-existent-provider,DIRECT
  - MATCH,DIRECT
`
	raw, err := UnmarshalRaw([]byte(yamlData))
	if err != nil {
		t.Fatalf("UnmarshalRaw failed: %v", err)
	}

	err = ValidateRaw(raw)
	if err == nil {
		t.Fatalf("expected error for unreferenced rule-set, got nil")
	}
}
