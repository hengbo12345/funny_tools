# Code Review 修复实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 2026-09-08 两个提交（`c07e8e0`、`ce73c0c`）code review 发现的 10 项问题（8 项采纳修复，2 项经用户决策保持现状）。

**Architecture:** 四条独立修复线——(1) proxy-groups 默认代理保留行为加 opt-out 开关并排除 replace 组；(2) User-Agent 策略统一为单一通配符匹配路径 + 配置校验 + `/status` 门控中间件；(3) 旧默认 UA 从运行时特判迁移为 loader 一次性迁移；(4) 若干清理（maps.Equal、共享 testutil、helper 去重）。

**Tech Stack:** Go 1.26.5，标准库（`maps`、`net/http`），gopkg.in/yaml.v3。测试框架为标准 `testing` + `httptest`。

**用户已确认的决策：**
1. Finding #1（preserve 无条件覆盖 prepend/inject）→ **加 opt-out 开关**（默认保留现有行为）
2. Finding #5（header-only 更新不递增 Version）→ **保持现状**（不修）
3. Finding #6（legacy UA 运行时特判）→ **loader 一次性迁移**
4. Finding #4 部分（/status 门控）→ **/status 也加门控**；`/health` 保持开放（docker-compose healthcheck 用 wget 访问 `/health`，其 UA 不含 clash）

**明确不修（验证驳回或用户决策）：**
- `matchWildcard` 算法（验证证明对 `*`-only 模式正确；也不用 `path.Match` 替换——其 `*` 不跨 `/`，而 UA 含 `/` 如 `clash-verge/v1.7.7`）
- `headersEqual` 的 nil/空 map 相等性（行为正确）
- header-only 路径的 Version 语义（保持现状）
- token 403 分支删除（等价简化，无信息丢失）
- `design-v1.1.md` 中的旧 UA 引用（历史设计快照，不回写）

**附带设计决定（执行时注意，用户 review 计划时可见）：**
- preserve 后置步骤**排除被 Replace 的组**：Replace 语义是"用户给出全新组定义"，显式新顺序应胜出。现有测试未覆盖此场景，本计划新增测试固化。
- `g["default"]` 键处理直接删除（mihomo proxy-group 无此 YAML 字段，仓库内无任何代码设置它）。

---

## File Structure

| 文件 | 改动 | 职责 |
|---|---|---|
| `internal/config/loader.go` | 修改 | UA/白名单常量定义、DefaultConfig、Load 时一次性迁移 |
| `internal/config/schema.go` | 修改 | `ProxyGroupsExtension.PreserveDefault` 字段 + 便捷方法 |
| `internal/config/validator.go` | 修改 | AllowedUserAgents / RequiredHeaders 校验 |
| `internal/config/config_test.go` | 修改 | 迁移测试 + 校验测试 |
| `internal/source/fetcher.go` | 修改 | 删除 legacy UA 特判，改用 config 常量 |
| `internal/source/source_test.go` | 修改 | 显式 UA 透传测试 |
| `internal/server/server.go` | 修改 | gate 中间件、预计算 UA patterns、单路径匹配、/status 门控 |
| `internal/server/server_test.go` | 修改 | 门控测试、通配符测试、setupTestServerWithConfig |
| `internal/extension/proxy_groups.go` | 修改 | preserve 开关、排除 replace、删 default 键、proxyListValue helper |
| `internal/extension/extension_test.go` | 修改 | 开关测试、replace 排序测试 |
| `internal/generator/pipeline.go` | 修改 | headersEqual → maps.Equal |
| `internal/testutil/http.go` | 新建 | 跨包共享 GetWithUA |
| `internal/server/server_test.go`、`test/e2e_test.go` | 修改 | 删本地 getWithUA 副本 |
| `configs/config.yaml`、`README.md` | 修改 | 新配置项示例与文档 |

---

### Task 0: 创建修复分支

当前 HEAD 处于 detached 状态（`ce73c0c`），需先建分支再动手。

- [ ] **Step 1: 从当前 HEAD 创建分支**

```bash
git -C /home/ubuntu/code/github/funny_tools checkout -b fix/code-review-findings
```

预期输出：`Switched to a new branch 'fix/code-review-findings'`

---

### Task 1: config 常量统一 + Load 时一次性迁移旧 UA

解决 Finding #6（loader 迁移）与 #10（常量双重定义）。

**Files:**
- Modify: `internal/config/loader.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: 写失败测试**

在 `internal/config/config_test.go` 末尾追加：

```go
func writeTempConfig(t *testing.T, yamlContent string) string {
	t.Helper()
	configFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configFile, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	return configFile
}

func TestLoadMigratesLegacyUserAgent(t *testing.T) {
	// 大小写不同的 header key（user-agent）也应被迁移
	configFile := writeTempConfig(t, `
version: 1
source:
  url: "https://example.com/sub.yaml"
  headers:
    user-agent: "mihomo-sub-publisher/1.0"
`)
	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got := cfg.Source.Headers["user-agent"]; got != DefaultClientUserAgent {
		t.Errorf("expected legacy UA migrated to %q, got %q", DefaultClientUserAgent, got)
	}
}

