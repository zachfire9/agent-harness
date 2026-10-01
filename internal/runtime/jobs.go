package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	OutputPath    string    `json:"output_path,omitempty"`
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
	Run(paths Paths, now time.Time, cfg JobConfig) (JobState, error)
}

type Message struct {
	Job       string
	Body      string
	Timestamp time.Time
}

type Notifier interface {
	Send(ctx context.Context, message Message) error
}

type heartbeatJob struct{}

func (heartbeatJob) Run(paths Paths, now time.Time, cfg JobConfig) (JobState, error) {
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
	}, nil
}

type localCheckinJob struct{}

type localCheckinRecord struct {
	Job       string `json:"job"`
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
	Status    string `json:"status"`
}

func (localCheckinJob) Run(paths Paths, now time.Time, cfg JobConfig) (JobState, error) {
	outputPath := localCheckinOutputPath(cfg)
	if err := validateInstanceRelativePath(outputPath); err != nil {
		return JobState{OutputPath: outputPath}, err
	}
	record := localCheckinRecord{
		Job:       cfg.Name,
		Timestamp: now.UTC().Format(time.RFC3339),
		Message:   localCheckinMessage(cfg),
		Status:    string(JobSucceeded),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return JobState{}, err
	}
	data = append(data, '\n')
	absPath := filepath.Join(paths.Home, filepath.FromSlash(outputPath))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return JobState{OutputPath: outputPath}, err
	}
	file, err := os.OpenFile(absPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return JobState{OutputPath: outputPath}, err
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return JobState{OutputPath: outputPath}, err
	}
	return JobState{
		Name:          cfg.Name,
		Type:          cfg.Type,
		Status:        JobSucceeded,
		LastRunAt:     now.UTC(),
		LastSuccessAt: now.UTC(),
		LastError:     "",
		NextRunAt:     now.Add(cfg.Interval).UTC(),
		OutputPath:    outputPath,
	}, nil
}

func localCheckinMessage(cfg JobConfig) string {
	if strings.TrimSpace(cfg.Message) == "" {
		return "agent-harness is alive"
	}
	return cfg.Message
}

func localCheckinOutputPath(cfg JobConfig) string {
	if strings.TrimSpace(cfg.OutputPath) == "" {
		return "work/checkins.jsonl"
	}
	return filepath.ToSlash(filepath.Clean(cfg.OutputPath))
}

type notifyTestJob struct{}

func (notifyTestJob) Run(paths Paths, now time.Time, cfg JobConfig) (JobState, error) {
	return runNotifyTestWithNotifier(paths, now, cfg, fileOutboxNotifier{paths: paths, outboxPath: notifyTestOutboxPath(cfg)})
}

func runNotifyTestWithNotifier(paths Paths, now time.Time, cfg JobConfig, notifier Notifier) (JobState, error) {
	message := Message{Job: cfg.Name, Body: notifyTestMessage(cfg), Timestamp: now.UTC()}
	if err := notifier.Send(context.Background(), message); err != nil {
		return JobState{OutputPath: notifyTestOutboxPath(cfg)}, err
	}
	return JobState{
		Name:          cfg.Name,
		Type:          cfg.Type,
		Status:        JobSucceeded,
		LastRunAt:     now.UTC(),
		LastSuccessAt: now.UTC(),
		LastError:     "",
		NextRunAt:     now.Add(cfg.Interval).UTC(),
		OutputPath:    notifyTestOutboxPath(cfg),
	}, nil
}

func notifyTestMessage(cfg JobConfig) string {
	if strings.TrimSpace(cfg.Message) == "" {
		return "agent-harness notification test"
	}
	return cfg.Message
}

func notifyTestOutboxPath(cfg JobConfig) string {
	if strings.TrimSpace(cfg.OutboxPath) == "" {
		return "work/outbox.jsonl"
	}
	return filepath.ToSlash(filepath.Clean(cfg.OutboxPath))
}

type fileOutboxNotifier struct {
	paths      Paths
	outboxPath string
}

type fileOutboxRecord struct {
	Job       string `json:"job"`
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
	Transport string `json:"transport"`
}

