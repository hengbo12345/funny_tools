#!/usr/bin/env python3

"""
ollama2pi.py

Synchronize Ollama models to Pi Agent's models.json.

Requirements:
    Python >= 3.9

No third-party Python packages are required.

Default:
    Ollama:    http://127.0.0.1:11434
    Pi config: ~/.pi/agent/models.json

Environment variables:

    OLLAMA_HOST
        Ollama API endpoint.

    PI_CONFIG
        Pi Agent models.json.

    PI_OLLAMA_PROVIDER
        Provider name in models.json. Default: ollama

        Note: do NOT use PI_PROVIDER here. The pi agent exports
        PI_PROVIDER=<selected provider> into every shell command,
        which would silently rename the provider (and possibly
        override a built-in provider of the same name).

    MAX_TOKENS_MODE
        auto     -> default
        context  -> contextWindow
        fixed    -> MAX_TOKENS

    MAX_TOKENS
        Used when MAX_TOKENS_MODE=fixed.

    SHOW_WORKERS
        Number of concurrent /api/show requests.

Examples:

    python3 ollama2pi.py

    OLLAMA_HOST=http://192.168.1.100:11434 \
        python3 ollama2pi.py

    MAX_TOKENS_MODE=fixed \
    MAX_TOKENS=32768 \
        python3 ollama2pi.py
"""

from __future__ import annotations

import concurrent.futures
import hashlib
import json
import logging
import os
import shutil
import sys
import tempfile
import urllib.error
import urllib.request
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Any


# ============================================================================
# Configuration
# ============================================================================

OLLAMA_HOST = os.environ.get(
    "OLLAMA_HOST",
    "http://127.0.0.1:11434",
).rstrip("/")

PI_CONFIG = Path(
    os.environ.get(
        "PI_CONFIG",
        "~/.pi/agent/models.json",
    )
).expanduser()

PI_PROVIDER = os.environ.get(
    "PI_OLLAMA_PROVIDER",
    "ollama",
)

CACHE_FILE = PI_CONFIG.parent / "ollama-model-cache.json"

BACKUP_DIR = PI_CONFIG.parent / "backups"

MAX_TOKENS_MODE = os.environ.get(
    "MAX_TOKENS_MODE",
    "auto",
)

MAX_TOKENS = int(
    os.environ.get(
        "MAX_TOKENS",
        "32768",
    )
)

SHOW_WORKERS = int(
    os.environ.get(
        "SHOW_WORKERS",
        "4",
    )
)

HTTP_TIMEOUT = int(
    os.environ.get(
        "OLLAMA_TIMEOUT",
        "60",
    )
)


# ============================================================================
# Logging
# ============================================================================

logging.basicConfig(
    level=logging.INFO,
    format="[%(asctime)s] %(levelname)s %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S",
)

log = logging.getLogger("ollama2pi")


# ============================================================================
# Data model
# ============================================================================


@dataclass
class OllamaModel:
    name: str
    digest: str
    size: int
    modified_at: str


@dataclass
class ModelMetadata:
    name: str

    family: str | None = None
    format: str | None = None

    parameter_size: str | None = None
    parameter_count: int | None = None

    quantization: str | None = None

    context_window: int | None = None

    completion: bool = False
    tools: bool = False
    thinking: bool = False
    vision: bool = False
    embedding: bool = False

    model_info: dict[str, Any] | None = None

    raw: dict[str, Any] | None = None


# ============================================================================
# HTTP
# ============================================================================


def http_json(
    method: str,
    url: str,
    payload: dict[str, Any] | None = None,
) -> dict[str, Any]:
    """
    Perform HTTP request and decode JSON.
    """

    body = None

    headers = {
        "Accept": "application/json",
    }

    if payload is not None:
        body = json.dumps(payload).encode("utf-8")
        headers["Content-Type"] = "application/json"

    request = urllib.request.Request(
        url,
        data=body,
        headers=headers,
        method=method,
    )

    try:
        with urllib.request.urlopen(
            request,
            timeout=HTTP_TIMEOUT,
        ) as response:

            raw = response.read()

    except urllib.error.URLError as exc:
        raise RuntimeError(
            f"HTTP request failed: {url}: {exc}"
        ) from exc

    try:
        return json.loads(raw)

    except json.JSONDecodeError as exc:
        raise RuntimeError(
            f"Invalid JSON returned by {url}"
        ) from exc


# ============================================================================
# Ollama API
# ============================================================================


