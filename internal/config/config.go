package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	DefaultBaseURL            = "https://api.openai.com/v1"
	DefaultModel              = "gpt-4.1-mini"
	DefaultMaxContextMessages = 40
	DefaultMaxMessageChars    = 8000
	DefaultMaxToolResultChars = 4000
)

// Config contains the model/provider settings needed by the agent harness.
type Config struct {
	APIKey             string
	BaseURL            string
	Model              string
	MaxContextMessages int
	MaxMessageChars    int
	MaxToolResultChars int
}

// Load reads configuration from process environment variables and an optional
// local .env file in the current working directory. Process environment
// variables take precedence over values in .env.
func Load() (Config, error) {
	dotEnv := loadDotEnv(".env")

	apiKey := configValue("OPENAI_API_KEY", dotEnv)
	if apiKey == "" {
		return Config{}, errors.New("OPENAI_API_KEY is required")
	}

	baseURL := configValue("OPENAI_BASE_URL", dotEnv)
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	model := configValue("OPENAI_MODEL", dotEnv)
	if model == "" {
		model = DefaultModel
	}

	maxContextMessages, err := positiveIntConfigValue("AGENT_MAX_CONTEXT_MESSAGES", dotEnv, DefaultMaxContextMessages)
	if err != nil {
		return Config{}, err
	}
	maxMessageChars, err := positiveIntConfigValue("AGENT_MAX_MESSAGE_CHARS", dotEnv, DefaultMaxMessageChars)
	if err != nil {
		return Config{}, err
	}
	maxToolResultChars, err := positiveIntConfigValue("AGENT_MAX_TOOL_RESULT_CHARS", dotEnv, DefaultMaxToolResultChars)
	if err != nil {
		return Config{}, err
	}

	return Config{
		APIKey:             apiKey,
		BaseURL:            baseURL,
		Model:              model,
		MaxContextMessages: maxContextMessages,
		MaxMessageChars:    maxMessageChars,
		MaxToolResultChars: maxToolResultChars,
	}, nil
}

// SafeString returns a display-safe representation of Config without secrets.
func (c Config) SafeString() string {
	return fmt.Sprintf("OPENAI_API_KEY=<redacted> OPENAI_BASE_URL=%s OPENAI_MODEL=%s AGENT_MAX_CONTEXT_MESSAGES=%d AGENT_MAX_MESSAGE_CHARS=%d AGENT_MAX_TOOL_RESULT_CHARS=%d", c.BaseURL, c.Model, c.MaxContextMessages, c.MaxMessageChars, c.MaxToolResultChars)
}

func configValue(key string, dotEnv map[string]string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}

	return strings.TrimSpace(dotEnv[key])
}

func positiveIntConfigValue(key string, dotEnv map[string]string, defaultValue int) (int, error) {
	value := configValue(key, dotEnv)
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}

	return parsed, nil
}

func loadDotEnv(path string) map[string]string {
	values := map[string]string{}

	content, err := os.ReadFile(path)
	if err != nil {
		return values
	}

	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}

		values[key] = strings.Trim(strings.TrimSpace(value), `"'`)
	}

	return values
}
