可以。这个调整会让架构更简单，也更符合你的部署环境。

核心变化只有一个，但会影响几个设计点：

> **服务端不再托管、缓存或代理 Rule Provider；最终生成的 Mihomo/Clash 配置直接引用 `clash-rules-cn` 等外部 Rule Provider URL，由客户端自行访问 GitHub/CDN 拉取规则。**

因此服务端的职责变成：

```text
订阅源
  ↓
Go 服务
  ↓
Mihomo 解析
  ↓
DSL 扩展
  ↓
生成 rule-providers 引用
  ↓
Mihomo 最终校验
  ↓
发布完整配置
```

而不是：

```text
客户端 → Go 服务 → Rule Provider → GitHub
```

下面是重新整理后的 **技术设计 v1.1**。

---

# Mihomo Subscription Publisher 技术设计 v1.1

## 1. 项目概述

### 1.1 项目目标

基于 Go 实现一个轻量级 Mihomo/Clash 配置生成与发布服务：

1. 定期拉取一个 Mihomo/Clash YAML 订阅。
2. 使用 Mihomo 原生 Go 库解析和验证订阅。
3. 使用声明式 Go DSL 扩展节点、节点组和规则。
4. 支持 DNS、探测配置等常用配置修改。
5. 集成 `clash-rules-cn` 等外部 Rule Provider。
6. 最终配置直接引用外部 Rule Provider URL。
7. 由 Mihomo/Clash 客户端自行访问 GitHub/CDN 获取 Rule Provider。
8. 通过 Token + 过期时间 + 下载次数限制发布配置。
9. 不使用数据库。
10. 配置、状态、缓存使用本地文件和内存。
11. 配置支持热加载。
12. 新配置必须完整通过解析、扩展和最终 Mihomo 校验后才能发布。
13. 始终保留 last-known-good 配置。
14. HTTPS、限流等由前置 Nginx 处理。

---

# 2. 与 v1.0 的核心变化

删除：

```text
Rule Provider Proxy
Rule Provider Cache
Rule Provider Registry Runtime
/rules/*
```

改成：

```text
最终配置
   │
   ├── rule-providers
   │       │
   │       ├── github
   │       └── jsDelivr/CDN
   │
   └── rules
           │
           ▼
        Mihomo Client
           │
           ▼
      外部 Rule Provider
```

服务端只负责：

> **生成 Rule Provider 声明。**

服务端不负责：

> **下载 Rule Provider 内容。**

---

# 3. 总体架构

```text
                         ┌────────────────────┐
                         │ Source Subscription │
                         │    Mihomo YAML      │
                         └─────────┬──────────┘
                                   │
                                   ▼
                         ┌────────────────────┐
                         │ Subscription Fetch │
                         └─────────┬──────────┘
                                   │
                                   ▼
                         ┌────────────────────┐
                         │ Mihomo Raw Parser  │
                         │    + Validation    │
                         └─────────┬──────────┘
                                   │
                                   ▼
                         ┌────────────────────┐
                         │   Extension DSL    │
                         │                    │
                         │ proxies            │
                         │ proxy-groups       │
                         │ rules              │
                         │ dns                │
                         │ probe              │
                         └─────────┬──────────┘
                                   │
                                   ▼
                         ┌────────────────────┐
                         │ Rule Provider       │
                         │ Declaration        │
                         │                    │
                         │ clash-rules-cn      │
                         │ GitHub/CDN URL      │
                         └─────────┬──────────┘
                                   │
                                   ▼
                         ┌────────────────────┐
                         │ Mihomo Final Parse │
                         │     + Validate     │
                         └─────────┬──────────┘
                                   │
                              success
                                   │
                                   ▼
                         ┌────────────────────┐
                         │ Atomic Snapshot    │
                         │ last-known-good    │
                         └─────────┬──────────┘
                                   │
                                   ▼
                              HTTP Server
                                   │
                              Token / Quota
                                   │
                                   ▼
                                Nginx
                                   │
                                   ▼
                                Client
                                   │
                                   ▼
                        GitHub / CDN Rule Provider
```

---

# 4. 系统边界

这是 v1.1 最重要的边界。

## 服务端负责

