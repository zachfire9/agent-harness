package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/zachfire9/agent-harness/internal/llm"
)

const defaultSummaryPrompt = `You maintain a compact running summary for an AI agent session.

Existing summary:
%s

New messages to incorporate:
%s

Update the summary so it preserves:
- user goals and constraints
- decisions made
- important facts
- tool results that affect future work
- unresolved questions or TODOs
- errors encountered and relevant fixes

Rules:
- Do not invent facts.
- Prefer durable task-relevant details over chit-chat.
- Keep the summary under %d characters.
- Return only the updated summary.`

// ConversationSummary is compact state representing earlier messages that are
// no longer sent to the model as raw chat history.
type ConversationSummary struct {
	Content             string
	CoveredMessageCount int
}

// SummaryInput describes a running-summary update request.
type SummaryInput struct {
	ExistingSummary string
	NewMessages     []llm.Message
	MaxChars        int
}

// Summarizer updates a compact running summary using newly compacted messages.
type Summarizer interface {
	Summarize(ctx context.Context, input SummaryInput) (string, error)
}

// LLMSummarizer uses a chat model to update the running conversation summary.
type LLMSummarizer struct {
	chatClient llm.ChatClient
	model      string
}

// NewLLMSummarizer creates a model-backed summarizer. It usually shares API key
// and base URL with the main model client, but uses a separately configured model.
func NewLLMSummarizer(chatClient llm.ChatClient, model string) LLMSummarizer {
	return LLMSummarizer{chatClient: chatClient, model: model}
}

// Summarize sends the existing summary and new messages to the summary model and
// returns the updated summary text.
func (s LLMSummarizer) Summarize(ctx context.Context, input SummaryInput) (string, error) {
	if s.chatClient == nil {
		return "", fmt.Errorf("summary chat client is required")
	}
	messagesText := formatMessagesForSummary(input.NewMessages)
	if strings.TrimSpace(messagesText) == "" {
		return strings.TrimSpace(input.ExistingSummary), nil
	}
	existingSummary := strings.TrimSpace(input.ExistingSummary)
	if existingSummary == "" {
		existingSummary = "None yet."
	}

	prompt := fmt.Sprintf(defaultSummaryPrompt, existingSummary, messagesText, input.MaxChars)
	response, err := s.chatClient.Chat(ctx, llm.NewChatRequest(s.model,
		llm.Message{Role: llm.RoleSystem, Content: "You update compact running summaries for AI agent sessions."},
		llm.Message{Role: llm.RoleUser, Content: prompt},
	))
	if err != nil {
		return "", fmt.Errorf("summary model call failed: %w", err)
	}
	updatedSummary := strings.TrimSpace(response.Message.Content)
	if updatedSummary == "" {
		return "", fmt.Errorf("summary model returned empty summary")
	}
	return updatedSummary, nil
}

func formatMessagesForSummary(messages []llm.Message) string {
	var builder strings.Builder
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if content == "" && len(message.ToolCalls) == 0 {
			continue
		}
		label := string(message.Role)
		if message.Role == llm.RoleTool && message.ToolCallID != "" {
			label = fmt.Sprintf("tool %s", message.ToolCallID)
		}
		fmt.Fprintf(&builder, "- %s: %s", label, content)
		if len(message.ToolCalls) > 0 {
			fmt.Fprintf(&builder, " [requested tool calls: %d]", len(message.ToolCalls))
		}
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}
