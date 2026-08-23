# Mihomo Subscription Publisher

基于 Go 实现的轻量级 Mihomo/Clash 配置生成与发布服务。

## 🌟 核心特性

1. **上游订阅拉取与调度**：支持 HTTP/HTTPS、Header 自定义、Gzip 解压以及指数退避重试（1s, 2s, 4s）。
2. **原生 Mihomo 解析与校验**：基于 `github.com/metacubex/mihomo` 原生库，双重校验确保生成的配置合法有效。
3. **声明式 Go DSL 扩展**：
   - 节点列表（Proxies）：`prepend`, `append`, `replace` (按 name 精确匹配), `remove`。
   - 代理组（Proxy Groups）：`prepend`, `append`, `replace`, `remove`。
   - 路由规则（Rules）：`prepend`, `append`, `replace`, `remove`，保持 `prepend -> source -> append` 顺序并进行稳定精确去重。
   - DNS 覆盖：浅合并（shallow merge）覆盖顶层配置。
   - 探针健康检查（Probe）：覆盖 `url-test`, `fallback`, `load-balance` 代理组的 `url`, `interval`, `timeout`。
4. **外部 Rule Provider 声明**：
   - 服务端仅生成对外部 Rule Provider（如 `clash-rules-cn`）的声明与引用（URL 指向 GitHub/CDN）。
   - 服务端不托管、缓存或下载规则，由客户端 Mihomo 自行拉取。
5. **发布与配额管理**：
   - 基于 Token、过期时间 (`expires_at`) 以及下载次数 (`limit`) 鉴权。
   - 写后扣减（write-after deduction），支持高并发无锁操作。
6. **无数据库设计与热加载**：
   - 无数据库依赖，状态与缓存保存在本地文件（`0600`/`0700` 严格权限）。
   - Token 热加载采用**触发文件模型**（`data/token-reset-requests.json`），带防抖（Debounce）与幂等全量重置。
7. **Last-Known-Good 容灾保障**：
   - 原子快照替换（内存与磁盘同步切换），在上游源故障时始终对外提供最后已知可用配置。
8. **安全日志脱敏**：
   - 自动对 URL 中的 Token、Authorization、Cookie 及敏感凭据进行 Mask 脱敏。

---

## 📁 目录结构

```text
mihomo-sub-publisher/
├── cmd/
│   └── publisher/
│       └── main.go              # CLI 入口
├── internal/
│   ├── app/                     # 生命周期协调
│   ├── config/                  # 主配置 schema 与 loader/validator
│   ├── source/                  # 订阅拉取与调度器
│   ├── mihomo/                  # Mihomo adapter 与校验器
│   ├── extension/               # 声明式 DSL 引擎
│   ├── ruleproviders/           # Rule Provider registry 与 clash-rules-cn 定义
│   ├── generator/               # 串行化 Pipeline、快照与 Fingerprint
│   ├── token/                   # Token store、写后扣减与 Reset Watcher
│   ├── server/                  # HTTP Server (/health, /status, /config/{token})
│   ├── storage/                 # 原子文件读写与权限控制 (0600/0700)
│   └── logging/                 # 结构化脱敏日志 (log/slog)
├── configs/
│   ├── config.yaml              # 主配置
│   └── tokens.yaml              # Token 列表
├── test/
│   └── e2e_test.go              # 端到端集成测试
├── data/                        # 运行状态 (token-state.json, token-reset-requests.json)
└── cache/                       # 缓存与快照 (source.yaml, generated.yaml, generated.yaml.meta.json)
```

---

## 🚀 编译与运行

### 1. 编译二进制

```bash
go build -o bin/publisher ./cmd/publisher
```

### 2. 配置与启动

准备配置文件：
```bash
# 复制或编辑配置文件
mkdir -p configs data cache
cp configs/config.yaml configs/config.yaml
cp configs/tokens.yaml configs/tokens.yaml

# 启动服务
./bin/publisher -config configs/config.yaml -tokens configs/tokens.yaml
```

命令行参数：
- `-config`: 主配置文件路径（默认 `configs/config.yaml`）
- `-tokens`: Token 配置文件路径（默认 `configs/tokens.yaml`）
- `-json-log`: 是否输出 JSON 格式日志（默认 `true`）
- `-version`: 显示程序版本信息

### 3. Docker 与 Docker Compose 部署

#### 方式一：Docker 单容器运行
```bash
# 构建镜像
docker build -t mihomo-sub-publisher:latest .

# 运行容器
docker run -d \
  --name mihomo-publisher \
  -p 8080:8080 \
  -v $(pwd)/configs:/app/configs:ro \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/cache:/app/cache \
  mihomo-sub-publisher:latest
```

#### 方式二：Docker Compose + Nginx 反向代理
1. 配置 `configs/config.yaml` 与 `configs/tokens.yaml`。
2. 将 SSL 证书放入 `deploy/nginx/ssl/`（`fullchain.pem` 和 `privkey.pem`）。
3. 修改 `deploy/nginx/nginx.conf` 中的域名为您的实际域名。
4. 一键启动：
```bash
docker compose up -d
```

Nginx 示例配置包含：
- **Token 日志脱敏**：自动将访问日志中的 `/config/<token>` 转换为 `/config/***`，避免 Token 泄露。
- **速率限制**：配置 `limit_req` 防止高频滥刷。
- **安全隔离**：默认禁止外部公网直接访问 `/status` 管理端点。

---

## 🔄 Token 热加载与重置

当修改 `configs/tokens.yaml` 后，通过创建触发文件触发热重载与配额全量重置：

```bash
# 1. 修改 tokens.yaml
vi configs/tokens.yaml

# 2. 写入触发文件（支持防抖处理，默认 1m）
echo '{"triggered_at":"'$(date -Iseconds)'"}' > data/token-reset-requests.json
```

服务检测到触发文件后，将自动：
1. 重读并校验 `tokens.yaml`
2. 全量重置所有 token 的剩余配额至 limit
3. 剪除已删除的 token
4. 立即持久化至 `data/token-state.json`
5. 自动删除触发文件

---

## 🌐 API 端点

| 路径 | 方法 | 说明 | 示例响应 |
|---|---|---|---|
| `/health` | `GET` | 进程存活检查 | `{"status":"ok"}` (200 OK) |
| `/status` | `GET` | 状态与快照元数据（不含敏感信息） | `{"status":"ready","snapshot_version":1,...}` |
| `/config/{token}` | `GET` | 获取 Mihomo 完整订阅配置 | `Content-Type: application/yaml` |

错误响应统一为 JSON 格式：
```json
{"error": "not_found"}
{"error": "forbidden"}
{"error": "unavailable"}
```

---

## 🧪 运行测试

```bash
# 运行全部单元测试与集成测试
go test -v ./...
```
