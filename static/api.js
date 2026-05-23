import { elements, addConsoleLine, pollStatus, state } from './app.js';
import { drawSpectrum, drawWaterfall, drawSkyplot } from './canvas.js';
import { updateTelemetryBoard, updateSNRBars } from './telemetry.js';

export let receiverEventSource = null;
export let logEventSource = null;

export function setReceiverEventSource(val) {
	receiverEventSource = val;
}

export function setLogEventSource(val) {
	logEventSource = val;
}

export async function authedFetch(url, options = {}) {
	const token = localStorage.getItem('gps_sim_token');
	options.headers = options.headers || {};
	if (token) {
		options.headers['Authorization'] = `Bearer ${token}`;
	}
	if (!(options.body instanceof FormData)) {
		if (!options.headers['Content-Type'] && options.method === 'POST') {
			options.headers['Content-Type'] = 'application/json';
		}
	}
	try {
		const response = await fetch(url, options);
		if (response.status === 401) {
			import('./auth.js').then(m => m.showTokenModal());
		}
		return response;
	} catch (error) {
		console.error(`Fetch to ${url} failed:`, error);
		throw error;
	}
}

export function connectSSE() {
	if (logEventSource) {
		logEventSource.close();
	}
	const token = localStorage.getItem('gps_sim_token');
	const sseUrl = token ? `/api/simulation/logs?token=${encodeURIComponent(token)}` : '/api/simulation/logs';

	logEventSource = new EventSource(sseUrl);

	logEventSource.onmessage = (event) => {
		const text = event.data;
		if (text.includes("✅") || text.includes("成功")) {
			addConsoleLine(text, "success");
		} else if (text.includes("❌") || text.includes("错误") || text.includes("失败")) {
			addConsoleLine(text, "error");
		} else if (text.includes("⚠️") || text.includes("警告")) {
			addConsoleLine(text, "warning");
		} else if (text.includes("---")) {
			addConsoleLine(text, "system");
		} else {
			addConsoleLine(text);
		}
	};

	logEventSource.onerror = () => {
		addConsoleLine("⚠️ 后端日志断开，正在尝试重连...", "warning");
	};
}

export function connectReceiverSSE() {
	if (receiverEventSource) {
		receiverEventSource.close();
	}
	const token = localStorage.getItem('gps_sim_token');
	const sseUrl = token ? `/api/receiver/stream?token=${encodeURIComponent(token)}` : '/api/receiver/stream';

	receiverEventSource = new EventSource(sseUrl);

	receiverEventSource.onmessage = (event) => {
		try {
			const data = JSON.parse(event.data);
			if (data.type === 'sweep') {
				drawSpectrum(data.low, data.high, data.width, data.dbs);
				drawWaterfall(data.dbs);
			} else if (data.type === 'telemetry') {
				updateTelemetryBoard(data);
				drawSkyplot(data.satellites);
				updateSNRBars(data.satellites);
			}
		} catch (err) {
			console.error("Failed to parse receiver SSE message:", err);
		}
	};

	receiverEventSource.onerror = (err) => {
		console.error("Receiver EventSource error:", err);
	};
}

export async function startSimulation() {
	const lat = parseFloat(elements.inputLat.value);
	const lng = parseFloat(elements.inputLng.value);
	const alt = parseFloat(elements.inputAlt.value);
	const duration = parseInt(elements.inputDuration.value);
	const gain = parseInt(elements.inputGain.value);
	const ephemeris = elements.selectEphemeris.value;
	const system = elements.selectSystem.value;
	const band = elements.selectBand.value;

	const params = {
		lat, lng, alt, duration, gain, ephemeris, system, band
	};

	if (isNaN(lat) || isNaN(lng) || isNaN(alt)) {
		alert("请输入合法的经纬度！");
		return;
	}

	try {
		const response = await authedFetch('/api/simulation/start', {
			method: 'POST',
			body: JSON.stringify(params)
		});

		if (!response.ok) {
			const errMsg = await response.text();
			throw new Error(errMsg);
		}

		addConsoleLine("[SYSTEM] 发射启动请求成功已发送，后端正在处理...", "system");
		pollStatus();
	} catch (error) {
		alert(`启动模拟失败: ${error.message}`);
		addConsoleLine(`[SYSTEM] 启动模拟失败: ${error.message}`, "error");
	}
}

export async function stopSimulation() {
	try {
		const response = await authedFetch('/api/simulation/stop', { method: 'POST' });
		if (!response.ok) throw new Error("Stop failed");
		
		addConsoleLine("[SYSTEM] 强制终止指令已下发，正在清理发射任务...", "system");
		pollStatus();
	} catch (error) {
		alert("终止模拟发生异常，请刷新页面检查。");
	}
}

export async function autoDownloadEphemeris(type = 'gps') {
	const btn = type === 'multi' ? elements.btnDownloadEphemerisBds : elements.btnDownloadEphemeris;
	btn.disabled = true;
	const label = type === 'multi' ? '北斗多系统混合' : 'GPS';
	addConsoleLine(`[SYSTEM] ${label}星历后台自动下载任务已触发，请查看控制台实时输出...`, "system");
	
	try {
		const response = await authedFetch('/api/ephemeris/download', {
			method: 'POST',
			body: JSON.stringify({ type: type })
		});
		if (!response.ok) throw new Error("下载请求提交失败");
	} catch (error) {
		addConsoleLine(`[SYSTEM] 触发星历下载失败: ${error.message}`, "error");
	} finally {
		setTimeout(() => { btn.disabled = false; }, 3000);
	}
}