func (n fileOutboxNotifier) Send(ctx context.Context, message Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateInstanceRelativePath(n.outboxPath); err != nil {
		return err
	}
	record := fileOutboxRecord{
		Job:       message.Job,
		Timestamp: message.Timestamp.UTC().Format(time.RFC3339),
		Message:   message.Body,
		Transport: "file_outbox",
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	absPath := filepath.Join(n.paths.Home, filepath.FromSlash(n.outboxPath))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(absPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(data)
	return err
}

func defaultJobRegistry() map[string]Job {
	return map[string]Job{"heartbeat": heartbeatJob{}, "local_checkin": localCheckinJob{}, "notify_test": notifyTestJob{}}
}

func RunConfiguredJobs(paths Paths, cfg RuntimeConfig, now time.Time) error {
	jobsState, err := readJobsStateOrEmpty(paths.JobsPath)
	if err != nil {
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
		if _, err := runConfiguredJobWithState(paths, jobsState, jobCfg, now, registry); err != nil {
			if writeErr := WriteJobs(paths.JobsPath, jobsState); writeErr != nil {
				return writeErr
			}
			return err
		}
	}
	return WriteJobs(paths.JobsPath, jobsState)
}

func RunConfiguredJobNow(paths Paths, cfg RuntimeConfig, name string, now time.Time) (JobState, error) {
	return runConfiguredJobNowWithRegistry(paths, cfg, name, now, defaultJobRegistry())
}

func runConfiguredJobNowWithRegistry(paths Paths, cfg RuntimeConfig, name string, now time.Time, registry map[string]Job) (JobState, error) {
	jobCfg, ok := findJobConfig(cfg, name)
	if !ok {
		return JobState{}, fmt.Errorf("unknown job %q", name)
	}
	if !jobCfg.Enabled {
		return JobState{}, fmt.Errorf("job %q is disabled", name)
	}
	jobsState, err := readJobsStateOrEmpty(paths.JobsPath)
	if err != nil {
		return JobState{}, err
	}
	state, runErr := runConfiguredJobWithState(paths, jobsState, jobCfg, now, registry)
	if err := WriteJobs(paths.JobsPath, jobsState); err != nil {
		return JobState{}, err
	}
	return state, runErr
}

func findJobConfig(cfg RuntimeConfig, name string) (JobConfig, bool) {
	for _, jobCfg := range cfg.Jobs {
		if jobCfg.Name == name {
			return jobCfg, true
		}
	}
	return JobConfig{}, false
}

func runConfiguredJobWithState(paths Paths, jobsState JobsState, jobCfg JobConfig, now time.Time, registry map[string]Job) (JobState, error) {
	job, ok := registry[jobCfg.Type]
	if !ok {
		state := failedJobState(jobCfg, now, fmt.Errorf("unknown job type %q", jobCfg.Type))
		jobsState.Jobs[jobCfg.Name] = state
		return state, fmt.Errorf("unknown job type %q", jobCfg.Type)
	}
	state, err := job.Run(paths, now, jobCfg)
	if err != nil {
		failed := failedJobState(jobCfg, now, err)
		if state.OutputPath != "" {
			failed.OutputPath = state.OutputPath
		}
		state = failed
	} else {
		state = normalizeJobState(state, jobCfg, now)
	}
	jobsState.Jobs[jobCfg.Name] = state
	return state, err
}

func normalizeJobState(state JobState, jobCfg JobConfig, now time.Time) JobState {
	if state.Name == "" {
		state.Name = jobCfg.Name
	}
	if state.Type == "" {
		state.Type = jobCfg.Type
	}
	if state.Status == "" {
		state.Status = JobSucceeded
	}
	if state.LastRunAt.IsZero() {
		state.LastRunAt = now.UTC()
	}
	if state.Status == JobSucceeded && state.LastSuccessAt.IsZero() {
		state.LastSuccessAt = state.LastRunAt
	}
	if state.NextRunAt.IsZero() {
		state.NextRunAt = state.LastRunAt.Add(jobCfg.Interval).UTC()
	}
	return state
}

func failedJobState(jobCfg JobConfig, now time.Time, err error) JobState {
	interval := jobCfg.Interval
	if interval <= 0 {
		interval = defaultHeartbeatJobInterval
	}
	state := JobState{
		Name:      jobCfg.Name,
		Type:      jobCfg.Type,
		Status:    JobFailed,
		LastRunAt: now.UTC(),
		LastError: err.Error(),
		NextRunAt: now.Add(interval).UTC(),
	}
	if jobCfg.Type == "local_checkin" || jobCfg.Type == "notify_test" {
		state.OutputPath = localCheckinOutputPath(jobCfg)
		if jobCfg.Type == "notify_test" {
			state.OutputPath = notifyTestOutboxPath(jobCfg)
		}
	}
	return state
}

func readJobsStateOrEmpty(path string) (JobsState, error) {
	jobsState := JobsState{Jobs: map[string]JobState{}}
	if existing, err := ReadJobs(path); err == nil {
		jobsState = existing
	} else if !os.IsNotExist(err) {
		return JobsState{}, err
	}
	if jobsState.Jobs == nil {
		jobsState.Jobs = map[string]JobState{}
	}
	return jobsState, nil
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
