package runtime

import (
	"context"
	"os"
	"time"

	"github.com/zachfire9/agent-harness/internal/version"
)

// WriteHeartbeat writes a running status snapshot for the current process.
func WriteHeartbeat(paths Paths, instance string, startedAt time.Time, now time.Time, metadata version.Metadata) error {
	return WriteStatus(paths.StatusPath, Status{
		Instance:        instance,
		Status:          StateRunning,
		StartedAt:       startedAt.UTC(),
		LastHeartbeatAt: now.UTC(),
		PID:             os.Getpid(),
		Version:         metadata.Version,
		Commit:          metadata.Commit,
		BuildDate:       metadata.BuildDate,
		Dirty:           metadata.Dirty,
	})
}

// RunDaemon writes status heartbeats until the context is cancelled.
func RunDaemon(ctx context.Context, paths Paths, instance string, metadata version.Metadata, interval time.Duration) error {
	if interval <= 0 {
		interval = time.Minute
	}
	startedAt := time.Now().UTC()
	if err := WriteHeartbeat(paths, instance, startedAt, startedAt, metadata); err != nil {
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
			if err := WriteHeartbeat(paths, instance, startedAt, now, metadata); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}
