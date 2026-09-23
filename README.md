# funny_tools

[English](README.md) · [中文](README.zh.md)

Variety of hand-made tools living in branches.

## ollama2pi.py

Synchronize Ollama models into [pi agent](https://pi.dev) custom model
configuration (`~/.pi/agent/models.json`).

The script discovers models from Ollama's `/api/tags`, inspects each one via
`/api/show`, and writes an `ollama` provider entry so the models show up in
pi's `/model` picker. Only models that report the `completion` capability are
written (embedding-only models are skipped).

### Features

- Auto-discovery of Ollama models (`/api/tags`)
- Per-model metadata (`/api/show`): context window, parameter size/count,
  quantization, family/architecture, format
- Capability detection: `completion`, `tools`, `thinking`, `vision`, `embedding`
- Thinking-level mapping per model family (`thinkingLevelMap`), with
  `off -> reasoning_effort: "none"` so thinking can actually be disabled on
  Ollama's OpenAI-compatible endpoint
- Vision models get `input: ["text", "image"]`
- Preserves all other providers in `models.json`; only the `ollama` provider
  entry is replaced
- Leaves the config untouched when Ollama is unreachable or returns no models
- Atomic write (temp file + `os.replace`), automatic backup of the previous
  config, and a digest-based metadata cache so `/api/show` is only called when
  a model actually changed

### Requirements

- Python >= 3.9, no third-party packages

### Usage

```sh
chmod +x ollama2pi.py
./ollama2pi.py
```

### Defaults

| Setting    | Value                      |
|------------|----------------------------|
| Ollama     | `http://127.0.0.1:11434`   |
| Pi config  | `~/.pi/agent/models.json`  |
| Provider   | `ollama`                   |

### Environment variables

| Variable             | Default                  | Description                                        |
|----------------------|--------------------------|----------------------------------------------------|
| `OLLAMA_HOST`        | `http://127.0.0.1:11434` | Ollama API endpoint                                 |
| `PI_CONFIG`          | `~/.pi/agent/models.json`| Pi models.json path                                 |
| `PI_OLLAMA_PROVIDER` | `ollama`                 | Provider name written into models.json              |
| `MAX_TOKENS_MODE`    | `auto`                   | `auto` / `context` / `fixed`                        |
| `MAX_TOKENS`         | `32768`                  | Used when `MAX_TOKENS_MODE=fixed`                   |
| `SHOW_WORKERS`       | `4`                      | Concurrent `/api/show` requests                     |
| `OLLAMA_TIMEOUT`     | `60`                     | HTTP timeout in seconds                             |

`MAX_TOKENS_MODE`:

- `auto` — `contextWindow / 4`, clamped to `[1024, 32768]`
- `context` — the model's full context window
- `fixed` — `MAX_TOKENS`

### Examples

Remote Ollama:

```sh
OLLAMA_HOST=http://192.168.1.100:11434 ./ollama2pi.py
```

Fixed max tokens:

```sh
MAX_TOKENS_MODE=fixed MAX_TOKENS=32768 ./ollama2pi.py
```

Custom provider name:

```sh
PI_OLLAMA_PROVIDER=my-ollama OLLAMA_HOST=http://localhost:9981 ./ollama2pi.py
```

### Provider name warning

Do **not** rename the variable to `PI_PROVIDER`: the pi agent exports
`PI_PROVIDER=<currently selected provider>` into every shell command, so a
script run inside pi would silently pick up the session's provider name and
possibly override a built-in provider of the same name. Always use
`PI_OLLAMA_PROVIDER`.

### pi compatibility

Requires a pi version whose `models.json` schema validates
`thinkingLevelMap` values as `string | null` (verified with pi 0.87.1).
Works against Ollama's OpenAI-compatible endpoint (`/v1`), including the
`developer` role and `reasoning_effort` parameter.

## npm package release notes

npm enforces mandatory OTP (two-factor), and its authentication does not accept
authenticator apps or other software OTP code generators — only a hardware passkey
works. So publishing can only be done with:

```sh
npm publish --registry=https://registry.npmjs.org/ --auth-type=web
```

npm then prints an OTP authentication URL. Open that URL in a browser to complete
the authentication, then return to the terminal to continue.
