/**
 * Orchestrates the Ollama -> Pi Agent synchronization.
 *
 * Guarantees carried over from ollama2pi.py:
 * - Digest-based cache: /api/show is only called when a model changed.
 * - Non-completion (embedding-only) models are skipped.
 * - Pre-write validation so pi never rejects the generated entry.
 * - Previous config is backed up; the write is atomic (tmp + rename).
 * - When Ollama is unreachable or returns no models, the existing
 *   configuration is NOT modified.
 * - dryRun reports what would happen without touching any file
 *   (except the cache refresh, which is harmless).
 */

import { loadConfig, type SyncConfig } from "./config.js";
import { discoverModels, showModel, type OllamaModelRef } from "./ollama.js";
import { parseMetadata, type ModelMetadata } from "./metadata.js";
import { determineMaxTokens, metadataToPiModel, type PiModel } from "./pi-model.js";
import { validateModelForPi } from "./validate.js";
import {
	atomicWriteJson,
	backupPiConfig,
	cleanupCache,
	loadCache,
	loadPiConfig,
	mergePiConfig,
	saveCache,
	type MetadataCache,
} from "./store.js";

export interface SyncOptions {
	/** Report what would be written without modifying models.json. */
	dryRun?: boolean;
	/** Override the Ollama host (e.g. "http://192.168.1.100:11434"). */
	host?: string;
	/** Override the provider entry name in models.json. */
	provider?: string;
	/** Progress log sink. */
	log?: (message: string) => void;
}

export interface SyncResult {
	dryRun: boolean;
	host: string;
	provider: string;
	configPath: string;
	backupPath: string | null;
	discovered: number;
	skipped: Array<{ name: string; reason: string }>;
	models: Array<{
		name: string;
		contextWindow: number | null;
		maxTokens: number;
		reasoning: boolean;
		vision: boolean;
		tools: boolean;
		parameterSize: string | null;
		quantization: string | null;
	}>;
}

function resolveConfig(options: SyncOptions): SyncConfig {
	const cfg = loadConfig();
	if (options.host) {
		const host = options.host.replace(/\/+$/, "");
		cfg.host = host.endsWith("/v1") ? host.slice(0, -3) : host;
	}
	if (options.provider) {
		cfg.provider = options.provider;
	}
	return cfg;
}

async function collectMetadata(
	cfg: SyncConfig,
	models: OllamaModelRef[],
	cache: MetadataCache,
	log: (message: string) => void,
): Promise<Map<string, ModelMetadata>> {
	const result = new Map<string, ModelMetadata>();
	let index = 0;

	//
	// Bounded-concurrency worker pool (SHOW_WORKERS).
	//
	async function worker(): Promise<void> {
		while (index < models.length) {
			const model = models[index++];
			const cached = cache[model.name];

			// Digest identifies the model content.
			if (cached && cached.digest === model.digest && cached.metadata) {
				log(`Using cached metadata: ${model.name}`);
				result.set(model.name, parseMetadata(model, cached.metadata));
				continue;
			}

			log(`Querying Ollama /api/show: ${model.name}`);
			const raw = await showModel(cfg.host, cfg.httpTimeoutMs, model.name);

			cache[model.name] = {
				digest: model.digest,
				modified_at: model.modifiedAt,
				metadata: raw,
				updated_at: new Date().toISOString(),
			};

			result.set(model.name, parseMetadata(model, raw));
		}
	}

	await Promise.all(
		Array.from({ length: Math.min(cfg.showWorkers, models.length) }, () => worker()),
	);

	// Fail fast reproduces the Python behavior: any inspection error
	// aborts the whole run and leaves the config untouched.
	return result;
}

