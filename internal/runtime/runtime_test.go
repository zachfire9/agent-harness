package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zachfire9/agent-harness/internal/llm"
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
	secretsInfo, err := os.Stat(filepath.Join(paths.ConfigDir, "secrets"))
	if err != nil {
		t.Fatalf("expected google secrets directory: %v", err)
	}
	if secretsInfo.Mode().Perm() != 0o700 {
		t.Fatalf("expected secrets directory mode 0700, got %o", secretsInfo.Mode().Perm())
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

func TestReadRuntimeConfigParsesDailySchedule(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	writeRuntimeTestFile(t, paths.ConfigDir, "config.yaml", `jobs:
  - name: daily-heartbeat
    type: heartbeat
    enabled: true
    schedule:
      daily_at: "08:00"
      timezone: "America/New_York"
`)

	cfg, err := ReadRuntimeConfig(paths)
	if err != nil {
		t.Fatalf("read runtime config failed: %v", err)
	}
	if len(cfg.Jobs) != 1 {
		t.Fatalf("expected one job, got %#v", cfg.Jobs)
	}
	job := cfg.Jobs[0]
	if job.Schedule.DailyAt != "08:00" || job.Schedule.Timezone != "America/New_York" || job.Interval != 0 {
		t.Fatalf("unexpected daily schedule config: %#v", job)
	}
}

func TestReadRuntimeConfigRejectsInvalidDailySchedule(t *testing.T) {
	for name, config := range map[string]string{
		"bad-time": `jobs:
  - name: daily-heartbeat
    type: heartbeat
    enabled: true
    schedule:
      daily_at: "25:00"
      timezone: "America/New_York"
`,
		"bad-timezone": `jobs:
  - name: daily-heartbeat
    type: heartbeat
    enabled: true
    schedule:
      daily_at: "08:00"
      timezone: "Mars/Base"
`,
		"interval-and-schedule": `jobs:
  - name: daily-heartbeat
    type: heartbeat
    enabled: true
    interval_seconds: 60
    schedule:
      daily_at: "08:00"
      timezone: "America/New_York"
`,
	} {
		t.Run(name, func(t *testing.T) {
			paths, err := LinuxPaths(t.TempDir(), "default")
			if err != nil {
				t.Fatalf("expected paths, got %v", err)
			}
			writeRuntimeTestFile(t, paths.ConfigDir, "config.yaml", config)
			_, err = ReadRuntimeConfig(paths)
			if err == nil || !strings.Contains(err.Error(), "schedule") {
				t.Fatalf("expected schedule validation error, got %v", err)
			}
		})
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

func TestReadRuntimeConfigParsesProviderScopedLLMKeysAndAIEmailModelOverrides(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	writeRuntimeTestFile(t, paths.ConfigDir, "config.yaml", `llms:
  default:
    provider: openrouter
    model: openai/gpt-4o-mini
  providers:
    - provider: openrouter
      api_key_env: OPENROUTER_API_KEY
      models:
        - openai/gpt-4o-mini
        - anthropic/claude-3-5-haiku-latest
    - provider: openai
      api_key_env: OPENAI_API_KEY
      models:
        - gpt-4o-mini
        - gpt-4.1-mini
jobs:
  - name: daily-ai-email
    type: ai_email
    enabled: true
    interval_seconds: 86400
    prompt: "Write a concise daily learning note."
    max_chars: 1200
  - name: weekly-ai-email
    type: ai_email
    enabled: true
    interval_seconds: 604800
    prompt: "Write a practical software engineering tip."
    max_chars: 1500
    llm:
      provider: openai
      model: gpt-4o-mini
`)

	cfg, err := ReadRuntimeConfig(paths)
	if err != nil {
		t.Fatalf("expected config to parse, got %v", err)
	}
	if cfg.LLMs.Default.Provider != "openrouter" || cfg.LLMs.Default.Model != "openai/gpt-4o-mini" {
		t.Fatalf("unexpected default llm selector: %#v", cfg.LLMs.Default)
	}
	if got := cfg.Jobs[0].ResolvedLLM(); got.Profile != "openrouter" || got.Provider != "openrouter" || got.Model != "openai/gpt-4o-mini" || got.APIKeyEnv != "OPENROUTER_API_KEY" {
		t.Fatalf("expected first job to use default provider token and model, got %#v", got)
	}
	if cfg.Jobs[0].LLMOverride != (LLMSelector{}) {
		t.Fatalf("expected first job to inherit default selector without per-job key material, got %#v", cfg.Jobs[0])
	}
	if cfg.Jobs[0].Prompt != "Write a concise daily learning note." || cfg.Jobs[0].MaxChars != 1200 {
		t.Fatalf("unexpected ai_email fields: %#v", cfg.Jobs[0])
	}
	if got := cfg.Jobs[1].ResolvedLLM(); got.Profile != "openai" || got.Provider != "openai" || got.Model != "gpt-4o-mini" || got.APIKeyEnv != "OPENAI_API_KEY" {
		t.Fatalf("expected second job to use provider/model override with provider token, got %#v", got)
	}
	if cfg.Jobs[1].LLMOverride.Provider != "openai" || cfg.Jobs[1].LLMOverride.Model != "gpt-4o-mini" {
		t.Fatalf("expected job override to store only provider/model selector, got %#v", cfg.Jobs[1])
	}
}

func TestReadRuntimeConfigRejectsDuplicateLLMProviderWithoutExplicitProfile(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	writeRuntimeTestFile(t, paths.ConfigDir, "config.yaml", `llms:
  default:
    provider: openai
    model: gpt-4o-mini
  providers:
    - provider: openai
      api_key_env: OPENAI_API_KEY
      models:
        - gpt-4o-mini
    - provider: openai
      api_key_env: OPENAI_OTHER_API_KEY
      models:
        - gpt-4.1-mini
jobs:
  - name: daily-ai-email
    type: ai_email
    enabled: true
    interval_seconds: 86400
    prompt: "Write a note."
    max_chars: 1200
`)

	_, err = ReadRuntimeConfig(paths)
	if err == nil || !strings.Contains(err.Error(), "duplicate llm provider profile \"openai\"") {
		t.Fatalf("expected duplicate provider profile error, got %v", err)
	}
}

func TestReadRuntimeConfigRejectsAIEmailWithoutUsableLLMConfig(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	writeRuntimeTestFile(t, paths.ConfigDir, "config.yaml", `jobs:
  - name: daily-ai-email
    type: ai_email
    enabled: true
    interval_seconds: 86400
    prompt: "Write a note."
    max_chars: 1200
`)

	_, err = ReadRuntimeConfig(paths)
	if err == nil || !strings.Contains(err.Error(), "ai_email job \"daily-ai-email\" requires llm provider") {
		t.Fatalf("expected missing llm config error, got %v", err)
	}
}

func TestReadRuntimeConfigRejectsAIEmailPromptAndMaxCharsErrors(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	writeRuntimeTestFile(t, paths.ConfigDir, "config.yaml", `llms:
  default:
    provider: openrouter
    model: openai/gpt-4o-mini
  providers:
    - provider: openrouter
      api_key_env: OPENROUTER_API_KEY
      models:
        - openai/gpt-4o-mini
jobs:
  - name: daily-ai-email
    type: ai_email
    enabled: true
    interval_seconds: 86400
    max_chars: 0
`)

	_, err = ReadRuntimeConfig(paths)
	if err == nil || !strings.Contains(err.Error(), "ai_email job \"daily-ai-email\" requires prompt") {
		t.Fatalf("expected prompt validation error, got %v", err)
	}
}

func TestReadRuntimeConfigParsesGmailNotifierConfig(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance failed: %v", err)
	}
	config := `google:
  client_credentials_path: "config/secrets/google-client.json"
  token_path: "config/secrets/google-token.json"
  account_hint: "agent@example.com"
  scope_profile: "gmail_send"
notifier:
  type: gmail
  gmail:
    from: "agent@example.com"
    to:
      - "operator@example.com"
    subject_prefix: "[agent-harness]"
jobs:
  - name: notify-test
    type: notify_test
    enabled: true
    interval_seconds: 60
    message: "delivery adapter works"
`
	if err := os.WriteFile(filepath.Join(paths.ConfigDir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	cfg, err := ReadRuntimeConfig(paths)
	if err != nil {
		t.Fatalf("read runtime config failed: %v", err)
	}
	if cfg.Notifier.Type != "gmail" || cfg.Notifier.Gmail.From != "agent@example.com" || cfg.Notifier.Gmail.SubjectPrefix != "[agent-harness]" {
		t.Fatalf("unexpected notifier config: %#v", cfg.Notifier)
	}
	if len(cfg.Notifier.Gmail.To) != 1 || cfg.Notifier.Gmail.To[0] != "operator@example.com" {
		t.Fatalf("unexpected gmail recipients: %#v", cfg.Notifier.Gmail.To)
	}
	if cfg.Jobs[0].Notifier.Type != "gmail" {
		t.Fatalf("expected notify_test job to inherit notifier config, got %#v", cfg.Jobs[0].Notifier)
	}
}

func TestReadRuntimeConfigRejectsGmailNotifierWithoutGmailScope(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance failed: %v", err)
	}
	config := `google:
  token_path: "config/secrets/google-token.json"
  scope_profile: "docs_readonly"
notifier:
  type: gmail
  gmail:
    from: "agent@example.com"
    to:
      - "operator@example.com"
`
	if err := os.WriteFile(filepath.Join(paths.ConfigDir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	_, err = ReadRuntimeConfig(paths)
	if err == nil || !strings.Contains(err.Error(), "gmail notifier requires google scope_profile gmail_send") {
		t.Fatalf("expected gmail scope validation error, got %v", err)
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

func TestRunAIEmailWithNotifierUsesFakeLLMAndBoundsOutput(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	notifier := &recordingNotifier{}
	client := &recordingLLMClient{response: "0123456789abcdef"}
	cfg := JobConfig{
		Name:     "daily-ai-email",
		Type:     "ai_email",
		Enabled:  true,
		Interval: time.Hour,
		Prompt:   "Write one sentence.",
		MaxChars: 12,
		LLM:      LLMConfig{Provider: "openrouter", Model: "openai/gpt-4o-mini", APIKeyEnv: "OPENROUTER_API_KEY"},
	}

	state, err := runAIEmailWithClients(Paths{}, now, cfg, notifier, client)
	if err != nil {
		t.Fatalf("run ai email failed: %v", err)
	}
	if state.Status != JobSucceeded || state.LastError != "" {
		t.Fatalf("unexpected state: %#v", state)
	}
	if len(notifier.messages) != 1 {
		t.Fatalf("expected one notification, got %d", len(notifier.messages))
	}
	if notifier.messages[0].Body != "0123456789ab" {
		t.Fatalf("expected bounded generated body, got %q", notifier.messages[0].Body)
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected one LLM request, got %d", len(client.requests))
	}
	if client.requests[0].Model != "openai/gpt-4o-mini" {
		t.Fatalf("expected configured model, got %q", client.requests[0].Model)
	}
	if got := client.requests[0].Messages[len(client.requests[0].Messages)-1].Content; got != "Write one sentence." {
		t.Fatalf("expected prompt in request, got %q", got)
	}
}

func TestRunConfiguredAIEmailRecordsSafeLLMFailure(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := RuntimeConfig{Jobs: []JobConfig{{
		Name:     "daily-ai-email",
		Type:     "ai_email",
		Enabled:  true,
		Interval: time.Hour,
		Prompt:   "Do not leak secrets.",
		MaxChars: 100,
		LLM:      LLMConfig{Provider: "openrouter", Model: "openai/gpt-4o-mini", APIKeyEnv: "SECRET_API_KEY"},
	}}}
	secretErr := errors.New("provider failed with api key *** and full provider body")
	job := aiEmailJob{
		clientFactory: func(cfg LLMConfig) (llm.ChatClient, error) {
			return &recordingLLMClient{err: secretErr}, nil
		},
		notifierFactory: func(paths Paths, cfg JobConfig) Notifier {
			return &recordingNotifier{}
		},
	}

	state, err := runConfiguredJobNowWithRegistry(paths, cfg, "daily-ai-email", now, map[string]Job{"ai_email": job})
	if err == nil || !strings.Contains(err.Error(), "llm generation failed") {
		t.Fatalf("expected safe llm failure, got %v", err)
	}
	if strings.Contains(err.Error(), "***") || strings.Contains(state.LastError, "***") || strings.Contains(state.LastError, "full provider body") {
		t.Fatalf("expected secret-safe error, got err=%v state=%#v", err, state)
	}
	jobs, err := ReadJobs(paths.JobsPath)
	if err != nil {
		t.Fatalf("read jobs: %v", err)
	}
	persisted := jobs.Jobs["daily-ai-email"]
	if persisted.Status != JobFailed || persisted.LastError != "llm generation failed" {
		t.Fatalf("expected persisted safe failure, got %#v", persisted)
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

func TestGmailNotifierSendsThroughGmailAPIWithBearerToken(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	writeRuntimeTestFile(t, paths.Home, "config/secrets/google-token.json", `{"access_token":"ya29.test-token","scope":"https://www.googleapis.com/auth/gmail.send","expiry":"2099-01-02T03:04:05Z"}`)
	var gotPath, gotAuth string
	var gotRequest map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode gmail request: %v", err)
		}
		fmt.Fprintln(w, `{"id":"gmail-message-id"}`)
	}))
	defer server.Close()

	notifier := gmailNotifier{
		paths:    paths,
		google:   GoogleConfig{TokenPath: "config/secrets/google-token.json", ScopeProfile: "gmail_send"},
		config:   GmailNotifierConfig{From: "agent@example.com", To: []string{"operator@example.com"}, SubjectPrefix: "[agent-harness]"},
		endpoint: server.URL,
		client:   server.Client(),
	}
	message := Message{Job: "notify-test", Body: "delivery adapter works", Timestamp: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}

	if err := notifier.Send(context.Background(), message); err != nil {
		t.Fatalf("gmail send failed: %v", err)
	}
	if gotPath != "/gmail/v1/users/me/messages/send" || gotAuth != "Bearer ya29.test-token" {
		t.Fatalf("unexpected gmail request path/auth: path=%q auth=%q", gotPath, gotAuth)
	}
	if gotRequest["raw"] == "" || strings.Contains(gotRequest["raw"], "delivery adapter works") {
		t.Fatalf("expected compact encoded raw gmail payload without plain body leak, got %#v", gotRequest)
	}
}

func TestRunNotifyTestJobWithGmailNotifierRecordsSafeMissingTokenError(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := RuntimeConfig{
		Google:   GoogleConfig{TokenPath: "config/secrets/google-token.json", ScopeProfile: "gmail_send"},
		Notifier: NotifierConfig{Type: "gmail", Gmail: GmailNotifierConfig{From: "agent@example.com", To: []string{"operator@example.com"}}},
		Jobs:     []JobConfig{{Name: "notify-test", Type: "notify_test", Enabled: true, Interval: time.Minute, Message: "secret message body"}},
	}
	cfg.attachNotifierToJobs()

	state, err := RunConfiguredJobNow(paths, cfg, "notify-test", now)
	if err == nil || !strings.Contains(err.Error(), "google token is missing") {
		t.Fatalf("expected missing token error, got %v", err)
	}
	if state.Status != JobFailed || state.LastError == "" || state.OutputPath != "" || strings.Contains(state.LastError, "secret message body") || strings.Contains(state.LastError, "access_token") {
		t.Fatalf("expected safe failed state, got %#v", state)
	}
}

func TestGmailNotifierRefreshesExpiredTokenBeforeSending(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	writeRuntimeTestFile(t, paths.Home, "config/secrets/google-token.json", `{"access_token":"ya29.expired-token","refresh_token":"refresh-token","scope":"https://www.googleapis.com/auth/gmail.send","expiry":"2000-01-02T03:04:05Z"}`)
	writeRuntimeTestFile(t, paths.Home, "config/secrets/google-client.json", `{"installed":{"client_id":"client-id","client_secret":"client-secret"}}`)
	var gotTokenRefresh bool
	var gotSendAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			gotTokenRefresh = true
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token refresh form: %v", err)
			}
			if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "refresh-token" || r.Form.Get("client_id") != "client-id" || r.Form.Get("client_secret") != "client-secret" {
				t.Fatalf("unexpected refresh form: %#v", r.Form)
			}
			fmt.Fprintln(w, `{"access_token":"ya29.fresh-token","expires_in":3600,"scope":"https://www.googleapis.com/auth/gmail.send"}`)
		case "/gmail/v1/users/me/messages/send":
			gotSendAuth = r.Header.Get("Authorization")
			fmt.Fprintln(w, `{"id":"gmail-message-id"}`)
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()
	notifier := gmailNotifier{
		paths:         paths,
		google:        GoogleConfig{ClientCredentialsPath: "config/secrets/google-client.json", TokenPath: "config/secrets/google-token.json", ScopeProfile: "gmail_send"},
		config:        GmailNotifierConfig{From: "agent@example.com", To: []string{"operator@example.com"}},
		endpoint:      server.URL,
		oauthEndpoint: server.URL + "/token",
		client:        server.Client(),
	}

	if err := notifier.Send(context.Background(), Message{Job: "notify-test", Body: "hello", Timestamp: time.Now()}); err != nil {
		t.Fatalf("gmail send failed after refresh: %v", err)
	}
	if !gotTokenRefresh || gotSendAuth != "Bearer ya29.fresh-token" {
		t.Fatalf("expected refresh then send with fresh token, refreshed=%t auth=%q", gotTokenRefresh, gotSendAuth)
	}
	raw, err := os.ReadFile(filepath.Join(paths.Home, "config", "secrets", "google-token.json"))
	if err != nil {
		t.Fatalf("read refreshed token: %v", err)
	}
	if strings.Contains(string(raw), "ya29.expired-token") || !strings.Contains(string(raw), "ya29.fresh-token") || !strings.Contains(string(raw), "refresh-token") {
		t.Fatalf("expected token file to preserve refresh token and store fresh access token, got %s", string(raw))
	}
}

func TestGmailNotifierFailsSafelyWhenExpiredTokenHasNoRefreshToken(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	writeRuntimeTestFile(t, paths.Home, "config/secrets/google-token.json", `{"access_token":"ya29.expired-token","scope":"https://www.googleapis.com/auth/gmail.send","expiry":"2000-01-02T03:04:05Z"}`)
	notifier := gmailNotifier{
		paths:  paths,
		google: GoogleConfig{TokenPath: "config/secrets/google-token.json", ScopeProfile: "gmail_send"},
		config: GmailNotifierConfig{From: "agent@example.com", To: []string{"operator@example.com"}},
	}

	err = notifier.Send(context.Background(), Message{Job: "notify-test", Body: "hello", Timestamp: time.Now()})
	if err == nil || !strings.Contains(err.Error(), "google token cannot be refreshed") {
		t.Fatalf("expected cannot refresh error, got %v", err)
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

func TestRunConfiguredJobsComputesDailyScheduleForLaterToday(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	now := time.Date(2026, 3, 10, 7, 30, 0, 0, loc)
	wantNext := time.Date(2026, 3, 10, 8, 0, 0, 0, loc).UTC()
	cfg := RuntimeConfig{Jobs: []JobConfig{{Name: "daily-heartbeat", Type: "heartbeat", Enabled: true, Schedule: JobSchedule{DailyAt: "08:00", Timezone: "America/New_York"}}}}

	if err := RunConfiguredJobs(paths, cfg, now); err != nil {
		t.Fatalf("run configured jobs failed: %v", err)
	}
	jobs, err := ReadJobs(paths.JobsPath)
	if err != nil {
		t.Fatalf("read jobs failed: %v", err)
	}
	if !jobs.Jobs["daily-heartbeat"].NextRunAt.Equal(wantNext) {
		t.Fatalf("expected next run %s, got %#v", wantNext, jobs.Jobs["daily-heartbeat"])
	}
}

func TestRunConfiguredJobsComputesDailyScheduleForTomorrowWhenTimePassed(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	now := time.Date(2026, 3, 10, 8, 30, 0, 0, loc)
	wantNext := time.Date(2026, 3, 11, 8, 0, 0, 0, loc).UTC()
	cfg := RuntimeConfig{Jobs: []JobConfig{{Name: "daily-heartbeat", Type: "heartbeat", Enabled: true, Schedule: JobSchedule{DailyAt: "08:00", Timezone: "America/New_York"}}}}

	if err := RunConfiguredJobs(paths, cfg, now); err != nil {
		t.Fatalf("run configured jobs failed: %v", err)
	}
	jobs, err := ReadJobs(paths.JobsPath)
	if err != nil {
		t.Fatalf("read jobs failed: %v", err)
	}
	if !jobs.Jobs["daily-heartbeat"].NextRunAt.Equal(wantNext) {
		t.Fatalf("expected next run %s, got %#v", wantNext, jobs.Jobs["daily-heartbeat"])
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

type recordingLLMClient struct {
	requests []llm.ChatRequest
	response string
	err      error
}

func (c *recordingLLMClient) Chat(ctx context.Context, request llm.ChatRequest) (llm.ChatResponse, error) {
	if err := ctx.Err(); err != nil {
		return llm.ChatResponse{}, err
	}
	c.requests = append(c.requests, request)
	if c.err != nil {
		return llm.ChatResponse{}, c.err
	}
	return llm.ChatResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: c.response}}, nil
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

func TestReadRuntimeConfigParsesGoogleConfig(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance failed: %v", err)
	}
	config := `google:
  client_credentials_path: "config/secrets/google-client.json"
  token_path: "config/secrets/google-token.json"
  account_hint: "agent@example.com"
  scope_profile: "gmail_send"
jobs:
  - name: heartbeat
    type: heartbeat
    enabled: true
    interval_seconds: 60
`
	if err := os.WriteFile(filepath.Join(paths.ConfigDir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	cfg, err := ReadRuntimeConfig(paths)
	if err != nil {
		t.Fatalf("read runtime config failed: %v", err)
	}
	if cfg.Google.ClientCredentialsPath != "config/secrets/google-client.json" || cfg.Google.TokenPath != "config/secrets/google-token.json" || cfg.Google.AccountHint != "agent@example.com" || cfg.Google.ScopeProfile != "gmail_send" {
		t.Fatalf("unexpected google config: %#v", cfg.Google)
	}
	if got := cfg.Google.Scopes(); len(got) != 1 || got[0] != GoogleScopeGmailSend {
		t.Fatalf("unexpected gmail scopes: %#v", got)
	}
}

func TestReadRuntimeConfigRejectsUnknownGoogleScopeProfile(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance failed: %v", err)
	}
	config := `google:
  client_credentials_path: "config/secrets/google-client.json"
  token_path: "config/secrets/google-token.json"
  scope_profile: "drive_all"
`
	if err := os.WriteFile(filepath.Join(paths.ConfigDir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	_, err = ReadRuntimeConfig(paths)
	if err == nil || !strings.Contains(err.Error(), `unknown google scope_profile "drive_all"`) {
		t.Fatalf("expected unknown google scope error, got %v", err)
	}
}

func TestReadRuntimeConfigRejectsGoogleSecretPathOutsideInstanceHome(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance failed: %v", err)
	}
	config := `google:
  client_credentials_path: "/tmp/google-client.json"
  token_path: "../google-token.json"
  scope_profile: "docs_readonly"
`
	if err := os.WriteFile(filepath.Join(paths.ConfigDir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	_, err = ReadRuntimeConfig(paths)
	if err == nil || !strings.Contains(err.Error(), `invalid google client_credentials_path`) {
		t.Fatalf("expected invalid google secret path error, got %v", err)
	}
}

func TestGoogleAuthStatusRedactsTokenValues(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance failed: %v", err)
	}
	cfg := GoogleConfig{TokenPath: "config/secrets/google-token.json", ScopeProfile: "gmail_send"}
	secretToken := "ya29.secret-access-token"
	secretRefresh := "1//secret-refresh-token"
	writeRuntimeTestFile(t, paths.Home, cfg.TokenPath, `{"access_token":"`+secretToken+`","refresh_token":"`+secretRefresh+`","expiry":"2026-01-02T03:04:05Z","scope":"https://www.googleapis.com/auth/gmail.send"}`)

	status, err := GoogleAuthStatus(paths, cfg)
	if err != nil {
		t.Fatalf("google auth status failed: %v", err)
	}
	if !status.Connected || status.TokenExpiry != "2026-01-02T03:04:05Z" || status.ScopeProfile != "gmail_send" || status.TokenPath != cfg.TokenPath {
		t.Fatalf("unexpected google auth status: %#v", status)
	}
	rendered := status.SafeString()
	if strings.Contains(rendered, secretToken) || strings.Contains(rendered, secretRefresh) || strings.Contains(rendered, "access_token") || strings.Contains(rendered, "refresh_token") {
		t.Fatalf("google auth status leaked token material: %q", rendered)
	}
}

func TestRevokeGoogleTokenRemovesOnlyTokenFile(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatalf("expected paths, got %v", err)
	}
	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance failed: %v", err)
	}
	cfg := GoogleConfig{ClientCredentialsPath: "config/secrets/google-client.json", TokenPath: "config/secrets/google-token.json", ScopeProfile: "gmail_send"}
	writeRuntimeTestFile(t, paths.Home, cfg.ClientCredentialsPath, `{"installed":{"client_id":"placeholder"}}`)
	writeRuntimeTestFile(t, paths.Home, cfg.TokenPath, `{"access_token":"secret"}`)

	if err := RevokeGoogleToken(paths, cfg); err != nil {
		t.Fatalf("revoke google token failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.Home, filepath.FromSlash(cfg.TokenPath))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected token file removed, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.Home, filepath.FromSlash(cfg.ClientCredentialsPath))); err != nil {
		t.Fatalf("client credentials should remain: %v", err)
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

func writeRuntimeTestFile(t *testing.T, root string, relativePath string, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func assertPath(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("expected path %q, got %q", want, got)
	}
}
