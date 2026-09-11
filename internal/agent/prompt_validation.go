package agent

import (
	"fmt"

	"github.com/zachfire9/agent-harness/internal/llm"
)

// OversizedPromptError reports that the latest user prompt cannot fit within the configured per-message budget.
type OversizedPromptError struct {
	Limit       int
	ActualChars int
}

func (e OversizedPromptError) Error() string {
	return fmt.Sprintf("prompt is too large for AGENT_MAX_MESSAGE_CHARS: %d characters exceeds limit %d", e.ActualChars, e.Limit)
}

func ValidatePromptSize(prompt string, limits ContextLimits) error {
	if limits.MaxMessageChars <= 0 {
		return nil
	}
	actual := len(prompt)
	if actual <= limits.MaxMessageChars {
		return nil
	}
	return OversizedPromptError{Limit: limits.MaxMessageChars, ActualChars: actual}
}

func latestUserMessageIndex(messages []llm.Message) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleUser {
			return i
		}
	}
	return -1
}
