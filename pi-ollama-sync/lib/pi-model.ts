/**
 * Convert Ollama metadata into pi models.json model/provider entries.
 */

import type { ModelMetadata } from "./metadata.js";
import type { ThinkingLevelMap } from "./thinking.js";
import { thinkingLevelMap } from "./thinking.js";
import type { SyncConfig } from "./config.js";

export interface PiModel {
	id: string;
	name: string;
	reasoning: boolean;
	input: string[];
	cost: Record<string, number>;
	contextWindow: number;
	maxTokens: number;
	thinkingLevelMap?: ThinkingLevelMap;
	metadata: { ollama: Record<string, unknown> };
}

export function determineMaxTokens(cfg: SyncConfig, metadata: ModelMetadata): number {
	if (cfg.maxTokensMode === "fixed") return cfg.maxTokens;

	if (cfg.maxTokensMode === "context" && metadata.contextWindow) {
		return metadata.contextWindow;
	}

	//
	// auto (also the fallback for an unknown MAX_TOKENS_MODE)
	//
	if (metadata.contextWindow) {
		const value = Math.floor(metadata.contextWindow / 4);
		return Math.min(Math.max(value, 1024), 32768);
	}

	return cfg.maxTokens;
}

export function metadataToPiModel(cfg: SyncConfig, metadata: ModelMetadata): PiModel {
	const contextWindow = metadata.contextWindow ?? 32768;

	const model: PiModel = {
		id: metadata.name,
		name: metadata.name,
		reasoning: metadata.thinking,
		input: metadata.vision ? ["text", "image"] : ["text"],
		cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
		contextWindow,
		maxTokens: determineMaxTokens(cfg, metadata),
		metadata: {
			ollama: {
				family: metadata.family,
				format: metadata.format,
				parameterSize: metadata.parameterSize,
				parameterCount: metadata.parameterCount,
				quantization: metadata.quantization,
				capabilities: {
					completion: metadata.completion,
					tools: metadata.tools,
					thinking: metadata.thinking,
					vision: metadata.vision,
					embedding: metadata.embedding,
				},
			},
		},
	};

	const map = thinkingLevelMap(metadata);
	if (Object.keys(map).length > 0) {
		model.thinkingLevelMap = map;
	}

	return model;
}

export function generateProvider(cfg: SyncConfig, models: PiModel[]): Record<string, unknown> {
	return {
		baseUrl: `${cfg.host}/v1`,
		api: "openai-completions",
		apiKey: "ollama",
		models,
	};
}
