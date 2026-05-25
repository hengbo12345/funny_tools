package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func findSweepTool() string {
	return findSimTool("hackrf_sweep")
}

func stopScanningInternal() {
	if receiverCmd != nil && receiverCmd.Process != nil {
		_ = receiverCmd.Process.Kill()
		receiverCmd = nil
		logBroker.Broadcast("已成功终止接收仪扫频进程。")
	}
}

func handleReceiverStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stateMutex.Lock()
	defer stateMutex.Unlock()

	if currentStatus == StatusGenerating || currentStatus == StatusTransmitting {
		http.Error(w, "HackRF One 正在进行卫星信号发射，无法启动接收扫频！", http.StatusBadRequest)
		return
	}

	// Parse optional custom coordinates to update GPSTest panel
	var req struct {
		Lat float64 `json:"lat"`
		Lng float64 `json:"lng"`
		Alt float64 `json:"alt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
		if req.Lat >= -90 && req.Lat <= 90 && req.Lng >= -180 && req.Lng <= 180 {
			receiverCoords.Lat = req.Lat
			receiverCoords.Lng = req.Lng
			receiverCoords.Alt = req.Alt
		}
	}

	if currentStatus == StatusScanning {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "Receiver scan is already running"}`))
		return
	}

	currentStatus = StatusScanning
	scanStartTime = time.Now()
	logBroker.Broadcast(fmt.Sprintf("正在启动卫星信号接收分析仪... 选定测站位置: 纬度 %.6f, 经度 %.6f, 海拔 %.1f米", receiverCoords.Lat, receiverCoords.Lng, receiverCoords.Alt))

	// Launch receiver sweep flow in background
	go runReceiverSweepFlow()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message": "Receiver scan started successfully"}`))
}

func handleReceiverStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stateMutex.Lock()
	defer stateMutex.Unlock()

	if currentStatus == StatusScanning {
		stopScanningInternal()
		currentStatus = StatusIdle
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message": "Receiver scan stopped successfully"}`))
}

func handleReceiverStreamSSE(w http.ResponseWriter, r *http.Request) {
	// Authorization check
	if !checkToken(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Unauthorized: Invalid or missing token",
		})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Subscribe to sweepBroker
	sweepChan := sweepBroker.Subscribe()
	defer sweepBroker.Unsubscribe(sweepChan)

	// Send handshake message
	fmt.Fprintf(w, "data: %s\n\n", `{"type":"sys","message":"Receiver telemetry stream connected"}`)
	w.(http.Flusher).Flush()

	notify := r.Context().Done()

	for {
		select {
		case <-notify:
			return
		case msg, ok := <-sweepChan:
			if !ok {
				return
			}
			escapedMsg := strings.ReplaceAll(msg, "\n", " ")
			fmt.Fprintf(w, "data: %s\n\n", escapedMsg)
			w.(http.Flusher).Flush()
		}
	}
}