func TestLoadKeepsExplicitCustomUserAgent(t *testing.T) {
	configFile := writeTempConfig(t, `
version: 1
source:
  url: "https://example.com/sub.yaml"
  headers:
    User-Agent: "my-custom-ua/2.0"
`)
	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got := cfg.Source.Headers["User-Agent"]; got != "my-custom-ua/2.0" {
		t.Errorf("expected explicit UA preserved, got %q", got)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/config/ -run 'TestLoadMigratesLegacyUserAgent|TestLoadKeepsExplicitCustomUserAgent' -v
```

预期：`TestLoadMigratesLegacyUserAgent` FAIL（UA 仍是旧值 `mihomo-sub-publisher/1.0`）；`TestLoadKeepsExplicitCustomUserAgent` 可能 PASS（现状已透传，作为守护测试保留）。

- [ ] **Step 3: 实现**

`internal/config/loader.go`：import 块加 `"strings"` 与 `"mihomo-sub-publisher/internal/logging"`；文件顶部（`DefaultConfig` 之前）加常量；`DefaultConfig` 中 Headers 改用常量；`Load` 中 unmarshal 之后、Validate 之前插入迁移逻辑。修改后的完整文件：

```go
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"mihomo-sub-publisher/internal/logging"
)

const (
	// DefaultClientUserAgent is the default User-Agent sent to upstream subscriptions.
	DefaultClientUserAgent = "clash.meta"
	// LegacyClientUserAgent is the pre-1.2 default, migrated once at config load time.
	LegacyClientUserAgent = "mihomo-sub-publisher/1.0"
	// DefaultAllowedUserAgents are the wildcard UA patterns accepted on gated
	// endpoints (/config, /status) when server.allowed-user-agents is not configured.
	DefaultAllowedUserAgents = []string{"*clash*", "*mihomo*", "*stash*"}
)

// DefaultConfig returns a Config with default values populated.
func DefaultConfig() Config {
	return Config{
		Version: 1,
		Source: SourceConfig{
			Interval: 1 * time.Hour,
			Timeout:  30 * time.Second,
			Headers: map[string]string{
				"User-Agent": DefaultClientUserAgent,
			},
		},
		Server: ServerConfig{
			Listen:          "127.0.0.1:8080",
			ConfigPath:      "/config/{token}",
			ShutdownTimeout: 10 * time.Second,
		},
		Storage: StorageConfig{
			CacheDir: "./cache",
			DataDir:  "./data",
		},
		HotReload: HotReloadConfig{
			Debounce: 1 * time.Minute,
		},
		Rules: RulesConfig{
			Providers: map[string]ProviderConfig{
				"clash-rules-cn": {
					Enabled:          true,
					ClientPathPrefix: "./ruleset/",
				},
			},
		},
	}
}

// Load reads and parses a YAML configuration file.
func Load(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", filePath, err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", filePath, err)
	}

	// One-time migration: rewrite the legacy default User-Agent (header names are
	// case-insensitive, so match any spelling of the key). This runs at load time
	// only — an explicit, non-legacy UA is always preserved verbatim.
	for k, v := range cfg.Source.Headers {
		if strings.EqualFold(k, "User-Agent") && v == LegacyClientUserAgent {
			cfg.Source.Headers[k] = DefaultClientUserAgent
			logging.Logger().Info("config: migrated legacy default User-Agent",
				"from", LegacyClientUserAgent, "to", DefaultClientUserAgent)
		}
	}

	// Apply default provider client path prefixes if not set
	for name, provider := range cfg.Rules.Providers {
		if provider.ClientPathPrefix == "" {
			provider.ClientPathPrefix = "./ruleset/"
			cfg.Rules.Providers[name] = provider
		}
	}

	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("config validation error: %w", err)
	}

	return &cfg, nil
}
```

- [ ] **Step 4: 跑测试确认通过**

```bash
go test ./internal/config/ -v
```

预期：全部 PASS（含原有 `TestLoadValidConfig`、`TestConfigValidationErrors`）。

- [ ] **Step 5: Commit**

```bash
git -C /home/ubuntu/code/github/funny_tools add internal/config/loader.go internal/config/config_test.go
git -C /home/ubuntu/code/github/funny_tools commit -m "feat(config): centralize UA constants and migrate legacy default UA at load time"
```

---

### Task 2: fetcher 移除运行时 legacy 特判

解决 Finding #6 的另一半：每次抓取的运行时字符串比较删除。Task 1 的 loader 迁移已覆盖旧配置文件场景。

**Files:**
- Modify: `internal/source/fetcher.go:85-89`（常量块）、`:120-124`（UA 逻辑）
- Test: `internal/source/source_test.go`

- [ ] **Step 1: 写失败测试（守护显式 UA 透传）**

在 `internal/source/source_test.go` 末尾追加：

```go
func TestFetcherKeepsExplicitUserAgent(t *testing.T) {
	var receivedUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("mixed-port: 7890\nproxies: []\n"))
	}))
	defer server.Close()

	cfg := config.SourceConfig{
		URL:     server.URL,
		Timeout: 2 * time.Second,
		Headers: map[string]string{"User-Agent": "my-custom-ua/2.0"},
	}

	fetcher := NewFetcher(cfg)
	if _, err := fetcher.Fetch(context.Background()); err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if receivedUA != "my-custom-ua/2.0" {
		t.Errorf("expected explicit UA preserved, got %q", receivedUA)
	}
}
```

- [ ] **Step 2: 跑测试确认现状行为**

```bash
go test ./internal/source/ -run TestFetcherKeepsExplicitUserAgent -v
```

预期：PASS（`my-custom-ua/2.0` 不等于旧默认值，现状本就透传）。这是守护测试，防止 Step 3 重构时破坏。

- [ ] **Step 3: 实现**

`internal/source/fetcher.go`：

删除常量块（第 85-89 行）：

```go
const (
	// DefaultClientUserAgent is the default User-Agent sent to upstream subscriptions.
	DefaultClientUserAgent = "clash.meta"
	legacyDefaultUserAgent = "mihomo-sub-publisher/1.0"
)
```

将 doFetch 中 UA 逻辑（第 120-124 行）：

```go
	// Default to a Clash-compatible User-Agent if none set or if legacy default placeholder
	ua := strings.TrimSpace(req.Header.Get("User-Agent"))
	if ua == "" || ua == legacyDefaultUserAgent {
		req.Header.Set("User-Agent", DefaultClientUserAgent)
	}
