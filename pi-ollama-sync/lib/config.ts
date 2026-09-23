/**
 * Runtime configuration, resolved fresh on every sync run so that
 * environment changes (e.g. different OLLAMA_HOST per invocation)
 * are always honored.
 */

import os from "node:os";
import path from "node:path";

export interface SyncConfig {
	host: string;
	piConfig: string;
	provider: string;
	cacheFile: string;
	backupDir: string;
	maxTokensMode: "auto" | "context" | "fixed";
	maxTokens: number;
	showWorkers: number;
	httpTimeoutMs: number;
}

function expandUser(p: string): string {
	if (p === "~") return os.homedir();
	if (p.startsWith("~/")) return path.join(os.homedir(), p.slice(2));
	return p;
}

/**
 * Strip a trailing "/v1": the provider baseUrl appends it once.
 * A user-supplied ".../v1" would otherwise yield ".../v1/v1".
 */
function normalizeHost(raw: string): string {
	const host = raw.replace(/\/+$/, "");
	return host.endsWith("/v1") ? host.slice(0, -3) : host;
}

function intEnv(name: string, fallback: number): number {
	const raw = process.env[name];
	if (raw === undefined || raw.trim() === "") return fallback;
	const value = Number.parseInt(raw, 10);
	return Number.isFinite(value) ? value : fallback;
}

export function loadConfig(): SyncConfig {
	const piConfig = expandUser(
		process.env.PI_CONFIG || path.join(os.homedir(), ".pi", "agent", "models.json"),
	);

	const modeRaw = (process.env.MAX_TOKENS_MODE || "auto").toLowerCase();
	const maxTokensMode: SyncConfig["maxTokensMode"] =
		modeRaw === "context" || modeRaw === "fixed" ? modeRaw : "auto";

	return {
		host: normalizeHost(process.env.OLLAMA_HOST || "http://127.0.0.1:11434"),
		piConfig,
		provider: process.env.PI_OLLAMA_PROVIDER || "ollama",
		cacheFile: path.join(path.dirname(piConfig), "ollama-model-cache.json"),
		backupDir: path.join(path.dirname(piConfig), "backups"),
		maxTokensMode,
		maxTokens: intEnv("MAX_TOKENS", 32768),
		showWorkers: Math.max(1, intEnv("SHOW_WORKERS", 4)),
		httpTimeoutMs: intEnv("OLLAMA_TIMEOUT", 60) * 1000,
	};
}