```text
订阅拉取
配置解析
配置扩展
配置生成
配置验证
配置版本管理
Token
下载次数
配置发布
本地缓存
配置热加载
日志
```

## 服务端不负责

```text
Rule Provider 下载
Rule Provider 缓存
Rule Provider 更新
Rule Provider 内容代理
客户端代理测速
客户端网络连通性
```

---

# 5. 网络依赖

服务端只需要能够访问：

```text
Source Subscription
```

例如：

```text
https://example.com/subscription.yaml
```

不要求服务端能够访问：

```text
github.com
raw.githubusercontent.com
cdn.jsdelivr.net
```

这是本次架构调整的主要目的。

客户端需要能够访问：

```text
GitHub
或
jsDelivr
或其他 Rule Provider mirror
```

---

# 6. 项目目录

调整后的目录：

```text
mihomo-sub-publisher/
├── cmd/
│   └── publisher/
│       └── main.go
│
├── internal/
│   ├── app/
│   │   └── app.go
│   │
│   ├── config/
│   │   ├── loader.go
│   │   ├── schema.go
│   │   ├── validator.go
│   │   └── watcher.go
│   │
│   ├── source/
│   │   ├── fetcher.go
│   │   └── scheduler.go
│   │
│   ├── mihomo/
│   │   ├── parser.go
│   │   ├── validator.go
│   │   └── adapter.go
│   │
│   ├── extension/
│   │   ├── engine.go
│   │   ├── proxies.go
│   │   ├── proxy_groups.go
│   │   ├── rules.go
│   │   ├── dns.go
│   │   └── probe.go
│   │
│   ├── ruleproviders/
│   │   ├── provider.go
│   │   ├── registry.go
│   │   └── clash_rules_cn.go
│   │
│   ├── generator/
│   │   ├── pipeline.go
│   │   ├── snapshot.go
│   │   └── metadata.go
│   │
│   ├── token/
│   │   ├── store.go
│   │   ├── quota.go
│   │   ├── persistence.go
│   │   └── watcher.go
│   │
│   ├── server/
│   │   ├── server.go
│   │   └── handlers.go
│   │
│   ├── storage/
│   │   └── atomic.go
│   │
│   └── logging/
│       └── logging.go
│
├── configs/
│   ├── config.yaml
│   └── tokens.yaml
│
├── data/
│   ├── token-state.json
│   └── token-reset-requests.json
│
├── cache/
│   ├── source.yaml
│   ├── generated.yaml
│   └── generated.yaml.meta.json
│
├── go.mod
├── go.sum
└── README.md
```

相比 v1.0：

```text
cache/rules/
```

删除。

`ruleproviders` 模块也不再负责缓存或代理，只负责：

> Rule Provider 定义和配置生成。

---

# 7. 主配置

建议：

```yaml
version: 1

source:
  url: "https://example.com/subscription.yaml"

  interval: 1h
  timeout: 30s

  headers:
    User-Agent: "mihomo-sub-publisher/1.0"

extensions:

  proxies:
    prepend: []
    append: []
    replace: []
    remove: []

  proxy-groups:
    prepend: []
    append: []
    replace: []
    remove: []

  rules:
    prepend: []
    append: []
    replace: []
    remove: []

  dns:
    override: {}

  probe:
    override: {}

rules:
  providers:
    clash-rules-cn:
      enabled: true
      client-path-prefix: "./ruleset/"

server:
  listen: "127.0.0.1:8080"
  config-path: "/config/{token}"
  shutdown-timeout: 10s

storage:
  cache-dir: "./cache"
  data-dir: "./data"

hot-reload:
  debounce: 1m
```

---

# 8. Rule Provider 配置

因为服务端不再访问 Rule Provider，所以这里不需要：

```yaml
base-url:
mirror-url:
cache:
```

而只描述：

> **最终配置中应该启用哪些 Rule Provider。**

例如：

```yaml
rules:
  providers:

    clash-rules-cn:
      enabled: true
      client-path-prefix: "./ruleset/"
```

`client-path-prefix` 是客户端 Mihomo 的本地路径前缀，默认 `./ruleset/`。

服务端内部有一份静态 Provider 定义。

例如逻辑上：

