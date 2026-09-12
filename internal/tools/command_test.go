package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/zachfire9/agent-harness/internal/tools"
)

func TestRunCommandToolExecutesAllowedConfirmedCommand(t *testing.T) {
	tool := tools.NewRunCommandTool([]tools.AllowedCommand{{Command: "pwd"}}, time.Second)

	got, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"pwd","confirm":true}`))
	if err != nil {
		t.Fatalf("execute run_command: %v", err)
	}

	if !strings.Contains(got, `"exit_code":0`) {
		t.Fatalf("expected successful exit code in JSON result, got %q", got)
	}
	if !strings.Contains(got, `"stdout"`) || !strings.Contains(got, `"stderr"`) {
		t.Fatalf("expected stdout/stderr fields in JSON result, got %q", got)
	}
}

func TestRunCommandToolRejectsBlockedCommand(t *testing.T) {
	tool := tools.NewRunCommandTool([]tools.AllowedCommand{{Command: "pwd"}}, time.Second)

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"rm -rf .","confirm":true}`))
	if err == nil {
		t.Fatal("expected blocked command error")
	}
	if !strings.Contains(err.Error(), "command is not allowed") {
		t.Fatalf("expected allowlist error, got %q", err.Error())
	}
}

func TestRunCommandToolRequiresConfirmation(t *testing.T) {
	tool := tools.NewRunCommandTool([]tools.AllowedCommand{{Command: "pwd"}}, time.Second)

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"pwd"}`))
	if err == nil {
		t.Fatal("expected confirmation error")
	}
	if !strings.Contains(err.Error(), "confirmation required") {
		t.Fatalf("expected confirmation error, got %q", err.Error())
	}
}

func TestRunCommandToolEnforcesTimeout(t *testing.T) {
	tool := tools.NewRunCommandTool([]tools.AllowedCommand{{Command: "sleep 1"}}, time.Nanosecond)

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"sleep 1","confirm":true}`))
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "command timed out") {
		t.Fatalf("expected timeout error, got %q", err.Error())
	}
}

func TestRunCommandToolCapturesStdoutAndStderr(t *testing.T) {
	tool := tools.NewRunCommandTool([]tools.AllowedCommand{{Command: "go version"}}, time.Second)

	got, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"go version","confirm":true}`))
	if err != nil {
		t.Fatalf("execute run_command: %v", err)
	}
	if !strings.Contains(got, `"stdout"`) || !strings.Contains(got, "go version") {
		t.Fatalf("expected stdout to include go version output, got %q", got)
	}
}

func TestRunCommandToolSchemaIncludesAllowedCommandEnumAndDescriptions(t *testing.T) {
	tool := tools.NewRunCommandTool([]tools.AllowedCommand{
		{Command: "custom-index --dry-run", Description: "Validate the local custom index without writing changes"},
		{Command: "pwd"},
	}, time.Second)

	commandSchema, ok := tool.JSONSchema()["properties"].(map[string]any)["command"].(map[string]any)
	if !ok {
		t.Fatalf("expected command schema, got %#v", tool.JSONSchema())
	}

	enum, ok := commandSchema["enum"].([]string)
	if !ok {
		t.Fatalf("expected string enum, got %#v", commandSchema["enum"])
	}
	if strings.Join(enum, ",") != "custom-index --dry-run,pwd" {
		t.Fatalf("expected enum to expose allowed commands in order, got %#v", enum)
	}
	description := commandSchema["description"].(string)
	if !strings.Contains(description, "custom-index --dry-run: Validate the local custom index without writing changes") {
		t.Fatalf("expected custom command description in schema, got %q", description)
	}
	if !strings.Contains(description, "\n- pwd") {
		t.Fatalf("expected command without description to still be listed, got %q", description)
	}
}

func TestRunCommandToolSchemaDeclaresConfirmation(t *testing.T) {
	tool := tools.NewRunCommandTool([]tools.AllowedCommand{{Command: "pwd"}}, time.Second)

	schema := tool.JSONSchema()
	if schema["type"] != "object" {
		t.Fatalf("expected object schema, got %#v", schema)
	}
	if !strings.Contains(tool.Description(), "confirmation") {
		t.Fatalf("expected description to mention confirmation, got %q", tool.Description())
	}
}
