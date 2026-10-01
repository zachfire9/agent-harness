package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zachfire9/agent-harness/internal/version"
)

func TestValidateInstanceNameAcceptsSafeNames(t *testing.T) {
	for _, name := range []string{"default", "worker", "scheduler-1", "agent_2"} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateInstanceName(name); err != nil {
				t.Fatalf("expected %q to be valid, got %v", name, err)
			}
		})
	}
}

func TestValidateInstanceNameRejectsUnsafeNames(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../default", "default/x", "default x", "agent.service", "agent@default", "-bad"} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateInstanceName(name); err == nil {
				t.Fatalf("expected %q to be invalid", name)
			}
		})
	}
}

func TestLinuxPathResolverKeepsInstanceFilesUnderInstanceHome(t *testing.T) {
	root := t.TempDir()
	paths, err := LinuxPaths(root, "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}

	wantHome := filepath.Join(root, "agent-harness", "instances", "default")
	assertPath(t, paths.Home, wantHome)
	assertPath(t, paths.ConfigDir, filepath.Join(wantHome, "config"))
	assertPath(t, paths.StateDir, filepath.Join(wantHome, "state"))
	assertPath(t, paths.CacheDir, filepath.Join(wantHome, "cache"))
	assertPath(t, paths.WorkDir, filepath.Join(wantHome, "work"))
	assertPath(t, paths.LogDir, filepath.Join(wantHome, "logs"))
	assertPath(t, paths.StatusPath, filepath.Join(wantHome, "state", "status.json"))
}

func TestInitInstanceCreatesSelfContainedHome(t *testing.T) {
	root := t.TempDir()
	paths, err := LinuxPaths(root, "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}

	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance failed: %v", err)
	}

	for _, dir := range []string{paths.Home, paths.ConfigDir, paths.StateDir, paths.CacheDir, paths.WorkDir, paths.LogDir} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("expected directory %s: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("expected %s to be a directory", dir)
		}
	}
	if _, err := os.Stat(filepath.Join(paths.ConfigDir, "config.yaml")); err != nil {
		t.Fatalf("expected starter config.yaml: %v", err)
	}
}

func TestInitInstanceStarterConfigIncludesEnabledHeartbeatJob(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance failed: %v", err)
	}

	cfg, err := ReadRuntimeConfig(paths)
	if err != nil {
		t.Fatalf("read runtime config failed: %v", err)
	}
	if len(cfg.Jobs) != 1 {
		t.Fatalf("expected one starter job, got %#v", cfg.Jobs)
	}
	job := cfg.Jobs[0]
	if job.Name != "heartbeat" || job.Type != "heartbeat" || !job.Enabled || job.Interval != time.Minute {
		t.Fatalf("unexpected starter heartbeat config: %#v", job)
	}
}

func TestReadRuntimeConfigParsesConfiguredJobs(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance failed: %v", err)
	}
	configPath := filepath.Join(paths.ConfigDir, "config.yaml")
	config := `jobs:
  - name: heartbeat
    type: heartbeat
    enabled: true
    interval_seconds: 300
  - name: slow-heartbeat
    type: heartbeat
    enabled: true
    interval_seconds: 600
`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	cfg, err := ReadRuntimeConfig(paths)
	if err != nil {
		t.Fatalf("read runtime config failed: %v", err)
	}
	if len(cfg.Jobs) != 2 {
		t.Fatalf("expected two configured jobs, got %#v", cfg.Jobs)
	}
	if cfg.Jobs[0].Name != "heartbeat" || cfg.Jobs[0].Interval != 5*time.Minute {
		t.Fatalf("unexpected first job: %#v", cfg.Jobs[0])
	}
	if cfg.Jobs[1].Name != "slow-heartbeat" || cfg.Jobs[1].Interval != 10*time.Minute {
		t.Fatalf("unexpected second job: %#v", cfg.Jobs[1])
	}
}

