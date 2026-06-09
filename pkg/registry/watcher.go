package registry

import (
	"context"
	"log/slog"
	"time"
)

// StartExpiryWatcher starts a background goroutine that periodically checks for expired faults
// that failed to revert via normal timers (e.g., due to goroutine scheduling issues or engine hang).
// It runs until the provided context is canceled.
func (r *FaultRegistry) StartExpiryWatcher(ctx context.Context, currentRuntime string, reverter Reverter, netIface string, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.sweepExpired(ctx, currentRuntime, reverter, netIface)
			}
		}
	}()
}

// sweepExpired scans active records, identifies those that have expired, and attempts to revert them.
func (r *FaultRegistry) sweepExpired(ctx context.Context, currentRuntime string, reverter Reverter, netIface string) {
	active := r.ListActive()
	var sweepCount int

	for _, rec := range active {
		if rec.Runtime != currentRuntime {
			continue
		}

		if rec.IsExpired() {
			slog.Warn("registry: watcher found expired fault, forcing revert",
				slog.String("id", rec.ID),
				slog.String("target", rec.Target),
				slog.String("fault_type", string(rec.FaultType)),
			)

			// Use the same revert mapping as RecoverOrphans
			err := revertFault(ctx, rec, reverter, netIface)
			if err != nil {
				slog.Error("registry: watcher failed to revert expired fault",
					slog.String("id", rec.ID),
					slog.String("error", err.Error()),
				)
			} else {
				if markErr := r.MarkReverted(rec.ID); markErr == nil {
					sweepCount++
				} else {
					slog.Error("registry: watcher reverted fault but failed to mark in registry",
						slog.String("id", rec.ID),
						slog.String("error", markErr.Error()),
					)
				}
			}
		}
	}

	if sweepCount > 0 {
		_, _ = r.GarbageCollect()
	}
}
