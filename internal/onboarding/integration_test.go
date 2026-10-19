package onboarding

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegrationReviewAndSafety(t *testing.T) {
	d := t.TempDir()
	src := filepath.Join(d, "extracted ü.exe")
	os.WriteFile(src, []byte("fake-exe"), 0600)
	p := LocalIntegration{SettingsPath: filepath.Join(d, "settings.json"), Source: src, Run: func(string) (string, error) { return "C:\\existing", nil }}
	text, e := p.Review(IntegrationOptions{Copy: true, Path: true, Shortcut: true}, filepath.Join(d, "work ' ü"))
	if e != nil || !strings.Contains(text, "user PATH") || !strings.Contains(text, "Chat with Agent Harness.lnk") {
		t.Fatal(text, e)
	}
	os.MkdirAll(p.binDir(), 0700)
	os.WriteFile(p.exe(), []byte("other-installation"), 0600)
	if _, e = p.Review(IntegrationOptions{Copy: true}, d); e == nil {
		t.Fatal("replaced conflicting executable")
	}
}
func TestIntegrationCopyRerunAndOwnedCleanup(t *testing.T) {
	d := t.TempDir()
	src := filepath.Join(d, "source.exe")
	os.WriteFile(src, []byte("fake-exe"), 0600)
	var scripts []string
	p := LocalIntegration{SettingsPath: filepath.Join(d, "settings.json"), Source: src, Run: func(s string) (string, error) { scripts = append(scripts, s); return "", nil }}
	o := IntegrationOptions{Copy: true, Path: true}
	if e := p.Apply(o, d); e != nil {
		t.Fatal(e)
	}
	if e := p.Apply(o, d); e != nil {
		t.Fatal(e)
	}
	if e := p.Remove(); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(p.exe()); e != nil {
		t.Fatal("copied executable removed")
	}
	all := strings.Join(scripts, "\n")
	if strings.Contains(all, "Machine") || !strings.Contains(all, "'User'") {
		t.Fatal(all)
	}
}
func TestPowerShellQuoting(t *testing.T) {
	if psQuote("a'b ü") != "'a''b ü'" {
		t.Fatal("unsafe quote")
	}
	p := LocalIntegration{SettingsPath: filepath.Join(t.TempDir(), "settings.json"), Source: "relative", Run: func(string) (string, error) { return "", errors.New("fake-secret") }}
	if _, e := p.Review(IntegrationOptions{Path: true}, t.TempDir()); e == nil {
		t.Fatal("relative executable accepted")
	}
}