```text
clash-rules-cn
 ├── direct-domain
 ├── proxy-domain
 ├── reject-domain
 ├── private-domain
 ├── apple-direct
 ├── icloud-domain
 ├── ai-domain
 ├── telegram-ip
 └── china-ip
```

这些 URL 直接写进最终 Mihomo 配置。

---

# 9. 为什么 Provider Registry 仍然保留

虽然服务端不下载 Provider，但仍建议保留：

```text
internal/ruleproviders/registry.go
```

原因是：

> 避免把 GitHub URL 散落在 generator 代码里。

例如：

```go
type ProviderDefinition struct {
    Name     string
    Behavior string
    URL      string
}
```

逻辑上：

```text
Provider Registry
        │
        └── clash-rules-cn
                │
                ├── direct-domain
                ├── proxy-domain
                └── china-ip
```

Generator 只负责读取定义。

---

# 10. Rule Provider 示例

最终生成：

```yaml
rule-providers:

  direct-domain:
    type: http
    behavior: domain
    url: "https://raw.githubusercontent.com/mcxiaochenn/clash-rules-cn/rules/direct-domain.yaml"
    path: "./ruleset/direct-domain.yaml"
    interval: 86400

  proxy-domain:
    type: http
    behavior: domain
    url: "https://raw.githubusercontent.com/mcxiaochenn/clash-rules-cn/rules/proxy-domain.yaml"
    path: "./ruleset/proxy-domain.yaml"
    interval: 86400

  reject-domain:
    type: http
    behavior: domain
    url: "https://raw.githubusercontent.com/mcxiaochenn/clash-rules-cn/rules/reject-domain.yaml"
    path: "./ruleset/reject-domain.yaml"
    interval: 86400
```

这里的：

```yaml
path: "./ruleset/..."
```

是**客户端 Mihomo 的本地路径**。

不是服务端路径。

路径前缀 `./ruleset/` 可以通过 `client-path-prefix` 配置修改。

---

# 11. Rule Provider 的网络模型

客户端：

```text
GET /config/{token}
       │
       ▼
   Publisher
       │
       ▼
    config.yaml
       │
       │
       └──────────────┐
                      │
                      ▼
                  Mihomo
                      │
                      ▼
             Rule Provider URL
                      │
             ┌────────┴────────┐
             ▼                 ▼
           GitHub          jsDelivr
```

因此服务端完全不需要：

```text
GitHub
↓
下载
↓
缓存
↓
代理
```

---

# 12. Rule Provider Mirror

虽然服务端不做代理，但最终配置可以保留一个主 URL。

是否提供 mirror 由配置决定。

例如：

```yaml
rules:
  providers:
    clash-rules-cn:
      enabled: true
      url: "https://raw.githubusercontent.com/..."
```

如果未来 Mihomo 支持或者你的 Provider 定义需要多 URL，可以扩展：

```yaml
urls:
  - "https://raw.githubusercontent.com/..."
  - "https://cdn.jsdelivr.net/..."
```

但 v1 建议：

> 一个 Provider 一个明确 URL。

避免服务端生成逻辑复杂化。

---

# 13. Rule Provider 更新责任

更新责任现在明确分成：

| 内容                | 负责方           |
| ----------------- | ------------- |
| 主配置               | Publisher     |
| 主订阅               | Publisher     |
| DSL               | Publisher     |
| Rule Provider URL | Publisher     |
| Rule Provider 内容  | GitHub/CDN    |
| Rule Provider 下载  | Mihomo Client |
| Rule Provider 缓存  | Mihomo Client |
| Rule Provider 更新  | Mihomo Client |

这样系统职责非常清晰。

---

# 14. Last-known-good

仍然是系统核心原则：

```text
拉取
 ↓
解析
 ↓
DSL
 ↓
Provider Declaration
 ↓
最终 Mihomo 校验
 ↓
成功
 ↓
原子替换
```

任何一步失败：

```text
保留旧版本
```

注意：

> Rule Provider 本身无法在服务端验证内容是否可下载。

因此：

**“最终校验成功”只意味着 Mihomo 接受当前配置结构和 Rule Provider 声明。**

不意味着：

> GitHub 当前一定可访问。

这是架构调整后必须明确接受的行为。

---

# 15. Rule Provider 不作为生成失败条件

例如：

