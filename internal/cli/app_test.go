package cli_test

import (
	"bytes"
	"context"
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

func TestRunAskCommandPrintsAgentAnswer(t *testing.T) {
	fake := &recordingChatClient{
		response: llm.ChatResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: "Agents are loops around model calls."}},
	}
	stdout, stderr, exitCode := runApp(fake, "gpt-test", "agent-harness", "ask", "What is an agent?")

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}
	if stdout != "Agents are loops around model calls.\n" {
		t.Fatalf("expected agent answer, got %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if fake.request.Model != "gpt-test" {
		t.Fatalf("expected configured model, got %q", fake.request.Model)
	}
	if len(fake.request.Messages) != 2 {
		t.Fatalf("expected agent to send system and user messages, got %#v", fake.request.Messages)
	}
	if fake.request.Messages[0].Role != llm.RoleSystem || !strings.Contains(fake.request.Messages[0].Content, "helpful CLI assistant") {
		t.Fatalf("expected agent system message, got %#v", fake.request.Messages[0])
	}
	if fake.request.Messages[1].Role != llm.RoleUser || fake.request.Messages[1].Content != "What is an agent?" {
		t.Fatalf("expected user prompt message, got %#v", fake.request.Messages[1])
	}
}

func TestRunAskCommandWithTracePrintsTraceToStderr(t *testing.T) {
	fake := &recordingChatClient{
		response: llm.ChatResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: "trace answer"}},
	}
	stdout, stderr, exitCode := runApp(fake, "gpt-test", "agent-harness", "ask", "--trace", "What is an agent?")

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}
	if stdout != "trace answer\n" {
		t.Fatalf("expected answer on stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "[trace] model call step=1 model=gpt-test messages=2") {
		t.Fatalf("expected model call trace, got %q", stderr)
	}
	if !strings.Contains(stderr, "[trace] final answer step=1 chars=12") {
		t.Fatalf("expected final answer trace, got %q", stderr)
	}
	if strings.Contains(stderr, "What is an agent?") {
		t.Fatalf("trace should not dump prompt content, got %q", stderr)
	}
	if fake.request.Messages[1].Content != "What is an agent?" {
		t.Fatalf("expected --trace removed from prompt, got %#v", fake.request.Messages[1])
	}
}

func TestRunAskTraceRecordsToolCallAndResult(t *testing.T) {
	fake := &recordingChatClient{
		responses: []llm.ChatResponse{
			{Message: llm.Message{Role: llm.RoleAssistant}, ToolCalls: []llm.ToolCall{{ID: "call-1", Name: "echo", Arguments: json.RawMessage(`{"message":"hello"}`)}}},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}},
		},
	}
	stdout, stderr, exitCode := runApp(fake, "gpt-test", "agent-harness", "ask", "--trace", "use echo")

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stdout: %s; stderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stderr, "[trace] tool call step=1 name=echo id=call-1") {
		t.Fatalf("expected tool call trace, got %q", stderr)
	}
	if !strings.Contains(stderr, "[trace] tool result step=1 name=echo id=call-1 chars=5") {
		t.Fatalf("expected tool result summary trace, got %q", stderr)
	}
}

func TestRunAskTraceRecordsContextReport(t *testing.T) {
	fake := &recordingChatClient{
		responses: []llm.ChatResponse{
			{Message: llm.Message{Role: llm.RoleAssistant}, ToolCalls: []llm.ToolCall{{ID: "call-1", Name: "echo", Arguments: json.RawMessage(`{"message":"abcdefghijklmnopqrstuvwxyz"}`)}}},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}},
		},
	}
	stdout, stderr, exitCode := runAppWithContextLimits(fake, "gpt-test", agent.ContextLimits{MaxMessages: 10, MaxMessageChars: 100, MaxToolResultChars: 5}, "agent-harness", "ask", "--trace", "use echo")

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stdout: %s; stderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stderr, "[trace] context build step=2") {
		t.Fatalf("expected context build trace, got %q", stderr)
	}
	if !strings.Contains(stderr, "truncations=1") {
		t.Fatalf("expected context truncation count in trace, got %q", stderr)
	}
	if !strings.Contains(stderr, "[trace] context truncation step=2 role=tool") {
		t.Fatalf("expected context truncation details in trace, got %q", stderr)
	}
	if strings.Contains(stderr, "abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("trace should not dump full tool result content, got %q", stderr)
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

func TestRunAskCommandReturnsAgentError(t *testing.T) {
	fake := &recordingChatClient{err: errors.New("model unavailable")}
	stdout, stderr, exitCode := runApp(fake, "gpt-test", "agent-harness", "ask", "What is an agent?")

	if exitCode == 0 {
		t.Fatal("expected non-zero exit code for model error")
	}
	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "ask failed: chat failed: model unavailable") {
		t.Fatalf("expected helpful agent error, got %q", stderr)
	}
}