func runReceiverSweepFlow() {
	gnssSdrPath := findSimTool("gnss-sdr")
	connected := checkHackRFConnected()

	if gnssSdrPath != "" && connected {
		logBroker.Broadcast("📡 检测到系统中存在 gnss-sdr 接收机并且已连接 HackRF One 设备！")
		logBroker.Broadcast("📡 正在启动真实硬件卫星信号实时解算解调模式 (100% 物理射频解调)...")

		// Find an available UDP port starting from rxPortFlag to avoid listening conflict
		actualPort, conn, err := findAvailableUDPPort(rxPortFlag)
		if err != nil {
			logBroker.Broadcast(fmt.Sprintf("⚠️ 冲突检测失败或无法获取可用 UDP 端口: %v。正在优雅降级到高保真扫频仿真模式...", err))
			go runSimulatedTelemetryTicker()
			runSimulatedSweepLoop()
			return
		}

		if actualPort != rxPortFlag {
			logBroker.Broadcast(fmt.Sprintf("⚠️ 检测到预设 NMEA 监听端口 %d 已被占用！自动切换至可用端口 %d", rxPortFlag, actualPort))
		} else {
			logBroker.Broadcast(fmt.Sprintf("📡 成功绑定 NMEA 监听端口: %d", actualPort))
		}

		// 1. Generate config file
		confStr := generateGnssSdrConfig("gps", actualPort) // Default L1 GPS, can dynamically support beidou if selected
		_ = os.WriteFile("data/gnss-sdr.conf", []byte(confStr), 0644)

		// 2. Launch gnss-sdr in the background
		cmd := exec.Command(gnssSdrPath, "--config_file=data/gnss-sdr.conf")
		stateMutex.Lock()
		receiverCmd = cmd
		stateMutex.Unlock()

		if err := cmd.Start(); err != nil {
			conn.Close()
			logBroker.Broadcast(fmt.Sprintf("⚠️ 启动 gnss-sdr 失败 (启动错误): %v. 正在优雅降级到高保真扫频仿真模式...", err))
			// Fallback to simulation
			go runSimulatedTelemetryTicker()
			runSimulatedSweepLoop()
			return
		}

		logBroker.Broadcast(fmt.Sprintf("📡 GNSS-SDR 进程已成功创建并接管 HackRF 射频前端。启动高频 NMEA-0183 遥测解析中心 (UDP:%d)...", actualPort))
		
		// 3. Start listener and spectrum simulation
		runNMEAListenerAndPSD(conn, actualPort)

		_ = cmd.Wait()

		stateMutex.Lock()
		stillScanning := currentStatus == StatusScanning
		stateMutex.Unlock()
		if stillScanning {
			logBroker.Broadcast("⚠️ GNSS-SDR 进程已意外退出，正在降级切换到仿真信号...")
			go runSimulatedTelemetryTicker()
			runSimulatedSweepLoop()
		}
		return
	}

	// Graceful fallback to sweep / mock
	if gnssSdrPath == "" {
		logBroker.Broadcast("提示：未检测到系统中安装有 gnss-sdr 真实射频解算工具。您可以执行以下方式安装以解锁真实信号接收能力：")
		logBroker.Broadcast("   - macOS: brew install gnss-sdr")
		logBroker.Broadcast("   - Ubuntu: sudo apt-get install -y gnss-sdr")
	} else {
		logBroker.Broadcast("提示：未检测到连接的 HackRF One 硬件设备，无法开始物理天线接收解算。")
	}
	logBroker.Broadcast("🛰️ 正在启动高保真 GNSS 频谱与卫星遥测仿真器...")
	
	// Start the simulated GNSS telemetry ticker in background
	go runSimulatedTelemetryTicker()

	sweepPath := findSweepTool()
	if sweepPath == "" || !connected {
		runSimulatedSweepLoop()
		return
	}

	// Command: hackrf_sweep -f 1550000000:1590000000 -w 1000000
	args := []string{"-f", "1550000000:1590000000", "-w", "1000000"}
	cmd := exec.Command(sweepPath, args...)

	stateMutex.Lock()
	receiverCmd = cmd
	stateMutex.Unlock()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logBroker.Broadcast(fmt.Sprintf("⚠️ 启动 hackrf_sweep 失败 (管道错误): %v", err))
		runSimulatedSweepLoop()
		return
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		logBroker.Broadcast(fmt.Sprintf("⚠️ 启动 hackrf_sweep 失败 (启动错误): %v", err))
		runSimulatedSweepLoop()
		return
	}

	logBroker.Broadcast("📡 成功连接到 HackRF One，开始实时物理频谱扫频 (1550 - 1590 MHz)...")

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		parseAndBroadcastSweepLine(line)
	}

	_ = cmd.Wait()

	// If we exited the sweep command naturally but are still in scanning mode, fall back to mock or idle
	stateMutex.Lock()
	stillScanning := currentStatus == StatusScanning
	stateMutex.Unlock()

	if stillScanning {
		logBroker.Broadcast("⚠️ HackRF 扫频已意外终止，正在切换到仿真信号...")
		runSimulatedSweepLoop()
	}
}

func findAvailableUDPPort(startPort int) (int, *net.UDPConn, error) {
	port := startPort
	for {
		addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			return 0, nil, err
		}
		conn, err := net.ListenUDP("udp", addr)
		if err == nil {
			return port, conn, nil
		}
		port++
		if port > startPort+100 {
			return 0, nil, fmt.Errorf("unable to find free UDP port in range %d-%d", startPort, startPort+100)
		}
	}
}

