package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Tool is a named, schema-described function controlled by the harness.
type Tool interface {
	Name() string
	Description() string
	JSONSchema() map[string]any
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}

// Metadata describes a registered tool for model/tool-call prompts.
type Metadata struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Schema      map[string]any `json:"schema"`
}

// Registry stores tools by name and exposes deterministic lookup/listing behavior.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() Registry {
	return Registry{tools: map[string]Tool{}}
}

// Register validates and stores a tool by name.
func (r Registry) Register(tool Tool) error {
	if r.tools == nil {
		r.tools = map[string]Tool{}
	}

	name := strings.TrimSpace(tool.Name())
	if name == "" {
		return errors.New("tool name is required")
	}
	if strings.TrimSpace(tool.Description()) == "" {
		return fmt.Errorf("tool description is required: %s", name)
	}
	if tool.JSONSchema() == nil {
		return fmt.Errorf("tool schema is required: %s", name)
	}
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool already registered: %s", name)
	}

	r.tools[name] = tool
	return nil
}

// Get returns a registered tool by name.
func (r Registry) Get(name string) (Tool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

// Require returns a registered tool or a helpful error.
func (r Registry) Require(name string) (Tool, error) {
	tool, ok := r.Get(name)
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return tool, nil
}

// List returns registered tools sorted by name for deterministic tests/prompts.
func (r Registry) List() []Tool {
	names := r.sortedNames()
	listed := make([]Tool, 0, len(names))
	for _, name := range names {
		listed = append(listed, r.tools[name])
	}
	return listed
}

// Metadata returns tool metadata sorted by tool name.
func (r Registry) Metadata() []Metadata {
	names := r.sortedNames()
	metadata := make([]Metadata, 0, len(names))
	for _, name := range names {
		tool := r.tools[name]
		metadata = append(metadata, Metadata{
			Name:        name,
			Description: strings.TrimSpace(tool.Description()),
			Schema:      tool.JSONSchema(),
		})
	}
	return metadata
}

func (r Registry) sortedNames() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