def discover_models() -> list[OllamaModel]:
    """
    Discover models using /api/tags.
    """

    log.info(
        "Discovering Ollama models: %s/api/tags",
        OLLAMA_HOST,
    )

    data = http_json(
        "GET",
        f"{OLLAMA_HOST}/api/tags",
    )

    models = []

    for item in data.get("models", []):
        models.append(
            OllamaModel(
                name=item["name"],
                digest=item.get("digest", ""),
                size=item.get("size", 0),
                modified_at=item.get(
                    "modified_at",
                    "",
                ),
            )
        )

    return models


def show_model(name: str) -> dict[str, Any]:
    """
    Get detailed Ollama model metadata.
    """

    return http_json(
        "POST",
        f"{OLLAMA_HOST}/api/show",
        {
            "model": name,
        },
    )


# ============================================================================
# Metadata parsing
# ============================================================================


def find_context_window(
    model_info: dict[str, Any],
) -> int | None:
    """
    Find:

        llama.context_length
        qwen2.context_length
        qwen3.context_length
        gemma.context_length
        ...

    without hard-coding the architecture.
    """

    for key, value in model_info.items():

        if not key.endswith(".context_length"):
            continue

        if isinstance(value, int):
            return value

    return None


def find_parameter_count(
    model_info: dict[str, Any],
) -> int | None:

    value = model_info.get(
        "general.parameter_count"
    )

    if isinstance(value, int):
        return value

    if isinstance(value, float):
        return int(value)

    return None


def parse_metadata(
    name: str,
    raw: dict[str, Any],
) -> ModelMetadata:

    details = raw.get("details") or {}

    model_info = raw.get("model_info") or {}

    capabilities = set(
        raw.get("capabilities") or []
    )

    return ModelMetadata(

        name=name,

        family=details.get(
            "family"
        ),

        format=details.get(
            "format"
        ),

        parameter_size=details.get(
            "parameter_size"
        ),

        parameter_count=find_parameter_count(
            model_info
        ),

        quantization=details.get(
            "quantization_level"
        ),

        context_window=find_context_window(
            model_info
        ),

        completion="completion" in capabilities,

        tools="tools" in capabilities,

        thinking="thinking" in capabilities,

        vision="vision" in capabilities,

        embedding="embedding" in capabilities,

        model_info=model_info,

        raw=raw,
    )


# ============================================================================
# Cache
# ============================================================================


def load_cache() -> dict[str, Any]:

    if not CACHE_FILE.exists():
        return {}

    try:

        with CACHE_FILE.open(
            "r",
            encoding="utf-8",
        ) as f:

            data = json.load(f)

        if isinstance(data, dict):
            return data

    except Exception as exc:

        log.warning(
            "Cannot read cache: %s",
            exc,
        )

    return {}


def save_cache(
    cache: dict[str, Any],
) -> None:

    CACHE_FILE.parent.mkdir(
        parents=True,
        exist_ok=True,
    )

    atomic_write_json(
        CACHE_FILE,
        cache,
    )


# ============================================================================
# Model metadata retrieval
# ============================================================================


def get_model_metadata(
    model: OllamaModel,
    cache: dict[str, Any],
) -> ModelMetadata:

    cached = cache.get(model.name)

    #
    # Digest identifies the model content.
    #
    if (
        cached
        and cached.get("digest") == model.digest
        and cached.get("metadata")
    ):

        log.info(
            "Using cached metadata: %s",
            model.name,
        )

        return parse_metadata(
            model.name,
            cached["metadata"],
        )

    log.info(
        "Querying Ollama /api/show: %s",
        model.name,
    )

    raw = show_model(model.name)

    cache[model.name] = {
        "digest": model.digest,
        "modified_at": model.modified_at,
        "metadata": raw,
        "updated_at": datetime.now().isoformat(),
    }

    return parse_metadata(
        model.name,
        raw,
    )


def collect_metadata(
    models: list[OllamaModel],
    cache: dict[str, Any],
) -> dict[str, ModelMetadata]:

    result: dict[str, ModelMetadata] = {}

    with concurrent.futures.ThreadPoolExecutor(
        max_workers=SHOW_WORKERS,
    ) as executor:

        futures = {
            executor.submit(
                get_model_metadata,
                model,
                cache,
            ): model
            for model in models
        }

        for future in concurrent.futures.as_completed(
            futures
        ):

            model = futures[future]

            try:

                metadata = future.result()

                result[model.name] = metadata

            except Exception as exc:

                log.error(
                    "Failed to inspect %s: %s",
                    model.name,
                    exc,
                )

                raise

    return result


# ============================================================================
# maxTokens
# ============================================================================