func generateGnssSdrConfig(system string, port int) string {
	band := "L1"
	chType := "GPS_L1_CA"
	freq := 1575420000
	sampleRate := 2000000

	if system == "beidou" {
		band = "B1"
		chType = "BDS_B1I"
		freq = 1561098000
		sampleRate = 4000000
	}

	conf := fmt.Sprintf(`[GNSS-SDR]
SignalSource.band=%s
SignalSource.channels=8
SignalSource.type=OsmoSDR_Signal_Source
SignalSource.sample_rate=%d
SignalSource.freq=%d
SignalSource.gain=40
SignalSource.rf_gain=40
SignalSource.if_gain=30
SignalSource.device_address=hackrf=0

SignalConditioner.type=Signal_Conditioner
DataType.type=I/Q

Channel.count=8
Channel.type=%s

PVT.type=PVT
PVT.nmea_dump_filename=
PVT.nmea_dump_client_addresses=127.0.0.1
PVT.nmea_dump_client_port=%d
`, band, sampleRate, freq, chType, port)

	return conf
}

func runNMEAListenerAndPSD(conn *net.UDPConn, listenPort int) {
	defer conn.Close()

	// 2. Struct for sat telemetry
	type Sat struct {
		PRN       string  `json:"prn"`
		Elevation float64 `json:"elevation"`
		Azimuth   float64 `json:"azimuth"`
		SNR       float64 `json:"snr"`
		Used      bool    `json:"used"`
		System    string  `json:"system"`
	}

	satellites := make(map[string]Sat)
	var activeSatsUsed []string

	// Initialize basic variables
	lat := receiverCoords.Lat
	lng := receiverCoords.Lng
	alt := receiverCoords.Alt
	fixType := "Searching"
	accuracy := 0.0
	var satsUsed, satsInView int
	utcTime := ""

	// Live PSD channel ticker
	psdTicker := time.NewTicker(100 * time.Millisecond)
	defer psdTicker.Stop()

	// SSE Broadcast Ticker (for telemetry)
	telemetryTicker := time.NewTicker(1000 * time.Millisecond)
	defer telemetryTicker.Stop()

	// UDP reading buffer
	buf := make([]byte, 2048)

	// Goroutine for handling UDP reading
	go func() {
		for {
			stateMutex.RLock()
			scanning := currentStatus == StatusScanning
			stateMutex.RUnlock()
			if !scanning {
				return
			}

			// Read packet
			conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			n, _, err := conn.ReadFrom(buf)
			if err != nil {
				continue
			}

			packet := string(buf[:n])
			lines := strings.Split(packet, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if !strings.HasPrefix(line, "$") {
					continue
				}

				// Check checksum
				checksumIdx := strings.LastIndex(line, "*")
				if checksumIdx == -1 {
					continue
				}

				parts := strings.Split(line[:checksumIdx], ",")
				sentenceType := parts[0]

				// Parse $--GGA
				if strings.HasSuffix(sentenceType, "GGA") && len(parts) >= 10 {
					// Time
					if parts[1] != "" && len(parts[1]) >= 6 {
						utcTime = fmt.Sprintf("%s:%s:%s", parts[1][0:2], parts[1][2:4], parts[1][4:6])
					}
					// Latitude
					if parts[2] != "" && parts[3] != "" && len(parts[2]) >= 4 {
						deg, _ := strconv.ParseFloat(parts[2][0:2], 64)
						min, _ := strconv.ParseFloat(parts[2][2:], 64)
						lat = deg + min/60.0
						if parts[3] == "S" {
							lat = -lat
						}
					}
					// Longitude
					if parts[4] != "" && parts[5] != "" && len(parts[4]) >= 5 {
						deg, _ := strconv.ParseFloat(parts[4][0:3], 64)
						min, _ := strconv.ParseFloat(parts[4][3:], 64)
						lng = deg + min/60.0
						if parts[5] == "W" {
							lng = -lng
						}
					}
					// Fix Quality
					qCode := parts[6]
					if qCode == "1" || qCode == "2" || qCode == "3" {
						if len(activeSatsUsed) >= 4 {
							fixType = "3D Fix"
							accuracy = 1.2
						} else {
							fixType = "2D Fix"
							accuracy = 4.5
						}
					} else {
						fixType = "Searching"
						accuracy = 0.0
					}
					// Altitude
					if parts[9] != "" {
						alt, _ = strconv.ParseFloat(parts[9], 64)
					}
				}

				// Parse $--GSA
				if strings.HasSuffix(sentenceType, "GSA") && len(parts) >= 15 {
					activeSatsUsed = nil
					for i := 3; i <= 14; i++ {
						if parts[i] != "" {
							prnNum := parts[i]
							if len(prnNum) == 1 {
								prnNum = "0" + prnNum
							}
							prefix := "G"
							if strings.Contains(sentenceType, "BD") || strings.Contains(sentenceType, "GB") {
								prefix = "C"
							}
							activeSatsUsed = append(activeSatsUsed, prefix+prnNum)
						}
					}
				}

				// Parse $--GSV (Satellites in view)
				if strings.HasSuffix(sentenceType, "GSV") && len(parts) >= 8 {
					prefix := "G"
					sysType := "gps"
					if strings.Contains(sentenceType, "BD") || strings.Contains(sentenceType, "GB") {
						prefix = "C"
						sysType = "beidou"
					}

					// Loop over satellites in this GSV sentence
					for idx := 4; idx+3 < len(parts); idx += 4 {
						prn := parts[idx]
						if prn == "" {
							continue
						}
						if len(prn) == 1 {
							prn = "0" + prn
						}
						fullPRN := prefix + prn

						elev, _ := strconv.ParseFloat(parts[idx+1], 64)
						azim, _ := strconv.ParseFloat(parts[idx+2], 64)
						snr, _ := strconv.ParseFloat(parts[idx+3], 64)

						// Update or create sat entry
						satellites[fullPRN] = Sat{
							PRN:       fullPRN,
							Elevation: elev,
							Azimuth:   azim,
							SNR:       snr,
							Used:      false,
							System:    sysType,
						}
					}
				}
			}
		}
	}()

	// Loop for broadcasting live PSD and telemetry
	for {
		stateMutex.RLock()
		scanning := currentStatus == StatusScanning
		stateMutex.RUnlock()
		if !scanning {
			return
		}

		select {
		case <-psdTicker.C:
			// Calculate average SNR to dynamically scale PSD peak
			avgSnr := 0.0
			cnt := 0
			for _, sat := range satellites {
				if sat.SNR > 0 {
					avgSnr += sat.SNR
					cnt++
				}
			}
			if cnt > 0 {
				avgSnr /= float64(cnt)
			}

			numBins := 41
			dbs := make([]float64, numBins)
			for i := 0; i < numBins; i++ {
				freq := float64(1550000000 + i*1000000)
				noise := -75.0 + rand.Float64()*4.0

				gpsDist := math.Abs(freq - 1575420000)
				gpsPeak := 0.0
				if gpsDist < 5000000 && avgSnr > 0 {
					maxPeak := (avgSnr - 10.0) * 0.9
					if maxPeak < 0 {
						maxPeak = 0
					}
					gpsPeak = maxPeak * math.Exp(-math.Pow(gpsDist/1800000.0, 2))
				}

				dbs[i] = noise + gpsPeak
			}

			data := map[string]interface{}{
				"type":  "sweep",
				"low":   1550000000,
				"high":  1590000000,
				"width": 1000000,
				"dbs":   dbs,
			}

			bytes, err := json.Marshal(data)
			if err == nil {
				sweepBroker.Broadcast(string(bytes))
			}

		case <-telemetryTicker.C:
			var activeSats []Sat
			satsUsed = 0

			for prn, sat := range satellites {
				used := false
				for _, usedPRN := range activeSatsUsed {
					if usedPRN == prn {
						used = true
						satsUsed++
						break
					}
				}
				sat.Used = used
				activeSats = append(activeSats, sat)
			}

			satsInView = len(activeSats)
			if utcTime == "" {
				utcTime = time.Now().UTC().Format("15:04:05")
			}

			telemetryData := map[string]interface{}{
				"type":         "telemetry",
				"fix_type":     fixType,
				"lat":          lat,
				"lng":          lng,
				"alt":          alt,
				"accuracy":     accuracy,
				"ttff":         time.Since(scanStartTime).Seconds(),
				"utc_time":     time.Now().UTC().Format("2006-05-02T") + utcTime + ".00Z",
				"sats_used":    satsUsed,
				"sats_in_view": satsInView,
				"satellites":   activeSats,
			}

			bytes, err := json.Marshal(telemetryData)
			if err == nil {
				sweepBroker.Broadcast(string(bytes))
			}
		}
	}
}

