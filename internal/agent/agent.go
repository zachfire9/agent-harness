package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/zachfire9/agent-harness/internal/llm"
)

const defaultSystemPrompt = "You are a helpful CLI assistant. Answer clearly and concisely."

// Runner orchestrates one non-tool agent turn.
type Runner struct {
	chatClient   llm.ChatClient
	model        string
	systemPrompt string
}

// Result contains the final answer and complete message history for a run.
type Result struct {
	Answer   string
	Messages []llm.Message
}

// New creates a runner with the default system prompt.
func New(chatClient llm.ChatClient, model string) Runner {
	return Runner{
		chatClient:   chatClient,
		model:        model,
		systemPrompt: defaultSystemPrompt,
	}
}

// Run builds initial message history, calls the chat client once, and appends the assistant response.
func (r Runner) Run(ctx context.Context, prompt string) (Result, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return Result{}, errors.New("prompt is required")
	}
	if r.chatClient == nil {
		return Result{}, errors.New("chat client is required")
	}

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: r.systemPrompt},
		{Role: llm.RoleUser, Content: prompt},
	}

	response, err := r.chatClient.Chat(ctx, llm.NewChatRequest(r.model, messages...))
	if err != nil {
		return Result{}, fmt.Errorf("chat failed: %w", err)
	}

	messages = append(messages, response.Message)
	return Result{
		Answer:   response.Message.Content,
		Messages: messages,
	}, nil
}