```

替换为：

```go
	// Default to a Clash-compatible User-Agent if none set.
	// (Legacy-default migration happens once at config load, not per fetch.)
	if strings.TrimSpace(req.Header.Get("User-Agent")) == "" {
		req.Header.Set("User-Agent", config.DefaultClientUserAgent)
	}
```

- [ ] **Step 4: 跑全部 source 测试确认通过**

```bash
go test ./internal/source/ -v
```

预期：全部 PASS（`TestFetcherDefaultClashUAAndSubscriptionHeaders` 验证无 UA 时默认 `clash.meta`，依然成立）。

- [ ] **Step 5: Commit**

```bash
git -C /home/ubuntu/code/github/funny_tools add internal/source/fetcher.go internal/source/source_test.go
git -C /home/ubuntu/code/github/funny_tools commit -m "refactor(source): drop per-fetch legacy UA check, rely on load-time migration"
```

---

### Task 3: validator 校验 AllowedUserAgents / RequiredHeaders

解决 Finding #2：空白配置项静默导致全部请求 404 且无启动报错。

**Files:**
- Modify: `internal/config/validator.go:65-74`（Server 校验块内追加）
- Test: `internal/config/config_test.go`（TestConfigValidationErrors 表）

- [ ] **Step 1: 写失败测试**

在 `internal/config/config_test.go` 的 `TestConfigValidationErrors` 表（`tests` 切片）末尾、`zero version` 用例之后追加三个用例：

```go
		{
			name: "whitespace allowed-user-agents entry",
			modify: func(c *Config) {
				c.Server.AllowedUserAgents = []string{"  "}
			},
			expectError: true,
		},
		{
			name: "empty required-headers key",
			modify: func(c *Config) {
				c.Server.RequiredHeaders = map[string]string{"": "*"}
			},
			expectError: true,
		},
		{
			name: "whitespace required-headers value",
			modify: func(c *Config) {
				c.Server.RequiredHeaders = map[string]string{"X-Api-Key": "  "}
			},
			expectError: true,
		},
```

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/config/ -run TestConfigValidationErrors -v
```

预期：新增三个子测试 FAIL（期望 error 得到 nil）。

- [ ] **Step 3: 实现**

`internal/config/validator.go` 在 `// Validate Server` 块内、`if cfg.Server.ShutdownTimeout <= 0` 检查之后追加：

```go
	for i, ua := range cfg.Server.AllowedUserAgents {
		if strings.TrimSpace(ua) == "" {
			return fmt.Errorf("server.allowed-user-agents[%d] cannot be empty or whitespace", i)
		}
	}
	for key, val := range cfg.Server.RequiredHeaders {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("server.required-headers key cannot be empty or whitespace")
		}
		// "" and "*" both mean "header must be present, value unconstrained";
		// anything else is an exact (case-insensitive) match — whitespace-only is a typo.
		if val != "" && val != "*" && strings.TrimSpace(val) == "" {
			return fmt.Errorf("server.required-headers[%q] value cannot be whitespace-only (use \"*\" to require presence only)", key)
		}
	}
```

- [ ] **Step 4: 跑测试确认通过**

```bash
go test ./internal/config/ -v
```

预期：全部 PASS。

- [ ] **Step 5: Commit**

```bash
git -C /home/ubuntu/code/github/funny_tools add internal/config/validator.go internal/config/config_test.go
git -C /home/ubuntu/code/github/funny_tools commit -m "feat(config): validate allowed-user-agents and required-headers entries"
```

---

### Task 4: server UA 匹配统一 + 预计算 + /status 门控中间件

