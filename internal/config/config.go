package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	DefaultBaseURL = "https://api.openai.com/v1"
	DefaultModel   = "gpt-4.1-mini"
)

// Config contains the model/provider settings needed by the agent harness.
type Config struct {
	APIKey  string
	BaseURL string
	Model   string
}

// Load reads configuration from environment variables.
func Load() (Config, error) {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return Config{}, errors.New("OPENAI_API_KEY is required")
	}

	baseURL := strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	model := strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
	if model == "" {
		model = DefaultModel
	}

	return Config{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
	}, nil
}

// SafeString returns a display-safe representation of Config without secrets.
func (c Config) SafeString() string {
	return fmt.Sprintf("OPENAI_API_KEY=<redacted> OPENAI_BASE_URL=%s OPENAI_MODEL=%s", c.BaseURL, c.Model)
}
