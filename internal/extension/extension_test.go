package extension

import (
	"testing"

	mihomoCfg "github.com/metacubex/mihomo/config"
	"gopkg.in/yaml.v3"

	"mihomo-sub-publisher/internal/config"
)

func TestApplyProxies(t *testing.T) {
	initial := []map[string]any{
		{"name": "p1", "type": "ss"},
		{"name": "p2", "type": "vmess"},
		{"name": "p3", "type": "trojan"},
	}

	ext := config.ProxiesExtension{
		Prepend: []map[string]any{
			{"name": "p0", "type": "direct"},
		},
		Append: []map[string]any{
			{"name": "p4", "type": "socks5"},
		},
		Replace: []config.ProxyReplace{
			{Match: "p2", Value: map[string]any{"name": "p2-replaced", "type": "vmess-new"}},
			{Match: "non-existent", Value: map[string]any{"name": "none"}},
		},
		Remove: []string{"p3", "non-existent-2"},
	}

	res := ApplyProxies(initial, ext)
	if len(res) != 4 {
		t.Fatalf("expected 4 proxies, got %d", len(res))
	}

	names := make([]string, len(res))
	for i, p := range res {
		names[i] = p["name"].(string)
	}

	expected := []string{"p0", "p1", "p2-replaced", "p4"}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("pos %d: got %s, want %s", i, name, expected[i])
		}
	}
}

func TestApplyProxyGroupsInjection(t *testing.T) {
	groups := []map[string]any{
		{
			"name":    "Proxy",
			"type":    "select",
			"proxies": []any{"AUTO", "DIRECT"},
		},
		{
			"name":    "GPT",
			"type":    "select",
			"proxies": []any{"Proxy", "DIRECT"},
		},
	}

	ext := config.ProxyGroupsExtension{
		Inject: []config.ProxyGroupInject{
			{
				Target:         "*",
				PrependProxies: []string{"UK 自用代理", "JP 自用代理"},
			},
			{
				Target:        "GPT",
				AppendProxies: []string{"US 自用代理"},
			},
		},
	}

	res := ApplyProxyGroups(groups, ext)
	if len(res) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(res))
	}

	// Group "Proxy" should have UK, JP prepended
	proxyList := res[0]["proxies"].([]any)
	expectedProxy := []string{"UK 自用代理", "JP 自用代理", "AUTO", "DIRECT"}
	if len(proxyList) != len(expectedProxy) {
		t.Fatalf("expected %d proxies in Proxy group, got %d: %v", len(expectedProxy), len(proxyList), proxyList)
	}
	for i, exp := range expectedProxy {
		if proxyList[i] != exp {
			t.Errorf("Proxy[%d] = %v, want %v", i, proxyList[i], exp)
		}
	}

	// Group "GPT" should have UK, JP prepended and US appended
	gptList := res[1]["proxies"].([]any)
	expectedGPT := []string{"UK 自用代理", "JP 自用代理", "Proxy", "DIRECT", "US 自用代理"}
	if len(gptList) != len(expectedGPT) {
		t.Fatalf("expected %d proxies in GPT group, got %d: %v", len(expectedGPT), len(gptList), gptList)
	}
	for i, exp := range expectedGPT {
		if gptList[i] != exp {
			t.Errorf("GPT[%d] = %v, want %v", i, gptList[i], exp)
		}
	}
}

func TestApplyRulesDeduplication(t *testing.T) {
	initial := []string{
		"DOMAIN-SUFFIX,google.com,PROXY",
		"DOMAIN,example.com,DIRECT",
		"MATCH,DIRECT",
	}

	ext := config.RulesExtension{
		Prepend: []string{
			"RULE-SET,direct-domain,DIRECT",
			"DOMAIN-SUFFIX,google.com,PROXY", // Duplicate with source rule
		},
		Append: []string{
			"MATCH,PROXY",
			"MATCH,DIRECT", // Duplicate with source rule
		},
		Replace: []config.RuleReplace{
			{Match: "DOMAIN,example.com,DIRECT", Value: "DOMAIN,example.com,PROXY"},
		},
		Remove: []string{},
	}

	res := ApplyRules(initial, ext)

	// Prepend: [RULE-SET,direct-domain,DIRECT, DOMAIN-SUFFIX,google.com,PROXY]
	// Source: [DOMAIN,example.com,PROXY, MATCH,DIRECT] (note google.com was in source, but already seen in prepend, so first occurrence preserved)
	// Append: [MATCH,PROXY] (MATCH,DIRECT already seen)
	expected := []string{
		"RULE-SET,direct-domain,DIRECT",
		"DOMAIN-SUFFIX,google.com,PROXY",
		"DOMAIN,example.com,PROXY",
		"MATCH,DIRECT",
		"MATCH,PROXY",
	}

	if len(res) != len(expected) {
		t.Fatalf("expected %d rules, got %d: %v", len(expected), len(res), res)
	}

	for i, r := range res {
		if r != expected[i] {
			t.Errorf("rule[%d] = %q, want %q", i, r, expected[i])
		}
	}
}

func TestApplyProbe(t *testing.T) {
	groups := []map[string]any{
		{"name": "auto", "type": "url-test", "url": "http://old.url", "interval": 600},
		{"name": "select-grp", "type": "select", "url": "http://select.url"},
	}

	ext := config.ProbeExtension{
		Override: map[string]any{
			"url":      "https://www.gstatic.com/generate_204",
			"interval": 300,
			"timeout":  5000,
		},
	}

	res := ApplyProbe(groups, ext)
	if len(res) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(res))
	}

	// Auto group should be overridden
	if res[0]["url"] != "https://www.gstatic.com/generate_204" {
		t.Errorf("expected overridden url, got %v", res[0]["url"])
	}
	if res[0]["interval"] != 300 {
		t.Errorf("expected overridden interval, got %v", res[0]["interval"])
	}
	if res[0]["timeout"] != 5000 {
		t.Errorf("expected overridden timeout, got %v", res[0]["timeout"])
	}

	// Select group should remain untouched
	if res[1]["url"] != "http://select.url" {
		t.Errorf("expected select group url to remain untouched, got %v", res[1]["url"])
	}
}

func TestApplyDNS(t *testing.T) {
	var rawDNS mihomoCfg.RawDNS
	yaml.Unmarshal([]byte("enable: false\nenhanced-mode: redir-host\n"), &rawDNS)

	ext := config.DNSExtension{
		Override: map[string]any{
			"enable":        true,
			"enhanced-mode": "fake-ip",
		},
	}

	err := ApplyDNS(&rawDNS, ext)
	if err != nil {
		t.Fatalf("ApplyDNS failed: %v", err)
	}

	if !rawDNS.Enable {
		t.Errorf("expected enable true, got %v", rawDNS.Enable)
	}
	if rawDNS.EnhancedMode.String() != "fake-ip" {
		t.Errorf("expected enhanced-mode fake-ip, got %v", rawDNS.EnhancedMode)
	}
}