def determine_max_tokens(
    metadata: ModelMetadata,
) -> int:

    if MAX_TOKENS_MODE == "fixed":
        return MAX_TOKENS

    if (
        MAX_TOKENS_MODE == "context"
        and metadata.context_window
    ):
        return metadata.context_window

    #
    # auto
    #
    if metadata.context_window:

        value = metadata.context_window // 4

        #
        # Prevent excessively large generated outputs.
        #
        value = min(
            value,
            32768,
        )

        #
        # Avoid tiny values.
        #
        value = max(
            value,
            1024,
        )

        return value

    return MAX_TOKENS


# ============================================================================
# Thinking rules
# ============================================================================


def thinking_level_map(
    metadata: ModelMetadata,
) -> dict[str, Any]:

    if not metadata.thinking:
        return {}

    family = (
        metadata.family or ""
    ).lower()

    name = metadata.name.lower()

    #
    # GPT-OSS
    #
    # Values must be string | null (pi models.json schema).
    # null      -> level hidden from the UI.
    # string    -> value sent as reasoning_effort.
    # "none"    -> disables thinking on Ollama's OpenAI-compatible API.
    #
    if (
        "gpt-oss" in name
        or "gpt-oss" in family
    ):

        return {
            "off": "none",
            "minimal": None,
            "low": "low",
            "medium": "medium",
            "high": "high",
            "xhigh": None,
            "max": None,
        }

    #
    # Generic Ollama thinking models.
    #
    return {
        "off": "none",
        "minimal": "low",
        "low": "low",
        "medium": "medium",
        "high": "high",
        "xhigh": "max",
        "max": "max",
    }


# ============================================================================
# Pi model conversion
# ============================================================================


def metadata_to_pi_model(
    metadata: ModelMetadata,
) -> dict[str, Any]:

    context_window = (
        metadata.context_window
        or 32768
    )

    model = {

        "id": metadata.name,

        "name": metadata.name,

        "reasoning": metadata.thinking,

        "input": (
            ["text", "image"]
            if metadata.vision
            else ["text"]
        ),

        "cost": {
            "input": 0,
            "output": 0,
            "cacheRead": 0,
            "cacheWrite": 0,
        },

        "contextWindow": context_window,

        "maxTokens": determine_max_tokens(
            metadata
        ),

        #
        # Extra metadata is kept here for
        # diagnostics / future tooling.
        #
        "metadata": {
            "ollama": {
                "family": metadata.family,
                "format": metadata.format,
                "parameterSize": metadata.parameter_size,
                "parameterCount": metadata.parameter_count,
                "quantization": metadata.quantization,
                "capabilities": {
                    "completion": metadata.completion,
                    "tools": metadata.tools,
                    "thinking": metadata.thinking,
                    "vision": metadata.vision,
                    "embedding": metadata.embedding,
                },
            }
        },
    }

    thinking_map = thinking_level_map(
        metadata
    )

    if thinking_map:
        model["thinkingLevelMap"] = thinking_map

    return model


# ============================================================================
# Pi configuration
# ============================================================================


def load_pi_config() -> dict[str, Any]:

    if not PI_CONFIG.exists():
        return {}

    try:

        with PI_CONFIG.open(
            "r",
            encoding="utf-8",
        ) as f:

            data = json.load(f)

    except json.JSONDecodeError as exc:

        raise RuntimeError(
            f"Invalid Pi configuration: {PI_CONFIG}"
        ) from exc

    if not isinstance(data, dict):

        raise RuntimeError(
            f"Pi configuration root must be object: "
            f"{PI_CONFIG}"
        )

    return data


def generate_provider(
    models: list[dict[str, Any]],
) -> dict[str, Any]:

    return {

        "baseUrl": f"{OLLAMA_HOST}/v1",

        "api": "openai-completions",

        "apiKey": "ollama",

        "models": models,
    }


def merge_pi_config(
    config: dict[str, Any],
    models: list[dict[str, Any]],
) -> dict[str, Any]:

    #
    # Copy so the caller's structure isn't mutated.
    #
    result = json.loads(
        json.dumps(config)
    )

    providers = result.setdefault(
        "providers",
        {},
    )

    providers[PI_PROVIDER] = generate_provider(
        models
    )

    return result


# ============================================================================
# Atomic file operations
# ============================================================================


def atomic_write_json(
    path: Path,
    data: Any,
) -> None:

    path.parent.mkdir(
        parents=True,
        exist_ok=True,
    )

    fd, tmp_name = tempfile.mkstemp(
        prefix=f".{path.name}.",
        suffix=".tmp",
        dir=path.parent,
        text=True,
    )

    try:

        with os.fdopen(
            fd,
            "w",
            encoding="utf-8",
        ) as f:

            json.dump(
                data,
                f,
                ensure_ascii=False,
                indent=2,
            )

            f.write("\n")

            f.flush()

            os.fsync(
                f.fileno()
            )

        os.replace(
            tmp_name,
            path,
        )

    except Exception:

        try:
            os.unlink(tmp_name)
        except OSError:
            pass

        raise


