/**
 * Disk operations: digest cache, models.json load/merge, atomic write, backup.
 */

import { copyFile, mkdir, open, readFile, rename, stat as statFile, unlink } from "node:fs/promises";
import path from "node:path";
import { randomBytes } from "node:crypto";
import type { SyncConfig } from "./config.js";
import type { OllamaModelRef } from "./ollama.js";
import { generateProvider, type PiModel } from "./pi-model.js";

// ---------------------------------------------------------------------------
// Digest cache
// ---------------------------------------------------------------------------

export interface CacheEntry {
	digest: string;
	modified_at: string;
	metadata: Record<string, unknown>;
	updated_at: string;
}

export type MetadataCache = Record<string, CacheEntry>;

export async function loadCache(cfg: SyncConfig): Promise<MetadataCache> {
	let raw: string;
	try {
		raw = await readFile(cfg.cacheFile, "utf-8");
	} catch {
		return {};
	}

	try {
		const data = JSON.parse(raw);
		if (data !== null && typeof data === "object" && !Array.isArray(data)) {
			return data as MetadataCache;
		}
	} catch (err) {
		console.error(`[ollama-sync] warning: cannot read cache: ${err instanceof Error ? err.message : String(err)}`);
	}
	return {};
}

export async function saveCache(cfg: SyncConfig, cache: MetadataCache): Promise<void> {
	await atomicWriteJson(cfg.cacheFile, cache);
}

export function cleanupCache(cache: MetadataCache, models: OllamaModelRef[]): string[] {
	const current = new Set(models.map((m) => m.name));
	const stale = Object.keys(cache).filter((name) => !current.has(name));
	for (const name of stale) {
		delete cache[name];
	}
	return stale;
}

// ---------------------------------------------------------------------------
// Pi config
// ---------------------------------------------------------------------------

export async function loadPiConfig(cfg: SyncConfig): Promise<Record<string, unknown>> {
	let raw: string;
	try {
		raw = await readFile(cfg.piConfig, "utf-8");
	} catch {
		return {};
	}

	const data = JSON.parse(raw); // syntax errors must abort the run
	if (data === null || typeof data !== "object" || Array.isArray(data)) {
		throw new Error(`${cfg.piConfig}: top-level JSON value must be an object`);
	}
	return data as Record<string, unknown>;
}

export function mergePiConfig(cfg: SyncConfig, config: Record<string, unknown>, models: PiModel[]): Record<string, unknown> {
	const result = structuredClone(config) as Record<string, unknown>;
	const providers = (result.providers ?? {}) as Record<string, unknown>;
	providers[cfg.provider] = generateProvider(cfg, models);
	result.providers = providers;
	return result;
}

// ---------------------------------------------------------------------------
// Atomic write + backup
// ---------------------------------------------------------------------------

export async function atomicWriteJson(file: string, data: unknown): Promise<void> {
	const dir = path.dirname(file);
	await mkdir(dir, { recursive: true });

	const tmp = path.join(dir, `.${path.basename(file)}.${randomBytes(6).toString("hex")}.tmp`);

	try {
		const handle = await open(tmp, "w");
		try {
			await handle.writeFile(JSON.stringify(data, null, 2) + "\n", "utf-8");
			await handle.sync();
		} finally {
			await handle.close();
		}
		await rename(tmp, file);
	} catch (err) {
		await unlink(tmp).catch(() => {});
		throw err;
	}
}

function backupTimestamp(): string {
	const now = new Date();
	const pad = (n: number) => String(n).padStart(2, "0");
	return (
		`${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}` +
		`-${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`
	);
}

export async function backupPiConfig(cfg: SyncConfig): Promise<string | null> {
	try {
		const s = await statFile(cfg.piConfig);
		if (!s.isFile()) return null;
	} catch {
		return null; // nothing to back up yet
	}

	await mkdir(cfg.backupDir, { recursive: true });

	const backup = path.join(cfg.backupDir, `models.json.${backupTimestamp()}`);
	await copyFile(cfg.piConfig, backup);
	return backup;
}