```yaml
rule-providers:
  proxy-domain:
    type: http
    behavior: domain
    url: "https://raw.githubusercontent.com/..."
```

服务端不会：

```text
HTTP GET GitHub
```

因此不能因为 GitHub：

```text
timeout
403
404
```

而让 source update 失败。

服务端只验证：

```text
URL 合法
type 合法
behavior 合法
path 合法
Mihomo schema 合法
```

---

# 16. DSL

保持 v1.0 不变。

```yaml
extensions:

  proxies:
    prepend: []
    append: []
    replace: []
    remove: []

  proxy-groups:
    prepend: []
    append: []
    replace: []
    remove: []

  rules:
    prepend: []
    append: []
    replace: []
    remove: []
```

---

# 17. Proxies

```yaml
extensions:
  proxies:

    prepend:
      - name: "LOCAL"
        type: direct

    append:
      - name: "MY-SOCKS"
        type: socks5
        server: "127.0.0.1"
        port: 1080

    remove:
      - "节点 A"

    replace:
      - match: "节点 B"
        value:
          name: "节点 B"
          type: vmess
          server: "example.com"
          port: 443
          uuid: "..."
```

匹配：

```text
proxy.name
```

---

# 18. Proxy Group

```yaml
extensions:
  proxy-groups:

    prepend:
      - name: "节点选择"
        type: select
        proxies:
          - "自动选择"
          - DIRECT

    append:
      - name: "自动选择"
        type: url-test
        include-all: true
        url: "https://www.gstatic.com/generate_204"
        interval: 300

    remove:
      - "旧节点选择"

    replace:
      - match: "自动选择"
        value:
          name: "自动选择"
          type: url-test
          include-all: true
          url: "https://www.gstatic.com/generate_204"
          interval: 300
```

---

# 19. Proxy Group 优化原则

v1 不做服务端测速。

支持通过 Mihomo 本身的：

```text
select
url-test
fallback
load-balance
include-all
filter
exclude-filter
```

实现代理组优化。

例如：

```yaml
- name: "香港"
  type: url-test
  include-all: true
  filter: "(?i)香港|HK|Hong Kong"
  url: "https://www.gstatic.com/generate_204"
  interval: 300
```

最终测速和节点选择：

> 由客户端 Mihomo 完成。

---

# 20. Rules

```yaml
extensions:
  rules:

    prepend:
      - "DOMAIN-SUFFIX,example.com,DIRECT"

    append:
      - "MATCH,PROXY"

    remove:
      - "DOMAIN-SUFFIX,old.example.com,DIRECT"

    replace:
      - match: "DOMAIN-SUFFIX,foo.com,DIRECT"
        value: "DOMAIN-SUFFIX,foo.com,PROXY"
```

---

# 21. Rule 顺序

严格保持：

```text
prepend
↓
source rules
↓
append
```

`replace` 保留原位置。

最终再稳定去重。

---

# 22. Rule Provider 与 Rules 的关系

例如：

```yaml
rule-providers:

  direct-domain:
    type: http
    behavior: domain
    url: "https://..."

rules:

  - RULE-SET,direct-domain,DIRECT
  - MATCH,PROXY
```

DSL 可以：

```yaml
extensions:
  rules:
    prepend:
      - "RULE-SET,direct-domain,DIRECT"
```

但是这里需要应用层检查：

```text
RULE-SET name
```

是否对应已经声明的 provider。

如果不存在：

> 生成失败。

这样可以避免最终产生明显错误的配置。

---

# 23. DNS

保持 override：

```yaml
extensions:
  dns:
    override:
      enable: true
      ipv6: false
      enhanced-mode: fake-ip
      nameserver:
        - "https://dns.alidns.com/dns-query"
```

只覆盖用户指定字段。

---

# 24. Probe

```yaml
extensions:
  probe:
    override:
      url: "https://www.gstatic.com/generate_204"
      interval: 300
      timeout: 5000
```

具体字段最终映射到 Mihomo proxy-group health-check 配置。

---

# 25. Source

仍然只支持一个 source：

```yaml
source:
  url: "https://example.com/sub.yaml"
  interval: 1h
  timeout: 30s

  headers:
    User-Agent: "mihomo-sub-publisher/1.0"
```

支持：

