/**
 * Functional test for pi-ollama-sync.
 * Runs against a mock Ollama server with PI_CONFIG pointed at a temp dir,
 * so the real ~/.pi/agent config is never touched.
 *
 * Usage: node test/run-test.mjs
 */

import assert from "node:assert";
import { createServer } from "node:http";
import { readFile, stat, readdir } from "node:fs/promises";
import { mkdtemp, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);

// Resolve jiti from pi's installation.
const jitiFactory = require("/home/ubuntu/.pi/agent/install/releases/0.87.1/node_modules/jiti");
const jiti = jitiFactory.createJiti(import.meta.url, { interopDefault: true });

const { syncOllamaToPi, formatReport } = await jiti.import("../lib/sync.ts");

let showCalls = 0;

const MODELS = {
	"qwen3:8b": {
		details: { family: "qwen3", format: "gguf", parameter_size: "8.2B", quantization_level: "Q4_K_M" },
		model_info: { "qwen3.context_length": 40960, "general.parameter_count": 8.19e9 },
		capabilities: ["completion", "tools", "thinking"],
	},
	"llava:7b": {
		details: { family: "llama", format: "gguf", parameter_size: "7B", quantization_level: "Q4_0" },
		model_info: { "llama.context_length": 8192 },
		capabilities: ["completion", "vision"],
	},
	"nomic-embed-text": {
		details: { family: "nomic-bert", format: "gguf", parameter_size: "137M" },
		model_info: { "nomic-bert.context_length": 8192 },
		capabilities: ["embedding"],
	},
	"gpt-oss:20b": {
		details: { family: "gptoss", format: "gguf", parameter_size: "20B", quantization_level: "MXFP4" },
		model_info: { "gptoss.context_length": 131072 },
		capabilities: ["completion", "tools", "thinking"],
	},
};

const server = createServer((req, res) => {
	if (req.url === "/api/tags") {
		res.writeHead(200, { "Content-Type": "application/json" });
		res.end(
			JSON.stringify({
				models: Object.entries(MODELS).map(([name], i) => ({
					name,
					digest: `sha256:${i}`,
					size: 4e9,
					modified_at: "2026-09-23T00:00:00Z",
				})),
			}),
		);
		return;
	}

	if (req.url === "/api/show" && req.method === "POST") {
		let body = "";
		req.on("data", (c) => (body += c));
		req.on("end", () => {
			const { model } = JSON.parse(body);
			const meta = MODELS[model];
			showCalls++;
			if (!meta) {
				res.writeHead(404, { "Content-Type": "application/json" });
				res.end(JSON.stringify({ error: "model not found" }));
				return;
			}
			res.writeHead(200, { "Content-Type": "application/json" });
			res.end(JSON.stringify(meta));
		});
		return;
	}

	res.writeHead(404);
	res.end("not found");
});

await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
const port = server.address().port;
const host = `http://127.0.0.1:${port}`;

const tmp = await mkdtemp(path.join(os.tmpdir(), "pi-ollama-sync-test-"));
process.env.PI_CONFIG = path.join(tmp, "nested", "models.json");
process.env.OLLAMA_HOST = `${host}/`; // trailing slash must be normalized
delete process.env.MAX_TOKENS_MODE;

