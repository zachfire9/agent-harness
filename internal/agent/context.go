package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/zachfire9/agent-harness/internal/llm"
)

const (
	truncationNoticeFormat = "\n\n[truncated: omitted %d characters]"
	summaryMessagePrefix   = "Conversation summary of earlier omitted context:\n"
)

// ContextLimits controls how much run history is sent to the model for the next call.
type ContextLimits struct {
	// MaxMessages caps the number of messages sent to the model. Values <= 0 disable this cap.
	MaxMessages int
	// MaxMessageChars caps non-tool message content. Values <= 0 disable this cap.
	MaxMessageChars int
	// MaxToolResultChars caps tool-result message content. Values <= 0 disable this cap.
	MaxToolResultChars int
	// MaxSummaryChars caps the inserted running-summary message. Values <= 0 disable this cap.
	MaxSummaryChars int
	// SummaryMaxInputMessages caps messages sent to one summary-model update. Values <= 0 mean all pending messages.
	SummaryMaxInputMessages int
}

// ContextManager builds the model-facing context from complete run history.
type ContextManager struct {
	limits     ContextLimits
	summarizer Summarizer
}

// ContextSnapshot contains the messages to send to the model and a report of any omitted context.
type ContextSnapshot struct {
	Messages []llm.Message
	Report   ContextReport
	Summary  ConversationSummary
}

