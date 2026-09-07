package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zachfire9/agent-harness/internal/tools"
)

func TestRegistryRegistersAndListsTools(t *testing.T) {
	registry := tools.NewRegistry()
	search := fakeTool{
		name:        "search_files",
		description: "Search files in the workspace",
		schema:      map[string]any{"type": "object"},
	}
	read := fakeTool{
		name:        "read_file",
		description: "Read a workspace file",
		schema:      map[string]any{"type": "object"},
	}

	if err := registry.Register(search); err != nil {
		t.Fatalf("register search tool: %v", err)
	}
	if err := registry.Register(read); err != nil {
		t.Fatalf("register read tool: %v", err)
	}

	listed := registry.List()
	if len(listed) != 2 {
		t.Fatalf("expected two listed tools, got %#v", listed)
	}
	if listed[0].Name() != "read_file" || listed[1].Name() != "search_files" {
		t.Fatalf("expected deterministic name ordering, got %q then %q", listed[0].Name(), listed[1].Name())
	}
}

func TestRegistryRejectsDuplicateToolNames(t *testing.T) {
	registry := tools.NewRegistry()
	first := fakeTool{name: "read_file", description: "first", schema: map[string]any{"type": "object"}}
	duplicate := fakeTool{name: "read_file", description: "duplicate", schema: map[string]any{"type": "object"}}

	if err := registry.Register(first); err != nil {
		t.Fatalf("register first tool: %v", err)
	}

	err := registry.Register(duplicate)
	if err == nil {
		t.Fatal("expected duplicate registration error")
	}
	if !strings.Contains(err.Error(), "tool already registered: read_file") {
		t.Fatalf("expected duplicate tool error, got %q", err.Error())
	}
}

func TestRegistryRejectsInvalidToolMetadata(t *testing.T) {
	tests := []struct {
		name string
		tool fakeTool
		want string
	}{
		{
			name: "blank name",
			tool: fakeTool{name: "   ", description: "Blank name", schema: map[string]any{"type": "object"}},
			want: "tool name is required",
		},
		{
			name: "blank description",
			tool: fakeTool{name: "blank_description", description: "   ", schema: map[string]any{"type": "object"}},
			want: "tool description is required",
		},
		{
			name: "nil schema",
			tool: fakeTool{name: "nil_schema", description: "Nil schema"},
			want: "tool schema is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := tools.NewRegistry()
			err := registry.Register(tt.tool)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error, got %q", tt.want, err.Error())
			}
		})
	}
}

func TestRegistryLooksUpKnownTools(t *testing.T) {
	registry := tools.NewRegistry()
	read := fakeTool{name: "read_file", description: "Read a workspace file", schema: map[string]any{"type": "object"}}
	if err := registry.Register(read); err != nil {
		t.Fatalf("register read tool: %v", err)
	}

	found, ok := registry.Get("read_file")
	if !ok {
		t.Fatal("expected read_file to be found")
	}
	if found.Name() != "read_file" {
		t.Fatalf("expected read_file, got %q", found.Name())
	}
}

func TestRegistryUnknownToolReturnsHelpfulError(t *testing.T) {
	registry := tools.NewRegistry()

	_, err := registry.Require("missing_tool")
	if err == nil {
		t.Fatal("expected missing tool error")
	}
	if !strings.Contains(err.Error(), "unknown tool: missing_tool") {
		t.Fatalf("expected helpful unknown tool error, got %q", err.Error())
	}
}

func TestRegistryExposesSchemaMetadata(t *testing.T) {
	registry := tools.NewRegistry()
	tool := fakeTool{
		name:        "echo",
		description: "Echo a message",
		schema: map[string]any{
			"type":     "object",
			"required": []string{"message"},
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
		},
	}
	if err := registry.Register(tool); err != nil {
		t.Fatalf("register echo tool: %v", err)
	}

	metadata := registry.Metadata()
	if len(metadata) != 1 {
		t.Fatalf("expected one metadata entry, got %#v", metadata)
	}
	if metadata[0].Name != "echo" || metadata[0].Description != "Echo a message" {
		t.Fatalf("unexpected metadata: %#v", metadata[0])
	}

	schemaJSON, err := json.Marshal(metadata[0].Schema)
	if err != nil {
		t.Fatalf("schema should be JSON serializable: %v", err)
	}
	if !strings.Contains(string(schemaJSON), "message") {
		t.Fatalf("expected schema metadata to include message field, got %s", schemaJSON)
	}
}

type fakeTool struct {
	name        string
	description string
	schema      map[string]any
}

func (f fakeTool) Name() string               { return f.name }
func (f fakeTool) Description() string        { return f.description }
func (f fakeTool) JSONSchema() map[string]any { return f.schema }
func (f fakeTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return string(args), nil
}