* HTTP
* HTTPS
* redirect
* gzip
* 自定义 Header
* timeout

---

# 26. Source 更新流程

启动：

```text
Load Config
 ↓
Load Token
 ↓
Load Last Snapshot
 ↓
Start HTTP
 ↓
立即 Fetch
 ↓
Start Scheduler
```

更新：

```text
Fetch
 ↓
SHA256
 ↓
Mihomo Parse
 ↓
DSL
 ↓
Rule Provider Declaration
 ↓
Serialize
 ↓
Mihomo Parse
 ↓
Final Validate
 ↓
Atomic Commit
```

---

# 27. Source Hash 与跳过逻辑

跳过生成的条件是 Generation Fingerprint 没有变化：

```text
old fingerprint == new fingerprint
```

其中 fingerprint 包含：

```text
source_sha256
config_hash
generator_version
mihomo_version
```

因此以下任何变化都会触发重新生成：

```text
source changed
extension changed
rules changed
generator version changed
Mihomo version changed
```

不再单独比较 source SHA256 和 extension config hash，统一使用 fingerprint。

---

# 28. Generation Fingerprint

建议：

```text
fingerprint =
SHA256(
    source_sha256
    + config_hash
    + generator_version
    + mihomo_version
)
```

metadata：

```json
{
  "version": 12,
  "fingerprint": "...",
  "source_sha256": "...",
  "generated_at": "...",
  "source_updated_at": "..."
}
```

---

# 29. Snapshot

内存：

```go
type Snapshot struct {
    Version         uint64
    SourceUpdatedAt time.Time
    GeneratedAt     time.Time
    SHA256          string
    Content         []byte
}
```

使用：

```go
atomic.Pointer[Snapshot]
```

HTTP 请求直接获取当前 immutable snapshot。

---

# 30. HTTP API

v1：

```text
GET /health
GET /status
GET /config/{token}
```

不再有：

```text
GET /rules/*
```

这是本次架构调整后最明确的 API 变化。

---

# 31. Config API

```http
GET /config/{token}
```

成功：

```http
HTTP/1.1 200 OK
Content-Type: application/yaml
X-Source-Updated-At: 2026-08-23T10:00:00Z
X-Generated-At: 2026-08-23T10:01:12Z
X-Config-SHA256: ...
```

Body：

```yaml
mixed-port: 7890
...
```

---

# 32. Metadata

最终配置仍然加入 YAML comment：

```yaml
# source-updated-at: 2026-08-23T10:00:00Z
# generated-at: 2026-08-23T10:01:12Z
# source-sha256: ...
```

不增加 Mihomo 不认识的顶层字段。

同时 HTTP Header 提供相同信息。

---

# 33. Token

```yaml
version: 1

tokens:

  - name: personal
    token: "random-secret"
    expires_at: "2026-12-31T23:59:59+08:00"
    limit: 1000
    reset_remaining: false
```

运行状态：

```json
{
  "version": 1,
  "tokens": {
    "random-secret": {
      "remaining": 876
    }
  }
}
```

---

# 34. Token 行为

请求：

```text
token 不存在
 → 404

token 已过期
 → 403

remaining <= 0
 → 403

没有 last-known-good
 → 503

全部正常
 → 返回配置
 → 成功写 response
 → quota -1
```

错误响应 Body 统一为最小 JSON，不暴露任何细节：

```json
{"error": "not_found"}
{"error": "forbidden"}
{"error": "unavailable"}
```

不包含 message、reason 或其他调试信息。

---

# 35. Token 并发

必须保证：

```text
remaining = 1

A ──┐
B ──┼── concurrent
C ──┘
```

最终：

```text
A → success
B → 403
C → 403
```

不得出现：

```text
remaining = -1
```

或者：

```text
success > limit
```

---

# 36. Token Reset

管理员创建重置请求文件：

```text
data/token-reset-requests.json
```

内容：

```json
{
  "resets": ["personal"]
}
```

服务端定期检查（与 hot reload debounce 同步）：

```text
token-reset-requests.json 存在
 ↓
解析
 ↓
对每个 name：remaining = limit
 ↓
持久化 token-state.json
 ↓
删除 token-reset-requests.json
```

不修改 `tokens.yaml`。