func TestReadRuntimeConfigRejectsUnknownJobType(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance failed: %v", err)
	}
	config := `jobs:
  - name: surprise
    type: email
    enabled: true
    interval_seconds: 60
`
	if err := os.WriteFile(filepath.Join(paths.ConfigDir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	_, err = ReadRuntimeConfig(paths)
	if err == nil || !strings.Contains(err.Error(), `unknown job type "email"`) {
		t.Fatalf("expected unknown job type error, got %v", err)
	}
}

func TestReadRuntimeConfigRejectsInvalidJobInterval(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance failed: %v", err)
	}
	config := `jobs:
  - name: heartbeat
    type: heartbeat
    enabled: true
    interval_seconds: 0
`
	if err := os.WriteFile(filepath.Join(paths.ConfigDir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	_, err = ReadRuntimeConfig(paths)
	if err == nil || !strings.Contains(err.Error(), `invalid interval_seconds`) {
		t.Fatalf("expected invalid interval error, got %v", err)
	}
}

func TestReadRuntimeConfigParsesLocalCheckinJobFields(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance failed: %v", err)
	}
	config := `jobs:
  - name: daily-checkin
    type: local_checkin
    enabled: true
    interval_seconds: 86400
    message: "agent-harness is alive"
    output_path: "work/checkins.jsonl"
`
	if err := os.WriteFile(filepath.Join(paths.ConfigDir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	cfg, err := ReadRuntimeConfig(paths)
	if err != nil {
		t.Fatalf("read runtime config failed: %v", err)
	}
	if len(cfg.Jobs) != 1 {
		t.Fatalf("expected one job, got %#v", cfg.Jobs)
	}
	job := cfg.Jobs[0]
	if job.Name != "daily-checkin" || job.Type != "local_checkin" || job.Message != "agent-harness is alive" || job.OutputPath != "work/checkins.jsonl" || job.Interval != 24*time.Hour {
		t.Fatalf("unexpected local checkin config: %#v", job)
	}
}

func TestReadRuntimeConfigRejectsLocalCheckinOutputOutsideInstanceHome(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance failed: %v", err)
	}
	for _, outputPath := range []string{"../outside.jsonl", "/tmp/outside.jsonl"} {
		t.Run(outputPath, func(t *testing.T) {
			config := `jobs:
  - name: daily-checkin
    type: local_checkin
    enabled: true
    interval_seconds: 60
    output_path: "` + outputPath + `"
`
			if err := os.WriteFile(filepath.Join(paths.ConfigDir, "config.yaml"), []byte(config), 0o644); err != nil {
				t.Fatalf("write config failed: %v", err)
			}
			_, err = ReadRuntimeConfig(paths)
			if err == nil || !strings.Contains(err.Error(), "output_path") {
				t.Fatalf("expected output_path validation error, got %v", err)
			}
		})
	}
}

func TestReadRuntimeConfigParsesNotifyTestJobFields(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance failed: %v", err)
	}
	config := `jobs:
  - name: notify-test
    type: notify_test
    enabled: true
    interval_seconds: 60
    message: "delivery adapter works"
    outbox_path: "work/outbox.jsonl"
`
	if err := os.WriteFile(filepath.Join(paths.ConfigDir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	cfg, err := ReadRuntimeConfig(paths)
	if err != nil {
		t.Fatalf("read runtime config failed: %v", err)
	}
	job := cfg.Jobs[0]
	if job.Name != "notify-test" || job.Type != "notify_test" || job.Message != "delivery adapter works" || job.OutboxPath != "work/outbox.jsonl" || job.Interval != time.Minute {
		t.Fatalf("unexpected notify_test config: %#v", job)
	}
}

func TestReadRuntimeConfigRejectsNotifyTestOutboxOutsideInstanceHome(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance failed: %v", err)
	}
	config := `jobs:
  - name: notify-test
    type: notify_test
    enabled: true
    interval_seconds: 60
    outbox_path: "../outside.jsonl"
`
	if err := os.WriteFile(filepath.Join(paths.ConfigDir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	_, err = ReadRuntimeConfig(paths)
	if err == nil || !strings.Contains(err.Error(), "outbox_path") {
		t.Fatalf("expected outbox_path validation error, got %v", err)
	}
}

func TestHeartbeatJobStateRoundTripAndJSONShape(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	state, err := RunHeartbeatJob(paths, now, 5*time.Minute)
	if err != nil {
		t.Fatalf("run heartbeat job failed: %v", err)
	}
	if state.Name != "heartbeat" || state.Type != "heartbeat" || state.Status != JobSucceeded {
		t.Fatalf("unexpected heartbeat job state: %#v", state)
	}
	if !state.LastRunAt.Equal(now) || !state.LastSuccessAt.Equal(now) || !state.NextRunAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("unexpected heartbeat timing: %#v", state)
	}
	if state.LastError != "" {
		t.Fatalf("expected no error, got %q", state.LastError)
	}

	jobs, err := ReadJobs(paths.JobsPath)
	if err != nil {
		t.Fatalf("read jobs failed: %v", err)
	}
	got := jobs.Jobs["heartbeat"]
	if got.Name != state.Name || got.Type != state.Type || got.Status != state.Status || !got.NextRunAt.Equal(state.NextRunAt) {
		t.Fatalf("unexpected persisted heartbeat job state: %#v", got)
	}

	raw, err := os.ReadFile(paths.JobsPath)
	if err != nil {
		t.Fatalf("read raw jobs failed: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("jobs file is not JSON: %v", err)
	}
	jobsMap, ok := decoded["jobs"].(map[string]any)
	if !ok {
		t.Fatalf("expected top-level jobs object in %s", string(raw))
	}
	heartbeat, ok := jobsMap["heartbeat"].(map[string]any)
	if !ok {
		t.Fatalf("expected heartbeat job in %s", string(raw))
	}
	for _, key := range []string{"name", "type", "status", "last_run_at", "last_success_at", "last_error", "next_run_at"} {
		if _, ok := heartbeat[key]; !ok {
			t.Fatalf("expected heartbeat JSON key %q in %s", key, string(raw))
		}
	}
}

func TestRunConfiguredJobsSkipsDisabledJobsAndMaintainsIndependentState(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	cfg := RuntimeConfig{Jobs: []JobConfig{
		{Name: "fast", Type: "heartbeat", Enabled: true, Interval: time.Minute},
		{Name: "slow", Type: "heartbeat", Enabled: true, Interval: 10 * time.Minute},
		{Name: "off", Type: "heartbeat", Enabled: false, Interval: time.Minute},
	}}
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := RunConfiguredJobs(paths, cfg, now); err != nil {
		t.Fatalf("run configured jobs failed: %v", err)
	}

	jobs, err := ReadJobs(paths.JobsPath)
	if err != nil {
		t.Fatalf("read jobs failed: %v", err)
	}
	if len(jobs.Jobs) != 2 {
		t.Fatalf("expected only enabled jobs to run, got %#v", jobs.Jobs)
	}
	if _, ok := jobs.Jobs["off"]; ok {
		t.Fatalf("disabled job should not have state: %#v", jobs.Jobs["off"])
	}
	if !jobs.Jobs["fast"].NextRunAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("unexpected fast next run: %#v", jobs.Jobs["fast"])
	}
	if !jobs.Jobs["slow"].NextRunAt.Equal(now.Add(10 * time.Minute)) {
		t.Fatalf("unexpected slow next run: %#v", jobs.Jobs["slow"])
	}
}

func TestRunLocalCheckinJobWritesOneJSONRecordPerRun(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := RuntimeConfig{Jobs: []JobConfig{{
		Name:       "daily-checkin",
		Type:       "local_checkin",
		Enabled:    true,
		Interval:   time.Hour,
		Message:    "agent-harness is alive",
		OutputPath: "work/checkins.jsonl",
	}}}

	state, err := RunConfiguredJobNow(paths, cfg, "daily-checkin", now)
	if err != nil {
		t.Fatalf("run local checkin failed: %v", err)
	}
	if state.Status != JobSucceeded || state.OutputPath != "work/checkins.jsonl" {
		t.Fatalf("expected successful checkin state with output path, got %#v", state)
	}
	raw, err := os.ReadFile(filepath.Join(paths.Home, "work", "checkins.jsonl"))
	if err != nil {
		t.Fatalf("expected checkins file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one JSONL record, got %d in %q", len(lines), string(raw))
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("expected JSON record, got %q: %v", lines[0], err)
	}
	if record["job"] != "daily-checkin" || record["message"] != "agent-harness is alive" || record["status"] != "succeeded" || record["timestamp"] != "2026-01-01T12:00:00Z" {
		t.Fatalf("unexpected checkin record: %#v", record)
	}
}

func TestRunLocalCheckinJobUsesSafeDefaults(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := RuntimeConfig{Jobs: []JobConfig{{Name: "daily-checkin", Type: "local_checkin", Enabled: true, Interval: time.Hour}}}

	state, err := RunConfiguredJobNow(paths, cfg, "daily-checkin", now)
	if err != nil {
		t.Fatalf("run local checkin failed: %v", err)
	}
	if state.OutputPath != "work/checkins.jsonl" {
		t.Fatalf("expected default output path, got %#v", state)
	}
	raw, err := os.ReadFile(filepath.Join(paths.Home, state.OutputPath))
	if err != nil {
		t.Fatalf("expected checkins file: %v", err)
	}
	if !strings.Contains(string(raw), `"message":"agent-harness is alive"`) {
		t.Fatalf("expected default message in %s", string(raw))
	}
}

func TestRunLocalCheckinJobRecordsWriteFailure(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := os.MkdirAll(filepath.Join(paths.Home, "work"), 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.Home, "work", "blocked"), []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write blocked file: %v", err)
	}
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := RuntimeConfig{Jobs: []JobConfig{{Name: "daily-checkin", Type: "local_checkin", Enabled: true, Interval: time.Hour, OutputPath: "work/blocked/checkins.jsonl"}}}

	state, err := RunConfiguredJobNow(paths, cfg, "daily-checkin", now)
	if err == nil {
		t.Fatal("expected write failure")
	}
	if state.Status != JobFailed || state.LastError == "" || state.OutputPath != "work/blocked/checkins.jsonl" {
		t.Fatalf("expected failed state with output path and error, got %#v", state)
	}
	jobs, readErr := ReadJobs(paths.JobsPath)
	if readErr != nil {
		t.Fatalf("read jobs failed: %v", readErr)
	}
	if got := jobs.Jobs["daily-checkin"]; got.Status != JobFailed || got.LastError == "" || got.OutputPath != "work/blocked/checkins.jsonl" {
		t.Fatalf("expected persisted failed checkin state, got %#v", got)
	}
}

func TestRunNotifyTestJobWritesOutboxThroughNotifier(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := RuntimeConfig{Jobs: []JobConfig{{
		Name:       "notify-test",
		Type:       "notify_test",
		Enabled:    true,
		Interval:   time.Minute,
		Message:    "delivery adapter works",
		OutboxPath: "work/outbox.jsonl",
	}}}

	state, err := RunConfiguredJobNow(paths, cfg, "notify-test", now)
	if err != nil {
		t.Fatalf("run notify test failed: %v", err)
	}
	if state.Status != JobSucceeded || state.OutputPath != "work/outbox.jsonl" {
		t.Fatalf("expected successful notify_test state with outbox path, got %#v", state)
	}
	raw, err := os.ReadFile(filepath.Join(paths.Home, "work", "outbox.jsonl"))
	if err != nil {
		t.Fatalf("expected outbox file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one outbox record, got %d in %q", len(lines), string(raw))
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("expected JSON outbox record, got %q: %v", lines[0], err)
	}
	if record["job"] != "notify-test" || record["message"] != "delivery adapter works" || record["timestamp"] != "2026-01-01T12:00:00Z" || record["transport"] != "file_outbox" {
		t.Fatalf("unexpected outbox record: %#v", record)
	}
}

func TestNotifyTestJobUsesNotifierInterface(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	notifier := &recordingNotifier{}
	cfg := JobConfig{Name: "notify-test", Type: "notify_test", Enabled: true, Interval: time.Minute, Message: "hello"}

	state, err := runNotifyTestWithNotifier(Paths{}, now, cfg, notifier)
	if err != nil {
		t.Fatalf("run notify test with notifier failed: %v", err)
	}
	if state.Status != JobSucceeded || len(notifier.messages) != 1 || notifier.messages[0].Body != "hello" {
		t.Fatalf("expected notify_test to send through notifier, state=%#v messages=%#v", state, notifier.messages)
	}
}

func TestRunNotifyTestJobRecordsDeliveryFailure(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := os.MkdirAll(filepath.Join(paths.Home, "work"), 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.Home, "work", "blocked"), []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write blocked file: %v", err)
	}
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := RuntimeConfig{Jobs: []JobConfig{{Name: "notify-test", Type: "notify_test", Enabled: true, Interval: time.Minute, OutboxPath: "work/blocked/outbox.jsonl"}}}

	state, err := RunConfiguredJobNow(paths, cfg, "notify-test", now)
	if err == nil {
		t.Fatal("expected delivery failure")
	}
	if state.Status != JobFailed || state.LastError == "" || state.OutputPath != "work/blocked/outbox.jsonl" {
		t.Fatalf("expected failed notify_test state, got %#v", state)
	}
	jobs, readErr := ReadJobs(paths.JobsPath)
	if readErr != nil {
		t.Fatalf("read jobs failed: %v", readErr)
	}
	if got := jobs.Jobs["notify-test"]; got.Status != JobFailed || got.LastError == "" || got.OutputPath != "work/blocked/outbox.jsonl" {
		t.Fatalf("expected persisted failed notify_test state, got %#v", got)
	}
}

func TestRunConfiguredJobsHonorsNextRunAt(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := RuntimeConfig{Jobs: []JobConfig{{Name: "heartbeat", Type: "heartbeat", Enabled: true, Interval: time.Minute}}}
	if err := RunConfiguredJobs(paths, cfg, start); err != nil {
		t.Fatalf("first configured job run failed: %v", err)
	}
	beforeNextRun := start.Add(30 * time.Second)
	if err := RunConfiguredJobs(paths, cfg, beforeNextRun); err != nil {
		t.Fatalf("second configured job run failed: %v", err)
	}
	jobs, err := ReadJobs(paths.JobsPath)
	if err != nil {
		t.Fatalf("read jobs failed: %v", err)
	}
	if !jobs.Jobs["heartbeat"].LastRunAt.Equal(start) {
		t.Fatalf("job should not rerun before next_run_at, got %#v", jobs.Jobs["heartbeat"])
	}

	afterNextRun := start.Add(90 * time.Second)
	if err := RunConfiguredJobs(paths, cfg, afterNextRun); err != nil {
		t.Fatalf("third configured job run failed: %v", err)
	}
	jobs, err = ReadJobs(paths.JobsPath)
	if err != nil {
		t.Fatalf("read jobs failed: %v", err)
	}
	if !jobs.Jobs["heartbeat"].LastRunAt.Equal(afterNextRun) {
		t.Fatalf("job should rerun after next_run_at, got %#v", jobs.Jobs["heartbeat"])
	}
}

func TestRunConfiguredJobNowRecordsFailureState(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := RuntimeConfig{Jobs: []JobConfig{{Name: "broken", Type: "fake", Enabled: true, Interval: 5 * time.Minute}}}

	state, err := runConfiguredJobNowWithRegistry(paths, cfg, "broken", now, map[string]Job{"fake": failingJob{err: "fake failure"}})

	if err == nil || !strings.Contains(err.Error(), "fake failure") {
		t.Fatalf("expected fake failure error, got %v", err)
	}
	if state.Name != "broken" || state.Type != "fake" || state.Status != JobFailed || state.LastError != "fake failure" {
		t.Fatalf("expected failed job state, got %#v", state)
	}
	if !state.LastRunAt.Equal(now) || !state.NextRunAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("expected failure timing to be recorded, got %#v", state)
	}
	jobs, readErr := ReadJobs(paths.JobsPath)
	if readErr != nil {
		t.Fatalf("read jobs failed: %v", readErr)
	}
	if got := jobs.Jobs["broken"]; got.Status != JobFailed || got.LastError != "fake failure" {
		t.Fatalf("expected persisted failed state, got %#v", got)
	}
}

