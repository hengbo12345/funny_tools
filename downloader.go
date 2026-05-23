package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func handleDownloadEphemeris(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Type string `json:"type"` // "gps" or "multi"
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Type == "" {
		req.Type = "gps"
	}

	if req.Type == "multi" {
		go triggerMultiEphemerisDownload()
	} else {
		go triggerEphemerisDownload()
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message": "Ephemeris download triggered in background"}`))
}

func triggerMultiEphemerisDownload() {
	logBroker.Broadcast("正在尝试自动下载最新北斗多系统混合广播星历 (BRDC)...")
	
	// BKG mirror URL: https://igs.bkg.bund.de/root_ftp/IGS/BRDC/{YYYY}/{DDD}/BRDC00IGS_R_{YYYY}{DDD}0000_01D_MN.rnx.gz
	now := time.Now().UTC()
	success := false
	var err error

	for offset := 0; offset <= 2; offset++ {
		targetTime := now.AddDate(0, 0, -offset)
		year := targetTime.Year()
		doy := targetTime.YearDay()
		doyStr := fmt.Sprintf("%03d", doy)

		// RINEX 3 Mixed Navigation long name format
		url := fmt.Sprintf("https://igs.bkg.bund.de/root_ftp/IGS/BRDC/%d/%s/BRDC00IGS_R_%d%s0000_01D_MN.rnx.gz", year, doyStr, year, doyStr)
		filename := fmt.Sprintf("BRDC00IGS_R_%d%s0000_01D_MN.rnx.gz", year, doyStr)
		destFileName := fmt.Sprintf("BRDC00IGS_R_%d%s0000_01D_MN.rnx", year, doyStr)
		destPath := filepath.Join("data/ephemeris", destFileName)

		// If decompressed file already exists, don't download
		if _, statErr := os.Stat(destPath); statErr == nil {
			logBroker.Broadcast(fmt.Sprintf("混合星历文件 %s 已存在，无需重复下载。", destFileName))
			success = true
			break
		}

		logBroker.Broadcast(fmt.Sprintf("正在尝试从 BKG 镜像下载 (%d天前混合星历): %s ...", offset, url))
		
		err = downloadAndExtractGz(url, filename, destPath)
		if err == nil {
			logBroker.Broadcast(fmt.Sprintf("✅ 混合星历文件下载并解压成功: %s", destFileName))
			success = true
			break
		} else {
			logBroker.Broadcast(fmt.Sprintf("该日混合星历获取失败: %v", err))
		}
	}

	if !success {
		logBroker.Broadcast("❌ 自动下载混合星历失败！请确保您的服务器能够正常连接互联网，或使用手动上传。")
	}
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