服务端不会写回用户编辑的配置文件。

---

# 37. Token Hot Reload

```text
tokens.yaml
     │
     ▼
File Watcher
     │
     ▼
Debounce (1 分钟)
     │
     ▼
Parse
     │
     ▼
Validate
     │
 ┌───┴────┐
 │        │
失败     成功
 │        │
 ▼        ▼
旧配置    Atomic Swap
```

Debounce 时间由 `hot-reload.debounce` 配置，默认 1 分钟。

同时检查 `data/token-reset-requests.json` 是否存在并处理。

旧 token 修改后立即失效。

---

# 38. 主配置 Hot Reload

`config.yaml` 修改：

```text
source / extensions / rules changed
```

触发：

```text
Reload
 ↓
Debounce (1 分钟)
 ↓
Validate
 ↓
立即重新生成
```

Debounce 时间由 `hot-reload.debounce` 配置，默认 1 分钟。

生成失败：

```text
旧配置继续发布
```

---

# 39. HTTP Server Hot Reload

`server.listen`：

> v1 不支持动态修改。

修改后：

> 重启服务。

因为它属于基础设施级配置，不值得为了 v1 引入复杂 listener migration。

---

# 40. Last-known-good 文件

```text
cache/
├── source.yaml
├── generated.yaml
└── generated.yaml.meta.json
```

`generated.yaml`：

> 当前线上版本。

启动时先验证。

---

# 41. 启动恢复

如果：

```text
上游订阅无法访问
```

但：

```text
cache/generated.yaml
```

存在且通过 Mihomo 校验：

> 服务正常发布旧配置。

如果不存在：

```text
/health → 200
/status → NO_SNAPSHOT
/config/{token} → 503
```

---

# 42. 原子更新

```text
generated.yaml.tmp
        │
        ▼
      write
        │
        ▼
      fsync
        │
        ▼
      rename
        │
        ▼
generated.yaml
```

内存：

```text
new snapshot
     │
     ▼
atomic.Store()
```

文件和内存都采用原子切换。

---

# 43. 状态文件

```text
data/
├── token-state.json
└── token-reset-requests.json
```

quota 修改后**批量持久化**，每 1 分钟写入一次。

不在每次请求后同步写入。

如果服务 crash 发生在两次写入之间：

> 最多丢失 1 分钟内的 quota 扣减。

这意味着 crash 后 remaining 可能比实际偏高（多出最多 1 分钟内的下载次数）。

此行为已明确接受。

关闭时（SIGTERM）立即 flush 最终状态。

---

# 44. 文件权限

建议：

```text
configs/   0700
data/      0700
cache/     0700
```

文件：

```text
0600
```

尤其：

```text
tokens.yaml
token-state.json
token-reset-requests.json
source.yaml
generated.yaml
```

都应该按照敏感文件处理。

---

# 45. 日志

核心事件：

```text
source.fetch.start
source.fetch.success
source.fetch.failure

config.parse.success
config.parse.failure

extension.apply.success
extension.apply.failure

generation.success
generation.failure

snapshot.swap

token.reload
token.reload.failure
token.reset

token.download.success
token.download.denied
```

禁止日志输出：

```text
token
Authorization
Cookie
proxy credentials
完整敏感 URL
完整订阅内容
```

---

# 46. `/health`

只表示：

> Go 进程和 HTTP Server 是否正常。

因此上游订阅失败：

```http
GET /health
→ 200
```

不会导致 Nginx/监控系统误判服务死亡。

---

# 47. `/status`

用于查看：

```json
{
  "status": "ready",
  "snapshot_version": 12,
  "source_updated_at": "2026-08-23T10:00:00Z",
  "generated_at": "2026-08-23T10:01:12Z",
  "last_update_success": true
}
```

绝不返回：

```text
token
quota
source credentials
proxy credentials
```

建议仅允许本机/Nginx 内部访问。

---

# 48. Nginx

最终部署：

```text
                Internet
                   │
                   ▼
                Nginx
             ┌─────┴─────┐
             │           │
          HTTPS       Rate Limit
             │           │
             └─────┬─────┘
                   │
                   ▼
          127.0.0.1:8080
                   │
                   ▼
         Mihomo Subscription
              Publisher
```

Go 服务：

