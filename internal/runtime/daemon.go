package runtime

import (
	"context"
	"os"
	"time"

	"github.com/zachfire9/agent-harness/internal/version"
)

// WriteHeartbeat writes a running status snapshot for the current process.
func WriteHeartbeat(paths Paths, instance string, startedAt time.Time, now time.Time, metadata version.Metadata) error {
	return WriteHeartbeatWithRuntimeConfig(paths, instance, startedAt, now, metadata, DefaultRuntimeConfig())
}

func WriteHeartbeatWithRuntimeConfig(paths Paths, instance string, startedAt time.Time, now time.Time, metadata version.Metadata, cfg RuntimeConfig) error {
	if err := RunConfiguredJobs(paths, cfg, now); err != nil {
		return err
	}
	jobs, _ := ReadJobs(paths.JobsPath)
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
		Jobs:            jobs.Jobs,
	})
}

func WriteHeartbeatWithJobInterval(paths Paths, instance string, startedAt time.Time, now time.Time, metadata version.Metadata, jobInterval time.Duration) error {
	cfg := RuntimeConfig{Jobs: []JobConfig{{Name: "heartbeat", Type: "heartbeat", Enabled: true, Interval: jobInterval}}}
	if cfg.Jobs[0].Interval <= 0 {
		cfg.Jobs[0].Interval = defaultHeartbeatJobInterval
	}
	return WriteHeartbeatWithRuntimeConfig(paths, instance, startedAt, now, metadata, cfg)
}

// RunDaemon writes status heartbeats until the context is cancelled.
func RunDaemon(ctx context.Context, paths Paths, instance string, metadata version.Metadata, interval time.Duration) error {
	if interval <= 0 {
		interval = time.Minute
	}
	startedAt := time.Now().UTC()
	cfg, err := ReadRuntimeConfig(paths)
	if err != nil {
		_ = WriteStatus(paths.StatusPath, Status{
			Instance:  instance,
			Status:    StateConfigError,
			StartedAt: startedAt,
			PID:       os.Getpid(),
			Version:   metadata.Version,
			Commit:    metadata.Commit,
			BuildDate: metadata.BuildDate,
			Dirty:     metadata.Dirty,
			Error:     err.Error(),
		})
		return err
	}
	if err := WriteHeartbeatWithRuntimeConfig(paths, instance, startedAt, startedAt, metadata, cfg); err != nil {
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
			if err := WriteHeartbeatWithRuntimeConfig(paths, instance, startedAt, now, metadata, cfg); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}
