package llm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/zachfire9/agent-harness/internal/llm"
)

func TestMessageRolesAreRepresentedConsistently(t *testing.T) {
	cases := []struct {
		name string
		role llm.Role
		want string
	}{
		{name: "system", role: llm.RoleSystem, want: "system"},
		{name: "user", role: llm.RoleUser, want: "user"},
		{name: "assistant", role: llm.RoleAssistant, want: "assistant"},
		{name: "tool", role: llm.RoleTool, want: "tool"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.role) != tc.want {
				t.Fatalf("expected role %q, got %q", tc.want, tc.role)
			}
		})
	}
}

func TestNewChatRequestPreservesMessageOrdering(t *testing.T) {
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: "You are a concise tutor."},
		{Role: llm.RoleUser, Content: "What is an agent?"},
		{Role: llm.RoleAssistant, Content: "An agent can use tools."},
		{Role: llm.RoleUser, Content: "Give me one example."},
	}

	request := llm.NewChatRequest("test-model", messages...)

	if request.Model != "test-model" {
		t.Fatalf("expected model to be preserved, got %q", request.Model)
	}

	if len(request.Messages) != len(messages) {
		t.Fatalf("expected %d messages, got %d", len(messages), len(request.Messages))
	}

	for i, want := range messages {
		got := request.Messages[i]
		if got != want {
			t.Fatalf("message %d: expected %#v, got %#v", i, want, got)
		}
	}
}

func TestNewChatRequestCopiesMessages(t *testing.T) {
	messages := []llm.Message{{Role: llm.RoleUser, Content: "original"}}

	request := llm.NewChatRequest("test-model", messages...)
	messages[0].Content = "mutated"

	if request.Messages[0].Content != "original" {
		t.Fatalf("expected request messages to be copied, got %q", request.Messages[0].Content)
	}
}

func TestFakeClientSatisfiesChatClientAndReturnsDeterministicResponse(t *testing.T) {
	var client llm.ChatClient = llm.FakeClient{
		Response: llm.ChatResponse{
			Message: llm.Message{Role: llm.RoleAssistant, Content: "deterministic answer"},
		},
	}

	response, err := client.Chat(context.Background(), llm.NewChatRequest("test-model", llm.Message{
		Role:    llm.RoleUser,
		Content: "Hello",
	}))
	if err != nil {
		t.Fatalf("expected fake client response, got error: %v", err)
	}

	if response.Message.Role != llm.RoleAssistant {
		t.Fatalf("expected assistant response role, got %q", response.Message.Role)
	}

	if response.Message.Content != "deterministic answer" {
		t.Fatalf("expected deterministic response, got %q", response.Message.Content)
	}
}

func TestFakeClientReturnsConfiguredError(t *testing.T) {
	wantErr := errors.New("fake failure")
	client := llm.FakeClient{Err: wantErr}

	_, err := client.Chat(context.Background(), llm.NewChatRequest("test-model"))

	if !errors.Is(err, wantErr) {
		t.Fatalf("expected configured error, got %v", err)
	}
}
