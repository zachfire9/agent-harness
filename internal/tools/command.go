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
	allowedList     []AllowedCommand
	timeout         time.Duration
	approver        CommandApprover
}

// CommandApprover asks a person whether an allowlisted command may run.
//
// The model-supplied confirm field only shows the model meant to run the
// command; it is not human approval. An approver is the human gate. It is
// consulted only after the allowlist and confirm checks pass, so a person is
// never asked about a command that could not run anyway. When approved is
// false, reason is returned to the model so it can continue without the output.
type CommandApprover interface {
	ApproveCommand(ctx context.Context, command string) (approved bool, reason string)
}

// CommandApproverFunc adapts a function to CommandApprover.
type CommandApproverFunc func(ctx context.Context, command string) (bool, string)

// ApproveCommand calls f.
func (f CommandApproverFunc) ApproveCommand(ctx context.Context, command string) (bool, string) {
	return f(ctx, command)
}

type runCommandDeclined struct {
	Command  string `json:"command"`
	Approved bool   `json:"approved"`
	Reason   string `json:"reason"`
}

// AllowedCommand describes one exact command the model may request.
type AllowedCommand struct {
	Command     string
	Description string
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
func NewRunCommandTool(allowedCommands []AllowedCommand, timeout time.Duration) RunCommandTool {
	if timeout <= 0 {
		timeout = defaultRunCommandTimeout
	}
	allowed := make(map[string]bool, len(allowedCommands))
	allowedList := make([]AllowedCommand, 0, len(allowedCommands))
	for _, item := range allowedCommands {
		command := normalizeCommand(item.Command)
		if command != "" {
			if allowed[command] {
				continue
			}
			allowed[command] = true
			allowedList = append(allowedList, AllowedCommand{Command: command, Description: strings.TrimSpace(item.Description)})
		}
	}
	return RunCommandTool{allowedCommands: allowed, allowedList: allowedList, timeout: timeout}
}

// WithApprover returns a copy of the tool that asks approver before every run.
// Without an approver (for example the `tool` debug command, where the person
// typed the command themselves) allowlisted, confirmed commands run directly.
func (t RunCommandTool) WithApprover(approver CommandApprover) RunCommandTool {
	t.approver = approver
	return t
}

// Name returns the registry name for the gated command tool.
func (RunCommandTool) Name() string { return "run_command" }

// Description explains the tool policy to the model.
func (RunCommandTool) Description() string {
	return "Run an explicitly allowlisted local command only when confirmation is provided with confirm=true; captures stdout/stderr and enforces a timeout. The user may be asked to approve each run; if they decline, the result has approved=false and a reason, and the command did not run"
}

// JSONSchema declares run_command arguments.
func (t RunCommandTool) JSONSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"command", "confirm"},
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"enum":        t.allowedCommandNames(),
				"description": t.allowedCommandDescription(),
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
	if t.approver != nil {
		if approved, reason := t.approver.ApproveCommand(ctx, parsed.Command); !approved {
			// A declined command is a normal tool result, not an error, so the
			// agent turn continues and the model can answer without the output.
			payload, err := json.Marshal(runCommandDeclined{Command: parsed.Command, Approved: false, Reason: reason})
			if err != nil {
				return "", fmt.Errorf("encode declined command result: %w", err)
			}
			return string(payload), nil
		}
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

func (t RunCommandTool) allowedCommandNames() []string {
	names := make([]string, 0, len(t.allowedList))
	for _, item := range t.allowedList {
		names = append(names, item.Command)
	}
	return names
}

func (t RunCommandTool) allowedCommandDescription() string {
	if len(t.allowedList) == 0 {
		return "Exact allowlisted command to run. No commands are currently allowed."
	}

	var builder strings.Builder
	builder.WriteString("Exact allowlisted command to run. Allowed commands:")
	for _, item := range t.allowedList {
		builder.WriteString("\n- ")
		builder.WriteString(item.Command)
		if item.Description != "" {
			builder.WriteString(": ")
			builder.WriteString(item.Description)
		}
	}
	return builder.String()
}

func normalizeCommand(command string) string {
	return strings.Join(strings.Fields(command), " ")
}
