package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	harnessruntime "github.com/zachfire9/agent-harness/internal/runtime"
)

func loadDaemonEnvironmentForOS(paths harnessruntime.Paths, goos string) error {
	if goos == "windows" {
		return harnessruntime.LoadPrivateEnvironment(paths)
	}
	return nil
}

func (a App) runServiceForOS(args []string, stdout, stderr io.Writer, goos string) int {
	if goos == "linux" {
		return a.runSystemdService(args, stdout, stderr)
	}
	if goos != "windows" {
		fmt.Fprintln(stderr, "service error: runtime adapter not supported on this OS")
		return 1
	}
	if len(args) == 0 || args[0] != "install" {
		fmt.Fprintln(stderr, "service error: unsupported service command")
		return 1
	}
	// Windows installation is per-instance rather than a systemd template.
	for i := 1; i < len(args); i += 2 {
		if args[i] != "--instance" && args[i] != "--home" {
			fmt.Fprintln(stderr, "service error: expected --instance or --home")
			return 1
		}
	}
	opts, err := parseRuntimeOptions(args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "service error: %v\n", err)
		return 1
	}
	paths, err := resolveRuntimePathsForOS(opts, goos)
	if err == nil {
		paths = harnessruntime.PathsForHome(paths.Home)
		paths.Home, err = filepath.Abs(paths.Home)
		paths = harnessruntime.PathsForHome(paths.Home)
	}
	if err != nil {
		fmt.Fprintf(stderr, "service error: %v\n", err)
		return 1
	}
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, "service error: cannot resolve executable")
		return 1
	}
	path, err := harnessruntime.InstallWindowsTask(paths, opts.instance, executable)
	if err != nil {
		fmt.Fprintf(stderr, "service error: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "generated Windows Task Scheduler registration script: %s\n", path)
	fmt.Fprintln(stdout, "task not registered; review and run this script as the current user in PowerShell (Interactive token, Limited privileges; no password).")
	fmt.Fprintln(stdout, "See docs/windows-runtime.md for registration, start, stop, status, update and removal. Do not move the executable after registration.")
	return 0
}
