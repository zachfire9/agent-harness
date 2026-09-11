package agent

import (
	"fmt"

	"github.com/zachfire9/agent-harness/internal/llm"
)

const truncationNoticeFormat = "\n\n[truncated: omitted %d characters]"

// ContextLimits controls how much run history is sent to the model for the next call.
type ContextLimits struct {
	// MaxMessages caps the number of messages sent to the model. Values <= 0 disable this cap.
	MaxMessages int
	// MaxMessageChars caps non-tool message content. Values <= 0 disable this cap.
	MaxMessageChars int
	// MaxToolResultChars caps tool-result message content. Values <= 0 disable this cap.
	MaxToolResultChars int
}

// ContextManager builds the model-facing context from complete run history.
type ContextManager struct {
	limits ContextLimits
}

// ContextSnapshot contains the messages to send to the model and a report of any omitted context.
type ContextSnapshot struct {
	Messages []llm.Message
	Report   ContextReport
}

// ContextReport describes deterministic context-management decisions for future trace/logging.
type ContextReport struct {
	OmittedMessages int
	Truncations     []Truncation
}

// Truncation records one oversized message that was shortened before a model call.
type Truncation struct {
	Index         int
	Role          llm.Role
	OriginalChars int
	KeptChars     int
	OmittedChars  int
}

// NewContextManager creates a deterministic, rule-based context manager.
func NewContextManager(limits ContextLimits) ContextManager {
	return ContextManager{limits: limits}
}

// Build returns the model-facing message list. It preserves the system prompt
// and original user goal, keeps the most recent remaining messages, and adds
// explicit notices when oversized message content is truncated.
func (m ContextManager) Build(history []llm.Message) ContextSnapshot {
	messages, report := m.truncateMessages(history)
	messages, omitted := m.trimMessages(messages)
	report.OmittedMessages = omitted

	return ContextSnapshot{
		Messages: messages,
		Report:   report,
	}
}

func (m ContextManager) truncateMessages(history []llm.Message) ([]llm.Message, ContextReport) {
	messages := make([]llm.Message, len(history))
	copy(messages, history)

	report := ContextReport{}
	for i := range messages {
		limit := m.limits.MaxMessageChars
		if messages[i].Role == llm.RoleTool && m.limits.MaxToolResultChars > 0 {
			limit = m.limits.MaxToolResultChars
		}
		if limit <= 0 || len(messages[i].Content) <= limit {
			continue
		}

		originalChars := len(messages[i].Content)
		omittedChars := originalChars - limit
		messages[i].Content = messages[i].Content[:limit] + fmt.Sprintf(truncationNoticeFormat, omittedChars)
		report.Truncations = append(report.Truncations, Truncation{
			Index:         i,
			Role:          messages[i].Role,
			OriginalChars: originalChars,
			KeptChars:     limit,
			OmittedChars:  omittedChars,
		})
	}

	return messages, report
}

func (m ContextManager) trimMessages(messages []llm.Message) ([]llm.Message, int) {
	if m.limits.MaxMessages <= 0 || len(messages) <= m.limits.MaxMessages {
		copied := make([]llm.Message, len(messages))
		copy(copied, messages)
		return copied, 0
	}

	preservedIndexes := preservedContextIndexes(messages)
	if len(preservedIndexes) >= m.limits.MaxMessages {
		trimmed := make([]llm.Message, 0, m.limits.MaxMessages)
		for _, index := range preservedIndexes[:m.limits.MaxMessages] {
			trimmed = append(trimmed, messages[index])
		}
		return trimmed, len(messages) - len(trimmed)
	}

	selected := map[int]bool{}
	for _, index := range preservedIndexes {
		selected[index] = true
	}

	for i := len(messages) - 1; i >= 0 && len(selected) < m.limits.MaxMessages; i-- {
		if selected[i] {
			continue
		}
		selected[i] = true
	}

	trimmed := make([]llm.Message, 0, len(selected))
	for i, message := range messages {
		if selected[i] {
			trimmed = append(trimmed, message)
		}
	}

	return trimmed, len(messages) - len(trimmed)
}

func preservedContextIndexes(messages []llm.Message) []int {
	indexes := []int{}
	for i, message := range messages {
		if message.Role == llm.RoleSystem {
			indexes = append(indexes, i)
			break
		}
	}
	for i, message := range messages {
		if message.Role == llm.RoleUser {
			indexes = append(indexes, i)
			break
		}
	}
	return indexes
}
