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
	Profile   string
	Provider  string
	Model     string
	APIKeyEnv string
	BaseURL   string
}

func (c LLMConfig) IsZero() bool {
	return strings.TrimSpace(c.Profile) == "" && strings.TrimSpace(c.Provider) == "" && strings.TrimSpace(c.Model) == "" && strings.TrimSpace(c.APIKeyEnv) == "" && strings.TrimSpace(c.BaseURL) == ""
}

type LLMSelector struct {
	Profile  string
	Provider string
	Model    string
}

func (s LLMSelector) IsZero() bool {
	return strings.TrimSpace(s.Profile) == "" && strings.TrimSpace(s.Provider) == "" && strings.TrimSpace(s.Model) == ""
}

type LLMProviderConfig struct {
	Profile   string
	Provider  string
	APIKeyEnv string
	BaseURL   string
	Models    []string
}

type LLMRegistryConfig struct {
	Default    LLMSelector
	Providers  map[string]LLMProviderConfig
	Duplicates []string
}

func (c LLMRegistryConfig) IsZero() bool {
	return c.Default.IsZero() && len(c.Providers) == 0
}

type JobConfig struct {
	Name        string
	Type        string
	Enabled     bool
	Interval    time.Duration
	Message     string
	Prompt      string
	MaxChars    int
	OutputPath  string
	OutboxPath  string
	Notifier    NotifierConfig
	Google      GoogleConfig
	LLMOverride LLMSelector
	LLM         LLMConfig
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
		if resolved, ok := cfg.LLMs.Resolve(cfg.Jobs[i].LLMOverride); ok {
			cfg.Jobs[i].LLM = resolved
		}
	}
}

func (cfg LLMRegistryConfig) Resolve(selector LLMSelector) (LLMConfig, bool) {
	if cfg.IsZero() {
		return LLMConfig{}, false
	}
	if selector.IsZero() {
		selector = cfg.Default
	}
	profileName := strings.TrimSpace(selector.Profile)
	if profileName == "" {
		profileName = strings.TrimSpace(selector.Provider)
	}
	if profileName == "" {
		profileName = strings.TrimSpace(cfg.Default.Profile)
	}
	if profileName == "" {
		profileName = strings.TrimSpace(cfg.Default.Provider)
	}
	provider, ok := cfg.Providers[profileName]
	if !ok {
		return LLMConfig{}, false
	}
	model := strings.TrimSpace(selector.Model)
	if model == "" {
		model = strings.TrimSpace(cfg.Default.Model)
	}
	return LLMConfig{
		Profile:   provider.Profile,
		Provider:  provider.Provider,
		Model:     model,
		APIKeyEnv: provider.APIKeyEnv,
		BaseURL:   provider.BaseURL,
	}, true
}

