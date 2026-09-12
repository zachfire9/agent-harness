package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/zachfire9/agent-harness/internal/agent"
	"github.com/zachfire9/agent-harness/internal/llm"
	"github.com/zachfire9/agent-harness/internal/tools"
	"github.com/zachfire9/agent-harness/internal/vectorstore"
)

func TestRunnerCanDependOnVectorStoreInterface(t *testing.T) {
	runner := agent.NewWithVectorStore(&recordingClient{}, "gpt-test", vectorstore.NewNoopStore())

	if runner.VectorStoreProvider() != "none" {
		t.Fatalf("expected runner to expose vector store provider, got %q", runner.VectorStoreProvider())
	}
}

func TestRunBuildsInitialMessageHistory(t *testing.T) {
	client := &recordingClient{
		responses: []llm.ChatResponse{{Message: llm.Message{Role: llm.RoleAssistant, Content: "Agents use model calls and tools."}}},
	}
	runner := agent.New(client, "gpt-test")

	result, err := runner.Run(context.Background(), "What is an agent?")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if result.Answer != "Agents use model calls and tools." {
		t.Fatalf("expected assistant answer, got %q", result.Answer)
	}
	if len(result.Messages) != 3 {
		t.Fatalf("expected system, user, and assistant messages, got %#v", result.Messages)
	}
	if result.Messages[0].Role != llm.RoleSystem || !strings.Contains(result.Messages[0].Content, "helpful CLI assistant") {
		t.Fatalf("expected helpful system message first, got %#v", result.Messages[0])
	}
	if result.Messages[1].Role != llm.RoleUser || result.Messages[1].Content != "What is an agent?" {
		t.Fatalf("expected user prompt second, got %#v", result.Messages[1])
	}
	if result.Messages[2].Role != llm.RoleAssistant || result.Messages[2].Content != "Agents use model calls and tools." {
		t.Fatalf("expected assistant response third, got %#v", result.Messages[2])
	}
}

func TestRunSendsConfiguredModelAndInitialMessages(t *testing.T) {
	client := &recordingClient{
		responses: []llm.ChatResponse{{Message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}}},
	}
	runner := agent.New(client, "gpt-test")

	_, err := runner.Run(context.Background(), "Hello")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if client.requests[0].Model != "gpt-test" {
		t.Fatalf("expected configured model, got %q", client.requests[0].Model)
	}
	if len(client.requests[0].Messages) != 2 {
		t.Fatalf("expected system and user messages sent to model, got %#v", client.requests[0].Messages)
	}
	if client.requests[0].Messages[0].Role != llm.RoleSystem {
		t.Fatalf("expected system message first, got %#v", client.requests[0].Messages[0])
	}
	if client.requests[0].Messages[1].Role != llm.RoleUser || client.requests[0].Messages[1].Content != "Hello" {
		t.Fatalf("expected user prompt second, got %#v", client.requests[0].Messages[1])
	}
}

func TestRunFinalAnswerWithoutToolCallsReturnsImmediately(t *testing.T) {
	client := &recordingClient{
		responses: []llm.ChatResponse{{Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}}},
	}
	runner := agent.NewWithTools(client, "gpt-test", registryWith(&stubTool{name: "echo", result: "unused"}))

	result, err := runner.Run(context.Background(), "answer directly")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if result.Answer != "done" {
		t.Fatalf("expected final answer, got %q", result.Answer)
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected one model call, got %d", len(client.requests))
	}
	if len(client.requests[0].Tools) != 1 || client.requests[0].Tools[0].Name != "echo" {
		t.Fatalf("expected registered tool metadata in model request, got %#v", client.requests[0].Tools)
	}
}

func TestRunExecutesRequestedToolAndSendsResultBackToModel(t *testing.T) {
	tool := &stubTool{name: "echo", result: "echoed hi"}
	client := &recordingClient{
		responses: []llm.ChatResponse{
			{
				Message:   llm.Message{Role: llm.RoleAssistant},
				ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "echo", Arguments: json.RawMessage(`{"message":"hi"}`)}},
			},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "final after tool"}},
		},
	}
	runner := agent.NewWithTools(client, "gpt-test", registryWith(tool))

	result, err := runner.Run(context.Background(), "use echo")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if string(tool.gotArgs) != `{"message":"hi"}` {
		t.Fatalf("expected tool args to be passed through, got %s", tool.gotArgs)
	}
	if result.Answer != "final after tool" {
		t.Fatalf("expected final answer after tool result, got %q", result.Answer)
	}
	if len(client.requests) != 2 {
		t.Fatalf("expected two model calls, got %d", len(client.requests))
	}
	secondMessages := client.requests[1].Messages
	if len(secondMessages) != 4 {
		t.Fatalf("expected system, user, assistant tool request, and tool result messages, got %#v", secondMessages)
	}
	if secondMessages[2].Role != llm.RoleAssistant || len(secondMessages[2].ToolCalls) != 1 {
		t.Fatalf("expected assistant tool-call message before tool result, got %#v", secondMessages[2])
	}
	if secondMessages[3].Role != llm.RoleTool || secondMessages[3].Content != "echoed hi" || secondMessages[3].ToolCallID != "call_1" {
		t.Fatalf("expected tool result message sent back to model, got %#v", secondMessages[3])
	}
}