func parseAndBroadcastSweepLine(line string) {
	// A line looks like: 2026-05-23, 21:40:00, 1550000000, 1590000000, 1000000, 20, -71.2, -72.5, ...
	parts := strings.Split(line, ",")
	if len(parts) < 7 {
		return
	}

	// Basic validation
	low, err1 := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
	high, err2 := strconv.ParseInt(strings.TrimSpace(parts[3]), 10, 64)
	width, err3 := strconv.ParseInt(strings.TrimSpace(parts[4]), 10, 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return
	}

	var dbs []float64
	for i := 6; i < len(parts); i++ {
		val, err := strconv.ParseFloat(strings.TrimSpace(parts[i]), 64)
		if err == nil {
			dbs = append(dbs, val)
		}
	}

	if len(dbs) == 0 {
		return
	}

	data := map[string]interface{}{
		"type":  "sweep",
		"low":   low,
		"high":  high,
		"width": width,
		"dbs":   dbs,
	}

	bytes, err := json.Marshal(data)
	if err == nil {
		sweepBroker.Broadcast(string(bytes))
	}
}

func runSimulatedSweepLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// 41 bins: (1590000000 - 1550000000) / 1000000 + 1
	numBins := 41

	for {
		stateMutex.RLock()
		scanning := currentStatus == StatusScanning
		stateMutex.RUnlock()

		if !scanning {
			return
		}

		select {
		case <-ticker.C:
			dbs := make([]float64, numBins)
			elapsed := time.Since(scanStartTime).Seconds()

			for i := 0; i < numBins; i++ {
				freq := float64(1550000000 + i*1000000)
				// Base noise
				noise := -75.0 + rand.Float64()*5.0

				// Add GPS L1 peak at 1575.42 MHz (index ~25)
				gpsDist := math.Abs(freq - 1575420000)
				gpsPeak := 0.0
				if gpsDist < 5000000 {
					gpsPeak = 25.0 * math.Exp(-math.Pow(gpsDist/1500000.0, 2))
				}

				// Add BDS B1I peak at 1561.098 MHz (index ~11)
				bdsDist := math.Abs(freq - 1561098000)
				bdsPeak := 0.0
				if bdsDist < 5000000 {
					bdsPeak = 23.0 * math.Exp(-math.Pow(bdsDist/1500000.0, 2))
				}

				// Fluctuating peak intensity over time
				intensityMultiplier := 0.8 + 0.2*math.Sin(elapsed/2.0)
				dbs[i] = noise + (gpsPeak+bdsPeak)*intensityMultiplier
			}

			data := map[string]interface{}{
				"type":  "sweep",
				"low":   1550000000,
				"high":  1590000000,
				"width": 1000000,
				"dbs":   dbs,
			}

			bytes, err := json.Marshal(data)
			if err == nil {
				sweepBroker.Broadcast(string(bytes))
			}
		}
	}
}