func TestRunChatCommandLoopsUntilExit(t *testing.T) {
	fake := &recordingChatClient{
		responses: []llm.ChatResponse{
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "Agents are loops."}},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "Simpler: a helper that can call tools."}},
		},
	}
	stdout, stderr, exitCode := runAppWithInput(fake, "gpt-test", "What is an agent?\nCan you give a simpler example?\n/exit\n", "agent-harness", "chat")

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	for _, want := range []string{"You: ", "Agent: Agents are loops.\n", "Agent: Simpler: a helper that can call tools.\n", "Goodbye.\n"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected stdout to contain %q, got %q", want, stdout)
		}
	}
	if len(fake.requests) != 2 {
		t.Fatalf("expected two model requests, got %d", len(fake.requests))
	}
	secondMessages := fake.requests[1].Messages
	if len(secondMessages) != 4 {
		t.Fatalf("expected second turn to include prior conversation, got %#v", secondMessages)
	}
	if secondMessages[1].Role != llm.RoleUser || secondMessages[1].Content != "What is an agent?" {
		t.Fatalf("expected first user message in history, got %#v", secondMessages[1])
	}
	if secondMessages[2].Role != llm.RoleAssistant || secondMessages[2].Content != "Agents are loops." {
		t.Fatalf("expected first assistant message in history, got %#v", secondMessages[2])
	}
	if secondMessages[3].Role != llm.RoleUser || secondMessages[3].Content != "Can you give a simpler example?" {
		t.Fatalf("expected second user message in request, got %#v", secondMessages[3])
	}
}

func TestRunChatCommandExitAliasesEndCleanly(t *testing.T) {
	for _, input := range []string{"exit\n", "quit\n", "/exit\n", ""} {
		t.Run(input, func(t *testing.T) {
			stdout, stderr, exitCode := runAppWithInput(&recordingChatClient{}, "gpt-test", input, "agent-harness", "chat")
			if exitCode != 0 {
				t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
			}
			if stderr != "" {
				t.Fatalf("expected empty stderr, got %q", stderr)
			}
			if !strings.Contains(stdout, "You: ") || !strings.Contains(stdout, "Goodbye.\n") {
				t.Fatalf("expected prompt and goodbye, got %q", stdout)
			}
		})
	}
}

func TestRunChatCommandModelErrorDoesNotCorruptHistory(t *testing.T) {
	fake := &recordingChatClient{
		responses: []llm.ChatResponse{
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "Recovered answer."}},
		},
		errs: []error{errors.New("temporary model failure")},
	}
	stdout, stderr, exitCode := runAppWithInput(fake, "gpt-test", "bad turn\nsecond turn\nexit\n", "agent-harness", "chat")

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stderr, "chat turn failed: chat failed: temporary model failure") {
		t.Fatalf("expected controlled chat error, got %q", stderr)
	}
	if !strings.Contains(stdout, "Agent: Recovered answer.\n") {
		t.Fatalf("expected later successful answer, got %q", stdout)
	}
	if len(fake.requests) != 2 {
		t.Fatalf("expected two model requests, got %d", len(fake.requests))
	}
	if len(fake.requests[1].Messages) != 2 || fake.requests[1].Messages[1].Content != "second turn" {
		t.Fatalf("expected failed turn not to be retained in history, got %#v", fake.requests[1].Messages)
	}
}

