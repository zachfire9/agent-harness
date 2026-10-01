package runtime

import (
	"context"
	"os"
	"time"

	"github.com/zachfire9/agent-harness/internal/version"
)

// WriteHeartbeat writes a running status snapshot for the current process.
func WriteHeartbeat(paths Paths, instance string, startedAt time.Time, now time.Time, metadata version.Metadata) error {
	return WriteHeartbeatWithJobInterval(paths, instance, startedAt, now, metadata, defaultHeartbeatJobInterval)
}

func WriteHeartbeatWithJobInterval(paths Paths, instance string, startedAt time.Time, now time.Time, metadata version.Metadata, jobInterval time.Duration) error {
	if _, err := RunHeartbeatJob(paths, now, jobInterval); err != nil {
		return err
	}
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
	cfg, err := ReadRuntimeConfig(paths)
	if err != nil {
		return err
	}
	if err := WriteHeartbeatWithJobInterval(paths, instance, startedAt, startedAt, metadata, cfg.HeartbeatJobInterval); err != nil {
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
			if err := WriteHeartbeatWithJobInterval(paths, instance, startedAt, now, metadata, cfg.HeartbeatJobInterval); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}
