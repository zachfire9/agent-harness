package runtime

import (
	"context"
	"os"
	"time"
)

// WriteHeartbeat writes a running status snapshot for the current process.
func WriteHeartbeat(paths Paths, instance string, startedAt time.Time, now time.Time, version string) error {
	return WriteStatus(paths.StatusPath, Status{
		Instance:        instance,
		Status:          StateRunning,
		StartedAt:       startedAt.UTC(),
		LastHeartbeatAt: now.UTC(),
		PID:             os.Getpid(),
		Version:         version,
	})
}

// RunDaemon writes status heartbeats until the context is cancelled.
func RunDaemon(ctx context.Context, paths Paths, instance string, version string, interval time.Duration) error {
	if interval <= 0 {
		interval = time.Minute
	}
	startedAt := time.Now().UTC()
	if err := WriteHeartbeat(paths, instance, startedAt, startedAt, version); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		switch {
		case ctx.Err() != nil:
			return nil
		default:
		}
		select {
		case now := <-ticker.C:
			if err := WriteHeartbeat(paths, instance, startedAt, now, version); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}
