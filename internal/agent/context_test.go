package agent_test

import (
	"context"
	"errors"
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

	snapshot, err := manager.Build(context.Background(), history, agent.ConversationSummary{})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Messages) != len(history) {
		t.Fatalf("expected all messages to be preserved, got %#v", snapshot.Messages)
	}
	for i := range history {
		if snapshot.Messages[i].Role != history[i].Role || snapshot.Messages[i].Content != history[i].Content {
			t.Fatalf("message %d changed: want %#v, got %#v", i, history[i], snapshot.Messages[i])
		}
	}
	if snapshot.Report.OmittedMessages != 0 || snapshot.Report.SummaryInserted || len(snapshot.Report.Truncations) != 0 {
		t.Fatalf("expected empty report for unchanged history, got %#v", snapshot.Report)
	}
}

func TestContextManagerSummarizesOmittedMiddleMessages(t *testing.T) {
	summarizer := &fakeSummarizer{returnSummary: "Older context says Zach wants model-backed summaries."}
	manager := agent.NewContextManagerWithSummarizer(agent.ContextLimits{MaxMessages: 5, MaxMessageChars: 100, MaxToolResultChars: 100, MaxSummaryChars: 2000, SummaryMaxInputMessages: 10}, summarizer)
	history := []llm.Message{
		{Role: llm.RoleSystem, Content: "system"},
		{Role: llm.RoleUser, Content: "original goal"},
		{Role: llm.RoleAssistant, Content: "old answer"},
		{Role: llm.RoleUser, Content: "important older constraint"},
		{Role: llm.RoleAssistant, Content: "recent answer"},
		{Role: llm.RoleUser, Content: "latest question"},
	}

	snapshot, err := manager.Build(context.Background(), history, agent.ConversationSummary{})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	wantContents := []string{
		"system",
		"original goal",
		"Conversation summary of earlier omitted context:\nOlder context says Zach wants model-backed summaries.",
		"recent answer",
		"latest question",
	}
	if len(snapshot.Messages) != len(wantContents) {
		t.Fatalf("expected summary plus recent messages, got %#v", snapshot.Messages)
	}
	for i, want := range wantContents {
		if snapshot.Messages[i].Content != want {
			t.Fatalf("message %d: want %q, got %#v", i, want, snapshot.Messages[i])
		}
	}
	if len(summarizer.inputs) != 1 {
		t.Fatalf("expected one summary call, got %d", len(summarizer.inputs))
	}
	if len(summarizer.inputs[0].NewMessages) != 2 {
		t.Fatalf("expected two omitted messages summarized, got %#v", summarizer.inputs[0].NewMessages)
	}
	if snapshot.Report.OmittedMessages != 2 || snapshot.Report.SummarizedMessages != 2 || !snapshot.Report.SummaryInserted {
		t.Fatalf("expected summary report, got %#v", snapshot.Report)
	}
	if snapshot.Summary.Content != "Older context says Zach wants model-backed summaries." || snapshot.Summary.CoveredMessageCount != 4 {
		t.Fatalf("expected updated summary state, got %#v", snapshot.Summary)
	}
}

func TestContextManagerUsesExistingSummaryWithoutResummarizingCoveredMessages(t *testing.T) {
	summarizer := &fakeSummarizer{returnSummary: "should not be used"}
	manager := agent.NewContextManagerWithSummarizer(agent.ContextLimits{MaxMessages: 5, MaxMessageChars: 100, MaxToolResultChars: 100, MaxSummaryChars: 2000, SummaryMaxInputMessages: 10}, summarizer)
	summary := agent.ConversationSummary{Content: "Earlier choices are already summarized.", CoveredMessageCount: 4}
	history := []llm.Message{
		{Role: llm.RoleSystem, Content: "system"},
		{Role: llm.RoleUser, Content: "original goal"},
		{Role: llm.RoleAssistant, Content: "old answer"},
		{Role: llm.RoleUser, Content: "important older constraint"},
		{Role: llm.RoleAssistant, Content: "recent answer"},
		{Role: llm.RoleUser, Content: "latest question"},
	}

	snapshot, err := manager.Build(context.Background(), history, summary)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(summarizer.inputs) != 0 {
		t.Fatalf("expected no summary call for covered messages, got %d", len(summarizer.inputs))
	}
	if snapshot.Messages[2].Content != "Conversation summary of earlier omitted context:\nEarlier choices are already summarized." {
		t.Fatalf("expected existing summary inserted, got %#v", snapshot.Messages)
	}
}

