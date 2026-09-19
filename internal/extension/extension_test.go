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
				PrependProxies: []string{"node-a", "node-b"},
			},
			{
				Target:        "GPT",
				AppendProxies: []string{"node-c"},
			},
		},
	}

	res := ApplyProxyGroups(groups, ext)
	if len(res) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(res))
	}

	// Group "Proxy" has upstream default "AUTO", which is preserved at index 0
	proxyList := res[0]["proxies"].([]any)
	expectedProxy := []string{"AUTO", "node-a", "node-b", "DIRECT"}
	if len(proxyList) != len(expectedProxy) {
		t.Fatalf("expected %d proxies in Proxy group, got %d: %v", len(expectedProxy), len(proxyList), proxyList)
	}
	for i, exp := range expectedProxy {
		if proxyList[i] != exp {
			t.Errorf("Proxy[%d] = %v, want %v", i, proxyList[i], exp)
		}
	}

	// Group "GPT" has upstream default "Proxy", which is preserved at index 0
	gptList := res[1]["proxies"].([]any)
	expectedGPT := []string{"Proxy", "node-a", "node-b", "DIRECT", "node-c"}
	if len(gptList) != len(expectedGPT) {
		t.Fatalf("expected %d proxies in GPT group, got %d: %v", len(expectedGPT), len(gptList), gptList)
	}
	for i, exp := range expectedGPT {
		if gptList[i] != exp {
			t.Errorf("GPT[%d] = %v, want %v", i, gptList[i], exp)
		}
	}
}

