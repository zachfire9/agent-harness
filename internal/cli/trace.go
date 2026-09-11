package cli

import (
	"fmt"
	"io"

	"github.com/zachfire9/agent-harness/internal/agent"
)

func parseTraceFlag(args []string) (bool, []string) {
	traceEnabled := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--trace" {
			traceEnabled = true
			continue
		}
		filtered = append(filtered, arg)
	}
	return traceEnabled, filtered
}

func writeTraceConfig(stderr io.Writer, app App) {
	fmt.Fprintf(stderr, "[trace] config model=%s summary_model=%s max_messages=%d max_summary_input_messages=%d\n", app.model, app.summaryModel, app.contextLimits.MaxMessages, app.contextLimits.SummaryMaxInputMessages)
}

func writeTraceSummaryJob(stderr io.Writer, status string) {
	fmt.Fprintf(stderr, "[trace] summary job status=%s\n", status)
}

func writeTrace(stderr io.Writer, events []agent.TraceEvent) {
	for _, event := range events {
		switch event.Type {
		case agent.TraceEventContextBuild:
			report := event.ContextReport
			fmt.Fprintf(stderr, "[trace] context build step=%d input_messages=%d output_messages=%d max_messages=%d omitted=%d summarized=%d summary_inserted=%t summary_updates=%d truncations=%d\n", event.Step, event.InputMessages, event.OutputMessages, event.MaxMessages, report.OmittedMessages, report.SummarizedMessages, report.SummaryInserted, report.SummaryModelUpdates, len(report.Truncations))
			for _, truncation := range report.Truncations {
				fmt.Fprintf(stderr, "[trace] context truncation step=%d role=%s index=%d kept=%d omitted=%d original=%d\n", event.Step, truncation.Role, truncation.Index, truncation.KeptChars, truncation.OmittedChars, truncation.OriginalChars)
			}
		case agent.TraceEventModelCall:
			fmt.Fprintf(stderr, "[trace] model call step=%d model=%s messages=%d tools=%d\n", event.Step, event.Model, event.OutputMessages, event.ToolCount)
		case agent.TraceEventToolCall:
			fmt.Fprintf(stderr, "[trace] tool call step=%d name=%s id=%s\n", event.Step, event.ToolName, event.ToolCallID)
		case agent.TraceEventToolResult:
			fmt.Fprintf(stderr, "[trace] tool result step=%d name=%s id=%s chars=%d\n", event.Step, event.ToolName, event.ToolCallID, event.ToolResultChars)
		case agent.TraceEventFinalAnswer:
			fmt.Fprintf(stderr, "[trace] final answer step=%d chars=%d\n", event.Step, event.FinalAnswerChars)
		}
	}
}
