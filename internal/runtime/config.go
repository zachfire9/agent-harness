package runtime

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultHeartbeatJobInterval = time.Minute

type JobConfig struct {
	Name       string
	Type       string
	Enabled    bool
	Interval   time.Duration
	Message    string
	OutputPath string
	OutboxPath string
}

type RuntimeConfig struct {
	Jobs   []JobConfig
	Google GoogleConfig
}

func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{Jobs: []JobConfig{{
		Name:     "heartbeat",
		Type:     "heartbeat",
		Enabled:  true,
		Interval: defaultHeartbeatJobInterval,
	}}}
}

func ReadRuntimeConfig(paths Paths) (RuntimeConfig, error) {
	cfg := DefaultRuntimeConfig()
	configPath := filepath.Join(paths.ConfigDir, "config.yaml")
	file, err := os.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return RuntimeConfig{}, err
	}
	defer file.Close()

	parsedJobs, googleConfig, sawJobs, legacyInterval, err := parseRuntimeConfig(file)
	if err != nil {
		return RuntimeConfig{}, err
	}
	cfg.Google = googleConfig
	if sawJobs {
		cfg.Jobs = parsedJobs
	} else if legacyInterval > 0 {
		cfg.Jobs[0].Interval = legacyInterval
	}
	if err := validateRuntimeConfig(cfg); err != nil {
		return RuntimeConfig{}, err
	}
	return cfg, nil
}

func parseRuntimeConfig(file *os.File) ([]JobConfig, GoogleConfig, bool, time.Duration, error) {
	var jobs []JobConfig
	var current *JobConfig
	var google GoogleConfig
	var sawJobs bool
	var legacyInterval time.Duration
	var section string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == "google:" {
			section = "google"
			continue
		}
		if line == "jobs:" {
			section = "jobs"
			sawJobs = true
			continue
		}
		if strings.HasPrefix(line, "- ") {
			if section != "jobs" {
				continue
			}
			if current != nil {
				jobs = append(jobs, *current)
			}
			current = &JobConfig{Enabled: true, Interval: defaultHeartbeatJobInterval}
			line = strings.TrimSpace(strings.TrimPrefix(line, "- "))
			if line == "" {
				continue
			}
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)

		if !sawJobs && key == "heartbeat_job_interval_seconds" {
			seconds, err := parsePositiveSeconds(value, "heartbeat_job_interval_seconds")
			if err != nil {
				return nil, GoogleConfig{}, false, 0, err
			}
			legacyInterval = seconds
			continue
		}
		if section == "google" {
			switch key {
			case "client_credentials_path":
				google.ClientCredentialsPath = value
			case "token_path":
				google.TokenPath = value
			case "account_hint":
				google.AccountHint = value
			case "scope_profile":
				google.ScopeProfile = value
			}
			continue
		}
		if section != "jobs" || !sawJobs || current == nil {
			continue
		}
		switch key {
		case "name":
			current.Name = value
		case "type":
			current.Type = value
		case "enabled":
			enabled, err := strconv.ParseBool(value)
			if err != nil {
				return nil, google, true, 0, fmt.Errorf("invalid enabled for job %q: %q", current.Name, value)
			}
			current.Enabled = enabled
		case "interval_seconds":
			interval, err := parsePositiveSeconds(value, "interval_seconds")
			if err != nil {
				return nil, google, true, 0, err
			}
			current.Interval = interval
		case "message":
			current.Message = value
		case "output_path":
			current.OutputPath = value
		case "outbox_path":
			current.OutboxPath = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, google, sawJobs, legacyInterval, err
	}
	if current != nil {
		jobs = append(jobs, *current)
	}
	return jobs, google, sawJobs, legacyInterval, nil
}

func parsePositiveSeconds(value, field string) (time.Duration, error) {
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("invalid %s %q", field, value)
	}
	return time.Duration(seconds) * time.Second, nil
}

func validateRuntimeConfig(cfg RuntimeConfig) error {
	if err := cfg.Google.Validate(); err != nil {
		return err
	}
	seen := map[string]bool{}
	for i, job := range cfg.Jobs {
		if strings.TrimSpace(job.Name) == "" {
			return fmt.Errorf("job %d missing name", i)
		}
		if seen[job.Name] {
			return fmt.Errorf("duplicate job name %q", job.Name)
		}
		seen[job.Name] = true
		if job.Type != "heartbeat" && job.Type != "local_checkin" && job.Type != "notify_test" {
			return fmt.Errorf("unknown job type %q for job %q", job.Type, job.Name)
		}
		if job.Interval <= 0 {
			return fmt.Errorf("invalid interval_seconds for job %q", job.Name)
		}
		if job.Type == "local_checkin" {
			if err := validateInstanceRelativePath(job.OutputPath); err != nil {
				return fmt.Errorf("invalid output_path for job %q: %w", job.Name, err)
			}
		}
		if job.Type == "notify_test" {
			if err := validateInstanceRelativePath(job.OutboxPath); err != nil {
				return fmt.Errorf("invalid outbox_path for job %q: %w", job.Name, err)
			}
		}
	}
	return nil
}

func validateInstanceRelativePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if filepath.IsAbs(path) {
		return fmt.Errorf("must be relative")
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("must stay under instance home")
	}
	return nil
}
