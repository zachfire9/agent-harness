package cli

import (
	"os"
	"path/filepath"
	"testing"

	harnessruntime "github.com/zachfire9/agent-harness/internal/runtime"
)

func TestDaemonPrivateEnvironmentPlatformBoundary(t *testing.T) {
	paths := harnessruntime.PathsForHome(t.TempDir())
	if err := harnessruntime.InitInstance(paths); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.ConfigDir, "secrets", "env"), []byte("PATH=forbidden-placeholder\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := loadDaemonEnvironmentForOS(paths, "linux"); err != nil {
		t.Fatal("changed systemd environment semantics", err)
	}
	if err := loadDaemonEnvironmentForOS(paths, "windows"); err == nil {
		t.Fatal("Windows ignored invalid private env file")
	}
}
