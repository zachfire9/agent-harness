package runlog_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachfire9/agent-harness/internal/llm"
	"github.com/zachfire9/agent-harness/internal/runlog"
)

func TestWriterCreatesJSONLRunLog(t *testing.T) {
	dir := t.TempDir()
	writer, err := runlog.NewWriter(dir, true, nil)
	if err != nil {
		t.Fatalf("create writer: %v", err)
	}

	if err := writer.Write(runlog.Event{Type: "run.start", Prompt: "hello"}); err != nil {
		t.Fatalf("write start event: %v", err)
	}
	if err := writer.Write(runlog.Event{Type: "run.final", Messages: []llm.Message{{Role: llm.RoleAssistant, Content: "done"}}}); err != nil {
		t.Fatalf("write final event: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	entries := readJSONLEvents(t, writer.Path())
	if len(entries) != 2 {
		t.Fatalf("expected two JSONL entries, got %#v", entries)
	}
	if entries[0]["type"] != "run.start" || entries[0]["run_id"] == "" || entries[0]["time"] == "" {
		t.Fatalf("expected start entry with metadata, got %#v", entries[0])
	}
	if entries[0]["prompt"] != "hello" {
		t.Fatalf("expected prompt to be logged, got %#v", entries[0])
	}
	if entries[1]["type"] != "run.final" {
		t.Fatalf("expected final entry, got %#v", entries[1])
	}
}

func TestWriterRedactsConfiguredSecrets(t *testing.T) {
	writer, err := runlog.NewWriter(t.TempDir(), true, []string{"secret-key"})
	if err != nil {
		t.Fatalf("create writer: %v", err)
	}

	if err := writer.Write(runlog.Event{
		Type:   "run.start",
		Prompt: "use secret-key carefully",
		Messages: []llm.Message{{
			Role:    llm.RoleUser,
			Content: "secret-key appears in history",
		}},
	}); err != nil {
		t.Fatalf("write event: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	content, err := os.ReadFile(writer.Path())
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if strings.Contains(string(content), "secret-key") {
		t.Fatalf("expected secret to be redacted, got %s", string(content))
	}
	if !strings.Contains(string(content), "[REDACTED]") {
		t.Fatalf("expected redaction marker, got %s", string(content))
	}
}

func TestDisabledWriterDoesNotCreateLogFile(t *testing.T) {
	dir := t.TempDir()
	writer, err := runlog.NewWriter(dir, false, nil)
	if err != nil {
		t.Fatalf("create disabled writer: %v", err)
	}
	if err := writer.Write(runlog.Event{Type: "run.start", Prompt: "hello"}); err != nil {
		t.Fatalf("disabled write should be a no-op: %v", err)
	}
	if writer.Path() != "" {
		t.Fatalf("expected disabled writer to have no path, got %q", writer.Path())
	}

	matches, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		t.Fatalf("glob logs: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no log files, got %#v", matches)
	}
}

func readJSONLEvents(t *testing.T, path string) []map[string]any {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	entries := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("parse JSONL line %q: %v", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}
