/**
 * pi-ollama-sync
 *
 * Pi Agent extension that synchronizes Ollama models into pi's custom model
 * configuration (~/.pi/agent/models.json) so they appear in the /model picker.
 *
 * - Command: /ollama-sync [check] [provider=<name>] [host=<url>]
 * - Tool:    ollama_sync (model-callable, supports dry_run)
 *
 * TypeScript port of the original ollama2pi.py; no Python or third-party
 * dependencies required.
 */

import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { Type } from "typebox";
import { formatReport, syncOllamaToPi, type SyncOptions, type SyncResult } from "./lib/sync.js";

const SyncParams = Type.Object({
	dry_run: Type.Optional(
		Type.Boolean({
			description:
				"Preview only: discover models and report what would be written, without modifying models.json. Default false.",
		}),
	),
	host: Type.Optional(
		Type.String({
			description:
				"Ollama host base URL, e.g. http://192.168.1.100:11434. Overrides the OLLAMA_HOST environment variable. A trailing /v1 is stripped automatically.",
		}),
	),
	provider: Type.Optional(
		Type.String({
			description:
				'Provider entry name to write in models.json. Default "ollama". CAUTION: never use the value of the PI_PROVIDER env var (pi exports it per session); use an explicit name.',
		}),
	),
});

interface SyncDetails extends SyncResult {
	report: string;
	error?: undefined;
}

function parseCommandArgs(args: string): SyncOptions {
	const options: SyncOptions = {};

	for (const token of args.split(/\s+/).filter(Boolean)) {
		if (token === "check" || token === "--check" || token === "--dry-run" || token === "-n") {
			options.dryRun = true;
			continue;
		}

		const eq = token.indexOf("=");
		if (eq > 0) {
			const key = token.slice(0, eq).replace(/^--/, "");
			const value = token.slice(eq + 1);
			if (key === "provider") options.provider = value;
			else if (key === "host") options.host = value;
		}
	}

	return options;
}

export default function ollamaSyncExtension(pi: ExtensionAPI) {
	async function run(options: SyncOptions): Promise<{ report: string; result: SyncResult }> {
		const logs: string[] = [];
		const result = await syncOllamaToPi({
			...options,
			log: (message) => logs.push(message),
		});
		return { report: formatReport(result), result };
	}

	pi.registerCommand("ollama-sync", {
		description: "Sync Ollama models into pi models.json (append 'check' for a dry run)",
		getArgumentCompletions: (prefix) => {
			const suggestions = ["check", "provider=", "host="].filter((s) => s.startsWith(prefix));
			return suggestions.length > 0 ? suggestions.map((s) => ({ value: s, label: s })) : null;
		},
		handler: async (args, ctx) => {
			try {
				const { report } = await run(parseCommandArgs(args));
				if (ctx.hasUI) {
					ctx.ui.notify(report, "info");
				} else {
					console.log(report);
				}
			} catch (err) {
				const message = err instanceof Error ? err.message : String(err);
				if (ctx.hasUI) {
					ctx.ui.notify(`ollama-sync failed: ${message}`, "error");
				} else {
					console.error(`ollama-sync failed: ${message}`);
				}
			}
		},
	});

	pi.registerTool({
		name: "ollama_sync",
		label: "Ollama sync",
		description:
			"Synchronize models from an Ollama server into pi's models.json (~/.pi/agent/models.json) so they appear in the /model picker. " +
			"Discovers models via /api/tags, fetches metadata via /api/show, and writes an 'ollama' provider entry (OpenAI-compatible, " +
			"thinking-level mapping included; embedding-only models are skipped). The previous config is backed up and the write is atomic. " +
			"Use dry_run=true to preview. Requires a reachable Ollama server.",
		parameters: SyncParams,

		async execute(_toolCallId, params, _signal, _onUpdate, _ctx): Promise<{ content: Array<{ type: "text"; text: string }>; details: SyncDetails }> {
			try {
				const { report, result } = await run({
					dryRun: params.dry_run,
					host: params.host,
					provider: params.provider,
				});
				return {
					content: [{ type: "text", text: report }],
					details: { ...result, report },
				};
			} catch (err) {
				const message = err instanceof Error ? err.message : String(err);
				throw new Error(`ollama_sync failed: ${message}`);
			}
		},
	});
}
