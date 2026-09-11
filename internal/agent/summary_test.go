package agent_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zachfire9/agent-harness/internal/agent"
	"github.com/zachfire9/agent-harness/internal/llm"
)

var errSummaryUnavailable = errors.New("summary unavailable")

func TestLLMSummarizerCombinesExistingSummaryAndNewMessages(t *testing.T) {
	client := &recordingClient{
		responses: []llm.ChatResponse{{Message: llm.Message{Role: llm.RoleAssistant, Content: "Updated concise summary"}}},
	}
	summarizer := agent.NewLLMSummarizer(client, "gpt-4.1-mini")

	summary, err := summarizer.Summarize(context.Background(), agent.SummaryInput{
		ExistingSummary: "User wants a learning agent harness.",
		NewMessages: []llm.Message{
			{Role: llm.RoleUser, Content: "Add model-backed running summary support."},
			{Role: llm.RoleAssistant, Content: "Agreed and planned separate summary model config."},
		},
		MaxChars: 2000,
	})
	if err != nil {
		t.Fatalf("Summarize returned error: %v", err)
	}
	if summary != "Updated concise summary" {
		t.Fatalf("expected updated summary, got %q", summary)
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected one summary model call, got %d", len(client.requests))
	}
	request := client.requests[0]
	if request.Model != "gpt-4.1-mini" {
		t.Fatalf("expected separately configured summary model, got %q", request.Model)
	}
	prompt := request.Messages[1].Content
	for _, want := range []string{
		"Existing summary:\nUser wants a learning agent harness.",
		"New messages to incorporate:",
		"- user: Add model-backed running summary support.",
		"- assistant: Agreed and planned separate summary model config.",
		"Keep the summary under 2000 characters.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected summary prompt to contain %q, got %q", want, prompt)
		}
	}
}

func TestLLMSummarizerUsesNoneYetForEmptySummary(t *testing.T) {
	client := &recordingClient{
		responses: []llm.ChatResponse{{Message: llm.Message{Role: llm.RoleAssistant, Content: "Initial summary"}}},
	}
	summarizer := agent.NewLLMSummarizer(client, "gpt-4.1-mini")

	_, err := summarizer.Summarize(context.Background(), agent.SummaryInput{
		NewMessages: []llm.Message{{Role: llm.RoleUser, Content: "Start the task."}},
		MaxChars:    2000,
	})
	if err != nil {
		t.Fatalf("Summarize returned error: %v", err)
	}
	if !strings.Contains(client.requests[0].Messages[1].Content, "Existing summary:\nNone yet.") {
		t.Fatalf("expected prompt to state no existing summary, got %q", client.requests[0].Messages[1].Content)
	}
}

func TestLLMSummarizerReturnsExistingSummaryWhenNoNewMessages(t *testing.T) {
	client := &recordingClient{}
	summarizer := agent.NewLLMSummarizer(client, "gpt-4.1-mini")

	summary, err := summarizer.Summarize(context.Background(), agent.SummaryInput{
		ExistingSummary: "Already summarized.",
		NewMessages:     []llm.Message{{Role: llm.RoleAssistant, Content: "   "}},
		MaxChars:        2000,
	})
	if err != nil {
		t.Fatalf("Summarize returned error: %v", err)
	}
	if summary != "Already summarized." {
		t.Fatalf("expected existing summary, got %q", summary)
	}
	if len(client.requests) != 0 {
		t.Fatalf("expected no summary model call, got %d", len(client.requests))
	}
}

func TestLLMSummarizerReturnsControlledError(t *testing.T) {
	summarizer := agent.NewLLMSummarizer(&recordingClient{err: errSummaryUnavailable}, "gpt-4.1-mini")

	_, err := summarizer.Summarize(context.Background(), agent.SummaryInput{
		NewMessages: []llm.Message{{Role: llm.RoleUser, Content: "new info"}},
		MaxChars:    2000,
	})
	if err == nil {
		t.Fatal("expected summary error")
	}
	if !strings.Contains(err.Error(), "summary model call failed") {
		t.Fatalf("expected controlled summary error, got %q", err.Error())
	}
}
