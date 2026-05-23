package main

import (
	"bufio"
	"bytes"
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
	
	// Check extension
	if !strings.HasSuffix(baseName, ".n") && !strings.HasSuffix(baseName, ".brdc") && !strings.HasSuffix(baseName, "n") && !strings.HasSuffix(baseName, ".nav") && !strings.HasSuffix(baseName, ".rnx") {
		http.Error(w, "Invalid ephemeris file format. Must be an N-file (.n, .brdc, .nav, .rnx)", http.StatusBadRequest)
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

func runSimulationFlow(simPath string, lat, lng, alt float64, duration, gain int, ephemeris string, system string, band string) {
	ephemerisPath := filepath.Join("data/ephemeris", ephemeris)
	binPath := filepath.Clean("data/gps.bin")

	// Delete old file if exists
	os.Remove(binPath)

	// Step 1: Execute simulation tool to generate binary
	logBroker.Broadcast("---------------- [1/2] 正在生成基带仿真信号 ----------------")
	
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
	
	cmd := exec.Command(simPath, cmdArgs...)
	
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
				
				input, err := os.Open(srcBin)
				if err == nil {
					output, err := os.OpenFile(destBin, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
					if err == nil {
						_, _ = io.Copy(output, input)
						output.Close()
						logBroker.Broadcast("✅ gps-sdr-sim 编译并部署成功！")
					}
					input.Close()
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
				destBin := "./beidou-sdr-sim"
				
				input, err := os.Open(srcBin)
				if err == nil {
					output, err := os.OpenFile(destBin, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
					if err == nil {
						_, _ = io.Copy(output, input)
						output.Close()
						logBroker.Broadcast("✅ beidou-sdr-sim 北斗信号生成器编译并部署成功！")
					}
					input.Close()
				}
			}
		}
		_ = os.RemoveAll(buildDir)
	} else {
		logBroker.Broadcast(fmt.Sprintf("✅ 检测到系统中已有可用的 beidou-sdr-sim (%s)，无需重新编译。", beidouSimPath))
	}
	
	logBroker.Broadcast("=================== 依赖配置环境完成 ===================")
}
