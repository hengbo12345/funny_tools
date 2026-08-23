package token

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"mihomo-sub-publisher/internal/logging"
)

// Watcher watches for the token reset trigger file.
type Watcher struct {
	triggerFilePath string
	store           *Store
	debounce        time.Duration
	mu              sync.Mutex
}

// NewWatcher creates a new reset trigger watcher.
func NewWatcher(triggerFilePath string, store *Store, debounce time.Duration) *Watcher {
	if debounce <= 0 {
		debounce = 1 * time.Minute
	}
	return &Watcher{
		triggerFilePath: triggerFilePath,
		store:           store,
		debounce:        debounce,
	}
}

// Start starts watching for the trigger file.
func (w *Watcher) Start(ctx context.Context) {
	go w.run(ctx)
}

func (w *Watcher) run(ctx context.Context) {
	logger := logging.Logger()

	dir := filepath.Dir(w.triggerFilePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		logger.Error("failed to create watcher directory", "dir", dir, "error", err)
	}

	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Error("failed to create fsnotify watcher", "error", err)
	} else {
		defer fsWatcher.Close()
		_ = fsWatcher.Add(dir)
	}

	// Check on startup if trigger file already exists
	if w.triggerFileExists() {
		w.processTriggerWithDebounce(ctx)
	}

	// Fallback ticker to ensure we don't miss file creation across OS/filesystems
	fallbackTicker := time.NewTicker(2 * time.Second)
	defer fallbackTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-fsWatcher.Events:
			if !ok {
				return
			}
			if filepath.Clean(event.Name) == filepath.Clean(w.triggerFilePath) {
				if event.Op&(fsnotify.Create|fsnotify.Write) != 0 {
					w.processTriggerWithDebounce(ctx)
				}
			}

		case err, ok := <-fsWatcher.Errors:
			if !ok {
				return
			}
			logger.Warn("fsnotify error in token reset watcher", "error", err)

		case <-fallbackTicker.C:
			if w.triggerFileExists() {
				w.processTriggerWithDebounce(ctx)
			}
		}
	}
}

func (w *Watcher) triggerFileExists() bool {
	info, err := os.Stat(w.triggerFilePath)
	return err == nil && !info.IsDir()
}

func (w *Watcher) processTriggerWithDebounce(ctx context.Context) {
	w.mu.Lock()
	defer w.mu.Unlock()

	logger := logging.Logger()

	if !w.triggerFileExists() {
		return
	}

	logger.Info("token reset trigger detected, waiting debounce", "debounce", w.debounce)

	// Debounce timer
	select {
	case <-ctx.Done():
		return
	case <-time.After(w.debounce):
	}

	// Double check trigger file still exists
	if !w.triggerFileExists() {
		return
	}

	for {
		logger.Info("executing token reset and reload")
		err := w.store.ResetAllAndReload()
		if err != nil {
			logger.Error("token reset and reload failed, retaining last-known-good state", "error", err)
			// Do not delete trigger file on failure so it can be retried or inspected
			return
		}

		logger.Info("token reset succeeded, removing trigger file")
		_ = os.Remove(w.triggerFilePath)

		// Recheck: if a new trigger file appeared during processing, process again
		time.Sleep(100 * time.Millisecond)
		if !w.triggerFileExists() {
			break
		}
	}
}
