package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stateMutex.Lock()
	if currentStatus == StatusScanning {
		logBroker.Broadcast("检测到当前正在运行接收扫频分析。由于 HackRF One 为半双工模式，正在自动终止扫频并释放射频通道...")
		stopScanningInternal()
		currentStatus = StatusIdle
	}
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
		System    string  `json:"system"`
		Band      string  `json:"band"`
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
	if req.System == "" {
		req.System = "gps"
	}
	if req.Band == "" {
		if req.System == "beidou" {
			req.Band = "B1I"
		} else {
			req.Band = "L1"
		}
	}
	if req.Ephemeris == "" {
		// Use first available ephemeris
		files := listEphemerisFiles()
		if len(files) == 0 {
			http.Error(w, "No ephemeris file available. Please download or upload one first.", http.StatusBadRequest)
			return
		}
		req.Ephemeris = files[0]
	}

	// Find the matching GNSS band
	var targetBand *GNSSBand
	for _, s := range GNSSRegistry {
		if s.ID == req.System {
			for _, b := range s.Bands {
				if b.ID == req.Band {
					targetBand = &b
					break
				}
			}
		}
	}

	if targetBand == nil {
		http.Error(w, fmt.Sprintf("Unsupported system/band combination: %s/%s", req.System, req.Band), http.StatusBadRequest)
		return
	}

	simPath := findSimTool(targetBand.ToolName)
	if simPath == "" {
		http.Error(w, fmt.Sprintf("基带信号生成器可执行程序 %s 未找到。请点击下方“一键编译环境”或手动下载部署该二进制文件。", targetBand.ToolName), http.StatusInternalServerError)
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
		System:    req.System,
		Band:      req.Band,
	}
	recentErrorMsg = ""
	stateMutex.Unlock()

	// Run simulation in background goroutine
	go runSimulationFlow(simPath, req.Lat, req.Lng, req.Alt, req.Duration, req.Gain, req.Ephemeris, req.System, req.Band)

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
	baseName := filepath.Base(filename)
	lowerName := strings.ToLower(baseName)

	ext := filepath.Ext(lowerName)
	isGz := ext == ".gz"
	isZip := ext == ".zip"

	var reader io.Reader = file

	if isGz {
		// Decompress gzip on the fly
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			http.Error(w, "Failed to open gzip stream: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer gzReader.Close()
		reader = gzReader
		baseName = strings.TrimSuffix(baseName, ext)
		lowerName = strings.ToLower(baseName)
	} else if isZip {
		// Read zip into memory buffer to allow ReaderAt random access
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, file); err != nil {
			http.Error(w, "Error reading uploaded zip file", http.StatusInternalServerError)
			return
		}
		readerAt := bytes.NewReader(buf.Bytes())
		zipReader, err := zip.NewReader(readerAt, int64(buf.Len()))
		if err != nil {
			http.Error(w, "Invalid zip archive", http.StatusBadRequest)
			return
		}

		var targetZipFile *zip.File
		for _, f := range zipReader.File {
			if f.FileInfo().IsDir() {
				continue
			}
			fName := strings.ToLower(f.Name)
			if strings.HasSuffix(fName, ".n") || strings.HasSuffix(fName, ".brdc") ||
				strings.HasSuffix(fName, ".nav") || strings.HasSuffix(fName, ".rnx") ||
				strings.HasSuffix(fName, "n") || strings.Contains(fName, "brdc") {
				targetZipFile = f
				break
			}
		}

		if targetZipFile == nil {
			http.Error(w, "No valid ephemeris file (.n, .brdc, .nav, .rnx) found inside the zip archive", http.StatusBadRequest)
			return
		}

		zipFileReadCloser, err := targetZipFile.Open()
		if err != nil {
			http.Error(w, "Error opening file inside zip: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer zipFileReadCloser.Close()
		reader = zipFileReadCloser
		baseName = filepath.Base(targetZipFile.Name)
		lowerName = strings.ToLower(baseName)
	}

	// Relaxed extension checks for final uncompressed name
	matched := false
	if strings.HasSuffix(lowerName, ".n") || strings.HasSuffix(lowerName, ".brdc") ||
		strings.HasSuffix(lowerName, ".nav") || strings.HasSuffix(lowerName, ".rnx") ||
		strings.Contains(lowerName, "brdc") || strings.HasSuffix(lowerName, "n") {
		matched = true
	}

	if !matched {
		http.Error(w, "Invalid ephemeris file format. Must be an N-file (.n, .brdc, .nav, .rnx, .[yy]n) or compressed (.gz, .zip)", http.StatusBadRequest)
		return
	}

	targetPath := filepath.Join("data/ephemeris", baseName)
	dst, err := os.Create(targetPath)
	if err != nil {
		http.Error(w, "Unable to create local file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, reader); err != nil {
		// Clean up partial file
		os.Remove(targetPath)
		http.Error(w, "Error saving file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	logBroker.Broadcast(fmt.Sprintf("星历文件上传成功并就绪: %s", baseName))

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`{"message": "File uploaded and prepared successfully as %s"}`, baseName)))
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

func runSimulationFlow(simPath string, lat, lng, alt float64, duration, gain int, ephemeris string, system string, band string) {
	ephemerisPath := filepath.Join("data/ephemeris", ephemeris)
	binPath := filepath.Clean("data/gps.bin")

	// Delete old file if exists
	os.Remove(binPath)

	// Step 1: Execute simulation tool to generate binary
	logBroker.Broadcast("---------------- [1/2] 正在生成基带仿真信号 ----------------")
	
	var cmd *exec.Cmd
	if band == "B1C" {
		presetPath := "data/b1c_preset.json"
		// Generate preset
		presetTime := parseEphemerisTime(ephemeris)
		preset := PresetConfig{
			Version:     1.0,
			Description: "BDS B1C Simulation Preset",
			Time: PresetTime{
				Type:   "UTC",
				Year:   presetTime.Year(),
				Month:  int(presetTime.Month()),
				Day:    presetTime.Day(),
				Hour:   presetTime.Hour(),
				Minute: presetTime.Minute(),
				Second: presetTime.Second(),
			},
			Trajectory: PresetTrajectory{
				Name: "Static Scenario",
				InitPosition: PresetPosition{
					Type:      "LLA",
					Format:    "d",
					Longitude: lng,
					Latitude:  lat,
					Altitude:  alt,
				},
				InitVelocity: PresetVelocity{
					Type:   "SCU",
					Speed:  0,
					Course: 0,
				},
				TrajectoryList: []TrajectoryItem{
					{
						Type: "Const",
						Time: float64(duration),
					},
				},
			},
			Ephemeris: PresetEphemeris{
				Type: "RINEX",
				Name: ephemerisPath,
			},
			Output: PresetOutput{
				Type:       "IFdata",
				Format:     "IQ8",
				SampleFreq: 2.6, // 2.6 MHz sample rate for HackRF
				CenterFreq: 1575.42,
				Name:       binPath,
				Config: OutputConfig{
					ElevationMask: 5,
				},
				SystemSelect: []SystemSelectItem{
					{System: "GPS", Signal: "L1CA", Enable: false},
					{System: "GPS", Signal: "L1C", Enable: false},
					{System: "GPS", Signal: "L2C", Enable: false},
					{System: "GPS", Signal: "L2P", Enable: false},
					{System: "GPS", Signal: "L5", Enable: false},
					{System: "BDS", Signal: "B1C", Enable: true},
					{System: "BDS", Signal: "B1I", Enable: false},
					{System: "BDS", Signal: "B2a", Enable: false},
					{System: "BDS", Signal: "B2I", Enable: false},
					{System: "BDS", Signal: "B2b", Enable: false},
					{System: "BDS", Signal: "B3I", Enable: false},
					{System: "Galileo", Signal: "E1", Enable: false},
					{System: "Galileo", Signal: "E5a", Enable: false},
					{System: "Galileo", Signal: "E5b", Enable: false},
					{System: "Galileo", Signal: "E6", Enable: false},
					{System: "GLONASS", Signal: "G1", Enable: false},
					{System: "GLONASS", Signal: "G2", Enable: false},
					{System: "GLONASS", Signal: "G3", Enable: false},
				},
			},
			Power: PresetPower{
				NoiseFloor: -172,
				InitPower: InitPower{
					Unit:  "dBHz",
					Value: 45,
				},
				ElevationAdjust: true,
			},
		}

		jsonData, err := json.MarshalIndent(preset, "", "  ")
		if err != nil {
			handleFlowError(fmt.Sprintf("序列化 Preset JSON 失败: %v", err))
			return
		}
		if err := os.WriteFile(presetPath, jsonData, 0644); err != nil {
			handleFlowError(fmt.Sprintf("写入 Preset JSON 失败: %v", err))
			return
		}

		logBroker.Broadcast(fmt.Sprintf("已成功为 Rust 发生器生成 Scenario 配置文件: %s", presetPath))
		logBroker.Broadcast(fmt.Sprintf("执行命令: %s %s", simPath, presetPath))
		cmd = exec.Command(simPath, presetPath)
	} else {
		latStr := fmt.Sprintf("%f", lat)
		lngStr := fmt.Sprintf("%f", lng)
		altStr := fmt.Sprintf("%f", alt)
		durStr := fmt.Sprintf("%d", duration)

		cmdArgs := []string{
			"-b", "8",
			"-e", ephemerisPath,
			"-l", fmt.Sprintf("%s,%s,%s", latStr, lngStr, altStr),
			"-d", durStr,
			"-o", binPath,
		}

		logBroker.Broadcast(fmt.Sprintf("执行命令: %s %s", simPath, strings.Join(cmdArgs, " ")))
		cmd = exec.Command(simPath, cmdArgs...)
	}
	
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
		if strings.Contains(text, "Time index") || strings.Contains(text, "Processed") || strings.Contains(text, "%") || len(text) > 0 {
			logBroker.Broadcast(text)
		}
	}

	if err := cmd.Wait(); err != nil {
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
		handleFlowError("生成的信号基带文件不存在或为空！基带生成阶段失败。")
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

	// Dynamic frequency lookup based on band
	var freqHz float64 = 1575420000 // default L1
	for _, s := range GNSSRegistry {
		if s.ID == system {
			for _, b := range s.Bands {
				if b.ID == band {
					freqHz = b.Frequency
					break
				}
			}
		}
	}

	logBroker.Broadcast(fmt.Sprintf("发射系统: %s | 频段: %s | 发射频点: %.3f MHz", strings.ToUpper(system), band, freqHz/1000000.0))

	// Command: hackrf_transfer -t data/gps.bin -f <freq> -s 2600000 -a 1 -x <gain>
	txArgs := []string{
		"-t", binPath,
		"-f", fmt.Sprintf("%.0f", freqHz),
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
	logBroker.Broadcast("=================== 卫星信号模拟发射正常结束 ===================")
	logBroker.Broadcast(fmt.Sprintf("已成功传输完毕所有 %s %s 模拟信号基带数据。", strings.ToUpper(system), band))

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

func triggerSystemSetup() {
	logBroker.Broadcast("=================== 开始配置系统环境依赖 ===================")
	
	if runtime.GOOS == "windows" {
		logBroker.Broadcast("检测到当前操作系统为 Windows。")
		logBroker.Broadcast("请按照以下步骤手动进行配置：")
		logBroker.Broadcast("----------------------------------------------------------")
		logBroker.Broadcast("1. 安装 HackRF 驱动：通过 Zadig 将 HackRF 替换为 WinUSB 驱动。")
		logBroker.Broadcast("2. 安装 SDR 工具链：使用 'choco install pothossdr' 或从 GitHub 下载 PothosSDR 并配置 PATH。")
		logBroker.Broadcast("3. 放置可执行文件：下载并放置 gps-sdr-sim.exe 和 beidou-sdr-sim.exe 到根目录。")
		logBroker.Broadcast("----------------------------------------------------------")
		logBroker.Broadcast("=================== 依赖配置环境指南结束 ===================")
		return
	}
	
	// Check passwordless sudo access
	sudoCheck := exec.Command("sudo", "-n", "true")
	hasSudo := sudoCheck.Run() == nil

	if !hasSudo {
		logBroker.Broadcast("👉 检测到当前运行环境无免密 sudo 权限。")
		logBroker.Broadcast("👉 请手动在服务器终端执行以下命令安装 HackRF 射频包依赖：")
		logBroker.Broadcast("----------------------------------------------------------")
		logBroker.Broadcast("   sudo apt-get update && sudo apt-get install -y git build-essential libfftw3-dev hackrf")
		logBroker.Broadcast("----------------------------------------------------------")
		logBroker.Broadcast("提示：系统依赖库（如 hackrf_transfer）可能需要手动安装。正在继续进行本地仿真器的下载与编译...")
	} else {
		// Check standard dependencies: gcc, git, make, libfftw3
		logBroker.Broadcast("正在检查并安装基础包依赖 (apt-get)...")
		aptCmd := exec.Command("sudo", "apt-get", "update")
		aptCmd.Stdout = os.Stdout
		aptCmd.Stderr = os.Stderr
		_ = aptCmd.Run()

		installCmd := exec.Command("sudo", "apt-get", "install", "-y", "git", "build-essential", "libfftw3-dev", "hackrf")
		var installErr bytes.Buffer
		installCmd.Stderr = &installErr
		if err := installCmd.Run(); err != nil {
			logBroker.Broadcast(fmt.Sprintf("安装包可能需要手动运行: %s", installErr.String()))
		} else {
			logBroker.Broadcast("✅ 系统包依赖已安装。")
		}
	}

	// 1. Clone and compile gps-sdr-sim locally if it does not exist
	gpsSimPath := findGpsSdrSim()
	if gpsSimPath == "" {
		logBroker.Broadcast("检测到系统中未找到 gps-sdr-sim 可执行程序，正在进行本地自动下载与编译...")
		
		buildDir := "data/gps-sdr-sim-build"
		_ = os.RemoveAll(buildDir)

		logBroker.Broadcast("正在克隆项目: https://github.com/osqzss/gps-sdr-sim.git ...")
		cloneCmd := exec.Command("git", "clone", "--depth", "1", "https://github.com/osqzss/gps-sdr-sim.git", buildDir)
		if err := cloneCmd.Run(); err != nil {
			logBroker.Broadcast(fmt.Sprintf("❌ 克隆 gps-sdr-sim 失败: %v", err))
		} else {
			logBroker.Broadcast("正在编译 gps-sdr-sim ...")
			makeCmd := exec.Command("make")
			makeCmd.Dir = buildDir
			var makeErr bytes.Buffer
			makeCmd.Stderr = &makeErr
			if err := makeCmd.Run(); err != nil {
				logBroker.Broadcast(fmt.Sprintf("❌ 编译失败: %s. 请检查您的 GCC 和 Make 环境。", makeErr.String()))
			} else {
				srcBin := filepath.Join(buildDir, "gps-sdr-sim")
				destBin := "./gps-sdr-sim"
				
				if err := copyBinary(srcBin, destBin); err == nil {
					logBroker.Broadcast("✅ gps-sdr-sim 编译并部署成功！")
				} else {
					logBroker.Broadcast(fmt.Sprintf("❌ 部署 gps-sdr-sim 失败: %v", err))
				}
			}
		}
		_ = os.RemoveAll(buildDir)
	} else {
		logBroker.Broadcast(fmt.Sprintf("✅ 检测到系统中已有可用的 gps-sdr-sim (%s)，无需重新编译。", gpsSimPath))
	}

	// 2. Clone and compile beidou-sdr-sim locally if it does not exist
	beidouSimPath := findBeidouSdrSim()
	if beidouSimPath == "" {
		logBroker.Broadcast("检测到系统中未找到 beidou-sdr-sim 北斗信号生成程序，正在进行本地自动下载与编译...")
		
		buildDir := "data/beidou-sdr-sim-build"
		_ = os.RemoveAll(buildDir)

		logBroker.Broadcast("正在克隆北斗仿真项目: https://github.com/yangfan852219770/beidou-sdr-sim.git ...")
		cloneCmd := exec.Command("git", "clone", "--depth", "1", "https://github.com/yangfan852219770/beidou-sdr-sim.git", buildDir)
		if err := cloneCmd.Run(); err != nil {
			logBroker.Broadcast(fmt.Sprintf("❌ 克隆 beidou-sdr-sim 失败: %v", err))
		} else {
			logBroker.Broadcast("正在编译 beidou-sdr-sim ...")
			makeCmd := exec.Command("gcc", "beidou-sdr-sim.c", "-O3", "-lm", "-o", "beidou-sdr-sim")
			makeCmd.Dir = buildDir
			var makeErr bytes.Buffer
			makeCmd.Stderr = &makeErr
			if err := makeCmd.Run(); err != nil {
				logBroker.Broadcast(fmt.Sprintf("❌ 编译北斗生成器失败: %s. 请检查您的 GCC 环境。", makeErr.String()))
			} else {
				srcBin := filepath.Join(buildDir, "beidou-sdr-sim")
				
				err1 := copyBinary(srcBin, "./beidou-sdr-sim")
				err3 := copyBinary(srcBin, "./beidou-sdr-sim-b2a")
				
				if err1 == nil && err3 == nil {
					logBroker.Broadcast("✅ beidou-sdr-sim 北斗 B1I 和 B2a 信号发生器编译并部署成功！")
				} else {
					logBroker.Broadcast("❌ 部署 beidou-sdr-sim 副本失败")
				}
			}
		}
		_ = os.RemoveAll(buildDir)
	} else {
		logBroker.Broadcast(fmt.Sprintf("✅ 检测到系统中已有可用的 beidou-sdr-sim (%s)，无需重新编译。", beidouSimPath))
	}

	// 3. Clone and compile gnss-signal-simulator-rs locally for B1C if it does not exist
	b1cSimPath := findSimTool("beidou-sdr-sim-b1c")
	if b1cSimPath == "" {
		logBroker.Broadcast("检测到系统中未找到 beidou-sdr-sim-b1c (gnss-signal-simulator) 可执行程序，正在进行本地自动下载与编译...")
		
		_, cargoErr := exec.LookPath("cargo")
		if cargoErr != nil {
			logBroker.Broadcast("❌ 自动配置失败：未在系统 PATH 中检测到 Rust 编译环境 'cargo'。")
			logBroker.Broadcast("💡 提示：如需支持北斗 B1C 真正的商业信号仿真，请先安装 Rust 环境：https://rustup.rs/")
		} else {
			buildDir := "data/gnss-signal-simulator-rs-build"
			_ = os.RemoveAll(buildDir)

			logBroker.Broadcast("正在从 GitHub 克隆 Rust 基带发生器: https://github.com/danusha2345/gnss-signal-simulator-rs.git ...")
			cloneCmd := exec.Command("git", "clone", "--depth", "1", "https://github.com/danusha2345/gnss-signal-simulator-rs.git", buildDir)
			if err := cloneCmd.Run(); err != nil {
				logBroker.Broadcast(fmt.Sprintf("❌ 克隆 gnss-signal-simulator-rs 失败: %v", err))
			} else {
				logBroker.Broadcast("正在通过 Cargo 编译 gnss-signal-simulator (这可能需要 1-2 分钟)...")
				cargoCmd := exec.Command("cargo", "build", "--release", "--bin", "gnss_rust")
				cargoCmd.Dir = buildDir
				var cargoErr bytes.Buffer
				cargoCmd.Stderr = &cargoErr
				if err := cargoCmd.Run(); err != nil {
					logBroker.Broadcast(fmt.Sprintf("❌ 编译 Rust 发生器失败: %s. 请检查您的 Rust/Cargo 环境。", cargoErr.String()))
				} else {
					ext := ""
					if runtime.GOOS == "windows" {
						ext = ".exe"
					}
					srcBin := filepath.Join(buildDir, "target", "release", "gnss_rust"+ext)
					destBin := "./beidou-sdr-sim-b1c" + ext
					
					if err := copyBinary(srcBin, destBin); err == nil {
						logBroker.Broadcast("✅ beidou-sdr-sim-b1c (gnss-signal-simulator) 北斗 B1C 发生器编译并部署成功！")
					} else {
						logBroker.Broadcast(fmt.Sprintf("❌ 部署 beidou-sdr-sim-b1c 失败: %v", err))
					}
				}
			}
			_ = os.RemoveAll(buildDir)
		}
	} else {
		logBroker.Broadcast(fmt.Sprintf("✅ 检测到系统中已有可用的 beidou-sdr-sim-b1c (%s)，无需重新编译。", b1cSimPath))
	}
	
	logBroker.Broadcast("=================== 依赖配置环境完成 ===================")
}

func copyBinary(src, dest string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, input, 0755)
}

// gnss-signal-simulator-rs preset JSON configuration structs
type PresetTime struct {
	Type   string `json:"type"`
	Year   int    `json:"year"`
	Month  int    `json:"month"`
	Day    int    `json:"day"`
	Hour   int    `json:"hour"`
	Minute int    `json:"minute"`
	Second int    `json:"second"`
}

type PresetPosition struct {
	Type      string  `json:"type"`
	Format    string  `json:"format"`
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
	Altitude  float64 `json:"altitude"`
}

type PresetVelocity struct {
	Type   string  `json:"type"`
	Speed  float64 `json:"speed"`
	Course float64 `json:"course"`
}

type TrajectoryItem struct {
	Type string  `json:"type"`
	Time float64 `json:"time"`
}

type PresetTrajectory struct {
	Name           string           `json:"name"`
	InitPosition   PresetPosition   `json:"initPosition"`
	InitVelocity   PresetVelocity   `json:"initVelocity"`
	TrajectoryList []TrajectoryItem `json:"trajectoryList"`
}

type PresetEphemeris struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type SystemSelectItem struct {
	System string `json:"system"`
	Signal string `json:"signal"`
	Enable bool   `json:"enable"`
}

type OutputConfig struct {
	ElevationMask int `json:"elevationMask"`
}

type PresetOutput struct {
	Type         string             `json:"type"`
	Format       string             `json:"format"`
	SampleFreq   float64            `json:"sampleFreq"`
	CenterFreq   float64            `json:"centerFreq"`
	Name         string             `json:"name"`
	Config       OutputConfig       `json:"config"`
	SystemSelect []SystemSelectItem `json:"systemSelect"`
}

type InitPower struct {
	Unit  string  `json:"unit"`
	Value float64 `json:"value"`
}

type PresetPower struct {
	NoiseFloor      float64   `json:"noiseFloor"`
	InitPower       InitPower `json:"initPower"`
	ElevationAdjust bool      `json:"elevationAdjust"`
}

type PresetConfig struct {
	Version     float64          `json:"version"`
	Description string           `json:"description"`
	Time        PresetTime       `json:"time"`
	Trajectory  PresetTrajectory `json:"trajectory"`
	Ephemeris   PresetEphemeris  `json:"ephemeris"`
	Output      PresetOutput     `json:"output"`
	Power       PresetPower      `json:"power"`
}

func parseEphemerisTime(filename string) time.Time {
	filename = filepath.Base(filename)
	// Try parsing BKG format: BRDC00IGS_R_YYYYDDD...
	if strings.HasPrefix(filename, "BRDC00IGS_R_") && len(filename) >= 21 {
		yearStr := filename[12:16]
		doyStr := filename[16:19]
		year, err1 := strconv.Atoi(yearStr)
		doy, err2 := strconv.Atoi(doyStr)
		if err1 == nil && err2 == nil {
			t := time.Date(year, 1, 1, 12, 0, 0, 0, time.UTC) // mid day
			return t.AddDate(0, 0, doy-1)
		}
	}
	// Try parsing NOAA format: brdcDDD0.YYn
	if strings.HasPrefix(filename, "brdc") && len(filename) >= 11 {
		doyStr := filename[4:7]
		// Find the dot, then get the 2 chars after
		dotIdx := strings.LastIndex(filename, ".")
		if dotIdx != -1 && len(filename) >= dotIdx+3 {
			yyStr := filename[dotIdx+1 : dotIdx+3]
			doy, err1 := strconv.Atoi(doyStr)
			yy, err2 := strconv.Atoi(yyStr)
			if err1 == nil && err2 == nil {
				year := 2000 + yy
				t := time.Date(year, 1, 1, 12, 0, 0, 0, time.UTC) // mid day
				return t.AddDate(0, 0, doy-1)
			}
		}
	}
	// Default to current time if parsing fails
	return time.Now().UTC()
}
