package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsTaskRegistration(t *testing.T) {
	home := filepath.Join(t.TempDir(), "instance O'Brien & $() space")
	paths := PathsForHome(home)
	exe := filepath.Join(t.TempDir(), "program O'Brien & $()", "agent-harness.exe")
	script, err := RenderWindowsTaskRegistration(paths, "worker-1", exe)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"New-ScheduledTaskAction", "New-ScheduledTaskTrigger -AtLogOn -User $sid", "-LogonType Interactive", "-RunLevel Limited", "-MultipleInstances IgnoreNew", "-ExecutionTimeLimit ([TimeSpan]::Zero)", "-AllowStartIfOnBatteries", "-DontStopIfGoingOnBatteries", "agent-harness-worker-1", "O''Brien", "Register-ScheduledTask", "-WorkingDirectory $instanceHome", "-RestartCount 3", "daemon --instance worker-1 --home"} {
		if !strings.Contains(script, want) {
			t.Errorf("missing %q in script:\n%s", want, script)
		}
	}
	for _, bad := range []string{"-Password", "-RunLevel Highest", "Invoke-Expression", "cmd.exe", "secrets/env", "-Force"} {
		if strings.Contains(script, bad) {
			t.Errorf("unsafe script fragment %q", bad)
		}
	}
	path, err := InstallWindowsTask(paths, "worker-1", exe)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "\xef\xbb\xbf"+script {
		t.Fatal("PowerShell 5.1 script must use UTF-8 BOM")
	}
}

func TestWindowsTaskRejectsUnsafeInputs(t *testing.T) {
	for _, name := range []string{"../escape", "a/b", "x;whoami", "CON", "nul", "COM1", "LPT9", "", strings.Repeat("a", 65)} {
		if _, err := RenderWindowsTaskRegistration(PathsForHome(t.TempDir()), name, filepath.Join(t.TempDir(), "a.exe")); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	for _, value := range []string{"relative", "bad\"quote", "bad\nline", "bad\x00nul"} {
		if _, err := RenderWindowsTaskRegistration(PathsForHome(value), "default", filepath.Join(t.TempDir(), "a.exe")); err == nil {
			t.Errorf("accepted home %q", value)
		}
		if _, err := RenderWindowsTaskRegistration(PathsForHome(t.TempDir()), "default", value); err == nil {
			t.Errorf("accepted exe %q", value)
		}
	}
}

func TestWindowsPathsRequireLocalAppData(t *testing.T) {
	if _, err := WindowsPaths("", "default"); err == nil {
		t.Fatal("missing LOCALAPPDATA accepted")
	}
	root := t.TempDir()
	paths, err := WindowsPaths(root, "worker")
	if err != nil {
		t.Fatal(err)
	}
	if paths.Home != filepath.Join(root, "agent-harness", "instances", "worker") {
		t.Fatal(paths.Home)
	}
}

func TestPrivateEnvironmentParsing(t *testing.T) {
	env, err := ParsePrivateEnvironment([]byte("# comment\r\nOPENAI_API_KEY=fake $() & ' \" value\r\n\n"))
	if err != nil {
		t.Fatal(err)
	}
	if env["OPENAI_API_KEY"] != "fake $() & ' \" value" {
		t.Fatal("value was interpreted")
	}
	for _, raw := range []string{"export OPENAI_API_KEY=fake", "PATH=fake", "OPENAI_API_KEY=fake\nOPENAI_API_KEY=other", "OPENAI_API_KEY=fake\x00", "not a line", "=fake", "OPENAI_API_KEY="} {
		_, err := ParsePrivateEnvironment([]byte(raw))
		if err == nil {
			t.Errorf("accepted malformed environment %q", raw)
		} else if strings.Contains(err.Error(), "fake") {
			t.Fatal("secret in error")
		}
	}
}

func TestLoadPrivateEnvironment(t *testing.T) {
	paths := PathsForHome(t.TempDir())
	if err := InitInstance(paths); err != nil {
		t.Fatal(err)
	}
	if err := LoadPrivateEnvironment(paths); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_API_KEY", "inherited-fake")
	path := filepath.Join(paths.ConfigDir, "secrets", "env")
	if err := os.WriteFile(path, []byte("OPENAI_API_KEY=file-fake\n"), 0600); err != nil {
		t.Fatal(err)
	}
	setEnvironmentFixturePrivacy(t, path, false)
	if err := LoadPrivateEnvironment(paths); err == nil {
		t.Fatal("accepted broad fake credential permissions")
	}
	if os.Getenv("OPENAI_API_KEY") != "inherited-fake" {
		t.Fatal("loaded unsafe fixture before permission validation")
	}
	setEnvironmentFixturePrivacy(t, path, true)
	if err := LoadPrivateEnvironment(paths); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("OPENAI_API_KEY") != "file-fake" {
		t.Fatal("instance file must override inherited key")
	}
}
