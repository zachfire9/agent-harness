package onboarding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegrationRejectsConflictingUserPath(t *testing.T) {
	d := t.TempDir()
	source := filepath.Join(d, "source.exe")
	os.WriteFile(source, []byte("fake"), 0600)
	other := filepath.Join(d, "other")
	os.Mkdir(other, 0700)
	os.WriteFile(filepath.Join(other, "agent-harness.exe"), []byte("other"), 0600)
	p := LocalIntegration{SettingsPath: filepath.Join(d, "settings.json"), Source: source, Run: func(string) (string, error) { return other, nil }}
	if _, e := p.Review(IntegrationOptions{Copy: true, Path: true}, d); e == nil {
		t.Fatal("another user PATH installation must be reviewed, not silently shadowed")
	}
}
func TestIntegrationRejectsSymlinkBin(t *testing.T) {
	d := t.TempDir()
	other := t.TempDir()
	if e := os.Symlink(other, filepath.Join(d, "bin")); e != nil {
		t.Skip("symlink unavailable")
	}
	source := filepath.Join(d, "source.exe")
	os.WriteFile(source, []byte("fake"), 0600)
	p := LocalIntegration{SettingsPath: filepath.Join(d, "settings.json"), Source: source}
	if _, e := p.Review(IntegrationOptions{Copy: true}, d); e == nil {
		t.Fatal("symlink bin accepted")
	}
}
func TestPathRemovalUsesSingleCharacterPowerShellTrim(t *testing.T) {
	d := t.TempDir()
	p := LocalIntegration{SettingsPath: filepath.Join(d, "settings.json")}
	if err := os.WriteFile(p.manifest(), []byte(`{"path_owned":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	var script string
	p.Run = func(s string) (string, error) { script = s; return "", nil }
	if err := p.Remove(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, `TrimEnd('\','/')`) {
		t.Fatal("PowerShell TrimEnd must receive single-character backslash, not a C-style escape")
	}
}

func TestSettingsRejectsCredentialInModel(t *testing.T) {
	s := Settings{BaseURL: "https://example.com/v1", Model: "fake\nsecret", Workspace: t.TempDir()}
	if e := s.Validate(); e == nil || strings.Contains(e.Error(), "secret") {
		t.Fatal(e)
	}
}
