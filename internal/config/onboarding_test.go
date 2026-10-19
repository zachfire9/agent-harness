package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInteractivePrecedence(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(p, []byte("OPENAI_MODEL=dot-model\nOPENAI_API_KEY=dot-key\n"), 0600)
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("AGENT_RUN_LOGS_ENABLED", "")
	defaults := map[string]string{"OPENAI_MODEL": "saved-model", "OPENAI_BASE_URL": "https://example.com/v1", "AGENT_RUN_LOGS_ENABLED": "false"}
	called := false
	get := func() (string, error) { called = true; return "fake-stored-key", nil }
	c, e := LoadInteractive(p, defaults, get)
	if e != nil || c.Model != "dot-model" || c.APIKey != "dot-key" || c.RunLogsEnabled || called {
		t.Fatal(c.SafeString(), e, called)
	}
	t.Setenv("OPENAI_MODEL", "process-model")
	t.Setenv("OPENAI_API_KEY", "process-key")
	c, e = LoadInteractive(p, defaults, get)
	if e != nil || c.Model != "process-model" || c.APIKey != "process-key" || called {
		t.Fatal(c.SafeString(), e)
	}
	t.Setenv("OPENAI_API_KEY", "")
	os.Remove(p)
	c, e = LoadInteractive(p, defaults, get)
	if e != nil || c.Model != "process-model" || c.APIKey != "fake-stored-key" || !called {
		t.Fatal(e)
	}
}
func TestInteractiveCredentialFailureSafe(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	_, e := LoadInteractive(filepath.Join(t.TempDir(), ".env"), nil, func() (string, error) { return "", errors.New("fake-secret") })
	if e == nil || e.Error() == "fake-secret" {
		t.Fatal(e)
	}
}
