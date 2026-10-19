package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsRuntimeDefaultsAndExplicitHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", root)
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "wrong"))
	paths, err := resolveRuntimePathsForOS(runtimeOptions{instance: "worker"}, "windows")
	if err != nil {
		t.Fatal(err)
	}
	if paths.Home != filepath.Join(root, "agent-harness", "instances", "worker") {
		t.Fatal(paths.Home)
	}
	t.Setenv("LOCALAPPDATA", "")
	if _, err := resolveRuntimePathsForOS(runtimeOptions{instance: "worker"}, "windows"); err == nil {
		t.Fatal("silently used Linux fallback")
	}
	paths, err = resolveRuntimePathsForOS(runtimeOptions{instance: "worker", home: root}, "windows")
	if err != nil || paths.Home != root {
		t.Fatal(paths, err)
	}
}

func TestWindowsServiceInstallGeneratesOnly(t *testing.T) {
	home := filepath.Join(t.TempDir(), "O'Brien & space")
	var out, stderr bytes.Buffer
	code := (App{}).runServiceForOS([]string{"install", "--instance", "worker", "--home", home}, &out, &stderr, "windows")
	if code != 0 {
		t.Fatal(stderr.String())
	}
	path := filepath.Join(home, "config", "register-task.ps1")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"generated", "not registered", "Interactive", "register-task.ps1"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q: %s", want, &out)
		}
	}
	if strings.Contains(out.String(), "systemctl") {
		t.Fatal("Linux instructions on Windows")
	}
	for _, args := range [][]string{{"install", "--instance", "../escape", "--home", home}, {"install", "--home", home, "--test"}, {"install", "--config-home", home}} {
		out.Reset()
		stderr.Reset()
		if (App{}).runServiceForOS(args, &out, &stderr, "windows") == 0 {
			t.Fatal("accepted invalid options", args)
		}
	}
	out.Reset()
	stderr.Reset()
	if (App{}).runServiceForOS([]string{"install"}, &out, &stderr, "darwin") == 0 {
		t.Fatal("pretended macOS is Linux")
	}
}

func TestWindowsServiceHints(t *testing.T) {
	var out bytes.Buffer
	writeServiceHints(&out, "worker", "windows")
	if !strings.Contains(out.String(), "Get-ScheduledTask") || !strings.Contains(out.String(), "agent-harness-worker") || strings.Contains(out.String(), "journalctl") {
		t.Fatal(out.String())
	}
}
