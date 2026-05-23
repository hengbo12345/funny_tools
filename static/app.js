import { verifyAndSubmitToken, showTokenModal } from './auth.js';
import { initMap, searchAddress, map, marker } from './map.js';
import { 
	connectSSE, 
	startSimulation, 
	stopSimulation, 
	autoDownloadEphemeris, 
	uploadEphemeris, 
	systemSetup, 
	startReceiverScan, 
	stopReceiverScan,
	authedFetch
} from './api.js';
import { resizeCanvases } from './canvas.js';

// Global state cache exported for sub-modules
export const state = {
	waterfallLines: [],
	maxWaterfallLines: 60,
	isSimulationRunning: false
};

// UI DOM elements cache
export const elements = {
	// Badges
	hackrfVal: document.getElementById('hackrf-val'),
	hackrfIndicator: document.getElementById('hackrf-indicator'),
	tcxoVal: document.getElementById('tcxo-val'),
	tcxoIndicator: document.getElementById('tcxo-indicator'),
	simVal: document.getElementById('sim-val'),
	simIndicator: document.getElementById('sim-indicator'),

	// Inputs
	inputLat: document.getElementById('input-lat'),
	inputLng: document.getElementById('input-lng'),
	inputAlt: document.getElementById('input-alt'),
	selectSystem: document.getElementById('select-system'),
	selectBand: document.getElementById('select-band'),
	selectEphemeris: document.getElementById('select-ephemeris'),
	inputDuration: document.getElementById('input-duration'),
	inputGain: document.getElementById('input-gain'),
	addressSearch: document.getElementById('address-search'),

	// Action Buttons
	btnStart: document.getElementById('btn-start'),
	btnStop: document.getElementById('btn-stop'),
	btnSearch: document.getElementById('btn-search'),
	btnEphemerisRefresh: document.getElementById('btn-ephemeris-refresh'),
	btnDownloadEphemeris: document.getElementById('btn-download-ephemeris'),
	btnDownloadEphemerisBds: document.getElementById('btn-download-ephemeris-bds'),
	btnUploadTrigger: document.getElementById('btn-upload-trigger'),
	ephemerisUpload: document.getElementById('ephemeris-upload'),
	btnSystemSetup: document.getElementById('btn-system-setup'),
	btnConsoleClear: document.getElementById('btn-console-clear'),

	// Overlay & containers
	overlayLat: document.getElementById('overlay-lat'),
	overlayLng: document.getElementById('overlay-lng'),
	searchResults: document.getElementById('search-results'),
	terminalOutput: document.getElementById('terminal-output'),
	gpsSimMissingAlert: document.getElementById('gps-sim-missing-alert'),

	// Security Token Modal
	tokenModal: document.getElementById('token-modal'),
	tokenInput: document.getElementById('token-input'),
	btnTokenSubmit: document.getElementById('btn-token-submit'),
	btnTogglePassword: document.getElementById('btn-toggle-password'),
	tokenError: document.getElementById('token-error')
};

// -------------------------------------------------------------
// APP INITIALIZATION
// -------------------------------------------------------------

document.addEventListener('DOMContentLoaded', () => {
	initMap();
	updateBandOptions();
	setupEventListeners();
	connectSSE();
	pollStatus();
	setInterval(pollStatus, 3000); // Poll status every 3s
});

// -------------------------------------------------------------
// HELPER LOGGING FUNCTIONS
// -------------------------------------------------------------

export function addConsoleLine(text, type = "default") {
	const line = document.createElement('div');
	line.className = `terminal-line ${type}-line`;
	line.textContent = text;
	elements.terminalOutput.appendChild(line);
	elements.terminalOutput.scrollTop = elements.terminalOutput.scrollHeight;
}

// -------------------------------------------------------------
// STATUS POLLING & BADGES UI UPDATER
// -------------------------------------------------------------

export async function pollStatus() {
	try {
		const response = await authedFetch('/api/status');
		if (response.status === 401) {
			elements.hackrfVal.textContent = "待授权";
			elements.hackrfIndicator.className = "indicator gray";
			elements.tcxoVal.textContent = "待授权";
			elements.tcxoIndicator.className = "indicator gray";
			elements.simVal.textContent = "请先输入安全 Token 认证";
			elements.simIndicator.className = "indicator gray";
			elements.btnStart.disabled = true;
			elements.btnStop.disabled = true;
			return;
		}
		if (!response.ok) throw new Error("服务器无响应");
		const data = await response.json();
		
		updateStatusUI(data);
	} catch (error) {
		console.error("Error polling status:", error);
		elements.hackrfVal.textContent = "连接失败";
		elements.hackrfIndicator.className = "indicator red";
		elements.tcxoVal.textContent = "检测中断";
		elements.tcxoIndicator.className = "indicator red";
	}
}

