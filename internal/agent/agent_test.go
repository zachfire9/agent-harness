package agent_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zachfire9/agent-harness/internal/agent"
	"github.com/zachfire9/agent-harness/internal/llm"
)

func TestRunBuildsInitialMessageHistory(t *testing.T) {
	client := &recordingClient{
		response: llm.ChatResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: "Agents use model calls and tools."}},
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
	if result.Messages[1] != (llm.Message{Role: llm.RoleUser, Content: "What is an agent?"}) {
		t.Fatalf("expected user prompt second, got %#v", result.Messages[1])
	}
	if result.Messages[2] != (llm.Message{Role: llm.RoleAssistant, Content: "Agents use model calls and tools."}) {
		t.Fatalf("expected assistant response third, got %#v", result.Messages[2])
	}
}

func TestRunSendsConfiguredModelAndInitialMessages(t *testing.T) {
	client := &recordingClient{
		response: llm.ChatResponse{Message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}},
	}
	runner := agent.New(client, "gpt-test")

	_, err := runner.Run(context.Background(), "Hello")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if client.request.Model != "gpt-test" {
		t.Fatalf("expected configured model, got %q", client.request.Model)
	}
	if len(client.request.Messages) != 2 {
		t.Fatalf("expected system and user messages sent to model, got %#v", client.request.Messages)
	}
	if client.request.Messages[0].Role != llm.RoleSystem {
		t.Fatalf("expected system message first, got %#v", client.request.Messages[0])
	}
	if client.request.Messages[1] != (llm.Message{Role: llm.RoleUser, Content: "Hello"}) {
		t.Fatalf("expected user prompt second, got %#v", client.request.Messages[1])
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
	request  llm.ChatRequest
	response llm.ChatResponse
	err      error
}

func (r *recordingClient) Chat(ctx context.Context, request llm.ChatRequest) (llm.ChatResponse, error) {
	r.request = request
	if r.err != nil {
		return llm.ChatResponse{}, r.err
	}
	return r.response, nil
}
