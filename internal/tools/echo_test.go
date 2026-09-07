package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zachfire9/agent-harness/internal/tools"
)

func TestEchoToolReturnsMessageFromValidArgs(t *testing.T) {
	tool := tools.NewEchoTool()

	got, err := tool.Execute(context.Background(), json.RawMessage(`{"message":"hello tools"}`))
	if err != nil {
		t.Fatalf("execute echo tool: %v", err)
	}

	if got != "hello tools" {
		t.Fatalf("expected echoed message, got %q", got)
	}
}

func TestEchoToolInvalidJSONReturnsClearError(t *testing.T) {
	tool := tools.NewEchoTool()

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"message":`))
	if err == nil {
		t.Fatal("expected invalid JSON error")
	}
	if !strings.Contains(err.Error(), "invalid echo arguments") {
		t.Fatalf("expected clear invalid JSON error, got %q", err.Error())
	}
}

func TestEchoToolMissingMessageReturnsClearError(t *testing.T) {
	tool := tools.NewEchoTool()

	_, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected missing message error")
	}
	if !strings.Contains(err.Error(), "message is required") {
		t.Fatalf("expected missing message error, got %q", err.Error())
	}
}

func TestEchoToolSchemaDeclaresRequiredMessageField(t *testing.T) {
	tool := tools.NewEchoTool()

	if tool.Name() != "echo" {
		t.Fatalf("expected echo tool name, got %q", tool.Name())
	}
	if !strings.Contains(strings.ToLower(tool.Description()), "echo") {
		t.Fatalf("expected echo description, got %q", tool.Description())
	}

	schema := tool.JSONSchema()
	if schema["type"] != "object" {
		t.Fatalf("expected object schema, got %#v", schema)
	}

	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatalf("expected required fields as []string, got %#v", schema["required"])
	}
	if len(required) != 1 || required[0] != "message" {
		t.Fatalf("expected message to be required, got %#v", required)
	}

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map, got %#v", schema["properties"])
	}
	message, ok := properties["message"].(map[string]any)
	if !ok {
		t.Fatalf("expected message property schema, got %#v", properties["message"])
	}
	if message["type"] != "string" {
		t.Fatalf("expected message to be a string, got %#v", message)
	}
}

func TestEchoToolCanBeRegistered(t *testing.T) {
	registry := tools.NewRegistry()

	if err := registry.Register(tools.NewEchoTool()); err != nil {
		t.Fatalf("register echo tool: %v", err)
	}

	registered, ok := registry.Get("echo")
	if !ok {
		t.Fatal("expected echo tool to be registered")
	}
	got, err := registered.Execute(context.Background(), json.RawMessage(`{"message":"registered"}`))
	if err != nil {
		t.Fatalf("execute registered echo tool: %v", err)
	}
	if got != "registered" {
		t.Fatalf("expected registered echo output, got %q", got)
	}
}
