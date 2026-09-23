/**
 * Parse /api/show responses into a flat metadata record.
 */

import type { OllamaModelRef } from "./ollama.js";

export interface ModelMetadata {
	name: string;
	family: string | null;
	format: string | null;
	parameterSize: string | null;
	parameterCount: number | null;
	quantization: string | null;
	contextWindow: number | null;
	completion: boolean;
	tools: boolean;
	thinking: boolean;
	vision: boolean;
	embedding: boolean;
}

function asRecord(value: unknown): Record<string, unknown> {
	return value !== null && typeof value === "object" && !Array.isArray(value)
		? (value as Record<string, unknown>)
		: {};
}

/**
 * Find `<arch>.context_length` (llama.context_length, qwen3.context_length,
 * ...) without hard-coding the architecture.
 */
function findContextWindow(modelInfo: Record<string, unknown>): number | null {
	for (const [key, value] of Object.entries(modelInfo)) {
		if (key.endsWith(".context_length") && typeof value === "number" && Number.isInteger(value)) {
			return value;
		}
	}
	return null;
}

function findParameterCount(modelInfo: Record<string, unknown>): number | null {
	const value = modelInfo["general.parameter_count"];
	return typeof value === "number" ? Math.trunc(value) : null;
}

export function parseMetadata(ref: OllamaModelRef, raw: Record<string, unknown>): ModelMetadata {
	const details = asRecord(raw.details);
	const modelInfo = asRecord(raw.model_info);
	const capabilities = new Set(Array.isArray(raw.capabilities) ? raw.capabilities.map(String) : []);

	return {
		name: ref.name,
		family: typeof details.family === "string" ? details.family : null,
		format: typeof details.format === "string" ? details.format : null,
		parameterSize: typeof details.parameter_size === "string" ? details.parameter_size : null,
		parameterCount: findParameterCount(modelInfo),
		quantization: typeof details.quantization_level === "string" ? details.quantization_level : null,
		contextWindow: findContextWindow(modelInfo),
		completion: capabilities.has("completion"),
		tools: capabilities.has("tools"),
		thinking: capabilities.has("thinking"),
		vision: capabilities.has("vision"),
		embedding: capabilities.has("embedding"),
	};
}