def backup_pi_config() -> None:

    if not PI_CONFIG.exists():
        return

    BACKUP_DIR.mkdir(
        parents=True,
        exist_ok=True,
    )

    timestamp = datetime.now().strftime(
        "%Y%m%d-%H%M%S"
    )

    backup = (
        BACKUP_DIR
        / f"models.json.{timestamp}"
    )

    shutil.copy2(
        PI_CONFIG,
        backup,
    )

    log.info(
        "Backup created: %s",
        backup,
    )


# ============================================================================
# Cleanup stale cache
# ============================================================================


def cleanup_cache(
    cache: dict[str, Any],
    models: list[OllamaModel],
) -> None:

    current = {
        model.name
        for model in models
    }

    stale = [
        name
        for name in cache
        if name not in current
    ]

    for name in stale:

        log.info(
            "Removing stale cache entry: %s",
            name,
        )

        del cache[name]


# ============================================================================
# Display
# ============================================================================


def print_summary(
    metadata: dict[str, ModelMetadata],
) -> None:

    print()

    print(
        "=" * 120
    )

    print(
        "Ollama -> Pi Agent"
    )

    print(
        "=" * 120
    )

    print(
        f"{'MODEL':35} "
        f"{'CONTEXT':10} "
        f"{'MAX':10} "
        f"{'REASON':8} "
        f"{'VISION':8} "
        f"{'TOOLS':8} "
        f"{'PARAMS':12} "
        f"{'QUANT':12}"
    )

    print(
        "-" * 120
    )

    for name in sorted(metadata):

        m = metadata[name]

        if not m.completion:
            continue

        print(
            f"{name:35} "
            f"{str(m.context_window or '?'):10} "
            f"{str(determine_max_tokens(m)):10} "
            f"{str(m.thinking):8} "
            f"{str(m.vision):8} "
            f"{str(m.tools):8} "
            f"{str(m.parameter_size or '?'):12} "
            f"{str(m.quantization or '?'):12}"
        )

    print(
        "=" * 120
    )

    print()


# ============================================================================
# Main
# ============================================================================


def main() -> int:

    log.info(
        "Starting Ollama -> Pi Agent synchronization"
    )

    log.info(
        "Ollama: %s",
        OLLAMA_HOST,
    )

    log.info(
        "Pi config: %s",
        PI_CONFIG,
    )

    #
    # Step 1:
    # Discover models.
    #
    models = discover_models()

    if not models:

        log.warning(
            "Ollama returned zero models."
        )

        log.warning(
            "Existing Pi configuration will NOT be modified."
        )

        return 0

    log.info(
        "Found %d Ollama model(s)",
        len(models),
    )

    #
    # Step 2:
    # Load cache.
    #
    cache = load_cache()

    #
    # Step 3:
    # Remove models no longer present.
    #
    cleanup_cache(
        cache,
        models,
    )

    #
    # Step 4:
    # Query metadata.
    #
    metadata = collect_metadata(
        models,
        cache,
    )

    #
    # Step 5:
    # Save cache.
    #
    save_cache(cache)

    #
    # Step 6:
    # Convert to Pi models.
    #
    pi_models = []

    for name in sorted(metadata):

        m = metadata[name]

        #
        # Skip embedding-only models.
        #
        if not m.completion:

            log.info(
                "Skipping non-completion model: %s",
                name,
            )

            continue

        pi_models.append(
            metadata_to_pi_model(m)
        )

    if not pi_models:

        raise RuntimeError(
            "No completion-capable Ollama models found."
        )

    #
    # Step 7:
    # Load existing Pi config.
    #
    config = load_pi_config()

    #
    # Step 8:
    # Merge Ollama provider.
    #
    new_config = merge_pi_config(
        config,
        pi_models,
    )

    #
    # Step 9:
    # Backup.
    #
    backup_pi_config()

    #
    # Step 10:
    # Atomic update.
    #
    atomic_write_json(
        PI_CONFIG,
        new_config,
    )

    #
    # Step 11:
    # Summary.
    #
    print_summary(
        metadata
    )

    log.info(
        "Synchronization completed successfully."
    )

    return 0


if __name__ == "__main__":

    try:

        sys.exit(
            main()
        )

    except KeyboardInterrupt:

        log.warning(
            "Interrupted."
        )

        sys.exit(130)

    except Exception as exc:

        log.error(
            "%s",
            exc,
        )

        sys.exit(1)
