package cli_test

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/zachfire9/agent-harness/internal/cli"
	"github.com/zachfire9/agent-harness/internal/llm"
)

// git status --short is in the default allowlist on every OS.
const approvalTestCommand = "git status --short"

func commandRequestingClient() *recordingChatClient {
	return &recordingChatClient{responses: []llm.ChatResponse{
		{Message: llm.Message{Role: llm.RoleAssistant}, ToolCalls: []llm.ToolCall{{
			ID: "call-1", Name: "run_command",
			Arguments: json.RawMessage(`{"command":"` + approvalTestCommand + `","confirm":true}`),
		}}},
		{Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}},
	}}
}

func runAppWithInputMode(client llm.ChatClient, input string, interactive *bool, args ...string) (string, string, int) {
	var stdout, stderr bytes.Buffer
	app := cli.NewAppWithInput(client, "gpt-test", strings.NewReader(input))
	if interactive != nil {
		app = app.WithInteractiveInput(*interactive)
	}
	code := app.Run(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

// toolResultSentToModel returns the run_command result the model saw.
func toolResultSentToModel(t *testing.T, client *recordingChatClient) string {
	t.Helper()
	if len(client.requests) < 2 {
		t.Fatalf("expected a follow-up model call after the tool call, got %d calls", len(client.requests))
	}
	messages := client.requests[1].Messages
	last := messages[len(messages)-1]
	if last.Role != llm.RoleTool || last.ToolCallID != "call-1" {
		t.Fatalf("expected last message to be the run_command result, got %#v", last)
	}
	return last.Content
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH; approved-run tests need a real allowlisted command")
	}
}

func boolPtr(b bool) *bool { return &b }

func TestAskDeclinesCommandsWithoutInteractiveTerminal(t *testing.T) {
	client := commandRequestingClient()
	// Piped input that says "y" must not be read as approval.
	stdout, stderr, code := runAppWithInputMode(client, "y\n", nil, "agent-harness", "ask", "check status")

	if code != 0 || strings.TrimSpace(stdout) != "done" {
		t.Fatalf("expected the turn to finish after a declined command; code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if strings.Contains(stderr, "Allow?") {
		t.Fatalf("non-interactive ask must not prompt, got stderr %q", stderr)
	}
	result := toolResultSentToModel(t, client)
	if !strings.Contains(result, `"approved":false`) || !strings.Contains(result, "no interactive terminal") || !strings.Contains(result, "--approve-commands") {
		t.Fatalf("expected a no-terminal decline the model can explain, got %q", result)
	}
}

func TestAskApproveCommandsFlagRunsWithoutPrompting(t *testing.T) {
	requireGit(t)
	client := commandRequestingClient()
	stdout, stderr, code := runAppWithInputMode(client, "", nil, "agent-harness", "ask", "--approve-commands", "check status")

	if code != 0 || strings.TrimSpace(stdout) != "done" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if strings.Contains(stderr, "Allow?") {
		t.Fatalf("--approve-commands must not prompt, got stderr %q", stderr)
	}
	if got := client.requests[0].Messages[1].Content; got != "check status" {
		t.Fatalf("expected --approve-commands removed from the prompt, got %q", got)
	}
	if result := toolResultSentToModel(t, client); !strings.Contains(result, `"exit_code"`) {
		t.Fatalf("expected the command to run, got %q", result)
	}
}

func TestChatRunsCommandOnlyAfterPersonApproves(t *testing.T) {
	requireGit(t)
	client := commandRequestingClient()
	stdout, stderr, code := runAppWithInputMode(client, "check status\ny\nexit\n", boolPtr(true), "agent-harness", "chat")

	if code != 0 || !strings.Contains(stdout, "Agent: done") || !strings.Contains(stdout, "Goodbye.") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "The agent wants to run: "+approvalTestCommand+"\nAllow? [y/N]: ") {
		t.Fatalf("expected an approval prompt naming the command, got stderr %q", stderr)
	}
	if result := toolResultSentToModel(t, client); !strings.Contains(result, `"exit_code"`) {
		t.Fatalf("expected the approved command to run, got %q", result)
	}
}

func TestChatDeclinedCommandIsReportedAndChatContinues(t *testing.T) {
	for _, answer := range []string{"n", "", "no", "yes please"} {
		t.Run("answer="+answer, func(t *testing.T) {
			client := commandRequestingClient()
			stdout, stderr, code := runAppWithInputMode(client, "check status\n"+answer+"\nexit\n", boolPtr(true), "agent-harness", "chat")

			// "exit" must still be read as the next chat prompt, not swallowed.
			if code != 0 || !strings.Contains(stdout, "Agent: done") || !strings.Contains(stdout, "Goodbye.") {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			result := toolResultSentToModel(t, client)
			if !strings.Contains(result, `"approved":false`) || !strings.Contains(result, "the user declined") {
				t.Fatalf("expected a decline for answer %q, got %q", answer, result)
			}
		})
	}
}

func TestChatApprovalAtEndOfInputDeclines(t *testing.T) {
	client := commandRequestingClient()
	_, stderr, code := runAppWithInputMode(client, "check status\n", boolPtr(true), "agent-harness", "chat")

	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if result := toolResultSentToModel(t, client); !strings.Contains(result, `"approved":false`) || !strings.Contains(result, "did not answer") {
		t.Fatalf("end of input at the prompt must decline, got %q", result)
	}
}

func TestToolDebugCommandDoesNotAskForApproval(t *testing.T) {
	requireGit(t)
	stdout, stderr, code := runCLI("agent-harness", "tool", "run_command", `{"command":"`+approvalTestCommand+`","confirm":true}`)

	if code != 0 || strings.Contains(stderr, "Allow?") || !strings.Contains(stdout, `"exit_code"`) {
		t.Fatalf("the person typed this command; expected it to run without a prompt. code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}
