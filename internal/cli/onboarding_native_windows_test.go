//go:build windows

package cli

import (
	"bytes"
	"fmt"
	"github.com/zachfire9/agent-harness/internal/onboarding"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// Native smoke is opt-in: only the CI script supplies an isolated extracted
// executable. ConPTY exercises the real setup command and hidden console input,
// not a pipe bypass or a production-only automation flag.
func TestNativeOnboardingSmoke(t *testing.T) {
	exe := os.Getenv("HARNESS_ONBOARDING_SMOKE_BINARY")
	if exe == "" {
		t.Skip("run scripts/test-windows-onboarding.ps1 for the native ConPTY lifecycle")
	}
	for _, k := range []string{"OPENAI_API_KEY", "OPENAI_BASE_URL", "OPENAI_MODEL", "AGENT_RUN_LOGS_ENABLED"} {
		t.Setenv(k, "")
	}
	d := t.TempDir()
	p := filepath.Join(d, "profile ü", "settings.json")
	workspace := filepath.Join(d, "workspace ü")
	target := onboarding.Target(p)
	store := onboarding.NativeStore()
	defer func() {
		if err := store.Delete(target); err != nil {
			t.Error("synthetic credential cleanup failed", err)
		}
	}() // Only this test's unique target.
	defer func() {
		if err := (onboarding.LocalIntegration{SettingsPath: p, Source: exe, Run: runOnboardingPowerShell}).Remove(); err != nil {
			t.Error("synthetic integration cleanup failed", err)
		}
	}()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer fake-onboarding-key" {
			t.Error("incorrect synthetic credential")
		}
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"OK"}}]}`)
	}))
	defer srv.Close()
	steps := []consoleStep{{"OpenAI-compatible endpoint", srv.URL + "/v1"}, {"Model [", "fake-model"}, {"Workspace (only", workspace}, {"Credential: store", "store"}, {"API key (hidden):", "fake-onboarding-key"}, {"Enable diagnostic", "no"}, {"Copy executable", "yes"}, {"Add stable executable", "no"}, {"Create Chat with", "yes"}, {"Apply these changes?", "yes"}, {"Test connection?", "yes"}, {"Start a new chat now?", "no"}}
	output := runConsole(t, exe, []string{"setup", "--settings", p}, steps)
	if strings.Contains(output, "fake-onboarding-key") {
		t.Fatal("hidden key appeared in console output")
	}
	s, e := onboarding.Read(p)
	if e != nil || s.CredentialRef != target || s.RunLogs {
		t.Fatal("saved settings contract", e)
	}
	key, e := store.Get(target)
	if e != nil || key != "fake-onboarding-key" {
		t.Fatal("native credential round trip failed", e)
	}
	shortcut := filepath.Join(filepath.Dir(p), "Chat with Agent Harness.lnk")
	if _, e = os.Stat(shortcut); e != nil {
		t.Fatal("native shortcut missing", e)
	}
	script := "$s=([Activator]::CreateInstance([type]::GetTypeFromProgID('WScript.Shell'))).CreateShortcut('" + strings.ReplaceAll(shortcut, "'", "''") + "'); $s.WorkingDirectory; $s.Arguments; $s.TargetPath"
	shortcutInfo, e := runOnboardingPowerShell(script)
	if e != nil || !strings.Contains(shortcutInfo, workspace) || !strings.Contains(shortcutInfo, "--workspace saved") || strings.Contains(shortcutInfo, key) {
		t.Fatal("shortcut contract", e)
	}
	run := func(input string, args ...string) string {
		t.Helper()
		cmd := exec.Command(exe, args...)
		cmd.Stdin = strings.NewReader(input)
		out, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("fresh-process %v: %v: %s", args, e, out)
		}
		return string(out)
	}
	if out := run("", "config", "check", "--settings", p); !strings.Contains(out, "config ok") || strings.Contains(out, key) {
		t.Fatal("fresh config check unsafe")
	}
	if out := run("/exit\n", "chat", "--settings", p); !strings.Contains(out, "Goodbye.") {
		t.Fatal("chat exit failed")
	}
	if calls != 1 {
		t.Fatal("unexpected model requests", calls)
	}
	steps[3].answer = "keep"
	steps = append(steps[:4], steps[5:]...)
	steps[len(steps)-2].answer = "no"
	runConsole(t, exe, []string{"setup", "--settings", p}, steps)
	runConsole(t, exe, []string{"setup", "--settings", p, "--remove"}, []consoleStep{{"Remove?", "yes"}})
	if _, e = store.Get(target); e == nil {
		t.Fatal("credential not removed")
	}
	if _, e = os.Stat(p); !os.IsNotExist(e) {
		t.Fatal("settings not removed")
	}
	if _, e = os.Stat(shortcut); !os.IsNotExist(e) {
		t.Fatal("shortcut not removed")
	}
	if _, e = os.Stat(workspace); e != nil {
		t.Fatal("workspace was deleted")
	}
}

type consoleStep struct{ prompt, answer string }
type lockedOutput struct {
	sync.Mutex
	b bytes.Buffer
}

func (b *lockedOutput) Write(p []byte) (int, error) { b.Lock(); defer b.Unlock(); return b.b.Write(p) }
func (b *lockedOutput) String() string              { b.Lock(); defer b.Unlock(); return b.b.String() }
func runConsole(t *testing.T, exe string, args []string, steps []consoleStep) string {
	t.Helper()
	k := syscall.NewLazyDLL("kernel32.dll")
	create := k.NewProc("CreatePseudoConsole")
	closePC := k.NewProc("ClosePseudoConsole")
	initAttrs := k.NewProc("InitializeProcThreadAttributeList")
	update := k.NewProc("UpdateProcThreadAttribute")
	deleteAttrs := k.NewProc("DeleteProcThreadAttributeList")
	inputRead, inputWrite, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	defer inputRead.Close()
	defer inputWrite.Close()
	outputRead, outputWrite, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	defer outputRead.Close()
	defer outputWrite.Close()
	var output lockedOutput
	go func() {
		buf := make([]byte, 4096)
		for {
			n, e := outputRead.Read(buf)
			if n > 0 {
				output.Write(buf[:n])
			}
			if e != nil {
				return
			}
		}
	}()
	var hpc uintptr
	hr, _, _ := create.Call(120|50<<16, inputRead.Fd(), outputWrite.Fd(), 0, uintptr(unsafe.Pointer(&hpc)))
	if int32(hr) != 0 {
		t.Fatalf("CreatePseudoConsole HRESULT %x", hr)
	}
	defer closePC.Call(hpc)
	var size uintptr
	initAttrs.Call(0, 1, 0, uintptr(unsafe.Pointer(&size)))
	data := make([]byte, size)
	attrs := uintptr(unsafe.Pointer(&data[0]))
	ok, _, e := initAttrs.Call(attrs, 1, 0, uintptr(unsafe.Pointer(&size)))
	if ok == 0 {
		t.Fatal(e)
	}
	defer deleteAttrs.Call(attrs)
	ok, _, e = update.Call(attrs, 0, 0x00020016, hpc, unsafe.Sizeof(hpc), 0, 0)
	if ok == 0 {
		t.Fatal(e)
	}
	type startupEx struct {
		syscall.StartupInfo
		Attributes uintptr
	}
	si := startupEx{Attributes: attrs}
	si.Cb = uint32(unsafe.Sizeof(si))
	command := syscall.EscapeArg(exe)
	for _, a := range args {
		command += " " + syscall.EscapeArg(a)
	}
	app, _ := syscall.UTF16PtrFromString(exe)
	cmd, _ := syscall.UTF16PtrFromString(command)
	var pi syscall.ProcessInformation
	if e = syscall.CreateProcess(app, cmd, nil, nil, false, 0x00080000, nil, nil, &si.StartupInfo, &pi); e != nil {
		t.Fatal(e)
	}
	runtime.KeepAlive(data)
	defer syscall.CloseHandle(pi.Process)
	defer syscall.CloseHandle(pi.Thread)
	defer syscall.TerminateProcess(pi.Process, 1)
	pos := 0
	deadline := time.Now().Add(60 * time.Second)
	for _, step := range steps {
		for {
			text := output.String()
			if at := strings.Index(text[pos:], step.prompt); at >= 0 {
				pos += at + len(step.prompt)
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("native console timeout waiting for %q (console output withheld to protect hidden input)", step.prompt)
			}
			time.Sleep(10 * time.Millisecond)
		}
		if _, e = inputWrite.Write([]byte(step.answer + "\r")); e != nil {
			t.Fatal(e)
		}
	}
	wait, e := syscall.WaitForSingleObject(pi.Process, 30000)
	if e != nil || wait != 0 {
		t.Fatal("native console process did not exit", e)
	}
	var code uint32
	if e = syscall.GetExitCodeProcess(pi.Process, &code); e != nil || code != 0 {
		t.Fatalf("native setup failed with exit code %d (output withheld)", code)
	}
	return output.String()
}
