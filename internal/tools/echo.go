package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// EchoTool is a small deterministic demo tool for exercising tool execution.
type EchoTool struct{}

// NewEchoTool creates the echo demo tool.
func NewEchoTool() EchoTool {
	return EchoTool{}
}

// Name returns the registry name for the echo tool.
func (EchoTool) Name() string {
	return "echo"
}

// Description explains the echo tool for model/tool metadata.
func (EchoTool) Description() string {
	return "Echo a message back unchanged"
}

// JSONSchema declares the arguments accepted by the echo tool.
func (EchoTool) JSONSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"message"},
		"properties": map[string]any{
			"message": map[string]any{
				"type":        "string",
				"description": "Message to echo back unchanged",
			},
		},
	}
}

// Execute returns the provided message unchanged.
func (EchoTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var parsed struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(args, &parsed); err != nil {
		return "", fmt.Errorf("invalid echo arguments: %w", err)
	}
	if strings.TrimSpace(parsed.Message) == "" {
		return "", errors.New("message is required")
	}
	return parsed.Message, nil
}
