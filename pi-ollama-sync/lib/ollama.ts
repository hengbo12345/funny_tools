/**
 * Ollama HTTP API: /api/tags discovery and /api/show metadata.
 */

export interface OllamaModelRef {
	name: string;
	digest: string;
	size: number;
	modifiedAt: string;
}

export interface HttpLogInfo {
	url: string;
	status?: number;
	body?: string;
}

export class OllamaHttpError extends Error {
	readonly status: number;
	readonly url: string;
	readonly body: string;

	constructor(status: number, statusText: string, url: string, body: string) {
		const trimmed = body.length > 0 ? `: ${body.slice(0, 500)}` : "";
		super(`HTTP ${status} for ${url}: ${statusText}${trimmed}`);
		this.name = "OllamaHttpError";
		this.status = status;
		this.url = url;
		this.body = body;
	}
}

async function httpJson(
	method: "GET" | "POST",
	url: string,
	timeoutMs: number,
	payload?: unknown,
): Promise<Record<string, unknown>> {
	let response: Response;

	try {
		response = await fetch(url, {
			method,
			headers: { Accept: "application/json" },
			body: payload === undefined ? undefined : JSON.stringify(payload),
			signal: AbortSignal.timeout(timeoutMs),
		});
	} catch (err) {
		if (err instanceof Error && err.name === "TimeoutError") {
			throw new Error(`HTTP request timed out: ${url}`);
		}
		throw new Error(`HTTP request failed: ${url}: ${err instanceof Error ? err.message : String(err)}`);
	}

	if (!response.ok) {
		let body = "";
		try {
			body = await response.text();
		} catch {
			// body unavailable; fall through with empty text
		}
		throw new OllamaHttpError(response.status, response.statusText, url, body);
	}

	const raw = await response.text();

	try {
		const parsed = JSON.parse(raw);
		if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) {
			throw new Error("not an object");
		}
		return parsed as Record<string, unknown>;
	} catch {
		throw new Error(`Invalid JSON returned by ${url}`);
	}
}

export async function discoverModels(cfg: { host: string; httpTimeoutMs: number }): Promise<OllamaModelRef[]> {
	const data = await httpJson("GET", `${cfg.host}/api/tags`, cfg.httpTimeoutMs);

	const items = Array.isArray(data.models) ? data.models : [];

	return items.map((item) => {
		const entry = (item ?? {}) as Record<string, unknown>;
		return {
			name: String(entry.name ?? ""),
			digest: String(entry.digest ?? ""),
			size: typeof entry.size === "number" ? entry.size : 0,
			modifiedAt: String(entry.modified_at ?? ""),
		};
	});
}

export async function showModel(host: string, timeoutMs: number, name: string): Promise<Record<string, unknown>> {
	return httpJson("POST", `${host}/api/show`, timeoutMs, { model: name });
}
