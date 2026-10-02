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

type LLMConfig struct {
	Name      string
	Provider  string
	Model     string
	APIKeyEnv string
	BaseURL   string
}

func (c LLMConfig) IsZero() bool {
	return strings.TrimSpace(c.Name) == "" && strings.TrimSpace(c.Provider) == "" && strings.TrimSpace(c.Model) == "" && strings.TrimSpace(c.APIKeyEnv) == "" && strings.TrimSpace(c.BaseURL) == ""
}

type LLMRegistryConfig struct {
	Default  string
	Profiles map[string]LLMConfig
}

func (c LLMRegistryConfig) IsZero() bool {
	return strings.TrimSpace(c.Default) == "" && len(c.Profiles) == 0
}

type JobConfig struct {
	Name       string
	Type       string
	Enabled    bool
	Interval   time.Duration
	Message    string
	Prompt     string
	MaxChars   int
	OutputPath string
	OutboxPath string
	Notifier   NotifierConfig
	Google     GoogleConfig
	LLMProfile string
	LLM        LLMConfig
}

func (job JobConfig) ResolvedLLM() LLMConfig {
	return job.LLM
}

type RuntimeConfig struct {
	Jobs     []JobConfig
	Google   GoogleConfig
	Notifier NotifierConfig
	LLMs     LLMRegistryConfig
}

type NotifierConfig struct {
	Type  string
	Gmail GmailNotifierConfig
}

type GmailNotifierConfig struct {
	From          string
	To            []string
	SubjectPrefix string
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

	parsedJobs, googleConfig, notifierConfig, llmRegistry, sawJobs, legacyInterval, err := parseRuntimeConfig(file)
	if err != nil {
		return RuntimeConfig{}, err
	}
	cfg.Google = googleConfig
	cfg.Notifier = notifierConfig
	cfg.LLMs = llmRegistry
	if sawJobs {
		cfg.Jobs = parsedJobs
	} else if legacyInterval > 0 {
		cfg.Jobs[0].Interval = legacyInterval
	}
	cfg.attachNotifierToJobs()
	if err := validateRuntimeConfig(cfg); err != nil {
		return RuntimeConfig{}, err
	}
	return cfg, nil
}

func (cfg *RuntimeConfig) attachNotifierToJobs() {
	for i := range cfg.Jobs {
		cfg.Jobs[i].Notifier = cfg.Notifier
		cfg.Jobs[i].Google = cfg.Google
		profileName := cfg.LLMs.Default
		if strings.TrimSpace(cfg.Jobs[i].LLMProfile) != "" {
			profileName = cfg.Jobs[i].LLMProfile
		}
		if profileName != "" && cfg.LLMs.Profiles != nil {
			if profile, ok := cfg.LLMs.Profiles[profileName]; ok {
				cfg.Jobs[i].LLM = profile
			}
		}
	}
}

func parseRuntimeConfig(file *os.File) ([]JobConfig, GoogleConfig, NotifierConfig, LLMRegistryConfig, bool, time.Duration, error) {
	var jobs []JobConfig
	var current *JobConfig
	var google GoogleConfig
	var notifier NotifierConfig
	llmRegistry := LLMRegistryConfig{Profiles: map[string]LLMConfig{}}
	var currentLLM *LLMConfig
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
		if line == "llms:" {
			section = "llms"
			continue
		}
		if line == "profiles:" && section == "llms" {
			section = "llms.profiles"
			continue
		}
		if line == "notifier:" {
			section = "notifier"
			continue
		}
		if line == "gmail:" && strings.HasPrefix(section, "notifier") {
			section = "notifier.gmail"
			continue
		}
		if line == "jobs:" {
			section = "jobs"
			sawJobs = true
			continue
		}
		if strings.HasPrefix(line, "- ") {
			if section == "llms.profiles" {
				if currentLLM != nil {
					storeLLMProfile(llmRegistry.Profiles, *currentLLM)
				}
				currentLLM = &LLMConfig{}
				line = strings.TrimSpace(strings.TrimPrefix(line, "- "))
				key, value, ok := strings.Cut(line, ":")
				if ok {
					parseLLMConfigField(currentLLM, strings.TrimSpace(key), strings.Trim(strings.TrimSpace(value), `"'`))
				}
				continue
			} else if section == "notifier.gmail.to" {
				recipient := strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "- ")), `"'`)
				if recipient != "" {
					notifier.Gmail.To = append(notifier.Gmail.To, recipient)
				}
				continue
			}
			if section != "jobs" && section != "jobs.llm" {
				continue
			}
			section = "jobs"
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
				return nil, GoogleConfig{}, NotifierConfig{}, LLMRegistryConfig{}, false, 0, err
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
		if section == "llms" {
			if key == "default" {
				llmRegistry.Default = value
			}
			continue
		}
		if section == "llms.profiles" {
			if currentLLM != nil {
				parseLLMConfigField(currentLLM, key, value)
			}
			continue
		}
		if strings.HasPrefix(section, "notifier") {
			switch key {
			case "type":
				notifier.Type = value
			case "from":
				notifier.Gmail.From = value
			case "to":
				section = "notifier.gmail.to"
				if value != "" {
					notifier.Gmail.To = append(notifier.Gmail.To, value)
				}
			case "subject_prefix":
				notifier.Gmail.SubjectPrefix = value
			}
			continue
		}
		if section == "jobs.llm" {
			parseLLMConfigField(&current.LLM, key, value)
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
				return nil, google, notifier, llmRegistry, true, 0, fmt.Errorf("invalid enabled for job %q: %q", current.Name, value)
			}
			current.Enabled = enabled
		case "interval_seconds":
			interval, err := parsePositiveSeconds(value, "interval_seconds")
			if err != nil {
				return nil, google, notifier, llmRegistry, true, 0, err
			}
			current.Interval = interval
		case "message":
			current.Message = value
		case "prompt":
			current.Prompt = value
		case "max_chars":
			maxChars, err := strconv.Atoi(value)
			if err != nil {
				return nil, google, notifier, llmRegistry, true, 0, fmt.Errorf("invalid max_chars for job %q: %q", current.Name, value)
			}
			current.MaxChars = maxChars
		case "llm_profile":
			current.LLMProfile = value
		case "output_path":
			current.OutputPath = value
		case "outbox_path":
			current.OutboxPath = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, google, notifier, llmRegistry, sawJobs, legacyInterval, err
	}
	if currentLLM != nil {
		storeLLMProfile(llmRegistry.Profiles, *currentLLM)
	}
	if current != nil {
		jobs = append(jobs, *current)
	}
	return jobs, google, notifier, llmRegistry, sawJobs, legacyInterval, nil
}