export async function uploadEphemeris() {
	const file = elements.ephemerisUpload.files[0];
	if (!file) return;

	addConsoleLine(`[SYSTEM] 正在上传星历文件: ${file.name} ...`, "system");
	
	const formData = new FormData();
	formData.append('ephemeris', file);

	try {
		const response = await authedFetch('/api/ephemeris/upload', {
			method: 'POST',
			body: formData
		});

		if (!response.ok) {
			const text = await response.text();
			throw new Error(text);
		}

		addConsoleLine(`✅ 星历文件 ${file.name} 上传并解压保存完毕！`, "success");
		pollStatus();
	} catch (error) {
		addConsoleLine(`❌ 上传星历失败: ${error.message}`, "error");
		alert(`上传星历失败: ${error.message}`);
	} finally {
		elements.ephemerisUpload.value = ''; // Reset file input
	}
}

export async function systemSetup() {
	elements.btnSystemSetup.disabled = true;
	addConsoleLine("[SYSTEM] 环境自动配置已触发，正在检查并克隆编译 gps-sdr-sim，请观察控制台输出...", "system");

	try {
		const response = await authedFetch('/api/system/setup', { method: 'POST' });
		if (!response.ok) throw new Error("环境配置请求提交失败");
	} catch (error) {
		addConsoleLine(`[SYSTEM] 依赖环境配置失败: ${error.message}`, "error");
	}
}

export async function startReceiverScan() {
	const btnStart = document.getElementById('btn-rx-start');
	const btnStop = document.getElementById('btn-rx-stop');

	const params = {
		lat: parseFloat(elements.inputLat.value) || 39.9042,
		lng: parseFloat(elements.inputLng.value) || 116.4074,
		alt: parseFloat(elements.inputAlt.value) || 100.0
	};

	btnStart.disabled = true;
	btnStart.textContent = "启动中...";

	try {
		const response = await authedFetch('/api/receiver/start', {
			method: 'POST',
			body: JSON.stringify(params)
		});

		if (!response.ok) {
			const text = await response.text();
			throw new Error(text);
		}

		addConsoleLine("🔍 卫星信号接收仪已成功启动，正在建立高频遥测通道...", "success");
		btnStop.disabled = false;
		btnStart.textContent = "启动接收扫频";
		
		connectReceiverSSE();
		pollStatus();
	} catch (error) {
		btnStart.disabled = false;
		btnStart.textContent = "启动接收扫频";
		alert(`启动扫频失败: ${error.message}`);
		addConsoleLine(`❌ 启动接收仪失败: ${error.message}`, "error");
	}
}

export async function stopReceiverScan() {
	const btnStart = document.getElementById('btn-rx-start');
	const btnStop = document.getElementById('btn-rx-stop');

	btnStop.disabled = true;

	try {
		const response = await authedFetch('/api/receiver/stop', { method: 'POST' });
		if (!response.ok) throw new Error("Stop request failed");

		addConsoleLine("🔍 接收分析仪停止指令已执行。", "system");
		
		if (receiverEventSource) {
			receiverEventSource.close();
			receiverEventSource = null;
		}

		btnStart.disabled = false;
		resetReceiverUI();
		pollStatus();
	} catch (error) {
		btnStop.disabled = false;
		alert(`停止扫频失败: ${error.message}`);
	}
}

export function resetReceiverUI() {
	const rxFixType = document.getElementById('rx-fix-type');
	const rxAccuracy = document.getElementById('rx-accuracy');
	const rxCoords = document.getElementById('rx-coords');
	const rxAltitude = document.getElementById('rx-altitude');
	const rxTime = document.getElementById('rx-time');
	const rxSatsUsed = document.getElementById('rx-sats-used');
	const rxSatsInView = document.getElementById('rx-sats-inview');
	const badgeGps = document.getElementById('badge-gps');
	const badgeBds = document.getElementById('badge-bds');
	const container = document.getElementById('snr-bars-container');

	if (rxFixType) {
		rxFixType.textContent = "SEARCHING...";
		rxFixType.className = "telemetry-value text-searching";
	}
	if (rxAccuracy) rxAccuracy.textContent = "精度: --";
	if (rxCoords) rxCoords.textContent = "-- , --";
	if (rxAltitude) rxAltitude.textContent = "海拔: --";
	if (rxTime) rxTime.textContent = "--:--:--.--";
	if (rxSatsUsed) rxSatsUsed.textContent = "0";
	if (rxSatsInView) rxSatsInView.textContent = "0";
	if (badgeGps) badgeGps.className = "sat-badge gps-badge inactive";
	if (badgeBds) badgeBds.className = "sat-badge bds-badge inactive";
	if (container) container.innerHTML = '<div class="snr-placeholder">等待接收解算数据...</div>';

	const canvasSpectrum = document.getElementById('canvas-spectrum');
	const canvasWaterfall = document.getElementById('canvas-waterfall');
	const canvasSkyplot = document.getElementById('canvas-skyplot');

	if (canvasSpectrum) {
		const ctx = canvasSpectrum.getContext('2d');
		ctx.clearRect(0, 0, canvasSpectrum.width, canvasSpectrum.height);
	}
	if (canvasWaterfall) {
		const ctx = canvasWaterfall.getContext('2d');
		ctx.clearRect(0, 0, canvasWaterfall.width, canvasWaterfall.height);
	}
	if (canvasSkyplot) {
		const ctx = canvasSkyplot.getContext('2d');
		ctx.clearRect(0, 0, canvasSkyplot.width, canvasSkyplot.height);
	}

	state.waterfallLines = [];
}
