package token

import (
	"context"
	"time"

	"mihomo-sub-publisher/internal/logging"
)

// StartPeriodicPersistence starts a background loop that flushes token state every interval.
func StartPeriodicPersistence(ctx context.Context, store *Store, interval time.Duration) {
	if interval <= 0 {
		interval = 1 * time.Minute
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		logger := logging.Logger()

		for {
			select {
			case <-ctx.Done():
				// Final flush on shutdown
				if err := store.SaveState(); err != nil {
					logger.Error("final token state flush failed", "error", err)
				} else {
					logger.Info("final token state flushed successfully")
				}
				return
			case <-ticker.C:
				if err := store.SaveState(); err != nil {
					logger.Error("periodic token state flush failed", "error", err)
				}
			}
		}
	}()
}
