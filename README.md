# funny_tools

手工制作的多种工具，分散在不同分支中维护。

## ollama2pi.py

将 Ollama 上的模型同步到 [pi agent](https://pi.dev) 的自定义模型配置
（`~/.pi/agent/models.json`），让这些模型可以出现在 pi 的 `/model` 选择列表中。

脚本通过 Ollama 的 `/api/tags` 发现模型，再用 `/api/show` 逐个获取元数据，
最终写入一个名为 `ollama` 的 provider 条目。只写入带有 `completion` 能力的模型
（纯 embedding 模型会被跳过）。

### 特性

- 通过 `/api/tags` 自动发现 Ollama 模型
- 通过 `/api/show` 获取详细信息：上下文窗口、参数量/参数字符、量化方式、
  family/架构、文件格式
- 能力识别：`completion`（补全）、`tools`（工具）、`thinking`（思考）、
  `vision`（视觉）、`embedding`（嵌入）
- 为每个模型族生成思考级别映射（`thinkingLevelMap`），其中
   `off -> reasoning_effort: "none"`，从而能在 Ollama 的 OpenAI 兼容接口上真正关闭思考
- 视觉模型自动设置 `input: ["text", "image"]`
- 保留 `models.json` 中其他 provider，只替换 `ollama` 这一项
- 当 Ollama 无法访问或没有返回模型时，不修改现有配置
- 原子写入（临时文件 + `os.replace`），自动备份旧配置，并基于 digest 的
  元数据缓存，仅在模型发生变化时才重新调用 `/api/show`

### 环境要求

- Python >= 3.9，无需第三方依赖

### 使用方法

```sh
chmod +x ollama2pi.py
./ollama2pi.py
```

### 默认值

| 项目      | 默认值                       |
|-----------|-----------------------------|
| Ollama    | `http://127.0.0.1:11434`    |
| Pi 配置   | `~/.pi/agent/models.json`   |
| Provider  | `ollama`                    |

### 环境变量

| 变量                | 默认值                    | 说明                                        |
|--------------------|--------------------------|---------------------------------------------|
| `OLLAMA_HOST`      | `http://127.0.0.1:11434` | Ollama API 地址                              |
| `PI_CONFIG`        | `~/.pi/agent/models.json`| Pi 的 models.json 路径                       |
| `PI_OLLAMA_PROVIDER`| `ollama`                | 写入 models.json 时的 provider 名称          |
| `MAX_TOKENS_MODE`  | `auto`                   | `auto` / `context` / `fixed`                |
| `MAX_TOKENS`       | `32768`                  | 仅在 `MAX_TOKENS_MODE=fixed` 时生效          |
| `SHOW_WORKERS`     | `4`                      | 并发 `/api/show` 请求数                      |
| `OLLAMA_TIMEOUT`   | `60`                     | HTTP 超时（秒）                              |

`MAX_TOKENS_MODE` 取值：

- `auto` —— `contextWindow / 4`，并限制在 `[1024, 32768]` 区间内
- `context` —— 使用模型的完整上下文窗口
- `fixed` —— 固定为 `MAX_TOKENS`

### 示例

远程 Ollama：

```sh
OLLAMA_HOST=http://192.168.1.100:11434 ./ollama2pi.py
```

固定 max tokens：

```sh
MAX_TOKENS_MODE=fixed MAX_TOKENS=32768 ./ollama2pi.py
```

自定义 provider 名称：

```sh
PI_OLLAMA_PROVIDER=my-ollama OLLAMA_HOST=http://localhost:9981 ./ollama2pi.py
```

### Provider 名称警告

**不要**把变量名改回 `PI_PROVIDER`：pi agent 会在每条 shell 命令中导出
`PI_PROVIDER=<当前选中的 provider>`。如果在 pi 会话中运行本脚本，就会静默地
把 provider 写成会话当前的名称，甚至可能覆盖掉同名的内置 provider。
请务必使用 `PI_OLLAMA_PROVIDER`。

### pi 兼容性

要求 pi 版本的 `models.json` 校验支持 `thinkingLevelMap` 值为 `string | null`
（已在 pi 0.87.1 上验证通过）。兼容 Ollama 的 OpenAI 接口（`/v1`），
包括 `developer` 角色与 `reasoning_effort` 参数。
