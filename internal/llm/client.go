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
	Role    Role
	Content string
}

// ChatRequest contains the model name and ordered messages for one chat call.
type ChatRequest struct {
	Model    string
	Messages []Message
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