解决 Finding #3（两套匹配语义）、#4（门控不一致与层级）。统一为单一通配符路径，patterns 在 `NewServer` 预归一化（顺带解决效率候选），`/status` 与 `/config` 共用 `gate` 中间件，`/health` 保持开放。

**Files:**
- Modify: `internal/server/server.go:29-35`（struct）、`:49-56`（mux 注册）、`:149-153`（handleStatus 开头）、`:177-183`（handleConfig 开头）、`:249-290`（isExpectedRequest）
- Test: `internal/server/server_test.go`

- [ ] **Step 1: 写失败测试**

`internal/server/server_test.go`：

(a) 将 `setupTestServer` 拆出可定制版本（原函数改为薄包装，调用点不用改）：

```go
func setupTestServer(t *testing.T) (*Server, *token.Store, *generator.SnapshotManager) {
	return setupTestServerWithConfig(t, nil)
}

func setupTestServerWithConfig(t *testing.T, mutate func(*config.Config)) (*Server, *token.Store, *generator.SnapshotManager) {
	// ... 原 setupTestServer 的全部函数体原样搬入，仅在构造 cfg 之后、NewServer 之前插入：
	//
	//	if mutate != nil {
	//		mutate(cfg)
	//	}
	//
	// （tokensFile/stateFile/tokensYAML/snapMgr/cfg 构造与原函数逐行相同，末尾 return srv, tokenStore, snapMgr 不变）
}
```

(b) 文件末尾追加两个测试：

```go
func TestServerStatusGated(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	defer srv.Shutdown(context.Background())

	baseURL := "http://" + srv.Addr()

	// Clash UA -> 200
	resp, err := getWithUA(baseURL+"/status", "clash.meta")
	if err != nil {
		t.Fatalf("GET /status failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for clash UA, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Non-clash UA -> 404 (cloaking, consistent with /config)
	resp, err = getWithUA(baseURL+"/status", "curl/7.88.1")
	if err != nil {
		t.Fatalf("GET /status failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for non-clash UA, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestServerAllowedUserAgentsWildcard(t *testing.T) {
	srv, tokenStore, _ := setupTestServerWithConfig(t, func(c *config.Config) {
		// Pattern case should not matter (pre-lowered at construction).
		c.Server.AllowedUserAgents = []string{"Clash*"}
	})
	defer srv.Shutdown(context.Background())

	snapMgr := srv.snapshotMgr
	snapMgr.Swap(&generator.Snapshot{
		Metadata: generator.Metadata{
			Version:         1,
			SourceUpdatedAt: time.Now(),
			GeneratedAt:     time.Now(),
			SHA256:          "sha123",
		},
		Content: []byte("mixed-port: 7890\n"),
	})
	item, _ := tokenStore.Authorize("token-valid")
	item.Remaining.Store(100)

	baseURL := "http://" + srv.Addr()

	cases := []struct {
		ua   string
		want int
	}{
		{"clash.meta", http.StatusOK},                // prefix match
		{"ClashforWindows/0.20.39", http.StatusOK},   // prefix match, contains '/'
		{"clash-verge/v1.7.7", http.StatusOK},        // prefix match
		{"mihomo", http.StatusNotFound},              // no longer substring-matched
		{"Stash/2.6.0", http.StatusNotFound},         // outside configured patterns
	}
	for _, tc := range cases {
		resp, err := getWithUA(baseURL+"/config/token-valid", tc.ua)
		if err != nil {
			t.Fatalf("request failed for UA %q: %v", tc.ua, err)
		}
		if resp.StatusCode != tc.want {
			t.Errorf("UA %q expected %d, got %d", tc.ua, tc.want, resp.StatusCode)
		}
		resp.Body.Close()
	}
}
```

注意：`TestServerAllowedUserAgentsWildcard` 用到 `srv.snapshotMgr`——该字段当前未导出但同包测试可直接访问；token 扣减会消耗配额（`Remaining.Store(100)` 保证充足）。

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/server/ -run 'TestServerStatusGated|TestServerAllowedUserAgentsWildcard' -v
```

预期：`TestServerStatusGated` FAIL（默认 UA 的 `/status` 得 200 而非 404——第二个断言失败）；`TestServerAllowedUserAgentsWildcard` FAIL（`Clash*` 精确语义下 `clash.meta` 得 404）。

- [ ] **Step 3: 实现**

`internal/server/server.go`：

(a) `Server` struct 加预计算字段：

```go
// Server handles incoming HTTP requests.
type Server struct {
	httpServer  *http.Server
	cfg         *config.Config
	tokenStore  *token.Store
	snapshotMgr *generator.SnapshotManager
	listener    net.Listener
	// allowedUAPatterns holds the lowercased, trimmed UA wildcard patterns
	// used to gate /config and /status; computed once at construction.
	allowedUAPatterns []string
}
```

(b) `NewServer` 中 `mux` 注册改为包 gate，并在构造时预计算 patterns：

```go
	patterns := cfg.Server.AllowedUserAgents
	if len(patterns) == 0 {
		patterns = config.DefaultAllowedUserAgents
	}
	s.allowedUAPatterns = make([]string, 0, len(patterns))
	for _, p := range patterns {
		s.allowedUAPatterns = append(s.allowedUAPatterns, strings.ToLower(strings.TrimSpace(p)))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/status", s.gate(s.handleStatus))

	// Register config route matching template prefix
	// Default template is "/config/{token}"
	prefix, _ := parseConfigPathPrefix(cfg.Server.ConfigPath)
	mux.HandleFunc(prefix, s.gate(s.handleConfig))
```

(c) 新增 gate 中间件（放在 `isExpectedRequest` 之前）：

```go
// gate rejects requests whose User-Agent / required headers do not look like a
// Clash-compatible client, responding 404 so the endpoint is indistinguishable
// from a wrong path. /health stays open for liveness probes.
func (s *Server) gate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isExpectedRequest(r) {
			s.writeJSONError(w, http.StatusNotFound, "not_found")
			return
		}
		next(w, r)
	}
}
```

(d) `handleConfig` 开头（第 177-183 行）的 UA 检查块删除：

```go
	// Validate headers: only clash-like User-Agents and expected headers are accepted.
	// Unexpected headers return 404 Not Found.
	if !s.isExpectedRequest(r) {
		s.writeJSONError(w, http.StatusNotFound, "not_found")
		return
	}
