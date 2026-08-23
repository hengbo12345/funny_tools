package ruleproviders

const (
	ClashRulesCNBaseURL = "https://raw.githubusercontent.com/mcxiaochenn/clash-rules-cn/rules"
)

// ClashRulesCNProviders returns the list of provider definitions in clash-rules-cn.
func ClashRulesCNProviders() []ProviderDefinition {
	return []ProviderDefinition{
		{
			Name:     "direct-domain",
			Behavior: "domain",
			URL:      ClashRulesCNBaseURL + "/direct-domain.yaml",
			Interval: 86400,
		},
		{
			Name:     "proxy-domain",
			Behavior: "domain",
			URL:      ClashRulesCNBaseURL + "/proxy-domain.yaml",
			Interval: 86400,
		},
		{
			Name:     "reject-domain",
			Behavior: "domain",
			URL:      ClashRulesCNBaseURL + "/reject-domain.yaml",
			Interval: 86400,
		},
		{
			Name:     "private-domain",
			Behavior: "domain",
			URL:      ClashRulesCNBaseURL + "/private-domain.yaml",
			Interval: 86400,
		},
		{
			Name:     "apple-direct",
			Behavior: "domain",
			URL:      ClashRulesCNBaseURL + "/apple-direct.yaml",
			Interval: 86400,
		},
		{
			Name:     "icloud-domain",
			Behavior: "domain",
			URL:      ClashRulesCNBaseURL + "/icloud-domain.yaml",
			Interval: 86400,
		},
		{
			Name:     "ai-domain",
			Behavior: "domain",
			URL:      ClashRulesCNBaseURL + "/ai-domain.yaml",
			Interval: 86400,
		},
		{
			Name:     "telegram-ip",
			Behavior: "ipcidr",
			URL:      ClashRulesCNBaseURL + "/telegram-ip.yaml",
			Interval: 86400,
		},
		{
			Name:     "china-ip",
			Behavior: "ipcidr",
			URL:      ClashRulesCNBaseURL + "/china-ip.yaml",
			Interval: 86400,
		},
	}
}