> 不直接暴露公网。

---

# 49. 客户端网络要求

使用本服务生成的配置后，客户端需要具备：

```text
访问 Publisher
+
访问 Rule Provider
```

即：

```text
Client
 │
 ├── Publisher
 │
 └── GitHub/CDN
```

如果某个客户端无法访问 GitHub：

> Publisher 无法解决这个问题。

这属于客户端网络环境问题。

如果以后确实需要，可以增加独立 mirror，但不属于当前 v1。

---

# 50. Rule Provider 网络故障的影响

这是调整后需要明确接受的 trade-off。

例如：

```text
Publisher 正常
GitHub 故障
```

那么：

```text
GET /config/{token}
→ 正常
```

客户端得到配置。

但是：

```text
Mihomo
 ↓
RULE-SET
 ↓
GitHub
 ↓
失败
```

那么客户端可能无法更新对应 Rule Provider。

这不会影响 Publisher 的 last-known-good。

---

# 51. 是否因此导致配置更新失败？

不会。

例如：

```text
Source Subscription
      ↓
成功
      ↓
Mihomo Parse
      ↓
成功
      ↓
DSL
      ↓
成功
      ↓
Rule Provider declaration
      ↓
成功
      ↓
Mihomo Validate
      ↓
成功
      ↓
Publish
```

Publisher 不尝试访问：

```text
raw.githubusercontent.com
```

所以：

> Rule Provider 网络状态不属于 Publisher 的生成成功条件。

---

# 52. 测试

重点测试：

### Source

```text
正常
timeout
HTTP 500
invalid YAML
invalid Mihomo config
redirect
gzip
```

### DSL

```text
prepend
append
remove
replace
duplicate
unknown proxy
unknown group
unknown Rule Provider
```

### Token

```text
不存在
过期
quota 0
quota 1
并发
reload
reset
```

### Snapshot

```text
首次成功
首次失败
旧版本恢复
新版本失败
原子替换
```

### Rule Provider

主要测试：

```text
provider definition 正确生成
RULE-SET 引用正确
provider URL 正确
Mihomo 可以接受最终配置
```

**不测试服务端下载 GitHub Rule Provider**，因为服务端已经没有这个职责。

---

# 53. Failure Injection

重点：

```text
source fetch failure
Mihomo parse failure
DSL failure
serialization failure
final validation failure
disk write failure
token state write failure
config reload failure
```

每一种情况下：

> last-known-good 必须继续存在。

---

# 54. Mihomo 版本

固定版本：

```go
github.com/metacubex/mihomo vX.Y.Z
```

不要：

```text
@main
```

同时记录：

```text
mihomo version
generator version
```

在生成 fingerprint 中加入版本。

---

# 55. Generator Fingerprint

```text
SHA256(
    source_sha256
    +
    config_hash
    +
    generator_version
    +
    mihomo_version
)
```

这样升级程序后，即使源订阅没变化，也可以触发重新生成。

---

# 56. 生命周期

启动：

```text
Load config
 ↓
Validate config
 ↓
Load token state
 ↓
Load generated snapshot
 ↓
Validate snapshot
 ↓
Start HTTP
 ↓
Start watchers
 ↓
立即 source update
 ↓
Start scheduler
```

关闭：

```text
SIGTERM
 ↓
Stop scheduler
 ↓
Stop watchers
 ↓
Shutdown HTTP (等待 shutdown-timeout，默认 10s)
 ↓
Flush token state
 ↓
Exit
```

`shutdown-timeout` 由 `server.shutdown-timeout` 配置。

超时后强制关闭未完成的连接。

---

# 57. 模块职责

| Module      | 职责                     |
| ----------- | ---------------------- |
| `config`    | 配置读取、校验、热加载            |
| `source`    | 订阅下载、调度                |
| `mihomo`    | Mihomo adapter、解析、最终验证 |
| `extension` | DSL                    |
| `ruleproviders`     | Rule Provider 声明       |
| `generator` | 配置生成 Pipeline          |
| `token`     | token、quota、状态         |
| `server`    | HTTP API               |
| `storage`   | 原子文件                   |
| `app`       | 生命周期协调                 |

---

# 58. v1 明确不做

仍然排除：

