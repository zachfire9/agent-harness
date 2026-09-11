package agent

// TraceEvent is a redacted, structured description of one observable agent step.
type TraceEvent struct {
	Type             string
	Step             int
	Model            string
	InputMessages    int
	OutputMessages   int
	MaxMessages      int
	ToolCount        int
	ToolName         string
	ToolCallID       string
	ToolResultChars  int
	FinalAnswerChars int
	ContextReport    ContextReport
}

const (
	TraceEventContextBuild = "context_build"
	TraceEventModelCall    = "model_call"
	TraceEventToolCall     = "tool_call"
	TraceEventToolResult   = "tool_result"
	TraceEventFinalAnswer  = "final_answer"
)