function updateStatusUI(data) {
	// 1. HackRF status
	if (data.hackrf_connected) {
		elements.hackrfVal.textContent = "已连接";
		elements.hackrfIndicator.className = "indicator green pulse";
	} else {
		elements.hackrfVal.textContent = "未连接";
		elements.hackrfIndicator.className = "indicator red";
	}

	// 2. TCXO Status
	elements.tcxoVal.textContent = data.tcxo_status;
	if (data.tcxo_status.includes("已锁定") || data.tcxo_status.includes("signal detected")) {
		elements.tcxoIndicator.className = "indicator green";
	} else if (data.tcxo_status.includes("未检测到") && data.status === "transmitting") {
		elements.tcxoIndicator.className = "indicator red";
	} else if (data.tcxo_status.includes("未检测到")) {
		elements.tcxoIndicator.className = "indicator amber";
	} else {
		elements.tcxoIndicator.className = "indicator gray";
	}

	// 3. Overall Simulation status
	state.isSimulationRunning = data.status === 'generating' || data.status === 'transmitting';
	
	if (data.active_sim) {
		if (elements.selectSystem.value !== data.active_sim.system) {
			elements.selectSystem.value = data.active_sim.system;
			updateBandOptions();
		}
		elements.selectBand.value = data.active_sim.band;
	}

	// Synchronize Ephemeris Select UI list
	if (data.ephemeris_files) {
		syncEphemerisList(data.ephemeris_files, data.active_sim ? data.active_sim.ephemeris : null);
	}

	switch (data.status) {
		case 'idle':
			elements.simVal.textContent = "空闲 (IDLE)";
			elements.simIndicator.className = "indicator gray";
			elements.btnStart.disabled = false;
			elements.btnStop.disabled = true;
			elements.selectSystem.disabled = false;
			elements.selectBand.disabled = false;
			break;
		case 'generating':
			elements.simVal.textContent = "正在生成信号基带... (GENERATING)";
			elements.simIndicator.className = "indicator amber pulse";
			elements.btnStart.disabled = true;
			elements.btnStop.disabled = false;
			elements.selectSystem.disabled = true;
			elements.selectBand.disabled = true;
			break;
		case 'transmitting':
			const sysName = data.active_sim && data.active_sim.system === 'beidou' ? '北斗' : 'GPS';
			const bandName = data.active_sim ? data.active_sim.band : 'L1';
			elements.simVal.textContent = `正在发射 ${sysName} ${bandName} 模拟信号... (TRANSMITTING)`;
			elements.simIndicator.className = "indicator green pulse";
			elements.btnStart.disabled = true;
			elements.btnStop.disabled = false;
			elements.selectSystem.disabled = true;
			elements.selectBand.disabled = true;
			break;
		case 'scanning':
			elements.simVal.textContent = "正在接收分析扫频中... (SCANNING)";
			elements.simIndicator.className = "indicator cyan pulse";
			elements.btnStart.disabled = false;
			elements.btnStop.disabled = true;
			elements.selectSystem.disabled = false;
			elements.selectBand.disabled = false;
			break;
		case 'error':
			elements.simVal.textContent = "发生错误 (ERROR)";
			elements.simIndicator.className = "indicator red pulse";
			elements.btnStart.disabled = false;
			elements.btnStop.disabled = true;
			elements.selectSystem.disabled = false;
			elements.selectBand.disabled = false;
			break;
	}

	// 4. Missing signal generator warning
	if (!data.gps_sim_exists || !data.beidou_sim_exists) {
		elements.gpsSimMissingAlert.classList.remove('hidden');
		let missing = [];
		if (!data.gps_sim_exists) missing.push('gps-sdr-sim');
		if (!data.beidou_sim_exists) missing.push('beidou-sdr-sim');
		elements.gpsSimMissingAlert.querySelector('h3').textContent = `系统缺少 ${missing.join(' 和 ')} 信号生成组件`;
	} else {
		elements.gpsSimMissingAlert.classList.add('hidden');
	}
}

function syncEphemerisList(files, activeFile) {
	const select = elements.selectEphemeris;
	const currentSelection = select.value;
	
	select.innerHTML = '';
	
	if (files.length === 0) {
		select.innerHTML = '<option value="" disabled selected>暂无可用星历，请在下方点击下载</option>';
		return;
	}

	files.forEach(f => {
		const opt = document.createElement('option');
		opt.value = f;
		opt.textContent = f;
		select.appendChild(opt);
	});

	if (activeFile && files.includes(activeFile)) {
		select.value = activeFile;
	} else if (currentSelection && files.includes(currentSelection)) {
		select.value = currentSelection;
	} else {
		select.selectedIndex = 0;
	}
}

// -------------------------------------------------------------
// GNSS BANDS DICTIONARY & SELECT MANAGEMENT
// -------------------------------------------------------------

const GNSS_BANDS = {
	gps: [
		{ value: 'L1', text: 'L1 - 1575.42 MHz' },
		{ value: 'L5', text: 'L5 - 1176.45 MHz (实验性)' }
	],
	beidou: [
		{ value: 'B1I', text: 'B1I - 1561.098 MHz' },
		{ value: 'B1C', text: 'B1C - 1575.42 MHz (实验性)' },
		{ value: 'B2a', text: 'B2a - 1176.45 MHz (实验性)' }
	]
};