func parseRuntimeConfig(file *os.File) ([]JobConfig, GoogleConfig, NotifierConfig, LLMRegistryConfig, bool, time.Duration, error) {
	var jobs []JobConfig
	var current *JobConfig
	var google GoogleConfig
	var notifier NotifierConfig
	llmRegistry := LLMRegistryConfig{Providers: map[string]LLMProviderConfig{}}
	var currentLLMProvider *LLMProviderConfig
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
		if line == "default:" && strings.HasPrefix(section, "llms") {
			section = "llms.default"
			continue
		}
		if line == "providers:" && strings.HasPrefix(section, "llms") {
			section = "llms.providers"
			continue
		}
		if line == "models:" && section == "llms.providers" {
			section = "llms.providers.models"
			continue
		}
		if line == "llm:" && section == "jobs" && current != nil {
			section = "jobs.llm"
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
			if section == "llms.providers.models" && !strings.HasPrefix(raw, "        - ") {
				section = "llms.providers"
			}
			if section == "llms.providers.models" {
				model := strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "- ")), `"'`)
				if model != "" && currentLLMProvider != nil {
					currentLLMProvider.Models = append(currentLLMProvider.Models, model)
				}
				continue
			}
			if section == "llms.providers" {
				if currentLLMProvider != nil {
					storeLLMProvider(&llmRegistry, *currentLLMProvider)
				}
				currentLLMProvider = &LLMProviderConfig{}
				line = strings.TrimSpace(strings.TrimPrefix(line, "- "))
				key, value, ok := strings.Cut(line, ":")
				if ok {
					parseLLMProviderField(currentLLMProvider, strings.TrimSpace(key), strings.Trim(strings.TrimSpace(value), `"'`))
				}
				continue
			}
			if section == "notifier.gmail.to" {
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
		if section == "llms.default" {
			parseLLMSelectorField(&llmRegistry.Default, key, value)
			continue
		}
		if section == "llms.providers" || section == "llms.providers.models" {
			if currentLLMProvider != nil {
				parseLLMProviderField(currentLLMProvider, key, value)
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
			parseLLMSelectorField(&current.LLMOverride, key, value)
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
			current.LLMOverride.Profile = value
		case "output_path":
			current.OutputPath = value
		case "outbox_path":
			current.OutboxPath = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, google, notifier, llmRegistry, sawJobs, legacyInterval, err
	}
	if currentLLMProvider != nil {
		storeLLMProvider(&llmRegistry, *currentLLMProvider)
	}
	if current != nil {
		jobs = append(jobs, *current)
	}
	return jobs, google, notifier, llmRegistry, sawJobs, legacyInterval, nil
}

func storeLLMProvider(registry *LLMRegistryConfig, provider LLMProviderConfig) {
	if strings.TrimSpace(provider.Profile) == "" {
		provider.Profile = strings.TrimSpace(provider.Provider)
	}
	if strings.TrimSpace(provider.Profile) == "" {
		return
	}
	if _, exists := registry.Providers[provider.Profile]; exists {
		registry.Duplicates = append(registry.Duplicates, provider.Profile)
		return
	}
	registry.Providers[provider.Profile] = provider
}

func parseLLMSelectorField(selector *LLMSelector, key, value string) {
	switch key {
	case "profile":
		selector.Profile = value
	case "provider":
		selector.Provider = value
	case "model":
		selector.Model = value
	}
}

func parseLLMProviderField(cfg *LLMProviderConfig, key, value string) {
	switch key {
	case "profile", "name":
		cfg.Profile = value
	case "provider":
		cfg.Provider = value
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
			if provider, ok := cfg.LLMs.Providers[job.ResolvedLLM().Profile]; ok && !stringInSlice(job.ResolvedLLM().Model, provider.Models) {
				return fmt.Errorf("ai_email job %q model %q is not listed for llm provider/profile %q", job.Name, job.ResolvedLLM().Model, job.ResolvedLLM().Profile)
			}
		}
	}
	return nil
}

func validateLLMRegistryConfig(llms LLMRegistryConfig) error {
	if llms.IsZero() {
		return nil
	}
	if len(llms.Duplicates) > 0 {
		return fmt.Errorf("duplicate llm provider profile %q", llms.Duplicates[0])
	}
	if strings.TrimSpace(llms.Default.Provider) == "" && strings.TrimSpace(llms.Default.Profile) == "" {
		return fmt.Errorf("llms default provider is required")
	}
	if strings.TrimSpace(llms.Default.Model) == "" {
		return fmt.Errorf("llms default model is required")
	}
	defaultProfile := llms.Default.Profile
	if strings.TrimSpace(defaultProfile) == "" {
		defaultProfile = llms.Default.Provider
	}
	defaultProvider, ok := llms.Providers[defaultProfile]
	if !ok {
		return fmt.Errorf("llms default provider/profile %q is not defined", defaultProfile)
	}
	if !stringInSlice(llms.Default.Model, defaultProvider.Models) {
		return fmt.Errorf("llms default model %q is not listed for provider/profile %q", llms.Default.Model, defaultProfile)
	}
	for profileName, provider := range llms.Providers {
		if strings.TrimSpace(profileName) == "" || strings.TrimSpace(provider.Profile) == "" {
			return fmt.Errorf("llm provider profile missing name")
		}
		if profileName != provider.Profile {
			return fmt.Errorf("llm provider profile mismatch %q != %q", profileName, provider.Profile)
		}
		if strings.TrimSpace(provider.Provider) == "" {
			return fmt.Errorf("llm provider profile %q requires provider", profileName)
		}
		if strings.TrimSpace(provider.APIKeyEnv) == "" {
			return fmt.Errorf("llm provider profile %q requires api_key_env", profileName)
		}
		if len(provider.Models) == 0 {
			return fmt.Errorf("llm provider profile %q requires at least one model", profileName)
		}
	}
	return nil
}

func stringInSlice(value string, items []string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func validateAIEmailJobConfig(job JobConfig) error {
	if strings.TrimSpace(job.Prompt) == "" {
		return fmt.Errorf("ai_email job %q requires prompt", job.Name)
	}
	if job.MaxChars <= 0 {
		return fmt.Errorf("ai_email job %q requires positive max_chars", job.Name)
	}
	llmConfig := job.ResolvedLLM()
	if !job.LLMOverride.IsZero() && llmConfig.IsZero() {
		return fmt.Errorf("ai_email job %q references unknown llm provider/profile", job.Name)
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