type recordingNotifier struct {
	messages []Message
}

func (r *recordingNotifier) Send(ctx context.Context, message Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.messages = append(r.messages, message)
	return nil
}

type failingJob struct {
	err string
}

func (f failingJob) Run(paths Paths, now time.Time, cfg JobConfig) (JobState, error) {
	return JobState{}, errors.New(f.err)
}

func TestRunDaemonRecordsConfigErrorForInvalidJobConfig(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance failed: %v", err)
	}
	config := `jobs:
  - name: broken
    type: email
    enabled: true
    interval_seconds: 60
`
	if err := os.WriteFile(filepath.Join(paths.ConfigDir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	err = RunDaemon(context.Background(), paths, "default", version.Metadata{Version: "dev"}, time.Minute)
	if err == nil || !strings.Contains(err.Error(), `unknown job type "email"`) {
		t.Fatalf("expected daemon config error, got %v", err)
	}
	status, readErr := ReadStatus(paths.StatusPath)
	if readErr != nil {
		t.Fatalf("read status failed: %v", readErr)
	}
	if status.Status != StateConfigError || !strings.Contains(status.Error, `unknown job type "email"`) {
		t.Fatalf("expected config-error status, got %#v", status)
	}
}

func TestStatusRoundTripAndJSONShape(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	started := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	status := Status{
		Instance:        "default",
		Status:          StateRunning,
		StartedAt:       started,
		LastHeartbeatAt: started.Add(time.Minute),
		PID:             12345,
		Version:         "0.1.0-dev",
		Commit:          "abc1234",
		BuildDate:       "2026-10-01T14:00:00Z",
		Dirty:           "false",
	}

	if err := WriteStatus(paths.StatusPath, status); err != nil {
		t.Fatalf("write status failed: %v", err)
	}
	got, err := ReadStatus(paths.StatusPath)
	if err != nil {
		t.Fatalf("read status failed: %v", err)
	}
	if got.Instance != status.Instance || got.Status != status.Status || got.PID != status.PID || got.Version != status.Version || got.Commit != status.Commit || got.BuildDate != status.BuildDate || got.Dirty != status.Dirty {
		t.Fatalf("unexpected status round trip: %#v", got)
	}

	raw, err := os.ReadFile(paths.StatusPath)
	if err != nil {
		t.Fatalf("read raw status failed: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("status file is not JSON: %v", err)
	}
	for _, key := range []string{"instance", "status", "started_at", "last_heartbeat_at", "pid", "version", "commit", "build_date", "dirty"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("expected status JSON key %q in %s", key, string(raw))
		}
	}
}

func TestClassifyStatusDetectsMissingStaleAndRunning(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if got := ClassifyStatus(Status{}, now, time.Minute); got != StateUnknown {
		t.Fatalf("expected unknown for empty status, got %q", got)
	}
	if got := ClassifyStatus(Status{Status: StateStopped}, now, time.Minute); got != StateStopped {
		t.Fatalf("expected stopped to remain stopped, got %q", got)
	}
	fresh := Status{Status: StateRunning, LastHeartbeatAt: now.Add(-30 * time.Second)}
	if got := ClassifyStatus(fresh, now, time.Minute); got != StateRunning {
		t.Fatalf("expected fresh running status, got %q", got)
	}
	stale := Status{Status: StateRunning, LastHeartbeatAt: now.Add(-2 * time.Minute)}
	if got := ClassifyStatus(stale, now, time.Minute); got != StateStale {
		t.Fatalf("expected stale status, got %q", got)
	}
}

func TestSystemdUserUnitUsesInstanceHomeAndNoMachineSpecificPaths(t *testing.T) {
	unit := RenderSystemdUserUnit()
	for _, want := range []string{
		"ExecStart=%h/.local/bin/agent-harness daemon --instance %i --home %h/.local/share/agent-harness/instances/%i",
		"WorkingDirectory=%h/.local/share/agent-harness/instances/%i",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("expected unit to contain %q, got:\n%s", want, unit)
		}
	}
	if strings.Contains(unit, "/home/") {
		t.Fatalf("unit must not contain machine-specific home paths:\n%s", unit)
	}
}

func assertPath(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("expected path %q, got %q", want, got)
	}
}
