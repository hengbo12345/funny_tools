/**
 * Pre-write guard so we never emit a models.json that pi would reject
 * (a rejected entry blocks the whole provider, as happened with the old
 * thinkingLevelMap off:false bug).
 */

import type { PiModel } from "./pi-model.js";

export function validateModelForPi(model: PiModel): void {
	const wid = model.id;

	if (!Number.isInteger(model.contextWindow) || model.contextWindow <= 0) {
		throw new Error(`${wid}: contextWindow must be a positive int`);
	}

	if (!Number.isInteger(model.maxTokens) || model.maxTokens <= 0) {
		throw new Error(`${wid}: maxTokens must be a positive int`);
	}

	if (model.maxTokens > model.contextWindow) {
		// Warning only; pi accepts this, it is just suspicious.
		console.error(
			`[ollama-sync] warning: ${wid}: maxTokens (${model.maxTokens}) exceeds contextWindow (${model.contextWindow})`,
		);
	}

	const mapping = model.thinkingLevelMap;
	if (mapping !== undefined) {
		for (const [level, value] of Object.entries(mapping)) {
			if (!(typeof value === "string" || value === null)) {
				throw new Error(`${wid}: thinkingLevelMap['${level}'] must be string or null, got ${JSON.stringify(value)}`);
			}
		}
	}
}