func TestRunChatCommandAllowsToolCallsInsideTurn(t *testing.T) {
	fake := &recordingChatClient{
		responses: []llm.ChatResponse{
			{
				Message:   llm.Message{Role: llm.RoleAssistant},
				ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "echo", Arguments: []byte(`{"message":"from tool"}`)}},
			},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "Tool said from tool."}},
		},
	}
	stdout, stderr, exitCode := runAppWithInput(fake, "gpt-test", "use a tool\nexit\n", "agent-harness", "chat")

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "Agent: Tool said from tool.\n") {
		t.Fatalf("expected final tool-backed answer, got %q", stdout)
	}
	if len(fake.requests) != 2 {
		t.Fatalf("expected model to be called before and after tool execution, got %d calls", len(fake.requests))
	}
	messagesAfterTool := fake.requests[1].Messages
	if got := messagesAfterTool[len(messagesAfterTool)-1]; got.Role != llm.RoleTool || got.Content != "from tool" || got.ToolCallID != "call_1" {
		t.Fatalf("expected tool result message in second request, got %#v", got)
	}
}

func TestRunAskCommandSendsWorkspaceFileToolsToModel(t *testing.T) {
	fake := &recordingChatClient{
		response: llm.ChatResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}},
	}
	stdout, stderr, exitCode := runApp(fake, "gpt-test", "agent-harness", "ask", "What files are here?")

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stdout: %s; stderr: %s", exitCode, stdout, stderr)
	}
	toolNames := map[string]bool{}
	for _, tool := range fake.request.Tools {
		toolNames[tool.Name] = true
	}
	for _, want := range []string{"echo", "list_files", "read_file", "search_files"} {
		if !toolNames[want] {
			t.Fatalf("expected model request to include %s tool metadata, got %#v", want, fake.request.Tools)
		}
	}
}

func TestRunToolCommandExecutesListFilesTool(t *testing.T) {
	root := t.TempDir()
	writeCLITestFile(t, root, "README.md", "hello")
	writeCLITestFile(t, root, "docs/intro.txt", "intro")
	t.Chdir(root)

	stdout, stderr, exitCode := runCLI("agent-harness", "tool", "list_files", `{"path":"."}`)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "README.md") || !strings.Contains(stdout, "docs/intro.txt") {
		t.Fatalf("expected listed workspace files, got %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
}

func TestRunToolCommandExecutesReadFileTool(t *testing.T) {
	root := t.TempDir()
	writeCLITestFile(t, root, "docs/intro.txt", "hello workspace")
	t.Chdir(root)

	stdout, stderr, exitCode := runCLI("agent-harness", "tool", "read_file", `{"path":"docs/intro.txt"}`)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}
	if stdout != "hello workspace\n" {
		t.Fatalf("expected file output, got %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
}

func TestRunToolCommandExecutesSearchFilesTool(t *testing.T) {
	root := t.TempDir()
	writeCLITestFile(t, root, "README.md", "agent harness")
	writeCLITestFile(t, root, "notes.txt", "nothing")
	t.Chdir(root)

	stdout, stderr, exitCode := runCLI("agent-harness", "tool", "search_files", `{"query":"agent","path":"."}`)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "README.md: agent harness") || strings.Contains(stdout, "notes.txt") {
		t.Fatalf("expected only matching search output, got %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
}

func TestRunToolCommandRejectsWorkspaceEscape(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	stdout, stderr, exitCode := runCLI("agent-harness", "tool", "read_file", `{"path":"../outside.txt"}`)

	if exitCode == 0 {
		t.Fatal("expected non-zero exit code for path outside workspace")
	}
	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "path escapes workspace") {
		t.Fatalf("expected workspace escape error, got %q", stderr)
	}
}

func TestRunToolCommandExecutesEchoTool(t *testing.T) {
	stdout, stderr, exitCode := runCLI("agent-harness", "tool", "echo", `{"message":"hi"}`)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}
	if stdout != "hi\n" {
		t.Fatalf("expected tool output, got %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
}

func TestRunToolCommandRequiresToolNameAndJSONArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing tool name", args: []string{"agent-harness", "tool"}, want: "tool requires a tool name and JSON args"},
		{name: "missing JSON args", args: []string{"agent-harness", "tool", "echo"}, want: "tool requires a tool name and JSON args"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := runCLI(tt.args...)
			if exitCode == 0 {
				t.Fatal("expected non-zero exit code")
			}
			if stdout != "" {
				t.Fatalf("expected empty stdout, got %q", stdout)
			}
			if !strings.Contains(stderr, tt.want) {
				t.Fatalf("expected helpful usage error containing %q, got %q", tt.want, stderr)
			}
		})
	}
}

