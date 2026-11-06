package llm

import (
	"context"
	"encoding/json"
)

// Role identifies who produced a chat message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is one item in a chat transcript.
type Message struct {
	Role       Role
	Content    string
	ToolCallID string
	ToolCalls  []ToolCall
}

// ToolSpec describes one tool the model may request.
type ToolSpec struct {
	Name        string
	Description string
	Schema      map[string]any
}

// ChatRequest contains the model name, ordered messages, and available tools for one chat call.
type ChatRequest struct {
	Model    string
	Messages []Message
	Tools    []ToolSpec
}

// ToolCall is a model-requested function/tool invocation.
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// ChatResponse contains the assistant message returned by a chat client.
type ChatResponse struct {
	Message   Message
	ToolCalls []ToolCall
}

// IsFinalAnswer reports whether the response is an assistant text answer rather
// than a request for the harness to execute tools.
func (r ChatResponse) IsFinalAnswer() bool {
	return len(r.ToolCalls) == 0
}

// ChatClient is the minimal interface the agent runtime needs from an LLM provider.
type ChatClient interface {
	Chat(ctx context.Context, request ChatRequest) (ChatResponse, error)
}

// StreamingChatClient is a ChatClient that can also deliver assistant text as
// the model generates it. ChatStream calls onText with each text fragment, in
// order, and returns the same complete response Chat would: the full message
// text plus any tool calls, which are only returned once fully assembled.
type StreamingChatClient interface {
	ChatClient
	ChatStream(ctx context.Context, request ChatRequest, onText func(text string)) (ChatResponse, error)
}

// NewChatRequest builds a chat request while copying messages so callers cannot
// mutate the request by changing their original slice after construction.
func NewChatRequest(model string, messages ...Message) ChatRequest {
	copiedMessages := make([]Message, len(messages))
	copy(copiedMessages, messages)

	return ChatRequest{
		Model:    model,
		Messages: copiedMessages,
	}
}

// FakeClient is a deterministic chat client for unit tests.
type FakeClient struct {
	Response ChatResponse
	Err      error
}

// Chat returns the configured response or error without making network calls.
func (f FakeClient) Chat(ctx context.Context, request ChatRequest) (ChatResponse, error) {
	if f.Err != nil {
		return ChatResponse{}, f.Err
	}

	return f.Response, nil
}