```text
多订阅合并
JavaScript Script
服务端节点测速
数据库
Web 管理后台
在线编辑配置
用户系统
OAuth
多租户
服务端 Rule Provider Proxy
服务端 Rule Provider Cache
服务端 GitHub 访问
服务端规则内容更新
Prometheus
AI 自动分组
```

尤其是：

> **服务端不需要具备访问 GitHub 的能力。**

---

# 59. 最终配置示例

最终 Publisher 返回给客户端的配置可能类似：

```yaml
# source-updated-at: 2026-08-23T10:00:00Z
# generated-at: 2026-08-23T10:01:12Z

mixed-port: 7890

proxies:
  - name: "节点 A"
    type: vmess
    ...

proxy-groups:
  - name: "自动选择"
    type: url-test
    include-all: true
    url: "https://www.gstatic.com/generate_204"
    interval: 300

rule-providers:

  direct-domain:
    type: http
    behavior: domain
    url: "https://raw.githubusercontent.com/mcxiaochenn/clash-rules-cn/rules/direct-domain.yaml"
    path: "./ruleset/direct-domain.yaml"
    interval: 86400

  proxy-domain:
    type: http
    behavior: domain
    url: "https://raw.githubusercontent.com/mcxiaochenn/clash-rules-cn/rules/proxy-domain.yaml"
    path: "./ruleset/proxy-domain.yaml"
    interval: 86400

rules:
  - RULE-SET,direct-domain,DIRECT
  - RULE-SET,proxy-domain,PROXY
  - MATCH,PROXY
```

整个过程：

```text
Publisher
  ├── 生成 proxies
  ├── 生成 proxy-groups
  ├── 生成 rule-providers
  └── 生成 rules
          │
          ▼
       返回 YAML
          │
          ▼
       Mihomo Client
          │
          ├── Publisher
          │
          └── GitHub/CDN
```

---

# 60. 最终架构定稿

因此 v1.1 的核心定义可以正式确定为：

> **这是一个 Go 实现的 Mihomo Subscription Publisher。服务端从单一 Mihomo YAML 订阅源定期拉取配置，使用 Mihomo 原生库完成解析和最终验证，通过声明式 Go DSL 对 proxies、proxy-groups、rules、DNS 和探测配置进行扩展，并生成对外部 Rule Provider（如 `clash-rules-cn`）的引用。服务端不访问、缓存或代理 Rule Provider，客户端 Mihomo/Clash 自行从 GitHub/CDN 拉取规则。生成过程采用 last-known-good 和原子替换机制，通过 Token、过期时间和下载次数控制配置发布。系统不使用数据库，采用本地文件和内存快照，配置支持热加载，Nginx 负责 HTTPS 和限流。**

### 最终数据流

```text
                    ┌──────────────────┐
                    │  Mihomo Source   │
                    └────────┬─────────┘
                             │
                             ▼
                    ┌──────────────────┐
                    │     Fetcher      │
                    └────────┬─────────┘
                             │
                             ▼
                    ┌──────────────────┐
                    │ Mihomo Parse     │
                    └────────┬─────────┘
                             │
                             ▼
                    ┌──────────────────┐
                    │ Extension DSL    │
                    │                  │
                    │ proxies          │
                    │ proxy-groups     │
                    │ rules            │
                    │ dns              │
                    │ probe            │
                    └────────┬─────────┘
                             │
                             ▼
                    ┌──────────────────┐
                    │ Provider         │
                    │ Declaration      │
                    │                  │
                    │ GitHub/CDN URL   │
                    └────────┬─────────┘
                             │
                             ▼
                    ┌──────────────────┐
                    │ Mihomo Validate  │
                    └────────┬─────────┘
                             │
                         SUCCESS
                             │
                             ▼
                    ┌──────────────────┐
                    │ Last-Known-Good  │
                    │ Atomic Snapshot  │
                    └────────┬─────────┘
                             │
                             ▼
                    ┌──────────────────┐
                    │ GET /config/:id  │
                    │ Token + Quota    │
                    └────────┬─────────┘
                             │
                             ▼
                           Nginx
                             │
                             ▼
                          Client
                         /      \
                        /        \
                       ▼          ▼
                 Publisher     GitHub/CDN
                              Rule Provider
```

