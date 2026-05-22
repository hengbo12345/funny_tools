package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed static/*
var staticFS embed.FS

// SimStatus represents the state of the simulation
type SimStatus string

const (
	StatusIdle         SimStatus = "idle"
	StatusGenerating   SimStatus = "generating"
	StatusTransmitting SimStatus = "transmitting"
	StatusError        SimStatus = "error"
)

// ActiveSim holds information about the currently running simulation
type ActiveSim struct {
	Lat       float64   `json:"lat"`
	Lng       float64   `json:"lng"`
	Alt       float64   `json:"alt"`
	Duration  int       `json:"duration"`
	Gain      int       `json:"gain"`
	Ephemeris string    `json:"ephemeris"`
	StartTime time.Time `json:"start_time"`
}

// SystemStatus holds overall backend system and hardware status
type SystemStatus struct {
	TokenRequired   bool       `json:"token_required"`
	Status          SimStatus  `json:"status"`
	HackrfConnected bool       `json:"hackrf_connected"`
	TcxoStatus      string     `json:"tcxo_status"`
	ActiveSim       *ActiveSim `json:"active_sim"`
	EphemerisFiles  []string   `json:"ephemeris_files"`
	GpsSimExists    bool       `json:"gps_sim_exists"`
}

// Global variables for CLI flags and state
var (
	hostFlag  string
	portFlag  int
	tokenFlag string

	stateMutex      sync.RWMutex
	currentStatus   = StatusIdle
	activeSimInfo   *ActiveSim
	activeCmd       *exec.Cmd
	recentErrorMsg  string
	
	// Channels for streaming logs
	logBroker = NewLogBroker()
)

// checkToken validates if the request has the correct token
func checkToken(r *http.Request) bool {
	if tokenFlag == "" {
		return true // token not required
	}

	// 1. Check Authorization Header
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		// Accept both "Bearer <token>" and direct "<token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			if parts[1] == tokenFlag {
				return true
			}
		} else if authHeader == tokenFlag {
			return true
		}
	}

	// 2. Check Custom Header
	customHeader := r.Header.Get("X-GPS-Token")
	if customHeader == tokenFlag {
		return true
	}

	// 3. Check URL Query parameter (needed for EventSource SSE logs)
	queryToken := r.URL.Query().Get("token")
	if queryToken == tokenFlag {
		return true
	}

	return false
}

// withAuth is a middleware wrapper for authenticated API routes
func withAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !checkToken(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Unauthorized: Invalid or missing token",
			})
			return
		}
		handler(w, r)
	}
}

// LogBroker implements simple Pub-Sub for streaming stdout/stderr to frontend via SSE
type LogBroker struct {
	clients map[chan string]bool
	mutex   sync.Mutex
}

func NewLogBroker() *LogBroker {
	return &LogBroker{
		clients: make(map[chan string]bool),
	}
}

func (b *LogBroker) Subscribe() chan string {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	ch := make(chan string, 100)
	b.clients[ch] = true
	return ch
}

func (b *LogBroker) Unsubscribe(ch chan string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	delete(b.clients, ch)
	close(ch)
}

func (b *LogBroker) Broadcast(msg string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	// Format log message with timestamp
	formatted := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)
	for ch := range b.clients {
		select {
		case ch <- formatted:
		default:
			// Client channel full, skip to avoid blocking
		}
	}
}

func main() {
	// 1. Setup CLI flags
	flag.StringVar(&hostFlag, "host", "0.0.0.0", "Web server listening host IP")
	flag.IntVar(&portFlag, "port", 8080, "Web server listening port")
	flag.StringVar(&tokenFlag, "token", "", "Access authentication token (empty to disable)")

	flag.Usage = func() {
		fmt.Printf("==================================================\n")
		fmt.Printf("      GPS Signal Simulator Web Console Help\n")
		fmt.Printf("==================================================\n\n")
		fmt.Printf("Usage: ./gps-simulator [options]\n\n")
		fmt.Printf("Options:\n")
		fmt.Printf("  -host string\n")
		fmt.Printf("    	Web server listening host IP (default \"0.0.0.0\")\n")
		fmt.Printf("  -port int\n")
		fmt.Printf("    	Web server listening port (default 8080)\n")
		fmt.Printf("  -token string\n")
		fmt.Printf("    	Access authentication token (empty to disable)\n")
		fmt.Printf("  -h, --help\n")
		fmt.Printf("    	Display this customized help message\n\n")
		fmt.Printf("Examples:\n")
		fmt.Printf("  Run on LAN with port 9000:\n")
		fmt.Printf("    ./gps-simulator -host 0.0.0.0 -port 9000\n\n")
		fmt.Printf("  Enable secure token authentication:\n")
		fmt.Printf("    ./gps-simulator -token Secure123!\n\n")
	}

	flag.Parse()

	// Create directories if they don't exist
	if err := os.MkdirAll("data/ephemeris", 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	// API Routes (wrapped in withAuth, except for /api/status which handles its own check)
	http.HandleFunc("/api/status", handleStatus)
	http.HandleFunc("/api/simulation/start", withAuth(handleStart))
	http.HandleFunc("/api/simulation/stop", withAuth(handleStop))
	http.HandleFunc("/api/simulation/logs", withAuth(handleLogsSSE))
	http.HandleFunc("/api/ephemeris/download", withAuth(handleDownloadEphemeris))
	http.HandleFunc("/api/ephemeris/upload", withAuth(handleUploadEphemeris))
	http.HandleFunc("/api/system/setup", withAuth(handleSystemSetup))

	// Embedded Static File Server
	// staticFS has prefix "static", so we need to strip it to map / to static/index.html
	fileServer := http.FileServer(http.FS(staticFS))
	http.Handle("/", http.StripPrefix("/", fileServer))

	// Start server
	addr := fmt.Sprintf("%s:%d", hostFlag, portFlag)
	
	fmt.Printf("==================================================\n")
	fmt.Printf("   GPS Signal Simulator Web Console Loaded!\n")
	fmt.Printf("   Server running on: http://%s\n", addr)
	if tokenFlag != "" {
		fmt.Printf("   Security Token:   ENABLED [🔐 %s]\n", tokenFlag)
	} else {
		fmt.Printf("   Security Token:   DISABLED [🔓 Open Access]\n")
	}
	fmt.Printf("==================================================\n")
	
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// -------------------------------------------------------------
// HELPER FUNCTIONS FOR HARDWARE & BINARY CHECKS
// -------------------------------------------------------------

// findGpsSdrSim searches for gps-sdr-sim executable
func findGpsSdrSim() string {
	paths := []string{
		"./gps-sdr-sim",
		"./bin/gps-sdr-sim",
		"/usr/local/bin/gps-sdr-sim",
		"/usr/bin/gps-sdr-sim",
	}
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	// Check standard PATH
	if p, err := exec.LookPath("gps-sdr-sim"); err == nil {
		return p
	}
	return ""
}

// checkHackRFConnected runs hackrf_info to detect device
func checkHackRFConnected() bool {
	cmd := exec.Command("hackrf_info")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	err := cmd.Run()
	
	if err != nil {
		return false
	}
	// If "Found HackRF" is in output, it is connected
	return strings.Contains(stdout.String(), "Found HackRF")
}

// checkTcxoStatus runs hackrf_clock -i
func checkTcxoStatus(isTransmitting bool) string {
	if !checkHackRFConnected() {
		return "未连接 HackRF"
	}
	
	cmd := exec.Command("hackrf_clock", "-i")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	err := cmd.Run()
	
	if err != nil {
		return "检测失败 (未安装 hackrf_clock)"
	}
	
	output := stdout.String()
	if strings.Contains(output, "CLKIN status: clock signal detected") {
		return "已锁定 (10MHz TCXO)"
	} else if strings.Contains(output, "CLKIN status: no clock signal detected") {
		if !isTransmitting {
			return "未检测到 (提示：部分设备在发射启动后才会激活时钟检测)"
		}
		return "未检测到 (内部晶振)"
	}
	return "未知状态"
}

// listEphemerisFiles reads data/ephemeris/ directory
func listEphemerisFiles() []string {
	var files []string
	entries, err := os.ReadDir("data/ephemeris")
	if err != nil {
		return files
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}
	return files
}

// -------------------------------------------------------------
// API HANDLERS
// -------------------------------------------------------------

func handleStatus(w http.ResponseWriter, r *http.Request) {
	tokenRequired := tokenFlag != ""
	authorized := checkToken(r)

	w.Header().Set("Content-Type", "application/json")

	if tokenRequired && !authorized {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"token_required": true,
		})
		return
	}

	stateMutex.RLock()
	status := currentStatus
	active := activeSimInfo
	stateMutex.RUnlock()

	connected := checkHackRFConnected()
	isTx := status == StatusTransmitting
	tcxo := checkTcxoStatus(isTx)
	ephemerisList := listEphemerisFiles()
	gpsSimPath := findGpsSdrSim()

	sysStatus := SystemStatus{
		TokenRequired:   tokenRequired,
		Status:          status,
		HackrfConnected: connected,
		TcxoStatus:      tcxo,
		ActiveSim:       active,
		EphemerisFiles:  ephemerisList,
		GpsSimExists:    gpsSimPath != "",
	}

	json.NewEncoder(w).Encode(sysStatus)
}

func handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stateMutex.Lock()
	if currentStatus == StatusGenerating || currentStatus == StatusTransmitting {
		stateMutex.Unlock()
		http.Error(w, "Simulation is already running", http.StatusBadRequest)
		return
	}
	stateMutex.Unlock()

	// Parse inputs
	var req struct {
		Lat       float64 `json:"lat"`
		Lng       float64 `json:"lng"`
		Alt       float64 `json:"alt"`
		Duration  int     `json:"duration"`
		Gain      int     `json:"gain"`
		Ephemeris string  `json:"ephemeris"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request parameters", http.StatusBadRequest)
		return
	}

	// Validate inputs
	if req.Lat < -90 || req.Lat > 90 || req.Lng < -180 || req.Lng > 180 {
		http.Error(w, "Coordinates out of range", http.StatusBadRequest)
		return
	}
	if req.Duration <= 0 || req.Duration > 3600 {
		req.Duration = 300 // default 5 minutes
	}
	if req.Gain < 0 || req.Gain > 47 {
		req.Gain = 0 // default minimum gain
	}
	if req.Ephemeris == "" {
		// Use first available ephemeris
		files := listEphemerisFiles()
		if len(files) == 0 {
			http.Error(w, "No GPS ephemeris file available. Please download or upload one first.", http.StatusBadRequest)
			return
		}
		req.Ephemeris = files[0]
	}

	gpsSimPath := findGpsSdrSim()
	if gpsSimPath == "" {
		http.Error(w, "gps-sdr-sim executable not found. Please click 'Install Dependencies' or compile it.", http.StatusInternalServerError)
		return
	}

	// Setup active simulation state
	stateMutex.Lock()
	currentStatus = StatusGenerating
	activeSimInfo = &ActiveSim{
		Lat:       req.Lat,
		Lng:       req.Lng,
		Alt:       req.Alt,
		Duration:  req.Duration,
		Gain:      req.Gain,
		Ephemeris: req.Ephemeris,
		StartTime: time.Now(),
	}
	recentErrorMsg = ""
	stateMutex.Unlock()

	logBroker.Broadcast("=================== 启动 GPS 模拟任务 ===================")
	logBroker.Broadcast(fmt.Sprintf("位置: 纬度=%f, 经度=%f, 海拔=%f米", req.Lat, req.Lng, req.Alt))
	logBroker.Broadcast(fmt.Sprintf("时长: %d 秒 | 发射增益: %d dB", req.Duration, req.Gain))
	logBroker.Broadcast(fmt.Sprintf("所用星历: %s", req.Ephemeris))

	// Run simulation in background goroutine
	go runSimulationFlow(gpsSimPath, req.Lat, req.Lng, req.Alt, req.Duration, req.Gain, req.Ephemeris)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message": "Simulation started successfully"}`))
}

func handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stateMutex.Lock()
	defer stateMutex.Unlock()

	logBroker.Broadcast("正在收到停止指令，正在终止当前进程...")

	if activeCmd != nil && activeCmd.Process != nil {
		// Kill the process group or process
		err := activeCmd.Process.Kill()
		if err != nil {
			logBroker.Broadcast(fmt.Sprintf("停止进程失败: %v", err))
		} else {
			logBroker.Broadcast("成功终止运行中的进程。")
		}
		activeCmd = nil
	}

	currentStatus = StatusIdle
	activeSimInfo = nil

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message": "Simulation stopped successfully"}`))
}

// handleLogsSSE sends console output to Web UI in real-time
func handleLogsSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	logChan := logBroker.Subscribe()
	defer logBroker.Unsubscribe(logChan)

	// Send initial message to establish connection
	fmt.Fprintf(w, "data: %s\n\n", "已成功连接到 GPS 模拟日志中心。")
	w.(http.Flusher).Flush()

	notify := r.Context().Done()

	for {
		select {
		case <-notify:
			return
		case msg, ok := <-logChan:
			if !ok {
				return
			}
			// Escape any newlines in message
			escapedMsg := strings.ReplaceAll(msg, "\n", " ")
			fmt.Fprintf(w, "data: %s\n\n", escapedMsg)
			w.(http.Flusher).Flush()
		}
	}
}

func handleDownloadEphemeris(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	go triggerEphemerisDownload()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message": "Ephemeris download triggered in background"}`))
}

func handleUploadEphemeris(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Limit request size to 10MB
	r.ParseMultipartForm(10 << 20)

	file, handler, err := r.FormFile("ephemeris")
	if err != nil {
		http.Error(w, "Error retrieving file from form", http.StatusBadRequest)
		return
	}
	defer file.Close()

	filename := handler.Filename
	// Clean filename to prevent path traversal
	baseName := filepath.Base(filename)
	
	// Check extension
	if !strings.HasSuffix(baseName, ".n") && !strings.HasSuffix(baseName, ".brdc") && !strings.HasSuffix(baseName, "n") {
		http.Error(w, "Invalid ephemeris file format. Must be an N-file (.n, .brdc)", http.StatusBadRequest)
		return
	}

	targetPath := filepath.Join("data/ephemeris", baseName)
	dst, err := os.Create(targetPath)
	if err != nil {
		http.Error(w, "Unable to create local file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		http.Error(w, "Error saving file", http.StatusInternalServerError)
		return
	}

	logBroker.Broadcast(fmt.Sprintf("星历文件上传成功: %s", baseName))
	
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`{"message": "File uploaded successfully as %s"}`, baseName)))
}

func handleSystemSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	go triggerSystemSetup()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message": "Dependency setup triggered in background"}`))
}

// -------------------------------------------------------------
// CORE BUSINESS LOGIC (BACKGROUND WORKERS)
// -------------------------------------------------------------

func runSimulationFlow(gpsSimPath string, lat, lng, alt float64, duration, gain int, ephemeris string) {
	ephemerisPath := filepath.Join("data/ephemeris", ephemeris)
	binPath := "data/gps.bin"

	// Delete old file if exists
	os.Remove(binPath)

	// Step 1: Execute gps-sdr-sim to generate binary
	logBroker.Broadcast("---------------- [1/2] 正在生成 GPS 基带信号 ----------------")
	
	latStr := fmt.Sprintf("%f", lat)
	lngStr := fmt.Sprintf("%f", lng)
	altStr := fmt.Sprintf("%f", alt)
	durStr := fmt.Sprintf("%d", duration)

	// Command: gps-sdr-sim -b 8 -e <ephemeris> -l <lat,lng,alt> -d <dur> -o <output>
	cmdArgs := []string{
		"-b", "8",
		"-e", ephemerisPath,
		"-l", fmt.Sprintf("%s,%s,%s", latStr, lngStr, altStr),
		"-d", durStr,
		"-o", binPath,
	}

	logBroker.Broadcast(fmt.Sprintf("执行命令: %s %s", gpsSimPath, strings.Join(cmdArgs, " ")))
	
	cmd := exec.Command(gpsSimPath, cmdArgs...)
	
	stateMutex.Lock()
	activeCmd = cmd
	stateMutex.Unlock()

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		handleFlowError(fmt.Sprintf("无法获取基带生成器的输出管道: %v", err))
		return
	}
	cmd.Stderr = cmd.Stdout // Redirect stderr to stdout

	if err := cmd.Start(); err != nil {
		handleFlowError(fmt.Sprintf("启动基带生成器失败: %v", err))
		return
	}

	// Stream generator output line-by-line
	scanner := bufio.NewScanner(stdoutPipe)
	for scanner.Scan() {
		text := scanner.Text()
		// Filter progress or important logs to avoid overwhelming the log
		if strings.Contains(text, "Time index") || strings.Contains(text, "Processed") || strings.Contains(text, "%") || len(text) > 0 {
			logBroker.Broadcast(text)
		}
	}

	if err := cmd.Wait(); err != nil {
		// If killed intentionally, don't set error
		stateMutex.RLock()
		isIdle := currentStatus == StatusIdle
		stateMutex.RUnlock()
		if isIdle {
			logBroker.Broadcast("基带生成进程已被手动停止。")
			return
		}
		handleFlowError(fmt.Sprintf("基带生成失败 (进程退出异常): %v", err))
		return
	}

	// Check if output file exists and is not empty
	stat, err := os.Stat(binPath)
	if err != nil || stat.Size() == 0 {
		handleFlowError("生成的 gps.bin 信号文件不存在或为空！基带生成阶段失败。")
		return
	}

	logBroker.Broadcast(fmt.Sprintf("基带信号生成完毕！大小: %.2f MB", float64(stat.Size())/(1024*1024)))

	// Check if HackRF is connected
	if !checkHackRFConnected() {
		handleFlowError("未检测到连接的 HackRF One 设备，无法开始发射！请连接设备并检查。")
		return
	}

	// Step 2: Execute hackrf_transfer to broadcast signal
	logBroker.Broadcast("---------------- [2/2] 正在通过 HackRF One 发射信号 ----------------")
	
	stateMutex.Lock()
	currentStatus = StatusTransmitting
	stateMutex.Unlock()

	// Command: hackrf_transfer -t data/gps.bin -f 1575420000 -s 2600000 -a 1 -x <gain>
	txArgs := []string{
		"-t", binPath,
		"-f", "1575420000", // GPS L1 frequency
		"-s", "2600000",    // 2.6 MHz sample rate
		"-a", "1",          // Enable amplifier
		"-x", strconv.Itoa(gain),
	}

	logBroker.Broadcast(fmt.Sprintf("执行命令: hackrf_transfer %s", strings.Join(txArgs, " ")))
	
	cmdTx := exec.Command("hackrf_transfer", txArgs...)
	
	stateMutex.Lock()
	activeCmd = cmdTx
	stateMutex.Unlock()

	stdoutTx, err := cmdTx.StdoutPipe()
	if err != nil {
		handleFlowError(fmt.Sprintf("无法获取发射器输出管道: %v", err))
		return
	}
	cmdTx.Stderr = cmdTx.Stdout

	if err := cmdTx.Start(); err != nil {
		handleFlowError(fmt.Sprintf("启动 hackrf_transfer 失败: %v", err))
		return
	}

	// Stream transfer logs
	scannerTx := bufio.NewScanner(stdoutTx)
	for scannerTx.Scan() {
		text := scannerTx.Text()
		// Only broadcast periodic status updates
		if strings.Contains(text, "MiB/s") || strings.Contains(text, "Sound") || strings.Contains(text, "Error") || strings.Contains(text, "detect") {
			logBroker.Broadcast(text)
		}
	}

	if err := cmdTx.Wait(); err != nil {
		stateMutex.RLock()
		isIdle := currentStatus == StatusIdle
		stateMutex.RUnlock()
		if isIdle {
			logBroker.Broadcast("发射进程已被手动停止。")
			return
		}
		handleFlowError(fmt.Sprintf("发射终止 (进程异常): %v", err))
		return
	}

	// Completion
	logBroker.Broadcast("=================== 模拟发射正常结束 ===================")
	logBroker.Broadcast("已成功传输完毕所有 GPS 模拟基带数据。")

	stateMutex.Lock()
	currentStatus = StatusIdle
	activeSimInfo = nil
	activeCmd = nil
	stateMutex.Unlock()

	// Clean up temporary bin file
	os.Remove(binPath)
}

func handleFlowError(msg string) {
	logBroker.Broadcast("❌ 错误: " + msg)
	stateMutex.Lock()
	currentStatus = StatusError
	recentErrorMsg = msg
	activeSimInfo = nil
	activeCmd = nil
	stateMutex.Unlock()
}

func triggerEphemerisDownload() {
	logBroker.Broadcast("正在尝试自动下载最新 GPS 每日星历文件...")

	// We'll try downloading NOAA CORS Ephemeris
	// NOAA path: https://geodesy.noaa.gov/corsdata/rinex/{YYYY}/{DDD}/brdc{DDD}0.{YY}n.gz
	now := time.Now().UTC()
	
	// Since daily ephemeris might have some delay, let's try today, and if it fails, try yesterday.
	success := false
	var err error

	for offset := 0; offset <= 2; offset++ {
		targetTime := now.AddDate(0, 0, -offset)
		year := targetTime.Year()
		yyd := targetTime.Format("06") // 2 digit year
		doy := targetTime.YearDay()
		doyStr := fmt.Sprintf("%03d", doy)

		url := fmt.Sprintf("https://geodesy.noaa.gov/corsdata/rinex/%d/%s/brdc%s0.%sn.gz", year, doyStr, doyStr, yyd)
		filename := fmt.Sprintf("brdc%s0.%sn.gz", doyStr, yyd)
		destFileName := fmt.Sprintf("brdc%s0.%sn", doyStr, yyd)
		destPath := filepath.Join("data/ephemeris", destFileName)

		// If decompressed file already exists, don't download
		if _, statErr := os.Stat(destPath); statErr == nil {
			logBroker.Broadcast(fmt.Sprintf("星历文件 %s 已存在，无需重复下载。", destFileName))
			success = true
			break
		}

		logBroker.Broadcast(fmt.Sprintf("正在尝试从 NOAA 镜像下载 (%d天前星历): %s ...", offset, url))
		
		err = downloadAndExtractGz(url, filename, destPath)
		if err == nil {
			logBroker.Broadcast(fmt.Sprintf("✅ 星历文件下载并解压成功: %s", destFileName))
			success = true
			break
		} else {
			logBroker.Broadcast(fmt.Sprintf("该日星历获取失败: %v", err))
		}
	}

	if !success {
		logBroker.Broadcast("❌ 自动下载最新星历失败！请确保您的服务器能够正常连接互联网，或使用手动上传。")
	}
}

func downloadAndExtractGz(url, zipFilename, destPath string) error {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "GPS-SDR-Sim-Console/1.0 (hackrf-toys)")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP error: %s", resp.Status)
	}

	// Decompress gzip directly on the fly
	gzipReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to open gzip stream: %w", err)
	}
	defer gzipReader.Close()

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, gzipReader)
	if err != nil {
		// Clean up partial file
		os.Remove(destPath)
		return fmt.Errorf("failed to write decompressed data: %w", err)
	}

	return nil
}

func triggerSystemSetup() {
	logBroker.Broadcast("=================== 开始配置系统环境依赖 ===================")
	
	// Check standard dependencies: gcc, git, make, libfftw3
	logBroker.Broadcast("正在检查并安装基础包依赖 (apt-get)...")
	aptCmd := exec.Command("sudo", "apt-get", "update")
	aptCmd.Stdout = os.Stdout
	aptCmd.Stderr = os.Stderr
	logBroker.Broadcast("执行: sudo apt-get update (如果需要，请在后台终端输入密码)")
	
	// Note: We won't block completely or fail if sudo fails, we'll try our best.
	_ = aptCmd.Run()

	installCmd := exec.Command("sudo", "apt-get", "install", "-y", "git", "build-essential", "libfftw3-dev", "hackrf")
	logBroker.Broadcast("执行: sudo apt-get install -y git build-essential libfftw3-dev hackrf")
	var installErr bytes.Buffer
	installCmd.Stderr = &installErr
	if err := installCmd.Run(); err != nil {
		logBroker.Broadcast(fmt.Sprintf("安装包可能需要交互或权限不足: %s. 请手动运行安装命令。", installErr.String()))
	} else {
		logBroker.Broadcast("✅ 系统包依赖已全部安装完毕。")
	}

	// Clone and compile gps-sdr-sim locally if it does not exist
	gpsSimPath := findGpsSdrSim()
	if gpsSimPath == "" {
		logBroker.Broadcast("检测到系统中未找到 gps-sdr-sim 可执行程序，正在进行本地自动下载与编译...")
		
		// Clean compile directory
		buildDir := "data/gps-sdr-sim-build"
		_ = os.RemoveAll(buildDir)

		logBroker.Broadcast("正在克隆项目: https://github.com/osqzss/gps-sdr-sim.git ...")
		cloneCmd := exec.Command("git", "clone", "--depth", "1", "https://github.com/osqzss/gps-sdr-sim.git", buildDir)
		if err := cloneCmd.Run(); err != nil {
			logBroker.Broadcast(fmt.Sprintf("❌ 克隆 gps-sdr-sim 失败: %v", err))
			return
		}

		logBroker.Broadcast("正在编译 gps-sdr-sim ...")
		makeCmd := exec.Command("make")
		makeCmd.Dir = buildDir
		var makeErr bytes.Buffer
		makeCmd.Stderr = &makeErr
		if err := makeCmd.Run(); err != nil {
			logBroker.Broadcast(fmt.Sprintf("❌ 编译失败: %s. 请检查您的 GCC 和 Make 环境。", makeErr.String()))
			return
		}

		// Copy compiled binary to current path
		srcBin := filepath.Join(buildDir, "gps-sdr-sim")
		destBin := "./gps-sdr-sim"
		
		input, err := os.Open(srcBin)
		if err != nil {
			logBroker.Broadcast(fmt.Sprintf("❌ 无法打开编译后的二进制文件: %v", err))
			return
		}
		defer input.Close()

		output, err := os.OpenFile(destBin, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
		if err != nil {
			logBroker.Broadcast(fmt.Sprintf("❌ 无法复制二进制文件: %v", err))
			return
		}
		defer output.Close()

		_, err = io.Copy(output, input)
		if err != nil {
			logBroker.Broadcast(fmt.Sprintf("❌ 复制失败: %v", err))
			return
		}

		// Clean up build directory
		_ = os.RemoveAll(buildDir)
		
		logBroker.Broadcast("✅ gps-sdr-sim 编译并部署成功！现在位于项目根目录下，您可以开始模拟了！")
	} else {
		logBroker.Broadcast(fmt.Sprintf("✅ 检测到系统中已有可用的 gps-sdr-sim (%s)，无需重新编译。", gpsSimPath))
	}
	
	logBroker.Broadcast("=================== 依赖配置环境完成 ===================")
}
