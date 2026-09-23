/**
 * Thinking-level mapping per model family.
 *
 * Values must be string | null (pi models.json schema):
 *   null   -> level hidden from the UI
 *   string -> value sent as reasoning_effort
 *   "none" -> disables thinking on Ollama's OpenAI-compatible API
 */

export type ThinkingLevelMap = Record<string, string | null>;

export function thinkingLevelMap(metadata: { thinking: boolean; family: string | null; name: string }): ThinkingLevelMap {
	if (!metadata.thinking) return {};

	const family = (metadata.family ?? "").toLowerCase();
	const name = metadata.name.toLowerCase();

	//
	// GPT-OSS
	//
	if (name.includes("gpt-oss") || family.includes("gpt-oss")) {
		return {
			off: "none",
			minimal: null,
			low: "low",
			medium: "medium",
			high: "high",
			xhigh: null,
			max: null,
		};
	}

	//
	// Generic Ollama thinking models.
	//
	return {
		off: "none",
		minimal: "low",
		low: "low",
		medium: "medium",
		high: "high",
		xhigh: "max",
		max: "max",
	};
}