```

(e) `isExpectedRequest` 改为单一匹配路径（删除 `uaMatched` 标志与 else Contains 分支）：

```go
func (s *Server) isExpectedRequest(r *http.Request) bool {
	// 1. User-Agent check (must match one of the configured wildcard patterns;
	// defaults to *clash*/*mihomo*/*stash* — i.e. substring semantics).
	ua := strings.TrimSpace(r.Header.Get("User-Agent"))
	if ua == "" {
		return false
	}
	lowerUA := strings.ToLower(ua)

	matched := false
	for _, pattern := range s.allowedUAPatterns {
		if matchWildcard(pattern, lowerUA) {
			matched = true
			break
		}
	}
	if !matched {
		return false
	}

	// 2. Required headers check (if configured)
	for reqKey, reqVal := range s.cfg.Server.RequiredHeaders {
		actualVal := r.Header.Get(reqKey)
		if actualVal == "" {
			return false
		}
		if reqVal != "" && reqVal != "*" {
			if !strings.EqualFold(actualVal, reqVal) {
				return false
			}
		}
	}

	return true
}
```

- [ ] **Step 4: 更新受影响的既有测试**

`TestServerHealthAndStatus`（`server_test.go:82-84` 附近）中 `GET /status` 改用 clash UA：

```go
	// 2. Status without snapshot (clash UA required since /status is gated)
	resp, err = getWithUA(baseURL+"/status", "clash.meta")
```

以及该测试后续带 snapshot 的 `/status` 请求（原 `http.Get(baseURL + "/status")`）同样改为 `getWithUA(baseURL+"/status", "clash.meta")`。

`TestServerUserAgentAndHeaderValidation` 无需改动：默认 patterns `*clash*/*mihomo*/*stash*` 与原 substring 分支语义一致（`ClashforWindows`、`Mihomo/1.19.0`、`Stash/2.6.0` 匹配；`curl`、`Mozilla` 等不匹配）。

- [ ] **Step 5: 跑包内全部测试确认通过**

```bash
go test ./internal/server/ -v
```

预期：全部 PASS。

- [ ] **Step 6: Commit**

```bash
git -C /home/ubuntu/code/github/funny_tools add internal/server/server.go internal/server/server_test.go
git -C /home/ubuntu/code/github/funny_tools commit -m "refactor(server): unify UA wildcard matching, precompute patterns, gate /status"
```

---

### Task 5: proxy-groups preserve 开关 + replace 排除 + 简化重构

解决 Finding #1（opt-out 开关）、#7（default 键死代码）、#9（helper 去重与不可达分支）。

**Files:**
- Modify: `internal/config/schema.go:49-56`（ProxyGroupsExtension）
- Modify: `internal/extension/proxy_groups.go`（全文件重构）
- Test: `internal/extension/extension_test.go`

- [ ] **Step 1: 写失败测试**

`internal/extension/extension_test.go` 末尾追加两个测试：

```go
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
```

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/extension/ -run 'TestApplyProxyGroupsPreserveDefaultDisabled|TestApplyProxyGroupsReplaceKeepsExplicitOrder' -v
```

预期：两个测试 FAIL（现状会把 `HK-01` 移回 index 0；`PreserveDefault` 字段尚不存在会编译错误——编译失败即失败，符合预期）。

- [ ] **Step 3: 实现 schema 字段**

`internal/config/schema.go` 的 `ProxyGroupsExtension` 改为：

