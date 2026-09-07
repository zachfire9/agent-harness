package cli_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zachfire9/agent-harness/internal/cli"
	"github.com/zachfire9/agent-harness/internal/llm"
)

func TestRunDefaultInvocationPrintsPlaceholder(t *testing.T) {
	stdout, stderr, exitCode := runCLI("agent-harness")

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}

	want := "agent-harness: staged learning CLI ready\n"
	if stdout != want {
		t.Fatalf("expected stdout %q, got %q", want, stdout)
	}

	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
}

func TestRunAskCommandPrintsAssistantResponse(t *testing.T) {
	fake := &recordingChatClient{
		response: llm.ChatResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: "Agents are loops around model calls."}},
	}
	stdout, stderr, exitCode := runApp(fake, "gpt-test", "agent-harness", "ask", "What is an agent?")

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}
	if stdout != "Agents are loops around model calls.\n" {
		t.Fatalf("expected assistant response, got %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if fake.request.Model != "gpt-test" {
		t.Fatalf("expected configured model, got %q", fake.request.Model)
	}
	if len(fake.request.Messages) != 2 {
		t.Fatalf("expected system and user messages, got %#v", fake.request.Messages)
	}
	if fake.request.Messages[0].Role != llm.RoleSystem || !strings.Contains(fake.request.Messages[0].Content, "helpful CLI assistant") {
		t.Fatalf("expected helpful system message, got %#v", fake.request.Messages[0])
	}
	if fake.request.Messages[1] != (llm.Message{Role: llm.RoleUser, Content: "What is an agent?"}) {
		t.Fatalf("expected user prompt message, got %#v", fake.request.Messages[1])
	}
}

func TestRunAskCommandRequiresPrompt(t *testing.T) {
	stdout, stderr, exitCode := runCLI("agent-harness", "ask")

	if exitCode == 0 {
		t.Fatal("expected non-zero exit code for missing prompt")
	}

	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}

	if !strings.Contains(stderr, "ask requires a prompt") {
		t.Fatalf("expected helpful missing prompt error, got %q", stderr)
	}
}

func TestRunAskCommandReturnsModelError(t *testing.T) {
	fake := &recordingChatClient{err: errors.New("model unavailable")}
	stdout, stderr, exitCode := runApp(fake, "gpt-test", "agent-harness", "ask", "What is an agent?")

	if exitCode == 0 {
		t.Fatal("expected non-zero exit code for model error")
	}
	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "ask failed: model unavailable") {
		t.Fatalf("expected helpful model error, got %q", stderr)
	}
}

func TestRunUnknownCommandReturnsHelpfulError(t *testing.T) {
	stdout, stderr, exitCode := runCLI("agent-harness", "dance")

	if exitCode == 0 {
		t.Fatal("expected non-zero exit code for unknown command")
	}

	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}

	if !strings.Contains(stderr, "unknown command: dance") {
		t.Fatalf("expected helpful unknown command error, got %q", stderr)
	}
}

func runCLI(args ...string) (stdout string, stderr string, exitCode int) {
	var stdoutBuffer bytes.Buffer
	var stderrBuffer bytes.Buffer

	exitCode = cli.Run(args, &stdoutBuffer, &stderrBuffer)

	return stdoutBuffer.String(), stderrBuffer.String(), exitCode
}

func runApp(client llm.ChatClient, model string, args ...string) (stdout string, stderr string, exitCode int) {
	var stdoutBuffer bytes.Buffer
	var stderrBuffer bytes.Buffer

	app := cli.NewApp(client, model)
	exitCode = app.Run(args, &stdoutBuffer, &stderrBuffer)

	return stdoutBuffer.String(), stderrBuffer.String(), exitCode
}

type recordingChatClient struct {
	request  llm.ChatRequest
	response llm.ChatResponse
	err      error
}

func (r *recordingChatClient) Chat(ctx context.Context, request llm.ChatRequest) (llm.ChatResponse, error) {
	r.request = request
	if r.err != nil {
		return llm.ChatResponse{}, r.err
	}
	return r.response, nil
}
