package main

import (
	"os/exec"
	"sync"
	"time"
)

// SimStatus represents the state of the simulation
type SimStatus string

const (
	StatusIdle         SimStatus = "idle"
	StatusGenerating   SimStatus = "generating"
	StatusTransmitting SimStatus = "transmitting"
	StatusScanning     SimStatus = "scanning"
	StatusError        SimStatus = "error"
)

// GNSSBand represents a civilian frequency band
type GNSSBand struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Frequency float64 `json:"frequency"` // in Hz
	ToolName  string  `json:"tool_name"`
}

// GNSSSystem represents a satellite constellation
type GNSSSystem struct {
	ID    string     `json:"id"`
	Name  string     `json:"name"`
	Bands []GNSSBand `json:"bands"`
}

var (
	GNSSRegistry = []GNSSSystem{
		{
			ID:   "gps",
			Name: "GPS 卫星定位系统",
			Bands: []GNSSBand{
				{ID: "L1", Name: "L1 - 1575.42 MHz", Frequency: 1575420000, ToolName: "gps-sdr-sim"},
				{ID: "L5", Name: "L5 - 1176.45 MHz (实验性)", Frequency: 1176450000, ToolName: "gps-sdr-sim-l5"},
			},
		},
		{
			ID:   "beidou",
			Name: "北斗卫星导航系统 (BDS)",
			Bands: []GNSSBand{
				{ID: "B1I", Name: "B1I - 1561.098 MHz", Frequency: 1561098000, ToolName: "beidou-sdr-sim"},
				{ID: "B1C", Name: "B1C - 1575.42 MHz (实验性)", Frequency: 1575420000, ToolName: "beidou-sdr-sim-b1c"},
				{ID: "B2a", Name: "B2a - 1176.45 MHz (实验性)", Frequency: 1176450000, ToolName: "beidou-sdr-sim-b2a"},
			},
		},
	}
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
	System    string    `json:"system"`
	Band      string    `json:"band"`
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
	BeidouSimExists bool       `json:"beidou_sim_exists"`
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

	// Receiver sweep globals
	receiverCmd   *exec.Cmd
	scanStartTime time.Time
	receiverCoords = struct {
		Lat float64 `json:"lat"`
		Lng float64 `json:"lng"`
		Alt float64 `json:"alt"`
	}{
		Lat: 39.9042,
		Lng: 116.4074,
		Alt: 100.0,
	}
)
