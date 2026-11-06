//go:build !windows

package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s: expected mode %o, got %o", path, want, got)
	}
}

func TestInitInstanceCreatesOwnerOnlyTree(t *testing.T) {
	paths := PathsForHome(filepath.Join(t.TempDir(), "default"))
	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance: %v", err)
	}
	for _, dir := range []string{paths.Home, paths.ConfigDir, paths.StateDir, paths.CacheDir, paths.WorkDir, paths.LogDir, filepath.Join(paths.ConfigDir, "secrets")} {
		assertMode(t, dir, 0o700)
	}
	assertMode(t, filepath.Join(paths.ConfigDir, "config.yaml"), 0o600)
}

func TestInitInstanceTightensExistingTreeWithoutRewritingConfig(t *testing.T) {
	paths := PathsForHome(filepath.Join(t.TempDir(), "default"))
	for _, dir := range []string{paths.Home, paths.ConfigDir, paths.StateDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	configPath := filepath.Join(paths.ConfigDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("custom: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(configPath, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := InitInstance(paths); err != nil {
		t.Fatalf("init instance: %v", err)
	}
	for _, dir := range []string{paths.Home, paths.ConfigDir, paths.StateDir} {
		assertMode(t, dir, 0o700)
	}
	assertMode(t, configPath, 0o600)
	if data, _ := os.ReadFile(configPath); string(data) != "custom: true\n" {
		t.Fatalf("existing config must not be rewritten, got %q", data)
	}
}

func TestStatusAndJobsFilesAreOwnerOnlyAndTightenedOnRewrite(t *testing.T) {
	paths := PathsForHome(filepath.Join(t.TempDir(), "default"))
	if err := ensurePrivateDir(paths.StateDir); err != nil {
		t.Fatal(err)
	}
	// Simulate files left world-readable by an older release.
	for _, path := range []string{paths.StatusPath, paths.JobsPath} {
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := WriteStatus(paths.StatusPath, Status{}); err != nil {
		t.Fatalf("write status: %v", err)
	}
	if err := WriteJobs(paths.JobsPath, JobsState{}); err != nil {
		t.Fatalf("write jobs: %v", err)
	}
	assertMode(t, paths.StatusPath, 0o600)
	assertMode(t, paths.JobsPath, 0o600)
	assertMode(t, paths.StateDir, 0o700)
}

func TestFileOutboxIsOwnerOnlyAndTightensExistingOutbox(t *testing.T) {
	paths := PathsForHome(filepath.Join(t.TempDir(), "default"))
	outbox := "state/outbox/test.jsonl"
	absPath := filepath.Join(paths.Home, filepath.FromSlash(outbox))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(absPath, 0o644); err != nil {
		t.Fatal(err)
	}

	notifier := fileOutboxNotifier{paths: paths, outboxPath: outbox}
	if err := notifier.Send(context.Background(), Message{Job: "notify-test", Body: "fake email body", Timestamp: time.Unix(0, 0).UTC()}); err != nil {
		t.Fatalf("send: %v", err)
	}
	assertMode(t, absPath, 0o600)
	assertMode(t, filepath.Dir(absPath), 0o700)
}
