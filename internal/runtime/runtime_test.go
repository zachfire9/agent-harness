package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestReadRuntimeConfigParsesHeartbeatJobInterval(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance failed: %v", err)
	}
	configPath := filepath.Join(paths.ConfigDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("heartbeat_job_interval_seconds: 300\n"), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	cfg, err := ReadRuntimeConfig(paths)
	if err != nil {
		t.Fatalf("read runtime config failed: %v", err)
	}
	if cfg.HeartbeatJobInterval != 5*time.Minute {
		t.Fatalf("expected 5 minute heartbeat interval, got %s", cfg.HeartbeatJobInterval)
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
