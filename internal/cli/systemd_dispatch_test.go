package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the Linux dispatch seam on every host, without invoking systemd.
func TestRunServiceInstallWritesSystemdUserTemplate(t *testing.T) {
	configHome := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := (App{}).runServiceForOS([]string{"install", "--config-home", configHome}, &stdout, &stderr, "linux")
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit %d: %s", code, &stderr)
	}
	unitPath := filepath.Join(configHome, "systemd", "user", "agent-harness@.service")
	unit, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"ExecStart=%h/.local/bin/agent-harness daemon --instance %i --home %h/.local/share/agent-harness/instances/%i",
		"WorkingDirectory=%h/.local/share/agent-harness/instances/%i",
		"EnvironmentFile=-%h/.local/share/agent-harness/instances/%i/config/secrets/env",
	} {
		if !strings.Contains(string(unit), want) {
			t.Fatalf("missing %q: %s", want, unit)
		}
	}
	for _, want := range []string{"installed systemd user service template", unitPath, "systemctl --user daemon-reload", "systemctl --user enable --now agent-harness@default.service"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("missing %q: %s", want, &stdout)
		}
	}
}
