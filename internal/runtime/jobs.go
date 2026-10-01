package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type JobStatus string

const (
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
)

type JobState struct {
	Name          string    `json:"name"`
	Type          string    `json:"type"`
	Status        JobStatus `json:"status"`
	LastRunAt     time.Time `json:"last_run_at"`
	LastSuccessAt time.Time `json:"last_success_at"`
	LastError     string    `json:"last_error"`
	NextRunAt     time.Time `json:"next_run_at"`
}

type JobsState struct {
	Jobs map[string]JobState `json:"jobs"`
}

func ReadJobs(path string) (JobsState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return JobsState{}, err
	}
	var jobs JobsState
	if err := json.Unmarshal(data, &jobs); err != nil {
		return JobsState{}, err
	}
	if jobs.Jobs == nil {
		jobs.Jobs = map[string]JobState{}
	}
	return jobs, nil
}

func WriteJobs(path string, jobs JobsState) error {
	if jobs.Jobs == nil {
		jobs.Jobs = map[string]JobState{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

type Job interface {
	Run(now time.Time, cfg JobConfig) JobState
}

type heartbeatJob struct{}

func (heartbeatJob) Run(now time.Time, cfg JobConfig) JobState {
	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultHeartbeatJobInterval
	}
	return JobState{
		Name:          cfg.Name,
		Type:          cfg.Type,
		Status:        JobSucceeded,
		LastRunAt:     now.UTC(),
		LastSuccessAt: now.UTC(),
		LastError:     "",
		NextRunAt:     now.Add(interval).UTC(),
	}
}

func defaultJobRegistry() map[string]Job {
	return map[string]Job{"heartbeat": heartbeatJob{}}
}

func RunConfiguredJobs(paths Paths, cfg RuntimeConfig, now time.Time) error {
	jobsState := JobsState{Jobs: map[string]JobState{}}
	if existing, err := ReadJobs(paths.JobsPath); err == nil {
		jobsState = existing
	} else if !os.IsNotExist(err) {
		return err
	}
	registry := defaultJobRegistry()
	for _, jobCfg := range cfg.Jobs {
		if !jobCfg.Enabled {
			continue
		}
		previous, hasPrevious := jobsState.Jobs[jobCfg.Name]
		if hasPrevious && previous.NextRunAt.After(now) {
			continue
		}
		job := registry[jobCfg.Type]
		jobsState.Jobs[jobCfg.Name] = job.Run(now, jobCfg)
	}
	return WriteJobs(paths.JobsPath, jobsState)
}

func RunHeartbeatJob(paths Paths, now time.Time, interval time.Duration) (JobState, error) {
	cfg := RuntimeConfig{Jobs: []JobConfig{{
		Name:     "heartbeat",
		Type:     "heartbeat",
		Enabled:  true,
		Interval: interval,
	}}}
	if cfg.Jobs[0].Interval <= 0 {
		cfg.Jobs[0].Interval = defaultHeartbeatJobInterval
	}
	if err := RunConfiguredJobs(paths, cfg, now); err != nil {
		return JobState{}, err
	}
	jobs, err := ReadJobs(paths.JobsPath)
	if err != nil {
		return JobState{}, err
	}
	return jobs.Jobs["heartbeat"], nil
}