```go
// ProxyGroupsExtension defines modifications to proxy-groups list.
type ProxyGroupsExtension struct {
	Prepend []map[string]any    `yaml:"prepend"`
	Append  []map[string]any    `yaml:"append"`
	Replace []ProxyGroupReplace `yaml:"replace"`
	Remove  []string            `yaml:"remove"`
	Inject  []ProxyGroupInject  `yaml:"inject"`
	// PreserveDefault keeps the upstream group's default (first) proxy at index 0
	// after prepend/inject operations. Defaults to true.
	PreserveDefault *bool `yaml:"preserve-default,omitempty"`
}

// PreserveUpstreamDefaults reports whether upstream default-proxy preservation
// is enabled. Defaults to true when PreserveDefault is unset.
func (e ProxyGroupsExtension) PreserveUpstreamDefaults() bool {
	return e.PreserveDefault == nil || *e.PreserveDefault
}
```

- [ ] **Step 4: 重构 proxy_groups.go**

`internal/extension/proxy_groups.go` 全文件替换为：

```go
package extension

import (
	"strings"

	"mihomo-sub-publisher/internal/config"
)

// ApplyProxyGroups applies prepend, append, replace, remove, and inject operations on proxy-groups list.
func ApplyProxyGroups(groups []map[string]any, ext config.ProxyGroupsExtension) []map[string]any {
	result := make([]map[string]any, 0, len(groups)+len(ext.Prepend)+len(ext.Append))

	// 1. Prepend
	for _, g := range ext.Prepend {
		result = append(result, cloneMap(g))
	}

	// 2. Process source groups (Replace and Remove)
	removeSet := make(map[string]struct{}, len(ext.Remove))
	for _, name := range ext.Remove {
		removeSet[name] = struct{}{}
	}

	replaceMap := make(map[string]map[string]any, len(ext.Replace))
	for _, r := range ext.Replace {
		replaceMap[r.Match] = r.Value
	}

	for _, g := range groups {
		name, _ := g["name"].(string)

		// Check remove
		if _, shouldRemove := removeSet[name]; shouldRemove {
			continue
		}

		// Check replace
		if newVal, shouldReplace := replaceMap[name]; shouldReplace {
			result = append(result, cloneMap(newVal))
		} else {
			result = append(result, cloneMap(g))
		}
	}

	// 3. Append
	for _, g := range ext.Append {
		result = append(result, cloneMap(g))
	}

	// 4. Inject proxies into target groups
	if len(ext.Inject) > 0 {
		for _, g := range result {
			applyProxyGroupInject(g, ext.Inject)
		}
	}

	// 5. Optionally keep the upstream default (first) proxy at index 0.
	// Replaced groups are skipped: a full replacement is an explicit new ordering.
	if ext.PreserveUpstreamDefaults() {
		preserveUpstreamDefaultProxies(result, groups, replaceMap)
	}

	return result
}

// proxyListValue normalizes a group's "proxies" value into []any
// ([]string entries are widened). ok is false when the key is missing,
// nil, or of an unsupported type.
func proxyListValue(raw any) (list []any, ok bool) {
	switch l := raw.(type) {
	case []any:
		return l, true
	case []string:
		out := make([]any, len(l))
		for i, s := range l {
			out[i] = s
		}
		return out, true
	}
	return nil, false
}

func applyProxyGroupInject(group map[string]any, injects []config.ProxyGroupInject) {
	name, _ := group["name"].(string)
	rawProxies, exists := group["proxies"]
	if !exists || rawProxies == nil {
		return
	}

	src, ok := proxyListValue(rawProxies)
	if !ok {
		return
	}
	currentList := make([]any, len(src))
	copy(currentList, src)

	seen := make(map[string]struct{}, len(currentList))
	for _, p := range currentList {
		if s, ok := p.(string); ok {
			seen[s] = struct{}{}
		}
	}

	for _, inj := range injects {
		if inj.Target != "*" && inj.Target != name {
			continue
		}

		// 1. Prepend proxies
		var toPrepend []any
		for _, pName := range inj.PrependProxies {
			if _, already := seen[pName]; !already {
				seen[pName] = struct{}{}
				toPrepend = append(toPrepend, pName)
			}
		}
		if len(toPrepend) > 0 {
			currentList = append(toPrepend, currentList...)
		}

		// 2. Append proxies
		for _, pName := range inj.AppendProxies {
			if _, already := seen[pName]; !already {
				seen[pName] = struct{}{}
				currentList = append(currentList, pName)
			}
		}
	}

	group["proxies"] = currentList
}

// preserveUpstreamDefaultProxies moves each upstream group's original first
// proxy back to index 0 after mutations. Groups present in skip (replaced
// groups) are left untouched.
func preserveUpstreamDefaultProxies(result []map[string]any, upstreamGroups []map[string]any, skip map[string]map[string]any) {
	defaults := make(map[string]string, len(upstreamGroups))
	for _, g := range upstreamGroups {
		name, _ := g["name"].(string)
		if name == "" {
			continue
		}
		pList, ok := proxyListValue(g["proxies"])
		if !ok || len(pList) == 0 {
			continue
		}
		if s, ok := pList[0].(string); ok && strings.TrimSpace(s) != "" {
			defaults[name] = strings.TrimSpace(s)
		}
	}
	if len(defaults) == 0 {
		return
	}

	for _, g := range result {
		name, _ := g["name"].(string)
		origDefault, ok := defaults[name]
		if !ok || origDefault == "" {
			continue
		}
		if _, replaced := skip[name]; replaced {
			continue
		}

		pList, ok := proxyListValue(g["proxies"])
		if !ok {
			continue
		}
		for i, p := range pList {
			if s, ok := p.(string); ok && s == origDefault && i > 0 {
				// Move the upstream default proxy back to index 0,
				// shifting the others right in place (relative order kept).
				copy(pList[1:i+1], pList[0:i])
				pList[0] = p
				g["proxies"] = pList
				break
			}
		}
	}
}
```