// ContextReport describes deterministic context-management decisions for future trace/logging.
type ContextReport struct {
	OmittedMessages     int
	SummarizedMessages  int
	SummaryInserted     bool
	SummaryModelUpdates int
	Truncations         []Truncation
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

// NewContextManagerWithSummarizer creates a context manager that uses a model-backed
// summarizer when older middle history must be compacted out of the raw model context.
func NewContextManagerWithSummarizer(limits ContextLimits, summarizer Summarizer) ContextManager {
	return ContextManager{limits: limits, summarizer: summarizer}
}

// Build returns the model-facing message list. It preserves the system prompt
// and original user goal, inserts a running summary when older messages are
// compacted, keeps the most recent remaining messages, and adds explicit notices
// when oversized message content is truncated.
func (m ContextManager) Build(ctx context.Context, history []llm.Message, summary ConversationSummary) (ContextSnapshot, error) {
	messages, report := m.truncateMessages(history)
	messages, summary, trimReport, err := m.trimMessages(ctx, messages, summary)
	if err != nil {
		return ContextSnapshot{}, err
	}
	report.OmittedMessages = trimReport.OmittedMessages
	report.SummarizedMessages = trimReport.SummarizedMessages
	report.SummaryInserted = trimReport.SummaryInserted
	report.SummaryModelUpdates = trimReport.SummaryModelUpdates
	report.Truncations = append(report.Truncations, trimReport.Truncations...)

	return ContextSnapshot{
		Messages: messages,
		Report:   report,
		Summary:  summary,
	}, nil
}

func (m ContextManager) truncateMessages(history []llm.Message) ([]llm.Message, ContextReport) {
	messages := make([]llm.Message, len(history))
	copy(messages, history)

	report := ContextReport{}
	latestUserIndex := latestUserMessageIndex(messages)
	for i := range messages {
		limit := m.limits.MaxMessageChars
		if messages[i].Role == llm.RoleTool && m.limits.MaxToolResultChars > 0 {
			limit = m.limits.MaxToolResultChars
		}
		if limit <= 0 || len(messages[i].Content) <= limit || i == latestUserIndex {
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

func (m ContextManager) trimMessages(ctx context.Context, messages []llm.Message, summary ConversationSummary) ([]llm.Message, ConversationSummary, ContextReport, error) {
	if m.limits.MaxMessages <= 0 || len(messages) <= m.limits.MaxMessages {
		copied := make([]llm.Message, len(messages))
		copy(copied, messages)
		return copied, summary, ContextReport{}, nil
	}

	preservedIndexes := preservedContextIndexes(messages)
	summarySlot := 0
	if m.summarizer != nil || strings.TrimSpace(summary.Content) != "" {
		summarySlot = 1
	}
	recentSlots := m.limits.MaxMessages - len(preservedIndexes) - summarySlot
	if recentSlots < 0 {
		recentSlots = 0
	}

	selected := map[int]bool{}
	for _, index := range preservedIndexes {
		if len(selected) < m.limits.MaxMessages {
			selected[index] = true
		}
	}
	for i := len(messages) - 1; i >= 0 && recentSlots > 0; i-- {
		if selected[i] {
			continue
		}
		selected[i] = true
		recentSlots--
	}

	omitted := make([]llm.Message, 0, len(messages)-len(selected))
	for i, message := range messages {
		if !selected[i] {
			omitted = append(omitted, message)
		}
	}

	report := ContextReport{OmittedMessages: len(omitted)}
	newMessages, coveredThrough := omittedMessagesAfterCoverage(messages, selected, summary.CoveredMessageCount)
	if len(newMessages) > 0 && m.summarizer != nil {
		updated, updates, err := m.updateSummary(ctx, summary, newMessages, coveredThrough)
		if err != nil {
			return nil, summary, ContextReport{}, fmt.Errorf("update running summary: %w", err)
		}
		summary = updated
		report.SummarizedMessages = len(newMessages)
		report.SummaryModelUpdates = updates
	}

	trimmed := make([]llm.Message, 0, m.limits.MaxMessages)
	headerIndexes := preservedIndexes
	if len(headerIndexes) > 2 {
		headerIndexes = headerIndexes[:2]
	}
	for _, index := range headerIndexes {
		trimmed = append(trimmed, messages[index])
	}
	if strings.TrimSpace(summary.Content) != "" && len(trimmed) < m.limits.MaxMessages {
		summaryMessage, truncation := m.summaryMessage(summary.Content)
		trimmed = append(trimmed, summaryMessage)
		if truncation != nil {
			report.Truncations = append(report.Truncations, *truncation)
		}
		report.SummaryInserted = true
	}
	for i, message := range messages {
		if !selected[i] || containsIndex(headerIndexes, i) {
			continue
		}
		if len(trimmed) >= m.limits.MaxMessages {
			break
		}
		trimmed = append(trimmed, message)
	}

	return trimmed, summary, report, nil
}

func (m ContextManager) updateSummary(ctx context.Context, summary ConversationSummary, messages []llm.Message, coveredThrough int) (ConversationSummary, int, error) {
	content := summary.Content
	updates := 0
	batchSize := m.limits.SummaryMaxInputMessages
	if batchSize <= 0 {
		batchSize = len(messages)
	}
	for start := 0; start < len(messages); start += batchSize {
		end := start + batchSize
		if end > len(messages) {
			end = len(messages)
		}
		updated, err := m.summarizer.Summarize(ctx, SummaryInput{
			ExistingSummary: content,
			NewMessages:     messages[start:end],
			MaxChars:        m.limits.MaxSummaryChars,
		})
		if err != nil {
			return summary, updates, err
		}
		content = updated
		updates++
	}
	return ConversationSummary{Content: content, CoveredMessageCount: coveredThrough}, updates, nil
}

func (m ContextManager) summaryMessage(summary string) (llm.Message, *Truncation) {
	content := summaryMessagePrefix + strings.TrimSpace(summary)
	if m.limits.MaxSummaryChars > 0 && len(content) > m.limits.MaxSummaryChars {
		originalChars := len(content)
		omitted := originalChars - m.limits.MaxSummaryChars
		content = content[:m.limits.MaxSummaryChars] + fmt.Sprintf(truncationNoticeFormat, omitted)
		return llm.Message{Role: llm.RoleSystem, Content: content}, &Truncation{
			Index:         -1,
			Role:          llm.RoleSystem,
			OriginalChars: originalChars,
			KeptChars:     m.limits.MaxSummaryChars,
			OmittedChars:  omitted,
		}
	}
	return llm.Message{Role: llm.RoleSystem, Content: content}, nil
}

func omittedMessagesAfterCoverage(messages []llm.Message, selected map[int]bool, coveredCount int) ([]llm.Message, int) {
	if coveredCount < 0 {
		coveredCount = 0
	}
	newMessages := []llm.Message{}
	coveredThrough := coveredCount
	for i, message := range messages {
		if selected[i] || i < coveredCount {
			continue
		}
		newMessages = append(newMessages, message)
		if i+1 > coveredThrough {
			coveredThrough = i + 1
		}
	}
	return newMessages, coveredThrough
}

func containsIndex(indexes []int, target int) bool {
	for _, index := range indexes {
		if index == target {
			return true
		}
	}
	return false
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
	lastIndex := len(messages) - 1
	if lastIndex >= 0 {
		alreadyPreserved := false
		for _, index := range indexes {
			if index == lastIndex {
				alreadyPreserved = true
				break
			}
		}
		if !alreadyPreserved {
			indexes = append(indexes, lastIndex)
		}
	}
	return indexes
}