func storeLLMProfile(profiles map[string]LLMConfig, profile LLMConfig) {
	if strings.TrimSpace(profile.Name) == "" {
		return
	}
	profiles[profile.Name] = profile
}

func parseLLMConfigField(cfg *LLMConfig, key, value string) {
	switch key {
	case "name":
		cfg.Name = value
	case "provider":
		cfg.Provider = value
	case "model":
		cfg.Model = value
	case "api_key_env":
		cfg.APIKeyEnv = value
	case "base_url":
		cfg.BaseURL = value
	}
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
	if err := validateNotifierConfig(cfg.Notifier, cfg.Google); err != nil {
		return err
	}
	if err := validateLLMRegistryConfig(cfg.LLMs); err != nil {
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
		if job.Type != "heartbeat" && job.Type != "local_checkin" && job.Type != "notify_test" && job.Type != "ai_email" {
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
		if job.Type == "ai_email" {
			if err := validateAIEmailJobConfig(job); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateLLMRegistryConfig(llms LLMRegistryConfig) error {
	if llms.IsZero() {
		return nil
	}
	if strings.TrimSpace(llms.Default) == "" {
		return fmt.Errorf("llms default profile is required")
	}
	if _, ok := llms.Profiles[llms.Default]; !ok {
		return fmt.Errorf("llms default profile %q is not defined", llms.Default)
	}
	for name, profile := range llms.Profiles {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(profile.Name) == "" {
			return fmt.Errorf("llm profile missing name")
		}
		if name != profile.Name {
			return fmt.Errorf("llm profile name mismatch %q != %q", name, profile.Name)
		}
		if strings.TrimSpace(profile.Provider) == "" {
			return fmt.Errorf("llm profile %q requires provider", name)
		}
		if strings.TrimSpace(profile.Model) == "" {
			return fmt.Errorf("llm profile %q requires model", name)
		}
		if strings.TrimSpace(profile.APIKeyEnv) == "" {
			return fmt.Errorf("llm profile %q requires api_key_env", name)
		}
	}
	return nil
}

func validateAIEmailJobConfig(job JobConfig) error {
	if strings.TrimSpace(job.Prompt) == "" {
		return fmt.Errorf("ai_email job %q requires prompt", job.Name)
	}
	if job.MaxChars <= 0 {
		return fmt.Errorf("ai_email job %q requires positive max_chars", job.Name)
	}
	llmConfig := job.ResolvedLLM()
	if strings.TrimSpace(job.LLMProfile) != "" && llmConfig.IsZero() {
		return fmt.Errorf("ai_email job %q references unknown llm_profile %q", job.Name, job.LLMProfile)
	}
	if strings.TrimSpace(llmConfig.Provider) == "" {
		return fmt.Errorf("ai_email job %q requires llm provider", job.Name)
	}
	if strings.TrimSpace(llmConfig.Model) == "" {
		return fmt.Errorf("ai_email job %q requires llm model", job.Name)
	}
	if strings.TrimSpace(llmConfig.APIKeyEnv) == "" {
		return fmt.Errorf("ai_email job %q requires llm api_key_env", job.Name)
	}
	return nil
}

func validateNotifierConfig(notifier NotifierConfig, google GoogleConfig) error {
	switch notifier.Type {
	case "", "file_outbox":
		return nil
	case "gmail":
		if google.ScopeProfile != GoogleScopeProfileGmailSend {
			return fmt.Errorf("gmail notifier requires google scope_profile gmail_send")
		}
		if strings.TrimSpace(notifier.Gmail.From) == "" {
			return fmt.Errorf("gmail notifier from is required")
		}
		if len(notifier.Gmail.To) == 0 {
			return fmt.Errorf("gmail notifier to is required")
		}
		for _, recipient := range notifier.Gmail.To {
			if strings.TrimSpace(recipient) == "" {
				return fmt.Errorf("gmail notifier to contains empty recipient")
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown notifier type %q", notifier.Type)
	}
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