export async function syncOllamaToPi(options: SyncOptions = {}): Promise<SyncResult> {
	const log = options.log ?? (() => {});
	const cfg = resolveConfig(options);

	if (process.env.MAX_TOKENS_MODE && !["auto", "context", "fixed"].includes(process.env.MAX_TOKENS_MODE.toLowerCase())) {
		log(`Unknown MAX_TOKENS_MODE=${JSON.stringify(process.env.MAX_TOKENS_MODE)}, falling back to auto`);
	}

	log(`Starting Ollama -> Pi Agent synchronization`);
	log(`Ollama: ${cfg.host}`);
	log(`Pi config: ${cfg.piConfig}`);

	//
	// Step 1: Discover models.
	//
	const models = await discoverModels(cfg);

	if (models.length === 0) {
		log("Ollama returned zero models. Existing Pi configuration will NOT be modified.");
		return {
			dryRun: options.dryRun === true,
			host: cfg.host,
			provider: cfg.provider,
			configPath: cfg.piConfig,
			backupPath: null,
			discovered: 0,
			skipped: [],
			models: [],
		};
	}

	log(`Found ${models.length} Ollama model(s)`);

	//
	// Step 2-3: Load cache, drop stale entries.
	//
	const cache = await loadCache(cfg);
	cleanupCache(cache, models);

	//
	// Step 4: Query metadata (parallel, bounded).
	//
	const metadata = await collectMetadata(cfg, models, cache, log);

	//
	// Step 5: Save cache.
	//
	await saveCache(cfg, cache);

	//
	// Step 6: Convert to Pi models.
	//
	const skipped: SyncResult["skipped"] = [];
	const piModels: PiModel[] = [];

	for (const name of [...metadata.keys()].sort()) {
		const m = metadata.get(name);
		if (!m) continue;

		if (!m.completion) {
			log(`Skipping non-completion model: ${name}`);
			skipped.push({ name, reason: "no completion capability" });
			continue;
		}

		piModels.push(metadataToPiModel(cfg, m));
	}

	if (piModels.length === 0) {
		throw new Error("No completion-capable Ollama models found.");
	}

	//
	// Step 6.5: Validate before touching disk.
	//
	for (const model of piModels) {
		validateModelForPi(model);
	}

	const modelSummaries = piModels.map((m) => {
		const meta = metadata.get(m.id)!;
		return {
			name: m.id,
			contextWindow: m.contextWindow,
			maxTokens: m.maxTokens,
			reasoning: meta.thinking,
			vision: meta.vision,
			tools: meta.tools,
			parameterSize: meta.parameterSize,
			quantization: meta.quantization,
		};
	});

	//
	// Dry run: stop right before side effects on models.json.
	//
	if (options.dryRun) {
		log(`Dry run: would write ${piModels.length} model(s) to ${cfg.piConfig} under provider "${cfg.provider}".`);
		return {
			dryRun: true,
			host: cfg.host,
			provider: cfg.provider,
			configPath: cfg.piConfig,
			backupPath: null,
			discovered: models.length,
			skipped,
			models: modelSummaries,
		};
	}

	//
	// Step 7-8: Load and merge.
	//
	const config = await loadPiConfig(cfg);
	const newConfig = mergePiConfig(cfg, config, piModels);

	//
	// Step 9-10: Backup, then atomic update.
	//
	const backupPath = await backupPiConfig(cfg);
	if (backupPath) log(`Backup created: ${backupPath}`);

	await atomicWriteJson(cfg.piConfig, newConfig);

	log("Synchronization completed successfully.");

	return {
		dryRun: false,
		host: cfg.host,
		provider: cfg.provider,
		configPath: cfg.piConfig,
		backupPath,
		discovered: models.length,
		skipped,
		models: modelSummaries,
	};
}

// ---------------------------------------------------------------------------
// Reporting
// ---------------------------------------------------------------------------

export function formatReport(result: SyncResult): string {
	const lines: string[] = [];

	lines.push(
		`${result.dryRun ? "[dry run] Would write" : "Wrote"} ${result.models.length} model(s) ` +
			`to ${result.configPath} (provider "${result.provider}", Ollama ${result.host})`,
	);
	if (result.backupPath) {
		lines.push(`Backup: ${result.backupPath}`);
	}

	if (result.models.length > 0) {
		lines.push("");
		lines.push(
			"MODEL".padEnd(35) +
				"CONTEXT".padEnd(10) +
				"MAX".padEnd(10) +
				"REASON".padEnd(8) +
				"VISION".padEnd(8) +
				"TOOLS".padEnd(8) +
				"PARAMS".padEnd(12) +
				"QUANT",
		);
		for (const m of result.models) {
			lines.push(
				m.name.padEnd(35) +
					String(m.contextWindow ?? "?").padEnd(10) +
					String(m.maxTokens).padEnd(10) +
					String(m.reasoning).padEnd(8) +
					String(m.vision).padEnd(8) +
					String(m.tools).padEnd(8) +
					(m.parameterSize ?? "?").padEnd(12) +
					(m.quantization ?? "?"),
			);
		}
	}

	if (result.skipped.length > 0) {
		lines.push("");
		lines.push(`Skipped ${result.skipped.length} non-completion model(s): ${result.skipped.map((s) => s.name).join(", ")}`);
	}

	return lines.join("\n");
}