要点说明（执行者注意）：
- 原 `extractGroupDefaultProxies` 的独立函数与 `g["default"]` 键读写已删除；默认代理提取并入 `preserveUpstreamDefaultProxies`（签名改为直接收 `upstreamGroups` 与 `skip`）。
- `proxyListValue` 是三处共用的归一化 helper；`applyProxyGroupInject` 改用它后，`[]any`/`[]string` 双分支只写一次。
- 移到 index 0 用原地 rotate（`copy` + 赋值），替代原先两份几乎相同的整表重建。
- `cloneMap` 不在本文件——确认它已存在于包内其它文件（`internal/extension/` 下），此重构不改动它。

- [ ] **Step 5: 跑包内全部测试确认通过**

```bash
go test ./internal/extension/ -v
```

预期：全部 PASS，包括既有的 `TestApplyProxyGroupsInjection`（默认 preserve=true 行为不变：`AUTO, node-a, node-b, DIRECT`）与 `TestApplyProxyGroupsPreserveDefaultProxy`。

- [ ] **Step 6: Commit**

```bash
git -C /home/ubuntu/code/github/funny_tools add internal/config/schema.go internal/extension/proxy_groups.go internal/extension/extension_test.go
git -C /home/ubuntu/code/github/funny_tools commit -m "feat(extension): add preserve-default opt-out and skip replaced groups in default-proxy preservation"
```

---

### Task 6: headersEqual → maps.Equal

解决 Finding #8。纯重构，由现有 generator 测试守护（`generator_test.go` 2.1 节覆盖 header 更新路径）。

**Files:**
- Modify: `internal/generator/pipeline.go:80`（调用处）、`:228-238`（删除函数）、import 块

- [ ] **Step 1: 修改调用处**

`internal/generator/pipeline.go` 第 80 行：

```go
		if !headersEqual(currentSnap.Headers, fetchRes.Headers) {
```

改为：

```go
		if !maps.Equal(currentSnap.Headers, fetchRes.Headers) {
```

- [ ] **Step 2: 删除 headersEqual 函数**

删除 `internal/generator/pipeline.go` 末尾（第 228-238 行）：

```go
func headersEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
```

- [ ] **Step 3: 加 import**

import 块加入 `"maps"`（与标准库其它 import 同组）。

- [ ] **Step 4: 跑测试确认通过**

```bash
go test ./internal/generator/ -v
```

预期：全部 PASS（含 header 更新路径测试 `snap3.Headers` 断言）。

- [ ] **Step 5: Commit**

```bash
git -C /home/ubuntu/code/github/funny_tools add internal/generator/pipeline.go
git -C /home/ubuntu/code/github/funny_tools commit -m "refactor(generator): replace hand-rolled headersEqual with maps.Equal"
```

---

### Task 7: 抽取共享 testutil.GetWithUA

解决清理项：`getWithUA` 在 `internal/server/server_test.go:113` 与 `test/e2e_test.go:349` 逐字节重复。

**Files:**
- Create: `internal/testutil/http.go`
- Modify: `internal/server/server_test.go`（删本地 helper，改调用）
- Modify: `test/e2e_test.go`（删本地 helper，改调用）

- [ ] **Step 1: 创建共享包**

`internal/testutil/http.go`：

```go
// Package testutil provides shared helpers for tests across packages.
package testutil

import "net/http"

// GetWithUA performs a GET request with the given User-Agent.
// An empty ua sends no User-Agent header.
func GetWithUA(url, ua string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	return http.DefaultClient.Do(req)
}
```

- [ ] **Step 2: 替换 server_test.go**

删除 `internal/server/server_test.go` 中的本地定义：

```go
func getWithUA(url, ua string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	return http.DefaultClient.Do(req)
}
```

import 加 `"mihomo-sub-publisher/internal/testutil"`，全文 `getWithUA(` 替换为 `testutil.GetWithUA(`。

- [ ] **Step 3: 替换 e2e_test.go**

删除 `test/e2e_test.go` 末尾的同名本地定义，import 加 `"mihomo-sub-publisher/internal/testutil"`，全文 `getWithUA(` 替换为 `testutil.GetWithUA(`。

- [ ] **Step 4: 跑受影响包的测试确认通过**

```bash
go test ./internal/server/ ./test/ -v
```

预期：全部 PASS（行为零变化，纯机械替换）。

- [ ] **Step 5: Commit**

```bash
git -C /home/ubuntu/code/github/funny_tools add internal/testutil/http.go internal/server/server_test.go test/e2e_test.go
git -C /home/ubuntu/code/github/funny_tools commit -m "refactor(test): extract shared GetWithUA helper into internal/testutil"
```

