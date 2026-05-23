package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
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
	// Start the simulated GNSS telemetry ticker in background
	go runSimulatedTelemetryTicker()

	sweepPath := findSweepTool()
	connected := checkHackRFConnected()

	if sweepPath == "" || !connected {
		if sweepPath == "" {
			logBroker.Broadcast("提示：系统未找到 hackrf_sweep 工具。")
		} else {
			logBroker.Broadcast("提示：未检测到 HackRF One 设备连接。")
		}
		logBroker.Broadcast("🛰️ 正在启动高保真 GNSS 频谱与卫星遥测仿真器...")
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
