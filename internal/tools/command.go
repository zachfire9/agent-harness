package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const defaultRunCommandTimeout = 30 * time.Second

// RunCommandTool executes a small allowlisted command after explicit confirmation.
type RunCommandTool struct {
	allowedCommands map[string]bool
	timeout         time.Duration
}

type runCommandArgs struct {
	Command string `json:"command"`
	Confirm bool   `json:"confirm"`
}

type runCommandResult struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// NewRunCommandTool creates a gated command execution tool.
func NewRunCommandTool(allowedCommands []string, timeout time.Duration) RunCommandTool {
	if timeout <= 0 {
		timeout = defaultRunCommandTimeout
	}
	allowed := make(map[string]bool, len(allowedCommands))
	for _, command := range allowedCommands {
		command = normalizeCommand(command)
		if command != "" {
			allowed[command] = true
		}
	}
	return RunCommandTool{allowedCommands: allowed, timeout: timeout}
}

// Name returns the registry name for the gated command tool.
func (RunCommandTool) Name() string { return "run_command" }

// Description explains the tool policy to the model.
func (RunCommandTool) Description() string {
	return "Run an explicitly allowlisted local command only when confirmation is provided with confirm=true; captures stdout/stderr and enforces a timeout"
}

// JSONSchema declares run_command arguments.
func (RunCommandTool) JSONSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"command", "confirm"},
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Exact allowlisted command to run, such as pwd or git status",
			},
			"confirm": map[string]any{
				"type":        "boolean",
				"description": "Must be true to confirm execution of the command",
			},
		},
	}
}

// Execute validates confirmation and allowlist membership before running the command.
func (t RunCommandTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var parsed runCommandArgs
	if err := json.Unmarshal(args, &parsed); err != nil {
		return "", fmt.Errorf("invalid run_command arguments: %w", err)
	}
	parsed.Command = normalizeCommand(parsed.Command)
	if parsed.Command == "" {
		return "", errors.New("command is required")
	}
	if !parsed.Confirm {
		return "", errors.New("confirmation required: set confirm to true to run an allowlisted command")
	}
	if !t.allowedCommands[parsed.Command] {
		return "", fmt.Errorf("command is not allowed: %s", parsed.Command)
	}

	fields := strings.Fields(parsed.Command)
	if len(fields) == 0 {
		return "", errors.New("command is required")
	}

	commandCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	cmd := exec.CommandContext(commandCtx, fields[0], fields[1:]...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if commandCtx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command timed out after %s: %s", t.timeout, parsed.Command)
	}

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return "", fmt.Errorf("run command: %w", err)
		}
	}

	payload, err := json.Marshal(runCommandResult{
		Command:  parsed.Command,
		ExitCode: exitCode,
		Stdout:   capOutput(stdout.String(), defaultFileToolMaxOutputBytes),
		Stderr:   capOutput(stderr.String(), defaultFileToolMaxOutputBytes),
	})
	if err != nil {
		return "", fmt.Errorf("encode command result: %w", err)
	}
	return string(payload), nil
}

func normalizeCommand(command string) string {
	return strings.Join(strings.Fields(command), " ")
}
