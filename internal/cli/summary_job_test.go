package cli

import (
	"context"
	"testing"

	"github.com/zachfire9/agent-harness/internal/agent"
	"github.com/zachfire9/agent-harness/internal/llm"
)

func TestStartSummaryJobReturnsNilWhenNextTurnStillFits(t *testing.T) {
	job := startSummaryJob(context.Background(), []llm.Message{
		{Role: llm.RoleSystem, Content: "system"},
		{Role: llm.RoleUser, Content: "goal"},
	}, agent.ConversationSummary{}, agent.ContextLimits{MaxMessages: 5}, &jobFakeSummarizer{})

	if job != nil {
		t.Fatal("expected no background summary job while next turn still fits")
	}
}

func TestStartSummaryJobPreparesSummaryInBackground(t *testing.T) {
	summarizer := &jobFakeSummarizer{returnSummary: "prepared summary"}
	job := startSummaryJob(context.Background(), []llm.Message{
		{Role: llm.RoleSystem, Content: "system"},
		{Role: llm.RoleUser, Content: "goal"},
		{Role: llm.RoleAssistant, Content: "old answer"},
		{Role: llm.RoleAssistant, Content: "recent answer"},
	}, agent.ConversationSummary{}, agent.ContextLimits{MaxMessages: 4, MaxSummaryChars: 2000, SummaryMaxInputMessages: 10}, summarizer)

	if job == nil {
		t.Fatal("expected background summary job")
	}
	summary, err := job.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if summary.Content != "prepared summary" {
		t.Fatalf("expected prepared summary, got %#v", summary)
	}
	if len(summarizer.inputs) != 1 {
		t.Fatalf("expected one summary call, got %d", len(summarizer.inputs))
	}
}

type jobFakeSummarizer struct {
	inputs        []agent.SummaryInput
	returnSummary string
}

func (f *jobFakeSummarizer) Summarize(ctx context.Context, input agent.SummaryInput) (string, error) {
	f.inputs = append(f.inputs, input)
	return f.returnSummary, nil
}