---

### Task 8: 配置示例与 README 文档更新

新配置面（`preserve-default`、`allowed-user-agents`、`required-headers`）与 `/status` 门控语义需要落到用户可见文档。

**Files:**
- Modify: `configs/config.yaml`
- Modify: `README.md`

- [ ] **Step 1: 更新 configs/config.yaml**

`proxy-groups` 段改为：

```yaml
  proxy-groups:
    prepend: []
    append: []
    replace: []
    remove: []
    inject: []
    # preserve-default: true  # 默认 true：保持上游组默认(首个)代理在 index 0；
    #                         # 置 false 让 prepend/inject 的前置代理成为组内第一项
```

`server` 段改为：

```yaml
server:
  listen: "127.0.0.1:8080"
  config-path: "/config/{token}"
  shutdown-timeout: 10s
  # allowed-user-agents: ["*clash*", "*mihomo*", "*stash*"]  # 默认值；通配符 * 匹配任意字符（含 /），
  #                                                          # 配置后按通配符精确匹配（非子串）
  # required-headers: {}  # 例: {"X-Api-Key": "*"} 要求该头存在；值为具体字符串时要求精确(忽略大小写)匹配
```

注意：configs/config.yaml 原本无 `inject: []` 行——本次顺带补上（与 schema 字段对齐）。

- [ ] **Step 2: 更新 README.md**

(a) 第 10-12 行核心特性列表，第 11 行改为：

```markdown
   - 代理组（Proxy Groups）：`prepend`, `append`, `replace`, `remove`, `inject`（向现有组注入代理）；默认保持上游组默认(首个)代理在首位，可用 `preserve-default: false` 关闭。
```

(b) 「🌐 API 端点」表格（第 144-148 行）`/status` 与 `/config/{token}` 行之间补充说明，表格下方追加一段：

```markdown
> **User-Agent 门控**：`/status` 与 `/config/{token}` 仅接受 Clash 系客户端 User-Agent（默认匹配 `clash` / `mihomo` / `stash` 子串，可用 `server.allowed-user-agents` 自定义通配符列表；配合 `server.required-headers` 可要求额外请求头）。不匹配的请求返回 `404`，与错误路径无法区分。`/health` 不做门控，供存活探针使用。
```

- [ ] **Step 3: 验证 config 示例可被加载**

```bash
cd /home/ubuntu/code/github/funny_tools && go run ./cmd/... -config configs/config.yaml 2>&1 | head -5
```

注：若 cmd 入口参数形式不同（先查看 `cmd/` 目录的 flag 定义），以实际为准；或用一次性 Go 测试验证 `config.Load("configs/config.yaml")` 无错。最低要求：`go build ./...` 通过且注释掉的行不影响 YAML 解析（注释行天然不解析，风险为零）。

- [ ] **Step 4: Commit**

```bash
git -C /home/ubuntu/code/github/funny_tools add configs/config.yaml README.md
git -C /home/ubuntu/code/github/funny_tools commit -m "docs: document preserve-default, allowed-user-agents, required-headers and /status gating"
```

---

### Task 9: 全量构建与测试验证

- [ ] **Step 1: 全量构建 + vet + 测试**

```bash
cd /home/ubuntu/code/github/funny_tools && go build ./... && go vet ./... && go test ./...
```

预期：编译零错误、vet 零告警、全部测试 PASS（含 e2e）。

- [ ] **Step 2: 检查未使用的 import / 死代码残留**

```bash
grep -rn "legacyDefaultUserAgent\|headersEqual\|extractGroupDefaultProxies" --include="*.go" .
```

预期：无任何匹配（旧符号全部清除）。

- [ ] **Step 3: 若全部通过，收尾提交（如有遗漏文件）并汇报**

```bash
git -C /home/ubuntu/code/github/funny_tools status
```

预期：working tree clean。如有未提交文件，补提交。

---

## Self-Review 记录

- **覆盖检查**：10 项 finding → Task 1/2（#6、#10）、Task 3（#2）、Task 4（#3、#4）、Task 5（#1、#7、#9）、Task 6（#8）、Task 7（getWithUA 重复）、Task 8（文档，覆盖 #3 的"未文档化"部分）；#5 用户决策保持现状不修。全部有对应任务。
- **占位符检查**：无 TBD/TODO；所有代码步骤含完整代码。Task 1 Step 3 与 Task 5 Step 4 给出完整文件/函数体；Task 4 Step 1(a) 的 setupTestServerWithConfig 因搬运原函数体而以精确指令描述（原文在本仓库 `server_test.go:18-63`，执行者照搬即可）。
- **类型一致性**：`config.DefaultClientUserAgent`（Task 1 定义、Task 2 使用）；`config.DefaultAllowedUserAgents`（Task 1 定义、Task 4 使用）；`ProxyGroupsExtension.PreserveDefault *bool` + `PreserveUpstreamDefaults() bool`（Task 5 内定义并使用）；`testutil.GetWithUA`（Task 7 定义并使用）；`proxyListValue`（Task 5 内定义并使用）。签名一致。
