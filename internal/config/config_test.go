package config_test

import (
	"strings"
	"testing"

	"github.com/zachfire9/agent-harness/internal/config"
)

func TestLoadRequiresAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_MODEL", "")

	_, err := config.Load()

	if err == nil {
		t.Fatal("expected missing API key error")
	}

	if !strings.Contains(err.Error(), "OPENAI_API_KEY is required") {
		t.Fatalf("expected clear missing API key error, got %q", err.Error())
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_MODEL", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("expected default base URL, got %q", cfg.BaseURL)
	}

	if cfg.Model != "gpt-4.1-mini" {
		t.Fatalf("expected default model, got %q", cfg.Model)
	}
}

func TestLoadUsesEnvironmentOverrides(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", "https://example.com/v1")
	t.Setenv("OPENAI_MODEL", "custom-model")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.APIKey != "test-key" {
		t.Fatalf("expected API key from env, got %q", cfg.APIKey)
	}

	if cfg.BaseURL != "https://example.com/v1" {
		t.Fatalf("expected base URL override, got %q", cfg.BaseURL)
	}

	if cfg.Model != "custom-model" {
		t.Fatalf("expected model override, got %q", cfg.Model)
	}
}

func TestSafeStringRedactsAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "secret-key")
	t.Setenv("OPENAI_BASE_URL", "https://example.com/v1")
	t.Setenv("OPENAI_MODEL", "custom-model")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	safe := cfg.SafeString()

	if strings.Contains(safe, "secret-key") {
		t.Fatalf("expected safe config string to redact API key, got %q", safe)
	}

	if !strings.Contains(safe, "OPENAI_API_KEY=<redacted>") {
		t.Fatalf("expected redacted API key marker, got %q", safe)
	}

	if !strings.Contains(safe, "OPENAI_BASE_URL=https://example.com/v1") {
		t.Fatalf("expected base URL in safe config string, got %q", safe)
	}

	if !strings.Contains(safe, "OPENAI_MODEL=custom-model") {
		t.Fatalf("expected model in safe config string, got %q", safe)
	}
}
