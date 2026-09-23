# pi-ollama-sync

[English](README.md) · [中文](README.zh.md)

一个 [pi agent](https://pi.dev) 扩展：把 [Ollama](https://ollama.com) 上的模型同步到 pi 的
自定义模型配置（`~/.pi/agent/models.json`），让它们出现在 pi 的 `/model` 选择列表中。

由 [`ollama2pi.py`](../ollama2pi.py) 移植的 TypeScript 版本——不需要 Python，
没有任何第三方运行时依赖。

## 安装

```sh
pi install npm:pi-ollama-sync
```

或者不安装先试用：

```sh
pi -e npm:pi-ollama-sync
```

## 使用

### 命令

```
/ollama-sync                 # 立即同步
/ollama-sync check           # 试运行：只报告，不写入
/ollama-sync provider=my-ollama host=http://192.168.1.100:11434
```

### 工具

扩展注册了 `ollama_sync` 工具，可以直接对 agent 说：

> 把我的 Ollama 模型同步到 pi / 先 dry run 看看有哪些模型

工具参数：`dry_run`（布尔）、`host`（字符串）、`provider`（字符串）。

## 做了什么

- 通过 `/api/tags` 发现模型，逐个用 `/api/show` 获取元数据
- 写入名为 `ollama` 的 provider 条目（`baseUrl: <host>/v1`、`api: openai-completions`）
- 只写入带 `completion` 能力的模型（纯 embedding 模型跳过）
- 能力映射：`reasoning`（思考）、`input: ["text", "image"]`（视觉）
- 按模型族生成 `thinkingLevelMap`，其中 `off -> reasoning_effort: "none"`，
  从而能在 Ollama 的 OpenAI 兼容接口上真正关闭思考
- `maxTokens` 策略由 `MAX_TOKENS_MODE` 控制（见下表）

## 安全性

- 保留 `models.json` 中其他 provider，只替换配置的这一项
- 当 Ollama 无法访问或没有返回模型时，不修改现有配置
- 写入前校验，拒绝 pi 会拒绝的条目（例如 `thinkingLevelMap` 出现
  非 `string|null` 的值），失败在写盘之前
- 自动备份旧配置（`~/.pi/agent/backups/models.json.<时间戳>`）
- 原子写入（临时文件 + rename，fsync）
- 基于 digest 的元数据缓存（`ollama-model-cache.json`）：只有模型变化时
  才重新调用 `/api/show`

## 配置（环境变量）

| 变量                  | 默认值                   | 说明                                      |
|-----------------------|--------------------------|-------------------------------------------|
| `OLLAMA_HOST`         | `http://127.0.0.1:11434` | Ollama API 地址（自动去掉末尾 `/v1`）     |
| `PI_CONFIG`           | `~/.pi/agent/models.json`| 目标 models.json 路径                     |
| `PI_OLLAMA_PROVIDER`  | `ollama`                 | 写入的 provider 名称                      |
| `MAX_TOKENS_MODE`     | `auto`                   | `auto` / `context` / `fixed`              |
| `MAX_TOKENS`          | `32768`                  | 仅在 `MAX_TOKENS_MODE=fixed` 时生效       |
| `SHOW_WORKERS`        | `4`                      | 并发 `/api/show` 请求数                   |
| `OLLAMA_TIMEOUT`      | `60`                     | HTTP 超时（秒）                           |

`MAX_TOKENS_MODE` 取值：

- `auto` —— `contextWindow / 4`，限制在 `[1024, 32768]` 区间内
- `context` —— 使用模型完整上下文窗口
- `fixed` —— 固定为 `MAX_TOKENS`

> **不要**把 provider 环境变量改回 `PI_PROVIDER`：pi agent 会在每条 shell
> 命令中导出 `PI_PROVIDER=<当前选中的 provider>`。在 pi 会话里运行同步就会
> 静默覆盖当前 provider。本扩展只读取 `PI_OLLAMA_PROVIDER`（以及工具调用里
> 显式传入的 `provider` 参数）。

## 兼容性

- 要求 pi 的 `models.json` 校验支持 `thinkingLevelMap` 值为 `string | null`
  （已在 pi 0.87.1 上验证通过）
- Node.js >= 18（内置 `fetch`）
- 兼容 Ollama 的 OpenAI 接口（`/v1`），包括 `developer` 角色与
  `reasoning_effort` 参数

## 许可

MIT