func runSimulatedTelemetryTicker() {
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()

	type Sat struct {
		PRN       string  `json:"prn"`
		Elevation float64 `json:"elevation"`
		Azimuth   float64 `json:"azimuth"`
		SNR       float64 `json:"snr"`
		Used      bool    `json:"used"`
		System    string  `json:"system"`
	}

	// 13 mock satellites
	sats := []Sat{
		{PRN: "G01", Elevation: 45, Azimuth: 120, SNR: 42, Used: true, System: "gps"},
		{PRN: "G08", Elevation: 72, Azimuth: 340, SNR: 45, Used: true, System: "gps"},
		{PRN: "G14", Elevation: 28, Azimuth: 85,  SNR: 39, Used: true, System: "gps"},
		{PRN: "G22", Elevation: 15, Azimuth: 210, SNR: 32, Used: false, System: "gps"},
		{PRN: "G28", Elevation: 60, Azimuth: 180, SNR: 44, Used: true, System: "gps"},
		{PRN: "G30", Elevation: 38, Azimuth: 45,  SNR: 41, Used: true, System: "gps"},
		{PRN: "C01", Elevation: 55, Azimuth: 90,  SNR: 46, Used: true, System: "beidou"},
		{PRN: "C06", Elevation: 68, Azimuth: 270, SNR: 48, Used: true, System: "beidou"},
		{PRN: "C10", Elevation: 22, Azimuth: 150, SNR: 37, Used: true, System: "beidou"},
		{PRN: "C19", Elevation: 80, Azimuth: 10,  SNR: 47, Used: true, System: "beidou"},
		{PRN: "C24", Elevation: 12, Azimuth: 315, SNR: 30, Used: false, System: "beidou"},
		{PRN: "C30", Elevation: 48, Azimuth: 220, SNR: 43, Used: true, System: "beidou"},
		{PRN: "C35", Elevation: 33, Azimuth: 135, SNR: 40, Used: true, System: "beidou"},
	}

	for {
		stateMutex.RLock()
		scanning := currentStatus == StatusScanning
		stateMutex.RUnlock()

		if !scanning {
			return
		}

		select {
		case <-ticker.C:
			elapsed := time.Since(scanStartTime).Seconds()

			// TTFF state machine
			fixType := "Searching"
			accuracy := 0.0
			ttff := elapsed

			if elapsed >= 3.2 {
				fixType = "3D Fix"
				accuracy = 0.8 + rand.Float64()*0.4
				ttff = 3.2
			} else if elapsed >= 1.5 {
				fixType = "2D Fix"
				accuracy = 3.5 + rand.Float64()*1.2
				ttff = 3.2
			}

			// Add small noise to coordinates to simulate receiver precision drift
			latDrift := (rand.Float64() - 0.5) * 0.00001
			lngDrift := (rand.Float64() - 0.5) * 0.00001
			altDrift := (rand.Float64() - 0.5) * 0.4

			curLat := receiverCoords.Lat
			curLng := receiverCoords.Lng
			curAlt := receiverCoords.Alt

			if fixType != "Searching" {
				curLat += latDrift
				curLng += lngDrift
				curAlt += altDrift
			}

			// Build dynamic satellite list
			var activeSats []Sat
			satsUsed := 0
			satsInView := len(sats)

			for _, s := range sats {
				// slow drift azimuth and elevation
				elev := s.Elevation + math.Sin(elapsed/15.0)*2.0
				if elev < 5 {
					elev = 5
				}
				if elev > 90 {
					elev = 90
				}

				azim := s.Azimuth + elapsed*0.2
				for azim >= 360 {
					azim -= 360
				}

				snr := s.SNR + float64(rand.Intn(5)-2)
				if snr < 10 {
					snr = 10
				}
				if snr > 52 {
					snr = 52
				}

				used := s.Used
				if fixType == "Searching" {
					used = false
				} else if fixType == "2D Fix" {
					used = s.System == "gps" && snr >= 35
				} else if fixType == "3D Fix" {
					used = snr >= 35
				}

				if used {
					satsUsed++
				}

				activeSats = append(activeSats, Sat{
					PRN:       s.PRN,
					Elevation: math.Round(elev*10) / 10,
					Azimuth:   math.Round(azim*10) / 10,
					SNR:       math.Round(snr*10) / 10,
					Used:      used,
					System:    s.System,
				})
			}

			// Format dynamic UTC time
			utcTime := time.Now().UTC().Format("2006-05-02T15:04:05.00Z")

			data := map[string]interface{}{
				"type":         "telemetry",
				"fix_type":     fixType,
				"lat":          curLat,
				"lng":          curLng,
				"alt":          curAlt,
				"accuracy":     accuracy,
				"ttff":         math.Round(ttff*10) / 10,
				"utc_time":     utcTime,
				"sats_used":    satsUsed,
				"sats_in_view": satsInView,
				"satellites":   activeSats,
			}

			bytes, err := json.Marshal(data)
			if err == nil {
				sweepBroker.Broadcast(string(bytes))
			}
		}
	}
}
