package cli

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/zachfire9/agent-harness/internal/config"
	"github.com/zachfire9/agent-harness/internal/onboarding"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"time"
	"unicode/utf16"
)

type interactiveResolution struct {
	cfg       config.Config
	workspace string
	args      []string
	announce  bool
}

func interactiveOptions(args []string) (clean []string, settings, workspace string, err error) {
	clean = append(clean, args[:2]...)
	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--settings", "--workspace":
			if i+1 >= len(args) {
				return nil, "", "", errors.New("--settings/--workspace requires a value")
			}
			if args[i] == "--settings" {
				settings = args[i+1]
			} else {
				workspace = args[i+1]
			}
			i++
		default:
			clean = append(clean, args[i])
		}
	}
	return
}
func resolveInteractive(args []string, platform string, store onboarding.CredentialStore) (interactiveResolution, error) {
	var r interactiveResolution
	clean, p, selection, e := interactiveOptions(args)
	if e != nil {
		return r, e
	}
	r.args = clean
	cwd, e := os.Getwd()
	if e != nil {
		return r, e
	}
	r.workspace = cwd
	var s onboarding.Settings
	if platform == "windows" || p != "" {
		if p == "" {
			p, e = onboarding.DefaultPath()
			if e != nil {
				return r, e
			}
		}
		if !filepath.IsAbs(p) {
			return r, errors.New("--settings must be absolute")
		}
		s, e = onboarding.Read(p)
		if e != nil {
			return r, e
		}
		if s.CredentialRef != "" && s.CredentialRef != onboarding.Target(p) {
			return r, errors.New("invalid onboarding credential reference; rerun setup")
		}
	}
	switch selection {
	case "current":
	case "saved":
		if s.Workspace == "" {
			return r, errors.New("no saved workspace; run setup or use --workspace current")
		}
		r.workspace = s.Workspace
	case "":
		if s.Workspace != "" {
			r.workspace = s.Workspace
		}
	default:
		if !filepath.IsAbs(selection) {
			return r, errors.New("--workspace must be current, saved or an absolute directory")
		}
		r.workspace = selection
	}
	info, e := os.Stat(r.workspace)
	if e != nil || !info.IsDir() {
		return r, errors.New("selected workspace is unavailable; use --workspace current or rerun setup")
	}
	defaults := map[string]string{}
	if s.BaseURL != "" {
		defaults["OPENAI_BASE_URL"] = s.BaseURL
		defaults["OPENAI_MODEL"] = s.Model
		defaults["AGENT_RUN_LOGS_ENABLED"] = strconv.FormatBool(s.RunLogs)
	}
	var get func() (string, error)
	if s.CredentialRef != "" {
		get = func() (string, error) {
			if store == nil {
				return "", errors.New("credential store unavailable")
			}
			return store.Get(s.CredentialRef)
		}
	}
	r.cfg, e = config.LoadInteractive(filepath.Join(r.workspace, ".env"), defaults, get)
	r.announce = s.Workspace != "" || selection != ""
	return r, e
}
func runSetup(args []string, stdout, stderr io.Writer) int {
	fail := func(e error) int { fmt.Fprintf(stderr, "setup error: %v\n", e); return 1 }
	if goruntime.GOOS != "windows" {
		return fail(errors.New("guided setup is Windows-only; existing environment/.env setup remains available"))
	}
	p, e := onboarding.DefaultPath()
	if e != nil {
		return fail(e)
	}
	remove := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--remove":
			remove = true
		case "--settings":
			i++
			if i >= len(args) || !filepath.IsAbs(args[i]) {
				return fail(errors.New("--settings requires an absolute path"))
			}
			p = args[i]
		default:
			return fail(errors.New("usage: setup [--settings <absolute path>] [--remove]"))
		}
	}
	ui, e := onboarding.NewConsole(os.Stdin, stdout)
	if e != nil {
		return fail(e)
	}
	exe, e := os.Executable()
	if e != nil {
		return fail(e)
	}
	user, e := os.UserHomeDir()
	if e != nil {
		return fail(e)
	}
	integration := onboarding.LocalIntegration{SettingsPath: p, Source: exe, Run: runOnboardingPowerShell}
	w := onboarding.Wizard{Path: p, DefaultWorkspace: filepath.Join(user, "AgentHarnessWorkspace"), UI: ui, Store: onboarding.NativeStore(), Integration: integration, Check: func(s onboarding.Settings, k string) error {
		return onboarding.CheckConnection(context.Background(), s, k)
	}}
	if remove {
		if e = w.Remove(); e != nil {
			return fail(e)
		}
		fmt.Fprintln(stdout, "Interactive settings removed. Workspace and copied executable retained.")
		return 0
	}
	r, e := w.Run()
	if e != nil {
		return fail(e)
	}
	fmt.Fprintln(stdout, "Settings saved. Later: agent-harness chat (or the optional shortcut). No conversation history is restored.")
	if r.StartChat {
		if r.SessionKey != "" { // In-process only: never export the key to child processes or persist it.
			defaults := map[string]string{"OPENAI_API_KEY": r.SessionKey, "OPENAI_BASE_URL": r.Settings.BaseURL, "OPENAI_MODEL": r.Settings.Model, "AGENT_RUN_LOGS_ENABLED": strconv.FormatBool(r.Settings.RunLogs)}
			cfg, e := config.LoadInteractive(filepath.Join(r.Settings.Workspace, ".env"), defaults, nil)
			if e != nil {
				return fail(e)
			}
			return runResolvedInteractive(interactiveResolution{cfg: cfg, workspace: r.Settings.Workspace, args: []string{"agent-harness", "chat"}, announce: true}, stdout, stderr)
		}
		return Run([]string{"agent-harness", "chat", "--settings", p, "--workspace", "saved"}, stdout, stderr)
	}
	if r.SessionKey != "" {
		fmt.Fprintln(stdout, "Session-only key discarded: start chat during setup or provide a process-environment key next time.")
	}
	return 0
}
func encodePowerShell(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, len(u)*2)
	for i, v := range u {
		binary.LittleEndian.PutUint16(b[i*2:], v)
	}
	return base64.StdEncoding.EncodeToString(b)
}
func runOnboardingPowerShell(script string) (string, error) {
	root := os.Getenv("SystemRoot")
	if root == "" {
		return "", errors.New("Windows system directory unavailable")
	}
	path := filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodePowerShell("[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false);"+script))
	b, e := cmd.Output()
	if e != nil {
		return "", errors.New("Windows integration command failed")
	}
	return string(b), nil
}
