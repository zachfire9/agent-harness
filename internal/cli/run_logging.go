package cli

import (
	"github.com/zachfire9/agent-harness/internal/agent"
	"github.com/zachfire9/agent-harness/internal/runlog"
)

func (a App) newRunLogger() (*runlog.Writer, error) {
	return runlog.NewWriter(a.runLogDir, a.runLogsEnabled, a.runLogSecretList)
}

func writeRunStart(logger *runlog.Writer, prompt string) {
	_ = logger.Write(runlog.Event{Type: "run.start", Prompt: prompt})
}

func writeRunError(logger *runlog.Writer, err error) {
	if err == nil {
		return
	}
	_ = logger.Write(runlog.Event{Type: "run.error", Error: err.Error()})
}

func writeRunSuccess(logger *runlog.Writer, result agent.Result) {
	for _, event := range result.TraceEvents {
		if event.Type == agent.TraceEventContextBuild {
			_ = logger.Write(runlog.Event{Type: "context.snapshot", Step: event.Step, Messages: event.ContextMessages, ContextReports: []agent.ContextReport{event.ContextReport}})
		}
	}
	for _, report := range result.ContextReports {
		for _, truncation := range report.Truncations {
			truncationCopy := truncation
			_ = logger.Write(runlog.Event{Type: "context.truncation", Truncation: &truncationCopy})
		}
	}
	_ = logger.Write(runlog.Event{Type: "run.final", Messages: result.Messages, TraceEvents: result.TraceEvents, ContextReports: result.ContextReports})
}
