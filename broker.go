package main

import (
	"fmt"
	"sync"
	"time"
)

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

// SweepBroker implements a Pub-Sub broker for streaming raw sweep & telemetry to client EventSources
type SweepBroker struct {
	clients map[chan string]bool
	mutex   sync.Mutex
}

func NewSweepBroker() *SweepBroker {
	return &SweepBroker{
		clients: make(map[chan string]bool),
	}
}

func (b *SweepBroker) Subscribe() chan string {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	ch := make(chan string, 200)
	b.clients[ch] = true
	return ch
}

func (b *SweepBroker) Unsubscribe(ch chan string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	delete(b.clients, ch)
	close(ch)
}

func (b *SweepBroker) Broadcast(msg string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	for ch := range b.clients {
		select {
		case ch <- msg:
		default:
			// Non-blocking write to avoid slow clients slowing down high-frequency sweeps
		}
	}
}

var (
	logBroker   = NewLogBroker()
	sweepBroker = NewSweepBroker()
)