function updateBandOptions() {
	const system = elements.selectSystem.value;
	const bands = GNSS_BANDS[system] || [];
	elements.selectBand.innerHTML = '';
	bands.forEach(b => {
		const opt = document.createElement('option');
		opt.value = b.value;
		opt.textContent = b.text;
		elements.selectBand.appendChild(opt);
	});
}

// -------------------------------------------------------------
// EVENT LISTENERS BINDING
// -------------------------------------------------------------

function setupEventListeners() {
	// Tab switching controls
	const tabBtnTx = document.getElementById('tab-btn-tx');
	const tabBtnRx = document.getElementById('tab-btn-rx');
	const tabContentTx = document.getElementById('tab-content-tx');
	const tabContentRx = document.getElementById('tab-content-rx');

	if (tabBtnTx && tabBtnRx) {
		tabBtnTx.addEventListener('click', () => {
			tabBtnTx.classList.add('active');
			tabBtnRx.classList.remove('active');
			tabContentTx.classList.remove('hidden');
			tabContentRx.classList.add('hidden');
			if (map) {
				setTimeout(() => map.invalidateSize(), 50);
			}
		});

		tabBtnRx.addEventListener('click', () => {
			tabBtnRx.classList.add('active');
			tabBtnTx.classList.remove('active');
			tabContentRx.classList.remove('hidden');
			tabContentTx.classList.add('hidden');
			resizeCanvases();
		});
	}

	// Receiver sweep controls
	const btnRxStart = document.getElementById('btn-rx-start');
	const btnRxStop = document.getElementById('btn-rx-stop');

	if (btnRxStart && btnRxStop) {
		btnRxStart.addEventListener('click', startReceiverScan);
		btnRxStop.addEventListener('click', stopReceiverScan);
	}

	// Start and stop controls
	elements.btnStart.addEventListener('click', startSimulation);
	elements.btnStop.addEventListener('click', stopSimulation);

	// Address Search (Nominatim)
	elements.btnSearch.addEventListener('click', searchAddress);
	elements.addressSearch.addEventListener('keydown', (e) => {
		if (e.key === 'Enter') {
			searchAddress();
		}
	});

	// Hide search result popup on clicking outside
	document.addEventListener('click', (e) => {
		if (e.target !== elements.addressSearch && e.target !== elements.searchResults) {
			elements.searchResults.classList.add('hidden');
		}
	});

	// Ephemeris operations
	elements.btnEphemerisRefresh.addEventListener('click', () => {
		addConsoleLine("[SYSTEM] 正在刷新可用星历列表...", "system");
		pollStatus();
	});

	elements.selectSystem.addEventListener('change', () => {
		updateBandOptions();
	});
	
	elements.btnDownloadEphemeris.addEventListener('click', () => autoDownloadEphemeris('gps'));
	elements.btnDownloadEphemerisBds.addEventListener('click', () => autoDownloadEphemeris('multi'));

	elements.btnUploadTrigger.addEventListener('click', () => {
		elements.ephemerisUpload.click();
	});

	elements.ephemerisUpload.addEventListener('change', uploadEphemeris);

	// Setup triggers
	elements.btnSystemSetup.addEventListener('click', systemSetup);

	// Monospace Console clears screen
	elements.btnConsoleClear.addEventListener('click', () => {
		elements.terminalOutput.innerHTML = '<div class="terminal-line system-line">[SYSTEM] 控制台已清屏。</div>';
	});

	// Update coordinates manually from inputs
	const coordChangeHandler = () => {
		const lat = parseFloat(elements.inputLat.value);
		const lng = parseFloat(elements.inputLng.value);
		if (!isNaN(lat) && !isNaN(lng) && lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180) {
			marker.setLatLng([lat, lng]);
			map.setView([lat, lng]);
			elements.overlayLat.textContent = lat.toFixed(6);
			elements.overlayLng.textContent = lng.toFixed(6);
		}
	};

	elements.inputLat.addEventListener('change', coordChangeHandler);
	elements.inputLng.addEventListener('change', coordChangeHandler);

	// Submit token on button click
	elements.btnTokenSubmit.addEventListener('click', verifyAndSubmitToken);

	// Submit token on Enter key inside input
	elements.tokenInput.addEventListener('keydown', (e) => {
		if (e.key === 'Enter') {
			verifyAndSubmitToken();
		}
	});

	// Toggle password visibility
	elements.btnTogglePassword.addEventListener('click', () => {
		const input = elements.tokenInput;
		const isPassword = input.type === 'password';
		input.type = isPassword ? 'text' : 'password';
		
		// Swap the eye icon SVG
		const svg = elements.btnTogglePassword.querySelector('svg');
		if (isPassword) {
			svg.innerHTML = '<path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24"></path><line x1="1" y1="1" x2="23" y2="23"></line>';
		} else {
			svg.innerHTML = '<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"></path><circle cx="12" cy="12" r="3"></circle>';
		}
	});
}
