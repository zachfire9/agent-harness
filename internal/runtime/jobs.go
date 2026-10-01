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

func RunHeartbeatJob(paths Paths, now time.Time, interval time.Duration) (JobState, error) {
	if interval <= 0 {
		interval = time.Minute
	}
	job := JobState{
		Name:          "heartbeat",
		Type:          "heartbeat",
		Status:        JobSucceeded,
		LastRunAt:     now.UTC(),
		LastSuccessAt: now.UTC(),
		LastError:     "",
		NextRunAt:     now.Add(interval).UTC(),
	}
	jobs := JobsState{Jobs: map[string]JobState{job.Name: job}}
	if err := WriteJobs(paths.JobsPath, jobs); err != nil {
		return JobState{}, err
	}
	return job, nil
}
