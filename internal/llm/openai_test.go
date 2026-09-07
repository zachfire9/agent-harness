package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zachfire9/agent-harness/internal/llm"
)

func TestOpenAIClientSendsChatCompletionRequest(t *testing.T) {
	var gotPath string
	var gotAuthorization string
	var gotPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
			t.Fatalf("expected JSON content type, got %q", contentType)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {"role": "assistant", "content": "Agents are loops around model calls."}
			}]
		}`))
	}))
	defer server.Close()

	client := llm.NewOpenAIClient(server.URL, "test-api-key")
	response, err := client.Chat(context.Background(), llm.NewChatRequest("gpt-test",
		llm.Message{Role: llm.RoleSystem, Content: "You are concise."},
		llm.Message{Role: llm.RoleUser, Content: "What is an agent?"},
	))
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}

	if gotPath != "/chat/completions" {
		t.Fatalf("expected /chat/completions path, got %q", gotPath)
	}
	if gotAuthorization != "Bearer test-api-key" {
		t.Fatalf("expected bearer auth header, got %q", gotAuthorization)
	}
	if gotPayload["model"] != "gpt-test" {
		t.Fatalf("expected model gpt-test, got %#v", gotPayload["model"])
	}
	messages, ok := gotPayload["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("expected two messages, got %#v", gotPayload["messages"])
	}
	first := messages[0].(map[string]any)
	second := messages[1].(map[string]any)
	if first["role"] != "system" || first["content"] != "You are concise." {
		t.Fatalf("unexpected first message: %#v", first)
	}
	if second["role"] != "user" || second["content"] != "What is an agent?" {
		t.Fatalf("unexpected second message: %#v", second)
	}
	if response.Message.Role != llm.RoleAssistant || response.Message.Content != "Agents are loops around model calls." {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestOpenAIClientTrimsTrailingBaseURLSlash(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()

	client := llm.NewOpenAIClient(server.URL+"/", "test-api-key")
	_, err := client.Chat(context.Background(), llm.NewChatRequest("gpt-test", llm.Message{Role: llm.RoleUser, Content: "hello"}))
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("expected normalized path /chat/completions, got %q", gotPath)
	}
}

func TestOpenAIClientParsesFinalAssistantResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"final answer"}}]}`))
	}))
	defer server.Close()

	client := llm.NewOpenAIClient(server.URL, "test-api-key")
	response, err := client.Chat(context.Background(), llm.NewChatRequest("gpt-test", llm.Message{Role: llm.RoleUser, Content: "hello"}))
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}

	if !response.IsFinalAnswer() {
		t.Fatalf("expected final answer response, got %#v", response)
	}
	if response.Message.Content != "final answer" {
		t.Fatalf("expected final answer content, got %q", response.Message.Content)
	}
	if len(response.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls, got %#v", response.ToolCalls)
	}
}

func TestOpenAIClientParsesSingleToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {
					"role": "assistant",
					"content": null,
					"tool_calls": [{
						"id": "call_1",
						"type": "function",
						"function": {"name": "echo", "arguments": "{\"message\":\"hi\"}"}
					}]
				}
			}]
		}`))
	}))
	defer server.Close()

	client := llm.NewOpenAIClient(server.URL, "test-api-key")
	response, err := client.Chat(context.Background(), llm.NewChatRequest("gpt-test", llm.Message{Role: llm.RoleUser, Content: "call echo"}))
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}

	if response.IsFinalAnswer() {
		t.Fatalf("expected tool-call response, got final answer %#v", response)
	}
	if len(response.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %#v", response.ToolCalls)
	}
	call := response.ToolCalls[0]
	if call.ID != "call_1" || call.Name != "echo" || string(call.Arguments) != `{"message":"hi"}` {
		t.Fatalf("unexpected tool call: %#v", call)
	}
}

func TestOpenAIClientParsesMultipleToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {
					"role": "assistant",
					"tool_calls": [
						{"id": "call_1", "type": "function", "function": {"name": "echo", "arguments": "{\"message\":\"one\"}"}},
						{"id": "call_2", "type": "function", "function": {"name": "echo", "arguments": "{\"message\":\"two\"}"}}
					]
				}
			}]
		}`))
	}))
	defer server.Close()

	client := llm.NewOpenAIClient(server.URL, "test-api-key")
	response, err := client.Chat(context.Background(), llm.NewChatRequest("gpt-test", llm.Message{Role: llm.RoleUser, Content: "call echo twice"}))
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}

	if len(response.ToolCalls) != 2 {
		t.Fatalf("expected two tool calls, got %#v", response.ToolCalls)
	}
	if response.ToolCalls[0].ID != "call_1" || response.ToolCalls[0].Name != "echo" {
		t.Fatalf("unexpected first tool call: %#v", response.ToolCalls[0])
	}
	if response.ToolCalls[1].ID != "call_2" || response.ToolCalls[1].Name != "echo" {
		t.Fatalf("unexpected second tool call: %#v", response.ToolCalls[1])
	}
}

func TestOpenAIClientRejectsMalformedToolArguments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {
					"role": "assistant",
					"tool_calls": [{
						"id": "call_bad",
						"type": "function",
						"function": {"name": "echo", "arguments": "{not json"}
					}]
				}
			}]
		}`))
	}))
	defer server.Close()

	client := llm.NewOpenAIClient(server.URL, "test-api-key")
	_, err := client.Chat(context.Background(), llm.NewChatRequest("gpt-test", llm.Message{Role: llm.RoleUser, Content: "call echo"}))
	if err == nil {
		t.Fatal("expected malformed tool arguments error")
	}
	if !strings.Contains(err.Error(), "invalid tool call arguments for echo") {
		t.Fatalf("expected useful malformed arguments error, got %q", err.Error())
	}
}

func TestOpenAIClientParsesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad API key"}}`))
	}))
	defer server.Close()

	client := llm.NewOpenAIClient(server.URL, "test-api-key")
	_, err := client.Chat(context.Background(), llm.NewChatRequest("gpt-test", llm.Message{Role: llm.RoleUser, Content: "hello"}))
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
	if !strings.Contains(err.Error(), "OpenAI-compatible chat request failed: status 401: bad API key") {
		t.Fatalf("expected useful API error, got %q", err.Error())
	}
}

func TestOpenAIClientRejectsEmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer server.Close()

	client := llm.NewOpenAIClient(server.URL, "test-api-key")
	_, err := client.Chat(context.Background(), llm.NewChatRequest("gpt-test", llm.Message{Role: llm.RoleUser, Content: "hello"}))
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
	if !strings.Contains(err.Error(), "no assistant message") {
		t.Fatalf("expected missing assistant message error, got %q", err.Error())
	}
}
