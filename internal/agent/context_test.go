package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/zachfire9/agent-harness/internal/agent"
	"github.com/zachfire9/agent-harness/internal/llm"
)

func TestContextManagerLeavesShortHistoryUnchanged(t *testing.T) {
	manager := agent.NewContextManager(agent.ContextLimits{MaxMessages: 10, MaxMessageChars: 100, MaxToolResultChars: 100})
	history := []llm.Message{
		{Role: llm.RoleSystem, Content: "system"},
		{Role: llm.RoleUser, Content: "original goal"},
		{Role: llm.RoleAssistant, Content: "answer"},
	}

	snapshot := manager.Build(history)

	if len(snapshot.Messages) != len(history) {
		t.Fatalf("expected all messages to be preserved, got %#v", snapshot.Messages)
	}
	for i := range history {
		if snapshot.Messages[i].Role != history[i].Role || snapshot.Messages[i].Content != history[i].Content {
			t.Fatalf("message %d changed: want %#v, got %#v", i, history[i], snapshot.Messages[i])
		}
	}
	if snapshot.Report.OmittedMessages != 0 || len(snapshot.Report.Truncations) != 0 {
		t.Fatalf("expected empty report for unchanged history, got %#v", snapshot.Report)
	}
}

func TestContextManagerPreservesSystemPromptAndOriginalGoalWhenTrimming(t *testing.T) {
	manager := agent.NewContextManager(agent.ContextLimits{MaxMessages: 4, MaxMessageChars: 100, MaxToolResultChars: 100})
	history := []llm.Message{
		{Role: llm.RoleSystem, Content: "system"},
		{Role: llm.RoleUser, Content: "original goal"},
		{Role: llm.RoleAssistant, Content: "old answer"},
		{Role: llm.RoleUser, Content: "old follow-up"},
		{Role: llm.RoleAssistant, Content: "recent answer"},
		{Role: llm.RoleUser, Content: "latest question"},
	}

	snapshot := manager.Build(history)

	want := []llm.Message{
		{Role: llm.RoleSystem, Content: "system"},
		{Role: llm.RoleUser, Content: "original goal"},
		{Role: llm.RoleAssistant, Content: "recent answer"},
		{Role: llm.RoleUser, Content: "latest question"},
	}
	if len(snapshot.Messages) != len(want) {
		t.Fatalf("expected trimmed messages, got %#v", snapshot.Messages)
	}
	for i := range want {
		if snapshot.Messages[i].Role != want[i].Role || snapshot.Messages[i].Content != want[i].Content {
			t.Fatalf("message %d mismatch: want %#v, got %#v", i, want[i], snapshot.Messages[i])
		}
	}
	if snapshot.Report.OmittedMessages != 2 {
		t.Fatalf("expected two omitted messages, got %#v", snapshot.Report)
	}
}

func TestContextManagerKeepsRecentAssistantAndToolMessagesPreferentially(t *testing.T) {
	manager := agent.NewContextManager(agent.ContextLimits{MaxMessages: 5, MaxMessageChars: 100, MaxToolResultChars: 100})
	history := []llm.Message{
		{Role: llm.RoleSystem, Content: "system"},
		{Role: llm.RoleUser, Content: "original goal"},
		{Role: llm.RoleAssistant, Content: "old answer"},
		{Role: llm.RoleUser, Content: "old follow-up"},
		{Role: llm.RoleAssistant, Content: "tool request"},
		{Role: llm.RoleTool, Content: "tool result", ToolCallID: "call_1"},
		{Role: llm.RoleAssistant, Content: "final context"},
	}

	snapshot := manager.Build(history)

	wantRoles := []llm.Role{llm.RoleSystem, llm.RoleUser, llm.RoleAssistant, llm.RoleTool, llm.RoleAssistant}
	wantContents := []string{"system", "original goal", "tool request", "tool result", "final context"}
	if len(snapshot.Messages) != len(wantRoles) {
		t.Fatalf("expected five messages, got %#v", snapshot.Messages)
	}
	for i := range wantRoles {
		if snapshot.Messages[i].Role != wantRoles[i] || snapshot.Messages[i].Content != wantContents[i] {
			t.Fatalf("message %d mismatch: got %#v", i, snapshot.Messages[i])
		}
	}
}

func TestContextManagerTruncatesOversizedToolResultsWithNotice(t *testing.T) {
	manager := agent.NewContextManager(agent.ContextLimits{MaxMessages: 10, MaxMessageChars: 100, MaxToolResultChars: 12})
	history := []llm.Message{
		{Role: llm.RoleSystem, Content: "system"},
		{Role: llm.RoleUser, Content: "original goal"},
		{Role: llm.RoleTool, Content: "0123456789abcdefghijklmnopqrstuvwxyz", ToolCallID: "call_1"},
	}

	snapshot := manager.Build(history)

	if len(snapshot.Messages) != 3 {
		t.Fatalf("expected messages to remain present, got %#v", snapshot.Messages)
	}
	toolContent := snapshot.Messages[2].Content
	if !strings.HasPrefix(toolContent, "0123456789ab") || !strings.Contains(toolContent, "[truncated") {
		t.Fatalf("expected explicit tool truncation notice, got %q", toolContent)
	}
	if len(snapshot.Report.Truncations) != 1 {
		t.Fatalf("expected one truncation report, got %#v", snapshot.Report)
	}
	if snapshot.Report.Truncations[0].Role != llm.RoleTool || snapshot.Report.Truncations[0].OriginalChars != 36 {
		t.Fatalf("expected tool truncation metadata, got %#v", snapshot.Report.Truncations[0])
	}
}

func TestRunnerSendsTrimmedContextAndReportsOmissions(t *testing.T) {
	client := &recordingClient{
		responses: []llm.ChatResponse{{Message: llm.Message{Role: llm.RoleAssistant, Content: "trimmed answer"}}},
	}
	runner := agent.NewWithContextLimits(client, "gpt-test", agent.ContextLimits{MaxMessages: 4, MaxMessageChars: 100, MaxToolResultChars: 100})
	history := []llm.Message{
		{Role: llm.RoleSystem, Content: "system"},
		{Role: llm.RoleUser, Content: "original goal"},
		{Role: llm.RoleAssistant, Content: "old answer"},
		{Role: llm.RoleUser, Content: "old follow-up"},
		{Role: llm.RoleAssistant, Content: "recent answer"},
	}

	result, err := runner.RunWithHistory(context.Background(), history, "latest question")
	if err != nil {
		t.Fatalf("RunWithHistory returned error: %v", err)
	}

	if len(client.requests) != 1 {
		t.Fatalf("expected one request, got %d", len(client.requests))
	}
	requestMessages := client.requests[0].Messages
	wantContents := []string{"system", "original goal", "recent answer", "latest question"}
	if len(requestMessages) != len(wantContents) {
		t.Fatalf("expected trimmed request messages, got %#v", requestMessages)
	}
	for i, want := range wantContents {
		if requestMessages[i].Content != want {
			t.Fatalf("request message %d: want %q, got %#v", i, want, requestMessages[i])
		}
	}
	if len(result.ContextReports) != 1 || result.ContextReports[0].OmittedMessages != 2 {
		t.Fatalf("expected omission report on result, got %#v", result.ContextReports)
	}
}