func TestRunToolCommandUnknownToolReturnsHelpfulError(t *testing.T) {
	stdout, stderr, exitCode := runCLI("agent-harness", "tool", "missing_tool", `{"message":"hi"}`)

	if exitCode == 0 {
		t.Fatal("expected non-zero exit code for unknown tool")
	}
	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "unknown tool: missing_tool") {
		t.Fatalf("expected helpful unknown tool error, got %q", stderr)
	}
}

func TestRunToolCommandInvalidJSONReturnsHelpfulError(t *testing.T) {
	stdout, stderr, exitCode := runCLI("agent-harness", "tool", "echo", `{"message":`)

	if exitCode == 0 {
		t.Fatal("expected non-zero exit code for invalid JSON")
	}
	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "tool failed: invalid echo arguments") {
		t.Fatalf("expected helpful invalid JSON error, got %q", stderr)
	}
}

func TestRunToolCommandToolExecutionErrorReturnsHelpfulError(t *testing.T) {
	stdout, stderr, exitCode := runCLI("agent-harness", "tool", "echo", `{}`)

	if exitCode == 0 {
		t.Fatal("expected non-zero exit code for tool execution error")
	}
	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "tool failed: message is required") {
		t.Fatalf("expected helpful tool execution error, got %q", stderr)
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

func TestRunChatLogsContextTruncationWarning(t *testing.T) {
	fake := &recordingChatClient{
		responses: []llm.ChatResponse{
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "this assistant response will be truncated on the next turn"}},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "second answer"}},
		},
	}
	stdout, stderr, exitCode := runAppWithInputAndContextLimits(fake, "gpt-test", agent.ContextLimits{MaxMessages: 10, MaxMessageChars: 12, MaxToolResultChars: 100}, "short\nagain\nexit\n", "agent-harness", "chat")

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stdout: %s; stderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stderr, "context warning: truncated assistant message") {
		t.Fatalf("expected truncation warning in stderr, got %q", stderr)
	}
	if !strings.Contains(stderr, "omitted=") {
		t.Fatalf("expected truncation details in stderr, got %q", stderr)
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

func runAppWithContextLimits(client llm.ChatClient, model string, limits agent.ContextLimits, args ...string) (stdout string, stderr string, exitCode int) {
	var stdoutBuffer bytes.Buffer
	var stderrBuffer bytes.Buffer

	app := cli.NewAppWithInputAndContextLimits(client, model, strings.NewReader(""), limits)
	exitCode = app.Run(args, &stdoutBuffer, &stderrBuffer)

	return stdoutBuffer.String(), stderrBuffer.String(), exitCode
}

func runAppWithInput(client llm.ChatClient, model string, input string, args ...string) (stdout string, stderr string, exitCode int) {
	var stdoutBuffer bytes.Buffer
	var stderrBuffer bytes.Buffer

	app := cli.NewAppWithInput(client, model, strings.NewReader(input))
	exitCode = app.Run(args, &stdoutBuffer, &stderrBuffer)

	return stdoutBuffer.String(), stderrBuffer.String(), exitCode
}

func runAppWithInputAndContextLimits(client llm.ChatClient, model string, limits agent.ContextLimits, input string, args ...string) (stdout string, stderr string, exitCode int) {
	var stdoutBuffer bytes.Buffer
	var stderrBuffer bytes.Buffer

	app := cli.NewAppWithInputAndContextLimits(client, model, strings.NewReader(input), limits)
	exitCode = app.Run(args, &stdoutBuffer, &stderrBuffer)

	return stdoutBuffer.String(), stderrBuffer.String(), exitCode
}

func writeCLITestFile(t *testing.T, root string, relativePath string, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

type recordingChatClient struct {
	request   llm.ChatRequest
	requests  []llm.ChatRequest
	response  llm.ChatResponse
	responses []llm.ChatResponse
	err       error
	errs      []error
}

func (r *recordingChatClient) Chat(ctx context.Context, request llm.ChatRequest) (llm.ChatResponse, error) {
	r.request = request
	r.requests = append(r.requests, request)
	if len(r.errs) > 0 {
		err := r.errs[0]
		r.errs = r.errs[1:]
		if err != nil {
			return llm.ChatResponse{}, err
		}
	}
	if r.err != nil {
		return llm.ChatResponse{}, r.err
	}
	if len(r.responses) > 0 {
		response := r.responses[0]
		r.responses = r.responses[1:]
		return response, nil
	}
	return r.response, nil
}