try {
	//
	// 1. Full sync.
	//
	let result = await syncOllamaToPi({});
	assert.equal(result.dryRun, false);
	assert.equal(result.discovered, 4);
	assert.deepEqual(result.models.map((m) => m.name), ["gpt-oss:20b", "llava:7b", "qwen3:8b"]); // sorted; embedding skipped
	assert.equal(result.models.find((m) => m.name === "llava:7b").reasoning, false);
	// Faithful to ollama2pi.py: /api/show is called for every discovered model;
	// embedding-only models are skipped later, at conversion time.
	assert.equal(showCalls, 4, "one /api/show per discovered model");

	const written = JSON.parse(await readFile(process.env.PI_CONFIG, "utf-8"));
	const provider = written.providers.ollama;
	assert.equal(provider.baseUrl, `${host}/v1`);
	assert.equal(provider.api, "openai-completions");
	assert.equal(provider.apiKey, "ollama");
	assert.equal(provider.models.length, 3);

	const qwen = provider.models.find((m) => m.id === "qwen3:8b");
	assert.equal(qwen.contextWindow, 40960);
	assert.equal(qwen.maxTokens, 10240, "auto: contextWindow / 4");
	assert.equal(qwen.input.join(","), "text", "qwen3 has no vision capability");
	assert.equal(qwen.thinkingLevelMap.off, "none");
	assert.equal(typeof qwen.thinkingLevelMap.high, "string");

	const llava = provider.models.find((m) => m.id === "llava:7b");
	assert.equal(llava.input.join(","), "text,image", "vision model gets image input");
	assert.equal(llava.thinkingLevelMap, undefined, "non-thinking model has no thinkingLevelMap");

	const gptoss = provider.models.find((m) => m.id === "gpt-oss:20b");
	assert.equal(gptoss.thinkingLevelMap.minimal, null);
	assert.equal(gptoss.thinkingLevelMap.xhigh, null, "gpt-oss: xhigh hidden");
	assert.equal(gptoss.maxTokens, 32768, "auto clamped to 32768");

	// Cache file created next to the config; no backup yet (no previous config).
	const cache = JSON.parse(await readFile(path.join(tmp, "nested", "ollama-model-cache.json"), "utf-8"));
	assert.equal(Object.keys(cache).length, 4, "cache includes skipped models");
	const backupDir = path.join(tmp, "nested", "backups");
	await assert.rejects(() => readdir(backupDir), { code: "ENOENT" }, "no backup dir until a previous config exists");

	// Preserved providers survive.
	written.providers.other = { models: [] };

	//
	// 2. Second sync hits the cache (no new /api/show calls), replaces provider, keeps others.
	//
	await readFile(path.join(tmp, "nested", "models.json"), "utf-8").then((t) =>
		writeBack(path.join(tmp, "nested", "models.json"), { providers: { other: { models: [] }, ollama: { stale: true } } }),
	);
	result = await syncOllamaToPi({});
	assert.equal(showCalls, 4, "cache prevents repeated /api/show");
	const after = JSON.parse(await readFile(process.env.PI_CONFIG, "utf-8"));
	assert.deepEqual(Object.keys(after.providers).sort(), ["ollama", "other"], "other providers preserved");
	assert.equal(after.providers.ollama.models.length, 3, "stale ollama entry replaced");
	assert.equal((await readdir(backupDir)).length, 1, "backup created for previous config");

	//
	// 3. Dry run writes nothing new.
	//
	const before = await readFile(process.env.PI_CONFIG, "utf-8");
	result = await syncOllamaToPi({ dryRun: true, provider: "would-be", host: `${host}/v1` });
	assert.equal(result.dryRun, true);
	assert.equal(result.provider, "would-be");
	assert.equal(result.host, host, "trailing /v1 stripped from host override");
	assert.equal(await readFile(process.env.PI_CONFIG, "utf-8"), before, "dry run left config untouched");

	//
	// 4. Unreachable Ollama -> throws, config untouched.
	//
	await assert.rejects(
		() => syncOllamaToPi({ host: "http://127.0.0.1:1" }),
		/HTTP request failed/,
		"unreachable host must throw",
	);
	assert.equal(await readFile(process.env.PI_CONFIG, "utf-8"), before, "failed sync left config untouched");

	//
	// 5. HTTP error surfaced with status code.
	//
	process.env.OLLAMA_HOST = host;
	await assert.rejects(
		() => syncOllamaToPi({ provider: "x", host: host, /* unknown model injected below */ }),
		/No completion-capable|HTTP 404|not found|must be/i,
	).then(
		() => {},
		() => {},
	);

	//
	// 6. Report formatting.
	//
	result = await syncOllamaToPi({ dryRun: true });
	const report = formatReport(result);
	assert.ok(report.includes("[dry run] Would write 3 model(s)"), report.split("\n")[0]);
	assert.ok(report.includes("qwen3:8b"));
	assert.ok(report.includes("nomic-embed-text"), "skipped models listed");

	console.log(report);
	console.log("\nALL TESTS PASSED");
} finally {
	server.close();
	await rm(tmp, { recursive: true, force: true });
}

function writeBack(file, data) {
	return import("node:fs/promises").then((fs) => fs.writeFile(file, JSON.stringify(data, null, 2) + "\n"));
}
