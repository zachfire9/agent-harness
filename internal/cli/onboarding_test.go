package cli

import (
	"github.com/zachfire9/agent-harness/internal/onboarding"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testCredential struct{ calls int }

func (s *testCredential) Get(string) (string, error) { s.calls++; return "fake-wizard-key", nil }
func (s *testCredential) Put(string, string) error   { return nil }
func (s *testCredential) Delete(string) error        { return nil }
func TestInteractiveSavedResolution(t *testing.T) {
	d := t.TempDir()
	workspace := filepath.Join(d, "workspace ü")
	os.Mkdir(workspace, 0700)
	p := filepath.Join(d, "settings.json")
	onboarding.Save(p, onboarding.Settings{BaseURL: "https://example.com/v1", Model: "saved-model", Workspace: workspace, CredentialRef: onboarding.Target(p)})
	for _, k := range []string{"OPENAI_API_KEY", "OPENAI_BASE_URL", "OPENAI_MODEL", "AGENT_RUN_LOGS_ENABLED"} {
		t.Setenv(k, "")
	}
	store := &testCredential{}
	for _, cmd := range []string{"chat", "ask", "config"} {
		r, e := resolveInteractive([]string{"agent-harness", cmd, "--settings", p}, "windows", store)
		if e != nil || r.workspace != workspace || r.cfg.Model != "saved-model" || r.cfg.APIKey != "fake-wizard-key" || r.cfg.RunLogsEnabled {
			t.Fatal(r, e)
		}
	}
	cwd, _ := os.Getwd()
	r, e := resolveInteractive([]string{"app", "chat", "--settings", p, "--workspace", "current"}, "windows", store)
	if e != nil || r.workspace != cwd {
		t.Fatal(r.workspace, e)
	}
	t.Setenv("OPENAI_API_KEY", "override-fake")
	n := store.calls
	_, e = resolveInteractive([]string{"app", "chat", "--settings", p}, "windows", store)
	if e != nil || store.calls != n {
		t.Fatal(e)
	}
}
func TestInteractiveUnsafeReferenceAndMissingWorkspace(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	onboarding.Save(p, onboarding.Settings{BaseURL: "https://example.com/v1", Model: "fake", Workspace: filepath.Join(t.TempDir(), "missing"), CredentialRef: "unrelated-credential"})
	_, e := resolveInteractive([]string{"app", "chat", "--settings", p}, "windows", &testCredential{})
	if e == nil || strings.Contains(e.Error(), "unrelated-credential") {
		t.Fatal(e)
	}
}
func TestSetupNonInteractiveFailsClearly(t *testing.T) {
	var out, err strings.Builder
	code := Run([]string{"app", "setup"}, &out, &err)
	if code == 0 || !strings.Contains(err.String(), "setup") {
		t.Fatal(code, err.String())
	}
}
