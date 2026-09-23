# pi-ollama-sync

[English](README.md) · [中文](README.zh.md)

A [pi agent](https://pi.dev) extension that synchronizes [Ollama](https://ollama.com)
models into pi's custom model configuration (`~/.pi/agent/models.json`) so they
show up in pi's `/model` picker.

TypeScript port of [`ollama2pi.py`](../ollama2pi.py) — no Python, no third-party
runtime dependencies.

## Install

```sh
pi install npm:pi-ollama-sync
```

Or try it without installing:

```sh
pi -e npm:pi-ollama-sync
```

## Usage

### Command

```
/ollama-sync                 # sync now
/ollama-sync check           # dry run: report, don't write
/ollama-sync provider=my-ollama host=http://192.168.1.100:11434
```

### Tool

The extension registers an `ollama_sync` tool, so you can simply ask the agent:

> Sync my Ollama models into pi / 先检查一下有哪些 Ollama 模型(dry run)

Tool parameters: `dry_run` (boolean), `host` (string), `provider` (string).

## What it does

- Discovers models via `/api/tags`, inspects each via `/api/show`
- Writes an `ollama` provider entry (`baseUrl: <host>/v1`, `api: openai-completions`)
- Only models with the `completion` capability are written (embedding-only models are skipped)
- Capability mapping: `reasoning` (thinking), `input: ["text", "image"]` (vision)
- Per-family `thinkingLevelMap` with `off -> reasoning_effort: "none"` so thinking
  can actually be disabled on Ollama's OpenAI-compatible endpoint
- `maxTokens` policy via `MAX_TOKENS_MODE` (see below)

## Safety

- Preserves every other provider in `models.json`; only the configured provider
  entry is replaced
- Leaves the config untouched when Ollama is unreachable or returns no models
- Pre-write validation rejects entries pi would refuse (e.g. non-`string|null`
  `thinkingLevelMap` values) before touching disk
- Automatic backup of the previous config (`~/.pi/agent/backups/models.json.<timestamp>`)
- Atomic write (temp file + rename, fsync)
- Digest-based metadata cache (`ollama-model-cache.json`): `/api/show` is only
  called when a model actually changed

## Configuration (environment variables)

| Variable              | Default                  | Meaning                                   |
|-----------------------|--------------------------|-------------------------------------------|
| `OLLAMA_HOST`         | `http://127.0.0.1:11434` | Ollama API base URL (trailing `/v1` stripped) |
| `PI_CONFIG`           | `~/.pi/agent/models.json`| Target models.json path                   |
| `PI_OLLAMA_PROVIDER`  | `ollama`                 | Provider entry name to write              |
| `MAX_TOKENS_MODE`     | `auto`                   | `auto` / `context` / `fixed`              |
| `MAX_TOKENS`          | `32768`                  | Used when `MAX_TOKENS_MODE=fixed`         |
| `SHOW_WORKERS`        | `4`                      | Concurrent `/api/show` requests           |
| `OLLAMA_TIMEOUT`      | `60`                     | HTTP timeout in seconds                   |

`MAX_TOKENS_MODE`:

- `auto` — `contextWindow / 4`, clamped to `[1024, 32768]`
- `context` — the model's full context window
- `fixed` — exactly `MAX_TOKENS`

> **Never** rename the provider env var to `PI_PROVIDER`: pi exports
> `PI_PROVIDER=<selected provider>` into every shell command, so running a sync
> inside a pi session would silently overwrite whichever provider is currently
> selected. This extension only reads `PI_OLLAMA_PROVIDER` (and the tool's
> explicit `provider` parameter).

## Compatibility

- Requires pi's `models.json` validation to accept `thinkingLevelMap` values of
  `string | null` (verified on pi 0.87.1)
- Node.js >= 18 (global `fetch`)
- Works with Ollama's OpenAI-compatible endpoint (`/v1`), including the
  `developer` role and the `reasoning_effort` parameter

## License

MIT
