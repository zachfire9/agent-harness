package cli

import (
	"context"

	"github.com/zachfire9/agent-harness/internal/agent"
	"github.com/zachfire9/agent-harness/internal/llm"
)

type summaryJob struct {
	done <-chan summaryJobResult
}

type summaryJobResult struct {
	summary agent.ConversationSummary
	err     error
}

func startSummaryJob(ctx context.Context, history []llm.Message, summary agent.ConversationSummary, limits agent.ContextLimits, summarizer agent.Summarizer) *summaryJob {
	if summarizer == nil || limits.MaxMessages <= 0 || len(history)+1 <= limits.MaxMessages {
		return nil
	}

	preparedHistory := append([]llm.Message(nil), history...)
	preparedHistory = append(preparedHistory, llm.Message{Role: llm.RoleUser, Content: "[next user message placeholder]"})
	done := make(chan summaryJobResult, 1)
	go func() {
		manager := agent.NewContextManagerWithSummarizer(limits, summarizer)
		snapshot, err := manager.Build(ctx, preparedHistory, summary)
		if err != nil {
			done <- summaryJobResult{err: err}
			return
		}
		done <- summaryJobResult{summary: snapshot.Summary}
	}()

	return &summaryJob{done: done}
}

func (j *summaryJob) Wait(ctx context.Context) (agent.ConversationSummary, error) {
	select {
	case result := <-j.done:
		return result.summary, result.err
	case <-ctx.Done():
		return agent.ConversationSummary{}, ctx.Err()
	}
}
