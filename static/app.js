/* ==========================================================================
   GPS SIGNAL SIMULATOR - FRONTEND CONTROLLER (JS)
   ========================================================================== */

document.addEventListener('DOMContentLoaded', () => {
    // State management
    let map = null;
    let marker = null;
    let eventSource = null;
    let availableEphemeris = [];
    let isSimulationRunning = false;

    // Default coordinates: Tiananmen Square, Beijing
    const defaultCoords = [39.9042, 116.4074];

    // UI elements
    const elements = {
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
    // SECURITY TOKEN AUTHENTICATION SYSTEM
    // -------------------------------------------------------------

    function showTokenModal() {
        elements.tokenModal.classList.remove('hidden');
        elements.tokenInput.focus();
    }

    function hideTokenModal() {
        elements.tokenModal.classList.add('hidden');
        elements.tokenError.classList.add('hidden');
        elements.tokenInput.value = '';
    }

    async function authedFetch(url, options = {}) {
        const token = localStorage.getItem('gps_sim_token');
        options.headers = options.headers || {};
        if (token) {
            options.headers['Authorization'] = `Bearer ${token}`;
        }

        // Avoid setting JSON headers if body is FormData
        if (!(options.body instanceof FormData)) {
            if (!options.headers['Content-Type'] && options.method === 'POST') {
                options.headers['Content-Type'] = 'application/json';
            }
        }

        try {
            const response = await fetch(url, options);
            if (response.status === 401) {
                showTokenModal();
            }
            return response;
        } catch (error) {
            console.error(`Fetch to ${url} failed:`, error);
            throw error;
        }
    }

    async function verifyAndSubmitToken() {
        const token = elements.tokenInput.value.trim();
        if (!token) {
            elements.tokenError.textContent = "⚠️ 请输入您的访问 Token 凭证！";
            elements.tokenError.classList.remove('hidden');
            return;
        }

        elements.btnTokenSubmit.disabled = true;
        elements.btnTokenSubmit.textContent = "正在验证...";

        try {
            // Probe /api/status using the candidate token
            const response = await fetch('/api/status', {
                headers: {
                    'Authorization': `Bearer ${token}`
                }
            });

            if (response.status === 401) {
                throw new Error("Token 校验未通过");
            }

            if (!response.ok) {
                throw new Error("服务器响应异常，请稍后再试");
            }

            // Success! Save token and initialize app state
            localStorage.setItem('gps_sim_token', token);
            hideTokenModal();
            
            // Reload status & logs with the correct credentials
            pollStatus();
            connectSSE();
            
            addConsoleLine("🔓 安全访问校验成功，设备管理面板已成功解锁！", "success");
        } catch (error) {
            elements.tokenError.textContent = `⚠️ 认证失败：${error.message === "Token 校验未通过" ? "Token 错误或已失效！" : error.message}`;
            elements.tokenError.classList.remove('hidden');
            elements.tokenInput.focus();
        } finally {
            elements.btnTokenSubmit.disabled = false;
            elements.btnTokenSubmit.textContent = "验证并进入控制台";
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
    // INITIALIZATION
    // -------------------------------------------------------------

    initMap();
    updateBandOptions();
    setupEventListeners();
    connectSSE();
    pollStatus();
    setInterval(pollStatus, 3000); // Poll status every 3s

    // -------------------------------------------------------------
    // MAP FUNCTIONS
    // -------------------------------------------------------------

    function initMap() {
        // Initialize Leaflet map
        map = L.map('map').setView(defaultCoords, 13);

        // Load CartoDB Dark Matter tiles (nice dark theme!)
        L.tileLayer('https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png', {
            attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors &copy; <a href="https://carto.com/attributions">CARTO</a>',
            subdomains: 'abcd',
            maxZoom: 20
        }).addTo(map);

        // Add draggable marker
        marker = L.marker(defaultCoords, {
            draggable: true
        }).addTo(map);

        // Marker drag handler
        marker.on('dragend', () => {
            const position = marker.getLatLng();
            updateCoordinateInputs(position.lat, position.lng);
        });

        // Map click handler (relocates marker)
        map.on('click', (e) => {
            if (isSimulationRunning) return; // Disable changing coordinates when simulating
            const lat = e.latlng.lat;
            const lng = e.latlng.lng;
            marker.setLatLng([lat, lng]);
            updateCoordinateInputs(lat, lng);
        });
    }

    function updateCoordinateInputs(lat, lng) {
        // Limit coordinates to 6 decimal places
        const fixedLat = lat.toFixed(6);
        const fixedLng = lng.toFixed(6);
        
        elements.inputLat.value = fixedLat;
        elements.inputLng.value = fixedLng;
        elements.overlayLat.textContent = fixedLat;
        elements.overlayLng.textContent = fixedLng;
    }

    // -------------------------------------------------------------
    // CONSOLE / LOGS FUNCTIONS
    // -------------------------------------------------------------

    function addConsoleLine(text, type = 'system') {
        const line = document.createElement('div');
        line.className = `terminal-line ${type}-line`;
        line.textContent = text;
        elements.terminalOutput.appendChild(line);

        // Limit scrollback to 200 lines to avoid memory leak
        while (elements.terminalOutput.children.length > 200) {
            elements.terminalOutput.removeChild(elements.terminalOutput.firstChild);
        }

        // Scroll to bottom
        elements.terminalOutput.scrollTop = elements.terminalOutput.scrollHeight;
    }

    function connectSSE() {
        if (eventSource) {
            eventSource.close();
        }

        const token = localStorage.getItem('gps_sim_token');
        const sseUrl = token ? `/api/simulation/logs?token=${encodeURIComponent(token)}` : '/api/simulation/logs';
        eventSource = new EventSource(sseUrl);

        eventSource.onmessage = (event) => {
            let msg = event.data;
            if (!msg) return;

            // Determine line style based on contents
            let type = 'system';
            if (msg.includes('❌') || msg.includes('Error') || msg.includes('failed') || msg.includes('错误')) {
                type = 'error';
            } else if (msg.includes('✅') || msg.includes('成功') || msg.includes('finished') || msg.includes('完毕')) {
                type = 'success';
            } else if (msg.includes('执行命令:')) {
                type = 'command';
            }

            addConsoleLine(msg, type);
        };

        eventSource.onerror = (err) => {
            console.error("SSE connection error, attempting reconnection...", err);
            addConsoleLine("[SYSTEM] 日志推送流中断，正在尝试重新连接...", "system-line");
            setTimeout(connectSSE, 5000);
        };
    }

    // -------------------------------------------------------------
    // HTTP API INTERACTIONS
    // -------------------------------------------------------------

    async function pollStatus() {
        try {
            const response = await authedFetch('/api/status');
            if (response.status === 401) {
                // Awaiting authentication
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
            elements.tcxoIndicator.className = "indicator amber"; // Warn / Standby
        } else {
            elements.tcxoIndicator.className = "indicator gray";
        }

        // 3. Overall Simulation status
        isSimulationRunning = data.status === 'generating' || data.status === 'transmitting';
        
        // Keep selects in sync if simulation is running
        if (data.active_sim) {
            if (elements.selectSystem.value !== data.active_sim.system) {
                elements.selectSystem.value = data.active_sim.system;
                updateBandOptions();
            }
            elements.selectBand.value = data.active_sim.band;
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
            elements.gpsSimMissingAlert.querySelector('h3').innerHTML = `系统缺少 <code>${missing.join(' / ')}</code> 信号生成组件`;
        } else {
            elements.gpsSimMissingAlert.classList.add('hidden');
        }

        // 5. Available Ephemeris Files Dropdown
        updateEphemerisDropdown(data.ephemeris_files, data.active_sim ? data.active_sim.ephemeris : null);
    }

    function updateEphemerisDropdown(files, selected) {
        if (!files || files.length === 0) {
            elements.selectEphemeris.innerHTML = '<option value="" disabled selected>暂无可用星历，请先拉取</option>';
            availableEphemeris = [];
            return;
        }

        // Compare if arrays are identical
        if (JSON.stringify(files) === JSON.stringify(availableEphemeris) && elements.selectEphemeris.value !== "") {
            return; // Cache hit, don't recreate elements to prevent resetting selection
        }

        availableEphemeris = files;
        const previousSelection = elements.selectEphemeris.value || selected;

        elements.selectEphemeris.innerHTML = '';
        files.forEach(file => {
            const option = document.createElement('option');
            option.value = file;
            // Mark a label if it looks like the default embedded one
            if (file.includes('brdc1350')) {
                option.textContent = `${file} (内置默认备用)`;
            } else {
                option.textContent = file;
            }
            elements.selectEphemeris.appendChild(option);
        });

        if (previousSelection && files.includes(previousSelection)) {
            elements.selectEphemeris.value = previousSelection;
        } else {
            elements.selectEphemeris.selectedIndex = 0;
        }
    }

    // -------------------------------------------------------------
    // ADDRESS GEOCONDING SEARCH (NOMINATIM API)
    // -------------------------------------------------------------

    async function searchAddress() {
        const query = elements.addressSearch.value.trim();
        if (!query) return;

        elements.btnSearch.disabled = true;
        elements.searchResults.innerHTML = '<li class="loading">正在联想搜索地址中...</li>';
        elements.searchResults.classList.remove('hidden');

        try {
            const url = `https://nominatim.openstreetmap.org/search?format=json&q=${encodeURIComponent(query)}&limit=5`;
            const response = await fetch(url, {
                headers: {
                    'User-Agent': 'GPS-SDR-Sim-Console/1.0 (hackrf-toys)'
                }
            });
            if (!response.ok) throw new Error("Search failed");
            
            const results = await response.json();
            displaySearchResults(results);
        } catch (error) {
            console.error("Geocoding failed:", error);
            elements.searchResults.innerHTML = '<li class="error">搜索失败，请检查网络或直接在地图点击选点。</li>';
        } finally {
            elements.btnSearch.disabled = false;
        }
    }

    function displaySearchResults(results) {
        elements.searchResults.innerHTML = '';
        
        if (!results || results.length === 0) {
            elements.searchResults.innerHTML = '<li>未找到匹配的地址。请更换关键词。</li>';
            return;
        }

        results.forEach(item => {
            const li = document.createElement('li');
            li.textContent = item.display_name;
            li.addEventListener('click', () => {
                const lat = parseFloat(item.lat);
                const lng = parseFloat(item.lon);
                
                // Pan map and set marker
                map.setView([lat, lng], 15);
                marker.setLatLng([lat, lng]);
                updateCoordinateInputs(lat, lng);
                
                elements.searchResults.classList.add('hidden');
                elements.addressSearch.value = item.display_name;
                
                addConsoleLine(`[SEARCH] 地图成功定位至: ${item.display_name} (${lat.toFixed(6)}, ${lng.toFixed(6)})`, 'system');
            });
            elements.searchResults.appendChild(li);
        });
    }

    // -------------------------------------------------------------
    // ACTION HANDLERS
    // -------------------------------------------------------------

    async function startSimulation() {
        if (isSimulationRunning) return;

        const params = {
            lat: parseFloat(elements.inputLat.value),
            lng: parseFloat(elements.inputLng.value),
            alt: parseFloat(elements.inputAlt.value),
            duration: parseInt(elements.inputDuration.value),
            gain: parseInt(elements.inputGain.value),
            ephemeris: elements.selectEphemeris.value,
            system: elements.selectSystem.value,
            band: elements.selectBand.value
        };

        if (isNaN(params.lat) || isNaN(params.lng)) {
            alert("请输入合法的经纬度！");
            return;
        }

        try {
            const response = await authedFetch('/api/simulation/start', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
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

    async function stopSimulation() {
        try {
            const response = await authedFetch('/api/simulation/stop', { method: 'POST' });
            if (!response.ok) throw new Error("Stop failed");
            
            addConsoleLine("[SYSTEM] 强制终止指令已下发，正在清理发射任务...", "system");
            pollStatus();
        } catch (error) {
            alert("终止模拟发生异常，请刷新页面检查。");
        }
    }

    async function autoDownloadEphemeris(type = 'gps') {
        const btn = type === 'multi' ? elements.btnDownloadEphemerisBds : elements.btnDownloadEphemeris;
        btn.disabled = true;
        const label = type === 'multi' ? '北斗多系统混合' : 'GPS';
        addConsoleLine(`[SYSTEM] ${label}星历后台自动下载任务已触发，请查看控制台实时输出...`, "system");
        
        try {
            const response = await authedFetch('/api/ephemeris/download', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ type: type })
            });
            if (!response.ok) throw new Error("下载请求提交失败");
        } catch (error) {
            addConsoleLine(`[SYSTEM] 触发星历下载失败: ${error.message}`, "error");
        } finally {
            setTimeout(() => { btn.disabled = false; }, 3000);
        }
    }

    async function uploadEphemeris() {
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

    async function systemSetup() {
        elements.btnSystemSetup.disabled = true;
        addConsoleLine("[SYSTEM] 环境自动配置已触发，正在检查并克隆编译 gps-sdr-sim，请观察控制台输出...", "system");

        try {
            const response = await authedFetch('/api/system/setup', { method: 'POST' });
            if (!response.ok) throw new Error("环境配置请求提交失败");
        } catch (error) {
            addConsoleLine(`[SYSTEM] 依赖环境配置失败: ${error.message}`, "error");
        }
    }

    // -------------------------------------------------------------
    // RECEIVER & SWEEP SPECTROMETER SYSTEM
    // -------------------------------------------------------------
    let receiverEventSource = null;
    let waterfallLines = [];
    const maxWaterfallLines = 60;

    const resizeCanvases = () => {
        const list = [
            { canvas: document.getElementById('canvas-spectrum'), width: 1.0 },
            { canvas: document.getElementById('canvas-waterfall'), width: 1.0 },
            { canvas: document.getElementById('canvas-skyplot'), width: 0.0 }
        ];

        list.forEach(item => {
            if (!item.canvas) return;
            const rect = item.canvas.parentElement.getBoundingClientRect();
            if (item.width > 0) {
                item.canvas.width = rect.width * window.devicePixelRatio;
                item.canvas.height = (rect.height || 160) * window.devicePixelRatio;
            } else {
                const side = Math.min(rect.width, rect.height || 270);
                item.canvas.width = side * window.devicePixelRatio;
                item.canvas.height = side * window.devicePixelRatio;
            }
        });
    };

    window.addEventListener('resize', resizeCanvases);

    function drawSpectrum(low, high, width, dbs) {
        const canvas = document.getElementById('canvas-spectrum');
        if (!canvas) return;
        const ctx = canvas.getContext('2d');
        const w = canvas.width;
        const h = canvas.height;

        ctx.clearRect(0, 0, w, h);
        ctx.fillStyle = '#020306';
        ctx.fillRect(0, 0, w, h);

        const paddingLeft = 45 * window.devicePixelRatio;
        const paddingRight = 15 * window.devicePixelRatio;
        const paddingTop = 25 * window.devicePixelRatio;
        const paddingBottom = 25 * window.devicePixelRatio;

        const graphW = w - paddingLeft - paddingRight;
        const graphH = h - paddingTop - paddingBottom;

        // Draw Grid
        ctx.strokeStyle = 'rgba(255, 255, 255, 0.03)';
        ctx.lineWidth = 1 * window.devicePixelRatio;
        ctx.fillStyle = 'rgba(255, 255, 255, 0.3)';
        ctx.font = `${Math.round(9 * window.devicePixelRatio)}px "JetBrains Mono", monospace`;
        ctx.textAlign = 'right';
        ctx.textBaseline = 'middle';

        // dB Grid lines
        const minDB = -90;
        const maxDB = -30;
        const dbSteps = [-80, -70, -60, -50, -40];
        
        dbSteps.forEach(db => {
            const y = paddingTop + graphH * (1 - (db - minDB) / (maxDB - minDB));
            ctx.beginPath();
            ctx.moveTo(paddingLeft, y);
            ctx.lineTo(w - paddingRight, y);
            ctx.stroke();
            ctx.fillText(db + ' dB', paddingLeft - 8 * window.devicePixelRatio, y);
        });

        // Frequency markers
        const numFreqSteps = 5;
        ctx.textAlign = 'center';
        ctx.textBaseline = 'top';
        for (let i = 0; i < numFreqSteps; i++) {
            const frac = i / (numFreqSteps - 1);
            const freq = low + frac * (high - low);
            const x = paddingLeft + frac * graphW;
            const mhz = (freq / 1000000).toFixed(1);
            
            ctx.beginPath();
            ctx.moveTo(x, paddingTop);
            ctx.lineTo(x, h - paddingBottom);
            ctx.stroke();
            
            ctx.fillText(mhz + ' MHz', x, h - paddingBottom + 6 * window.devicePixelRatio);
        }

        // Draw B1I and L1 markers
        const b1iFreq = 1561098000;
        const l1Freq = 1575420000;

        const drawMarker = (freq, label, color) => {
            if (freq >= low && freq <= high) {
                const frac = (freq - low) / (high - low);
                const x = paddingLeft + frac * graphW;
                ctx.strokeStyle = color;
                ctx.lineWidth = 1 * window.devicePixelRatio;
                ctx.setLineDash([4 * window.devicePixelRatio, 4 * window.devicePixelRatio]);
                ctx.beginPath();
                ctx.moveTo(x, paddingTop);
                ctx.lineTo(x, h - paddingBottom);
                ctx.stroke();
                ctx.setLineDash([]);

                ctx.fillStyle = color;
                ctx.font = `bold ${Math.round(8 * window.devicePixelRatio)}px "Montserrat", sans-serif`;
                ctx.textAlign = 'center';
                ctx.fillText(label, x, paddingTop - 12 * window.devicePixelRatio);
            }
        };

        drawMarker(b1iFreq, 'BDS B1I', '#10b981');
        drawMarker(l1Freq, 'GPS L1 / BDS B1C', '#00f2fe');

        // Plot spectrum line
        if (dbs && dbs.length > 0) {
            ctx.beginPath();
            ctx.lineWidth = 2 * window.devicePixelRatio;
            
            ctx.shadowBlur = 6 * window.devicePixelRatio;
            ctx.shadowColor = '#00f2fe';

            const grad = ctx.createLinearGradient(paddingLeft, 0, w - paddingRight, 0);
            grad.addColorStop(0, '#4facfe');
            grad.addColorStop(1, '#00f2fe');
            ctx.strokeStyle = grad;

            for (let i = 0; i < dbs.length; i++) {
                const frac = i / (dbs.length - 1);
                const x = paddingLeft + frac * graphW;
                const db = Math.max(minDB, Math.min(maxDB, dbs[i]));
                const y = paddingTop + graphH * (1 - (db - minDB) / (maxDB - minDB));

                if (i === 0) {
                    ctx.moveTo(x, y);
                } else {
                    ctx.lineTo(x, y);
                }
            }
            ctx.stroke();
            ctx.shadowBlur = 0; // reset shadow
        }
    }

    function drawWaterfall(dbs) {
        const canvas = document.getElementById('canvas-waterfall');
        if (!canvas) return;
        const ctx = canvas.getContext('2d');
        const w = canvas.width;
        const h = canvas.height;

        const paddingLeft = 45 * window.devicePixelRatio;
        const paddingRight = 15 * window.devicePixelRatio;
        const paddingTop = 2 * window.devicePixelRatio;
        const paddingBottom = 2 * window.devicePixelRatio;

        const graphW = w - paddingLeft - paddingRight;
        const graphH = h - paddingTop - paddingBottom;

        // Push new sweep line to history
        waterfallLines.unshift(dbs);
        if (waterfallLines.length > maxWaterfallLines) {
            waterfallLines.pop();
        }

        ctx.fillStyle = '#020306';
        ctx.fillRect(0, 0, w, h);

        const rowH = graphH / maxWaterfallLines;
        
        for (let r = 0; r < waterfallLines.length; r++) {
            const line = waterfallLines[r];
            const y = paddingTop + r * rowH;

            const cellW = graphW / line.length;
            
            for (let i = 0; i < line.length; i++) {
                const db = line[i];
                const x = paddingLeft + i * cellW;

                // Color mapping: thermal gradient (low: navy, mid: green, high: red/yellow)
                let color;
                const normalized = Math.max(0, Math.min(1, (db + 80) / 45));

                if (normalized < 0.3) {
                    const ratio = normalized / 0.3;
                    const red = Math.round(2 * ratio);
                    const green = Math.round(15 * ratio + 3);
                    const blue = Math.round(80 + 175 * ratio);
                    color = `rgb(${red},${green},${blue})`;
                } else if (normalized < 0.6) {
                    const ratio = (normalized - 0.3) / 0.3;
                    const red = 2;
                    const green = Math.round(18 + 160 * ratio);
                    const blue = Math.round(255 - 200 * ratio);
                    color = `rgb(${red},${green},${blue})`;
                } else if (normalized < 0.85) {
                    const ratio = (normalized - 0.6) / 0.25;
                    const red = Math.round(16 + 220 * ratio);
                    const green = Math.round(178 + 40 * ratio);
                    const blue = Math.round(55 - 45 * ratio);
                    color = `rgb(${red},${green},${blue})`;
                } else {
                    const ratio = (normalized - 0.85) / 0.15;
                    const red = 255;
                    const green = Math.round(218 - 180 * ratio);
                    const blue = 10;
                    color = `rgb(${red},${green},${blue})`;
                }

                ctx.fillStyle = color;
                ctx.fillRect(x, y, cellW + 1, rowH + 1);
            }
        }

        // Draw scale axis
        ctx.strokeStyle = 'rgba(255, 255, 255, 0.05)';
        ctx.lineWidth = 1 * window.devicePixelRatio;
        ctx.beginPath();
        ctx.moveTo(paddingLeft, paddingTop);
        ctx.lineTo(paddingLeft, h - paddingBottom);
        ctx.stroke();
    }

    function drawSkyplot(sats) {
        const canvas = document.getElementById('canvas-skyplot');
        if (!canvas) return;
        const ctx = canvas.getContext('2d');
        const w = canvas.width;
        const h = canvas.height;

        ctx.clearRect(0, 0, w, h);
        ctx.fillStyle = 'rgba(2, 3, 6, 0.5)';
        ctx.fillRect(0, 0, w, h);

        const centerX = w / 2;
        const centerY = h / 2;
        const radius = Math.min(centerX, centerY) - 16 * window.devicePixelRatio;

        // Draw concentric circles
        ctx.strokeStyle = 'rgba(255, 255, 255, 0.035)';
        ctx.lineWidth = 1 * window.devicePixelRatio;
        
        const rings = [30, 60, 90];
        rings.forEach(elev => {
            const r = radius * (1 - elev / 90);
            ctx.beginPath();
            ctx.arc(centerX, centerY, r, 0, 2 * Math.PI);
            ctx.stroke();

            if (elev < 90) {
                ctx.fillStyle = 'rgba(255, 255, 255, 0.12)';
                ctx.font = `${Math.round(8 * window.devicePixelRatio)}px "JetBrains Mono", monospace`;
                ctx.fillText(elev + '°', centerX + 4 * window.devicePixelRatio, centerY - r - 2 * window.devicePixelRatio);
            }
        });

        // Direction grid lines
        ctx.beginPath();
        ctx.moveTo(centerX, centerY - radius);
        ctx.lineTo(centerX, centerY + radius);
        ctx.moveTo(centerX - radius, centerY);
        ctx.lineTo(centerX + radius, centerY);
        ctx.stroke();

        // Direction Labels
        ctx.fillStyle = 'rgba(255, 255, 255, 0.4)';
        ctx.font = `bold ${Math.round(9 * window.devicePixelRatio)}px "Montserrat", sans-serif`;
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        
        ctx.fillText('N', centerX, centerY - radius - 8 * window.devicePixelRatio);
        ctx.fillText('S', centerX, centerY + radius + 8 * window.devicePixelRatio);
        ctx.fillText('E', centerX + radius + 8 * window.devicePixelRatio, centerY);
        ctx.fillText('W', centerX - radius - 8 * window.devicePixelRatio, centerY);

        // Render satellites
        if (sats && sats.length > 0) {
            sats.forEach(sat => {
                const r = radius * (1 - sat.elevation / 90);
                const angle = (sat.azimuth * Math.PI) / 180 - Math.PI / 2;
                const x = centerX + r * Math.cos(angle);
                const y = centerY + r * Math.sin(angle);

                const isBds = sat.system === 'beidou';
                const baseColor = isBds ? '#10b981' : '#00f2fe';

                if (sat.used) {
                    ctx.shadowBlur = 8 * window.devicePixelRatio;
                    ctx.shadowColor = baseColor;
                    ctx.fillStyle = baseColor;
                    ctx.beginPath();
                    ctx.arc(x, y, 7 * window.devicePixelRatio, 0, 2 * Math.PI);
                    ctx.fill();
                    ctx.shadowBlur = 0; // reset
                } else {
                    ctx.strokeStyle = baseColor;
                    ctx.lineWidth = 1.5 * window.devicePixelRatio;
                    ctx.fillStyle = 'rgba(13, 16, 27, 0.7)';
                    ctx.beginPath();
                    ctx.arc(x, y, 6 * window.devicePixelRatio, 0, 2 * Math.PI);
                    ctx.fill();
                    ctx.stroke();
                }

                ctx.fillStyle = sat.used ? '#06070b' : '#f3f4f6';
                ctx.font = `bold ${Math.round(8 * window.devicePixelRatio)}px "JetBrains Mono", monospace`;
                ctx.textAlign = 'center';
                ctx.textBaseline = 'middle';
                
                const prnNum = sat.prn.substring(1);
                if (sat.used) {
                    ctx.fillText(prnNum, x, y);
                } else {
                    ctx.fillStyle = 'rgba(243, 244, 246, 0.55)';
                    ctx.fillText(sat.prn, x, y + 11 * window.devicePixelRatio);
                }
            });
        }
    }

    function updateTelemetryBoard(data) {
        const rxFixType = document.getElementById('rx-fix-type');
        const rxAccuracy = document.getElementById('rx-accuracy');
        const rxCoords = document.getElementById('rx-coords');
        const rxAltitude = document.getElementById('rx-altitude');
        const rxTime = document.getElementById('rx-time');
        const rxTtff = document.getElementById('rx-ttff');
        const rxSatsUsed = document.getElementById('rx-sats-used');
        const rxSatsInView = document.getElementById('rx-sats-inview');
        const badgeGps = document.getElementById('badge-gps');
        const badgeBds = document.getElementById('badge-bds');

        if (!rxFixType) return;

        rxFixType.textContent = data.fix_type.toUpperCase() + (data.fix_type === 'Searching' ? '...' : '');
        rxFixType.className = 'telemetry-value';
        if (data.fix_type === 'Searching') {
            rxFixType.classList.add('text-searching');
        } else if (data.fix_type === '2D Fix') {
            rxFixType.classList.add('text-2dfix');
        } else if (data.fix_type === '3D Fix') {
            rxFixType.classList.add('text-3dfix');
        }

        if (data.fix_type !== 'Searching') {
            rxCoords.textContent = `${data.lat.toFixed(6)} , ${data.lng.toFixed(6)}`;
            rxAltitude.textContent = `海拔: ${data.alt.toFixed(1)} m`;
            rxAccuracy.textContent = `精度: ±${data.accuracy.toFixed(1)}m`;
        } else {
            rxCoords.textContent = '-- , --';
            rxAltitude.textContent = '海拔: --';
            rxAccuracy.textContent = '精度: --';
        }

        if (data.utc_time) {
            const parts = data.utc_time.split('T');
            if (parts.length > 1) {
                rxTime.textContent = parts[1].replace('Z', '');
            } else {
                rxTime.textContent = data.utc_time;
            }
        }
        rxTtff.textContent = `TTFF (冷启动): ${data.ttff.toFixed(1)} s`;

        rxSatsUsed.textContent = data.sats_used;
        rxSatsInView.textContent = data.sats_in_view;

        let hasGps = false;
        let hasBds = false;
        if (data.satellites && data.satellites.length > 0) {
            data.satellites.forEach(s => {
                if (s.used) {
                    if (s.system === 'gps') hasGps = true;
                    if (s.system === 'beidou') hasBds = true;
                }
            });
        }

        if (hasGps) {
            badgeGps.className = "sat-badge gps-badge active";
        } else {
            badgeGps.className = "sat-badge gps-badge inactive";
        }

        if (hasBds) {
            badgeBds.className = "sat-badge bds-badge active";
        } else {
            badgeBds.className = "sat-badge bds-badge inactive";
        }
    }

    function updateSNRBars(sats) {
        const container = document.getElementById('snr-bars-container');
        if (!container) return;

        if (!sats || sats.length === 0) {
            container.innerHTML = '<div class="snr-placeholder">等待接收解算数据...</div>';
            return;
        }

        const sorted = [...sats].sort((a, b) => a.prn.localeCompare(b.prn));
        
        container.innerHTML = '';
        sorted.forEach(sat => {
            const row = document.createElement('div');
            row.className = 'snr-bar-row';

            const prnDiv = document.createElement('div');
            prnDiv.className = `snr-bar-prn ${sat.system}`;
            prnDiv.textContent = sat.prn;

            const barOuter = document.createElement('div');
            barOuter.className = 'snr-bar-outer';

            const barInner = document.createElement('div');
            barInner.className = `snr-bar-inner ${sat.system}`;
            if (!sat.used) {
                barInner.classList.add('unused');
            }
            const percent = Math.max(0, Math.min(100, ((sat.snr - 10) / 45) * 100));
            barInner.style.width = `${percent}%`;

            barOuter.appendChild(barInner);

            const valDiv = document.createElement('div');
            valDiv.className = 'snr-bar-val';
            valDiv.textContent = Math.round(sat.snr);

            row.appendChild(prnDiv);
            row.appendChild(barOuter);
            row.appendChild(valDiv);

            container.appendChild(row);
        });
    }

    async function startReceiverScan() {
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
                headers: { 'Content-Type': 'application/json' },
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

    async function stopReceiverScan() {
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

    function resetReceiverUI() {
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

        waterfallLines = [];
    }

    function connectReceiverSSE() {
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

        elements.selectSystem.addEventListener('change', updateBandOptions);
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

        // ---------------------------------------------------------
        // SECURITY TOKEN MODAL EVENT LISTENERS
        // ---------------------------------------------------------

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
                // Show "eye-off" icon (password is now visible)
                svg.innerHTML = '<path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24"></path><line x1="1" y1="1" x2="23" y2="23"></line>';
            } else {
                // Show "eye" icon (password is now hidden)
                svg.innerHTML = '<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"></path><circle cx="12" cy="12" r="3"></circle>';
            }
        });
    }
});