func TestRunWithHistoryIncludesEarlierConversationOnNextTurn(t *testing.T) {
	client := &recordingClient{
		responses: []llm.ChatResponse{
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "Agents use loops."}},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "A simpler example is a helper that can use tools."}},
		},
	}
	runner := agent.New(client, "gpt-test")

	first, err := runner.Run(context.Background(), "What is an agent?")
	if err != nil {
		t.Fatalf("first Run returned error: %v", err)
	}
	second, err := runner.RunWithHistory(context.Background(), first.Messages, "Can you give a simpler example?")
	if err != nil {
		t.Fatalf("RunWithHistory returned error: %v", err)
	}

	if second.Answer != "A simpler example is a helper that can use tools." {
		t.Fatalf("expected second answer, got %q", second.Answer)
	}
	if len(client.requests) != 2 {
		t.Fatalf("expected two model requests, got %d", len(client.requests))
	}
	secondRequestMessages := client.requests[1].Messages
	want := []llm.Message{
		{Role: llm.RoleSystem, Content: "You are a helpful CLI assistant. Answer clearly and concisely."},
		{Role: llm.RoleUser, Content: "What is an agent?"},
		{Role: llm.RoleAssistant, Content: "Agents use loops."},
		{Role: llm.RoleUser, Content: "Can you give a simpler example?"},
	}
	if len(secondRequestMessages) != len(want) {
		t.Fatalf("expected history plus next user prompt, got %#v", secondRequestMessages)
	}
	for i := range want {
		if secondRequestMessages[i].Role != want[i].Role || secondRequestMessages[i].Content != want[i].Content {
			t.Fatalf("message %d mismatch: want %#v, got %#v", i, want[i], secondRequestMessages[i])
		}
	}
}

func TestRunStopsAtMaxStepLimit(t *testing.T) {
	client := &recordingClient{
		defaultResponse: llm.ChatResponse{
			Message:   llm.Message{Role: llm.RoleAssistant},
			ToolCalls: []llm.ToolCall{{ID: "call_loop", Name: "echo", Arguments: json.RawMessage(`{"message":"again"}`)}},
		},
	}
	runner := agent.NewWithTools(client, "gpt-test", registryWith(&stubTool{name: "echo", result: "again"}))

	_, err := runner.Run(context.Background(), "loop forever")
	if err == nil {
		t.Fatal("expected max step error")
	}
	if !strings.Contains(err.Error(), "max agent steps exceeded") {
		t.Fatalf("expected max step error, got %q", err.Error())
	}
	if len(client.requests) != 8 {
		t.Fatalf("expected default max of 8 model calls, got %d", len(client.requests))
	}
}

func TestRunReturnsControlledErrorForUnknownTool(t *testing.T) {
	client := &recordingClient{
		responses: []llm.ChatResponse{{
			Message:   llm.Message{Role: llm.RoleAssistant},
			ToolCalls: []llm.ToolCall{{ID: "call_missing", Name: "missing", Arguments: json.RawMessage(`{}`)}},
		}},
	}
	runner := agent.NewWithTools(client, "gpt-test", tools.NewRegistry())

	_, err := runner.Run(context.Background(), "use missing tool")
	if err == nil {
		t.Fatal("expected unknown tool error")
	}
	if !strings.Contains(err.Error(), "unknown tool: missing") {
		t.Fatalf("expected controlled unknown tool error, got %q", err.Error())
	}
}

func TestRunReturnsControlledErrorForToolExecutionFailure(t *testing.T) {
	client := &recordingClient{
		responses: []llm.ChatResponse{{
			Message:   llm.Message{Role: llm.RoleAssistant},
			ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "echo", Arguments: json.RawMessage(`{"message":""}`)}},
		}},
	}
	runner := agent.NewWithTools(client, "gpt-test", registryWith(&stubTool{name: "echo", err: errors.New("message is required")}))

	_, err := runner.Run(context.Background(), "bad tool args")
	if err == nil {
		t.Fatal("expected tool execution error")
	}
	if !strings.Contains(err.Error(), "tool echo failed: message is required") {
		t.Fatalf("expected controlled tool execution error, got %q", err.Error())
	}
}

func TestRunTrimsAndRequiresPrompt(t *testing.T) {
	runner := agent.New(&recordingClient{}, "gpt-test")

	_, err := runner.Run(context.Background(), "   ")
	if err == nil {
		t.Fatal("expected error for blank prompt")
	}
	if !strings.Contains(err.Error(), "prompt is required") {
		t.Fatalf("expected prompt required error, got %q", err.Error())
	}
}

func TestRunReturnsModelErrors(t *testing.T) {
	runner := agent.New(&recordingClient{err: errors.New("model unavailable")}, "gpt-test")

	_, err := runner.Run(context.Background(), "What is an agent?")
	if err == nil {
		t.Fatal("expected model error")
	}
	if !strings.Contains(err.Error(), "chat failed: model unavailable") {
		t.Fatalf("expected wrapped model error, got %q", err.Error())
	}
}

type recordingClient struct {
	requests        []llm.ChatRequest
	responses       []llm.ChatResponse
	defaultResponse llm.ChatResponse
	err             error
}

func (r *recordingClient) Chat(ctx context.Context, request llm.ChatRequest) (llm.ChatResponse, error) {
	r.requests = append(r.requests, request)
	if r.err != nil {
		return llm.ChatResponse{}, r.err
	}
	if len(r.responses) == 0 {
		return r.defaultResponse, nil
	}
	response := r.responses[0]
	r.responses = r.responses[1:]
	return response, nil
}

type stubTool struct {
	name    string
	result  string
	err     error
	gotArgs json.RawMessage
}

func (s stubTool) Name() string { return s.name }

func (s stubTool) Description() string { return "stub tool" }

func (s stubTool) JSONSchema() map[string]any { return map[string]any{"type": "object"} }

func (s *stubTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	s.gotArgs = args
	if s.err != nil {
		return "", s.err
	}
	return s.result, nil
}

func registryWith(tool tools.Tool) tools.Registry {
	registry := tools.NewRegistry()
	if err := registry.Register(tool); err != nil {
		panic(err)
	}
	return registry
}