func TestApplyProxyGroupsPreserveDefaultProxy(t *testing.T) {
	groups := []map[string]any{
		{
			"name":    "Proxy",
			"type":    "select",
			"proxies": []any{"HK-01", "US-01", "DIRECT"},
		},
		{
			"name":    "Media",
			"type":    "select",
			"proxies": []any{"SG-01", "DIRECT"},
		},
	}

	ext := config.ProxyGroupsExtension{
		Inject: []config.ProxyGroupInject{
			{
				Target:         "Proxy",
				PrependProxies: []string{"AUTO", "DIRECT"},
			},
		},
	}

	res := ApplyProxyGroups(groups, ext)
	if len(res) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(res))
	}

	// For group "Proxy", upstream default was "HK-01".
	// Even though "AUTO" was prepended, "HK-01" must remain at index 0.
	proxyList := res[0]["proxies"].([]any)
	if len(proxyList) < 1 || proxyList[0] != "HK-01" {
		t.Fatalf("expected first proxy in Proxy group to be HK-01, got %v", proxyList)
	}

	// For group "Media", upstream default was "SG-01", untouched.
	mediaList := res[1]["proxies"].([]any)
	if len(mediaList) < 1 || mediaList[0] != "SG-01" {
		t.Fatalf("expected first proxy in Media group to be SG-01, got %v", mediaList)
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

func TestApplyProxyGroupsPreserveDefaultDisabled(t *testing.T) {
	groups := []map[string]any{
		{
			"name":    "Proxy",
			"type":    "select",
			"proxies": []any{"HK-01", "US-01", "DIRECT"},
		},
	}

	no := false
	ext := config.ProxyGroupsExtension{
		PreserveDefault: &no,
		Inject: []config.ProxyGroupInject{
			{
				Target:         "Proxy",
				PrependProxies: []string{"AUTO"},
			},
		},
	}

	res := ApplyProxyGroups(groups, ext)
	proxyList := res[0]["proxies"].([]any)
	if len(proxyList) < 1 || proxyList[0] != "AUTO" {
		t.Fatalf("expected prepended AUTO at index 0 when preserve-default is false, got %v", proxyList)
	}
}

func TestApplyProxyGroupsReplaceKeepsExplicitOrder(t *testing.T) {
	groups := []map[string]any{
		{
			"name":    "Proxy",
			"type":    "select",
			"proxies": []any{"HK-01", "US-01", "DIRECT"},
		},
	}

	ext := config.ProxyGroupsExtension{
		Replace: []config.ProxyGroupReplace{
			{
				Match: "Proxy",
				Value: map[string]any{
					"name":    "Proxy",
					"type":    "select",
					"proxies": []any{"AUTO", "HK-01", "DIRECT"},
				},
			},
		},
	}

	res := ApplyProxyGroups(groups, ext)
	proxyList := res[0]["proxies"].([]any)
	if len(proxyList) < 1 || proxyList[0] != "AUTO" {
		t.Fatalf("expected replaced group's explicit first proxy AUTO at index 0, got %v", proxyList)
	}
}

func TestEnsureRuleProxyGroups_MissingGroupCreated(t *testing.T) {
	initialGroups := []map[string]any{
		{
			"name":    "FirstGroup",
			"type":    "select",
			"proxies": []any{"HK-01", "US-01", "DIRECT"},
		},
	}

	proxiesExt := config.ProxiesExtension{
		Prepend: []map[string]any{
			{"name": "SelfNode-1", "type": "ss"},
		},
		Append: []map[string]any{
			{"name": "SelfNode-2", "type": "vmess"},
		},
	}

	rulesExt := config.RulesExtension{
		Prepend: []string{
			"RULE-SET,telegram,TelegramGroup",
			"DOMAIN-SUFFIX,google.com,GoogleGroup",
		},
		Replace: []config.RuleReplace{
			{
				Match: "DOMAIN-SUFFIX,openai.com,DIRECT",
				Value: "DOMAIN-SUFFIX,openai.com,OpenAIGroup",
			},
		},
		Append: []string{
			"MATCH,FallbackGroup",
		},
	}

	res := EnsureRuleProxyGroups(initialGroups, rulesExt, proxiesExt, nil)

	// We expect FirstGroup + TelegramGroup + GoogleGroup + OpenAIGroup + FallbackGroup = 5 groups
	if len(res) != 5 {
		t.Fatalf("expected 5 groups, got %d", len(res))
	}

	expectedCreated := []string{"TelegramGroup", "GoogleGroup", "OpenAIGroup", "FallbackGroup"}
	expectedProxies := []any{"SelfNode-1", "SelfNode-2", "HK-01", "US-01", "DIRECT"}

	for i, groupName := range expectedCreated {
		group := res[i+1]
		if group["name"] != groupName {
			t.Errorf("group %d name: got %v, want %v", i+1, group["name"], groupName)
		}
		if group["type"] != "select" {
			t.Errorf("group %s type: got %v, want select", groupName, group["type"])
		}

		pList, ok := group["proxies"].([]any)
		if !ok {
			t.Fatalf("group %s proxies is not []any: %T", groupName, group["proxies"])
		}
		if len(pList) != len(expectedProxies) {
			t.Fatalf("group %s proxies len: got %d, want %d: %v", groupName, len(pList), len(expectedProxies), pList)
		}
		for j, exp := range expectedProxies {
			if pList[j] != exp {
				t.Errorf("group %s proxies[%d]: got %v, want %v", groupName, j, pList[j], exp)
			}
		}
	}
}

func TestEnsureRuleProxyGroups_BuiltinAndExistingProxiesIgnored(t *testing.T) {
	initialGroups := []map[string]any{
		{
			"name":    "FirstGroup",
			"type":    "select",
			"proxies": []any{"HK-01", "DIRECT"},
		},
	}

	existingProxies := []map[string]any{
		{"name": "StandaloneNode", "type": "ss"},
	}

	rulesExt := config.RulesExtension{
		Prepend: []string{
			"RULE-SET,direct,DIRECT",
			"RULE-SET,reject,REJECT",
			"RULE-SET,reject-drop,REJECT-DROP",
			"RULE-SET,pass,PASS",
			"MATCH,COMPATIBLE",
			"DOMAIN,example.com,FirstGroup",     // Already exists
			"DOMAIN,proxy.com,StandaloneNode", // Already a proxy node
		},
	}

	res := EnsureRuleProxyGroups(initialGroups, rulesExt, config.ProxiesExtension{}, existingProxies)

	// No new groups should be created
	if len(res) != 1 {
		t.Fatalf("expected 1 group, got %d: %v", len(res), res)
	}
}

func TestEnsureRuleProxyGroups_DeduplicationAndOrder(t *testing.T) {
	initialGroups := []map[string]any{
		{
			"name":    "AirportDefault",
			"type":    "select",
			"proxies": []any{"HK-01", "SelfNode-1", "US-01", "DIRECT"},
		},
	}

	proxiesExt := config.ProxiesExtension{
		Prepend: []map[string]any{
			{"name": "SelfNode-1", "type": "ss"}, // Overlaps with AirportDefault
		},
		Append: []map[string]any{
			{"name": "SelfNode-2", "type": "socks5"},
		},
		Remove: []string{"RemovedNode"},
	}

	rulesExt := config.RulesExtension{
		Prepend: []string{
			"DOMAIN-SUFFIX,a.com,NewGroup",
			"DOMAIN-SUFFIX,b.com,NewGroup", // Duplicate reference
		},
	}

	res := EnsureRuleProxyGroups(initialGroups, rulesExt, proxiesExt, nil)

	if len(res) != 2 {
		t.Fatalf("expected 2 groups (AirportDefault + NewGroup), got %d", len(res))
	}

	newGroup := res[1]
	if newGroup["name"] != "NewGroup" {
		t.Errorf("expected group name NewGroup, got %v", newGroup["name"])
	}

	pList := newGroup["proxies"].([]any)
	// Union: SelfNode-1, SelfNode-2 (custom first), then HK-01, US-01, DIRECT (AirportDefault nodes without SelfNode-1 duplicate)
	expected := []string{"SelfNode-1", "SelfNode-2", "HK-01", "US-01", "DIRECT"}
	if len(pList) != len(expected) {
		t.Fatalf("expected %d proxies, got %d: %v", len(expected), len(pList), pList)
	}
	for i, exp := range expected {
		if pList[i] != exp {
			t.Errorf("pos %d: got %v, want %v", i, pList[i], exp)
		}
	}
}

func TestEngineApply_AutoCreateProxyGroups(t *testing.T) {
	engine := NewEngine()

	var raw mihomoCfg.RawConfig
	raw.Proxy = []map[string]any{
		{"name": "HK-01", "type": "ss"},
	}
	raw.ProxyGroup = []map[string]any{
		{
			"name":    "Airport-Select",
			"type":    "select",
			"proxies": []any{"HK-01", "DIRECT"},
		},
	}
	raw.Rule = []string{
		"DOMAIN-SUFFIX,upstream.com,DIRECT",
	}

	ext := config.ExtensionConfig{
		Proxies: config.ProxiesExtension{
			Prepend: []map[string]any{
				{"name": "MyVPS", "type": "ss"},
			},
		},
		Rules: config.RulesExtension{
			Prepend: []string{
				"RULE-SET,direct-domain,DIRECT",
			},
			Append: []string{
				"MATCH,PROXY",
			},
		},
	}

	err := engine.Apply(&raw, ext)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if len(raw.ProxyGroup) != 2 {
		t.Fatalf("expected 2 proxy groups, got %d", len(raw.ProxyGroup))
	}

	createdGroup := raw.ProxyGroup[1]
	if createdGroup["name"] != "PROXY" {
		t.Errorf("expected PROXY group name, got %v", createdGroup["name"])
	}
	if createdGroup["type"] != "select" {
		t.Errorf("expected type select, got %v", createdGroup["type"])
	}

	pList := createdGroup["proxies"].([]any)
	expected := []string{"MyVPS", "HK-01", "DIRECT"}
	if len(pList) != len(expected) {
		t.Fatalf("expected %d proxies, got %d: %v", len(expected), len(pList), pList)
	}
	for i, exp := range expected {
		if pList[i] != exp {
			t.Errorf("PROXY proxies[%d] = %v, want %v", i, pList[i], exp)
		}
	}
}

func TestEnsureRuleProxyGroups_EmptyFallbackToDIRECT(t *testing.T) {
	// When there are no proxies and no groups, the created proxy-group must fallback to ["DIRECT"]
	// to avoid Clash client parse error: "proxy group must have at least one proxy"
	rulesExt := config.RulesExtension{
		Append: []string{"MATCH,PROXY"},
	}

	res := EnsureRuleProxyGroups(nil, rulesExt, config.ProxiesExtension{}, nil)
	if len(res) != 1 {
		t.Fatalf("expected 1 group created, got %d", len(res))
	}

	pList := res[0]["proxies"].([]any)
	if len(pList) != 1 || pList[0] != "DIRECT" {
		t.Fatalf("expected proxies to fallback to [DIRECT], got %v", pList)
	}
}

func TestEnsureRuleProxyGroups_FiltersGhostProxies(t *testing.T) {
	existingProxies := []map[string]any{
		{"name": "ValidNode", "type": "ss"},
	}

	proxiesExt := config.ProxiesExtension{
		Replace: []config.ProxyReplace{
			{Match: "NonExistent", Value: map[string]any{"name": "GhostNode"}},
		},
		Prepend: []map[string]any{
			{"name": "ValidNode", "type": "ss"},
		},
	}

	rulesExt := config.RulesExtension{
		Append: []string{"MATCH,PROXY"},
	}

	res := EnsureRuleProxyGroups(nil, rulesExt, proxiesExt, existingProxies)
	if len(res) != 1 {
		t.Fatalf("expected 1 group, got %d", len(res))
	}

	pList := res[0]["proxies"].([]any)
	for _, p := range pList {
		if p == "GhostNode" {
			t.Fatalf("GhostNode should have been filtered out as it does not exist in proxies")
		}
	}
}
