package cli_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachfire9/agent-harness/internal/agent"
	"github.com/zachfire9/agent-harness/internal/cli"
	"github.com/zachfire9/agent-harness/internal/llm"
)

func TestRunAskWritesSessionLog(t *testing.T) {
	logDir := t.TempDir()
	fake := &recordingChatClient{
		responses: []llm.ChatResponse{
			{Message: llm.Message{Role: llm.RoleAssistant}, ToolCalls: []llm.ToolCall{{ID: "call-1", Name: "echo", Arguments: json.RawMessage(`{"message":"abcdefghijklmnopqrstuvwxyz"}`)}}},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}},
		},
	}
	app := cli.NewAppWithInputAndContextLimits(fake, "gpt-test", strings.NewReader(""), agent.ContextLimits{MaxMessages: 10, MaxMessageChars: 100, MaxToolResultChars: 5})
	app = app.WithRunLogging(logDir, true, []string{"secret-key"})

	stdout, stderr, exitCode := runAppInstance(app, "agent-harness", "ask", "please do not reveal secret-key")
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stdout: %s; stderr: %s", exitCode, stdout, stderr)
	}

	logPath := singleRunLogPath(t, logDir)
	entries := readRunLogEntries(t, logPath)
	if len(entries) < 4 {
		t.Fatalf("expected several run log events, got %#v", entries)
	}
	if entries[0]["type"] != "run.start" {
		t.Fatalf("expected first entry to be run.start, got %#v", entries[0])
	}
	if strings.Contains(readFileString(t, logPath), "secret-key") {
		t.Fatalf("expected run log to redact configured secrets, got %s", readFileString(t, logPath))
	}
	if !containsEventType(entries, "context.truncation") {
		t.Fatalf("expected structured context.truncation event, got %#v", entries)
	}
	if !containsEventType(entries, "run.final") {
		t.Fatalf("expected run.final event, got %#v", entries)
	}
}

func TestRunAskLogsFailedRun(t *testing.T) {
	logDir := t.TempDir()
	app := cli.NewAppWithInputAndContextLimits(&recordingChatClient{err: errors.New("model unavailable")}, "gpt-test", strings.NewReader(""), agent.ContextLimits{})
	app = app.WithRunLogging(logDir, true, nil)

	_, _, exitCode := runAppInstance(app, "agent-harness", "ask", "fail please")
	if exitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}

	entries := readRunLogEntries(t, singleRunLogPath(t, logDir))
	if !containsEventType(entries, "run.error") {
		t.Fatalf("expected run.error event, got %#v", entries)
	}
}

func TestRunLoggingCanBeDisabled(t *testing.T) {
	logDir := t.TempDir()
	app := cli.NewAppWithInputAndContextLimits(&recordingChatClient{response: llm.ChatResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}}}, "gpt-test", strings.NewReader(""), agent.ContextLimits{})
	app = app.WithRunLogging(logDir, false, nil)

	_, _, exitCode := runAppInstance(app, "agent-harness", "ask", "hello")
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	matches, err := filepath.Glob(filepath.Join(logDir, "*.jsonl"))
	if err != nil {
		t.Fatalf("glob log files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no run logs, got %#v", matches)
	}
}

func singleRunLogPath(t *testing.T, dir string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		t.Fatalf("glob log files: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one run log, got %#v", matches)
	}
	return matches[0]
}

func readRunLogEntries(t *testing.T, path string) []map[string]any {
	t.Helper()
	content := strings.TrimSpace(readFileString(t, path))
	if content == "" {
		t.Fatal("expected non-empty run log")
	}
	lines := strings.Split(content, "\n")
	entries := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("parse log line %q: %v", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func containsEventType(entries []map[string]any, eventType string) bool {
	for _, entry := range entries {
		if entry["type"] == eventType {
			return true
		}
	}
	return false
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s: %v", path, err)
	}
	return string(content)
}
