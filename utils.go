package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// findSimTool searches for a given simulator tool executable
func findSimTool(toolName string) string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	paths := []string{
		"./" + toolName + ext,
		"./bin/" + toolName + ext,
		"/usr/local/bin/" + toolName + ext,
		"/usr/bin/" + toolName + ext,
	}
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	// Check standard PATH
	if p, err := exec.LookPath(toolName + ext); err == nil {
		return p
	}
	if runtime.GOOS == "windows" {
		if p, err := exec.LookPath(toolName); err == nil {
			return p
		}
	}
	return ""
}

func findGpsSdrSim() string {
	return findSimTool("gps-sdr-sim")
}

func findBeidouSdrSim() string {
	return findSimTool("beidou-sdr-sim")
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
		if runtime.GOOS == "windows" {
			return "检测失败 (Windows 下未找到 hackrf_clock，请确保其已加入 PATH)"
		}
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
			name := strings.ToLower(entry.Name())
			if strings.HasSuffix(name, ".n") || strings.HasSuffix(name, ".brdc") || 
				strings.HasSuffix(name, ".nav") || strings.HasSuffix(name, ".rnx") || 
				strings.Contains(name, "brdc") {
				files = append(files, entry.Name())
			}
		}
	}
	return files
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