func TestContextManagerBatchesSummaryUpdatesByConfiguredInputMessages(t *testing.T) {
	summarizer := &fakeSummarizer{returnSummary: "updated"}
	manager := agent.NewContextManagerWithSummarizer(agent.ContextLimits{MaxMessages: 4, MaxMessageChars: 100, MaxToolResultChars: 100, MaxSummaryChars: 2000, SummaryMaxInputMessages: 2}, summarizer)
	history := []llm.Message{
		{Role: llm.RoleSystem, Content: "system"},
		{Role: llm.RoleUser, Content: "original goal"},
		{Role: llm.RoleAssistant, Content: "old one"},
		{Role: llm.RoleUser, Content: "old two"},
		{Role: llm.RoleAssistant, Content: "old three"},
		{Role: llm.RoleAssistant, Content: "old four"},
		{Role: llm.RoleUser, Content: "latest question"},
	}

	snapshot, err := manager.Build(context.Background(), history, agent.ConversationSummary{})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(summarizer.inputs) != 2 {
		t.Fatalf("expected two batched summary updates, got %d", len(summarizer.inputs))
	}
	if len(summarizer.inputs[0].NewMessages) != 2 || len(summarizer.inputs[1].NewMessages) != 2 {
		t.Fatalf("expected two 2-message batches, got %#v", summarizer.inputs)
	}
	if snapshot.Report.SummaryModelUpdates != 2 {
		t.Fatalf("expected two summary update reports, got %#v", snapshot.Report)
	}
}

func TestContextManagerTruncatesOversizedToolResultsWithNotice(t *testing.T) {
	manager := agent.NewContextManager(agent.ContextLimits{MaxMessages: 10, MaxMessageChars: 100, MaxToolResultChars: 12})
	history := []llm.Message{
		{Role: llm.RoleSystem, Content: "system"},
		{Role: llm.RoleUser, Content: "original goal"},
		{Role: llm.RoleTool, Content: "0123456789abcdefghijklmnopqrstuvwxyz", ToolCallID: "call_1"},
	}

	snapshot, err := manager.Build(context.Background(), history, agent.ConversationSummary{})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	toolContent := snapshot.Messages[2].Content
	if !strings.HasPrefix(toolContent, "0123456789ab") || !strings.Contains(toolContent, "[truncated") {
		t.Fatalf("expected explicit tool truncation notice, got %q", toolContent)
	}
	if len(snapshot.Report.Truncations) != 1 {
		t.Fatalf("expected one truncation report, got %#v", snapshot.Report)
	}
}

func TestContextManagerReturnsSummarizerErrors(t *testing.T) {
	manager := agent.NewContextManagerWithSummarizer(agent.ContextLimits{MaxMessages: 4, MaxMessageChars: 100, MaxToolResultChars: 100, MaxSummaryChars: 2000, SummaryMaxInputMessages: 10}, &fakeSummarizer{err: errors.New("summary failed")})
	history := []llm.Message{
		{Role: llm.RoleSystem, Content: "system"},
		{Role: llm.RoleUser, Content: "original goal"},
		{Role: llm.RoleAssistant, Content: "old answer"},
		{Role: llm.RoleUser, Content: "old follow-up"},
		{Role: llm.RoleUser, Content: "latest question"},
	}

	_, err := manager.Build(context.Background(), history, agent.ConversationSummary{})
	if err == nil {
		t.Fatal("expected summary error")
	}
	if !strings.Contains(err.Error(), "update running summary") {
		t.Fatalf("expected controlled summary error, got %q", err.Error())
	}
}

func TestRunnerSendsSummaryContextAndKeepsFullHistory(t *testing.T) {
	client := &recordingClient{
		responses: []llm.ChatResponse{{Message: llm.Message{Role: llm.RoleAssistant, Content: "trimmed answer"}}},
	}
	summarizer := &fakeSummarizer{returnSummary: "Old messages were compacted."}
	runner := agent.NewWithToolsContextLimitsAndSummarizer(client, "gpt-test", registryWith(&stubTool{name: "echo", result: "unused"}), agent.ContextLimits{MaxMessages: 4, MaxMessageChars: 100, MaxToolResultChars: 100, MaxSummaryChars: 2000, SummaryMaxInputMessages: 10}, summarizer)
	history := []llm.Message{
		{Role: llm.RoleSystem, Content: "system"},
		{Role: llm.RoleUser, Content: "original goal"},
		{Role: llm.RoleAssistant, Content: "old answer"},
		{Role: llm.RoleAssistant, Content: "recent answer"},
	}

	result, err := runner.RunWithSummary(context.Background(), history, agent.ConversationSummary{}, "latest question")
	if err != nil {
		t.Fatalf("RunWithSummary returned error: %v", err)
	}

	requestMessages := client.requests[0].Messages
	wantContents := []string{"system", "original goal", "Conversation summary of earlier omitted context:\nOld messages were compacted.", "latest question"}
	if len(requestMessages) != len(wantContents) {
		t.Fatalf("expected summarized request messages, got %#v", requestMessages)
	}
	for i, want := range wantContents {
		if requestMessages[i].Content != want {
			t.Fatalf("request message %d: want %q, got %#v", i, want, requestMessages[i])
		}
	}
	if len(result.Messages) != len(history)+2 {
		t.Fatalf("expected full raw history plus prompt/answer, got %#v", result.Messages)
	}
	if result.Summary.Content != "Old messages were compacted." {
		t.Fatalf("expected returned summary state, got %#v", result.Summary)
	}
}

type fakeSummarizer struct {
	inputs        []agent.SummaryInput
	returnSummary string
	err           error
}

func (f *fakeSummarizer) Summarize(ctx context.Context, input agent.SummaryInput) (string, error) {
	f.inputs = append(f.inputs, input)
	if f.err != nil {
		return "", f.err
	}
	return f.returnSummary, nil
}
