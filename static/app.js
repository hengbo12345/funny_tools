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
    // INITIALIZATION
    // -------------------------------------------------------------

    initMap();
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
        
        switch (data.status) {
            case 'idle':
                elements.simVal.textContent = "空闲 (IDLE)";
                elements.simIndicator.className = "indicator gray";
                elements.btnStart.disabled = false;
                elements.btnStop.disabled = true;
                break;
            case 'generating':
                elements.simVal.textContent = "正在生成基带... (GENERATING)";
                elements.simIndicator.className = "indicator amber pulse";
                elements.btnStart.disabled = true;
                elements.btnStop.disabled = false;
                break;
            case 'transmitting':
                elements.simVal.textContent = "正在发射GPS模拟信号... (TRANSMITTING)";
                elements.simIndicator.className = "indicator green pulse";
                elements.btnStart.disabled = true;
                elements.btnStop.disabled = false;
                break;
            case 'error':
                elements.simVal.textContent = "发生错误 (ERROR)";
                elements.simIndicator.className = "indicator red pulse";
                elements.btnStart.disabled = false;
                elements.btnStop.disabled = true;
                break;
        }

        // 4. Missing gps-sdr-sim binary warning
        if (!data.gps_sim_exists) {
            elements.gpsSimMissingAlert.classList.remove('hidden');
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
            ephemeris: elements.selectEphemeris.value
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

    async function autoDownloadEphemeris() {
        elements.btnDownloadEphemeris.disabled = true;
        addConsoleLine("[SYSTEM] 星历后台自动下载任务已触发，请查看控制台实时输出...", "system");
        
        try {
            const response = await authedFetch('/api/ephemeris/download', { method: 'POST' });
            if (!response.ok) throw new Error("下载请求提交失败");
        } catch (error) {
            addConsoleLine(`[SYSTEM] 触发星历下载失败: ${error.message}`, "error");
        } finally {
            setTimeout(() => { elements.btnDownloadEphemeris.disabled = false; }, 3000);
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
    // EVENT LISTENERS BINDING
    // -------------------------------------------------------------

    function setupEventListeners() {
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

        elements.btnDownloadEphemeris.addEventListener('click', autoDownloadEphemeris);

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
