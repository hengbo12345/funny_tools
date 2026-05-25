package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

//go:embed static/*
var staticFS embed.FS

func main() {
	// 1. Setup CLI flags
	flag.StringVar(&hostFlag, "host", "0.0.0.0", "Web server listening host IP")
	flag.IntVar(&portFlag, "port", 8080, "Web server listening port")
	flag.StringVar(&tokenFlag, "token", "", "Access authentication token (empty to disable)")
	flag.IntVar(&rxPortFlag, "rxport", 9999, "UDP port for real-time NMEA listening")

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
		fmt.Printf("  -rxport int\n")
		fmt.Printf("    	UDP port for real-time NMEA listening (default 9999)\n")
		fmt.Printf("  -token string\n")
		fmt.Printf("    	Access authentication token (empty to disable)\n")
		fmt.Printf("  -h, --help\n")
		fmt.Printf("    	Display this customized help message\n\n")
		fmt.Printf("Examples:\n")
		fmt.Printf("  Run on LAN with port 9000:\n")
		fmt.Printf("    ./gps-simulator -host 0.0.0.0 -port 9000\n\n")
		fmt.Printf("  Enable secure token authentication:\n")
		fmt.Printf("    ./gps-simulator -token Secure123!\n\n")
		fmt.Printf("  Configure custom NMEA listening port:\n")
		fmt.Printf("    ./gps-simulator -rxport 10000\n\n")
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
	http.HandleFunc("/api/receiver/start", withAuth(handleReceiverStart))
	http.HandleFunc("/api/receiver/stop", withAuth(handleReceiverStop))
	http.HandleFunc("/api/receiver/stream", handleReceiverStreamSSE)

	// Embedded Static File Server
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

	sysStatus := SystemStatus{
		TokenRequired:   tokenRequired,
		Status:          status,
		HackrfConnected: connected,
		TcxoStatus:      tcxo,
		ActiveSim:       active,
		EphemerisFiles:  ephemerisList,
		GpsSimExists:    findGpsSdrSim() != "",
		BeidouSimExists: findBeidouSdrSim() != "",
	}

	json.NewEncoder(w).Encode(sysStatus)
}

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
			escapedMsg := strings.ReplaceAll(msg, "\n", " ")
			fmt.Fprintf(w, "data: %s\n\n", escapedMsg)
			w.(http.Flusher).Flush()
		}
	}
}
