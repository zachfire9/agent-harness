package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/zachfire9/agent-harness/internal/llm"
	"github.com/zachfire9/agent-harness/internal/tools"
)

const (
	defaultSystemPrompt = "You are a helpful CLI assistant. Answer clearly and concisely."
	defaultMaxSteps     = 8
)

// Runner orchestrates an agent turn, including model-requested tool calls.
type Runner struct {
	chatClient     llm.ChatClient
	model          string
	systemPrompt   string
	tools          tools.Registry
	maxSteps       int
	contextManager ContextManager
}

// Result contains the final answer and complete message history for a run.
type Result struct {
	Answer         string
	Messages       []llm.Message
	ContextReports []ContextReport
	Summary        ConversationSummary
	TraceEvents    []TraceEvent
}

// New creates a runner with the default system prompt and no tools.
func New(chatClient llm.ChatClient, model string) Runner {
	return Runner{
		chatClient:     chatClient,
		model:          model,
		systemPrompt:   defaultSystemPrompt,
		tools:          tools.NewRegistry(),
		maxSteps:       defaultMaxSteps,
		contextManager: NewContextManager(ContextLimits{}),
	}
}

// NewWithTools creates a runner with the default system prompt and a tool registry.
func NewWithTools(chatClient llm.ChatClient, model string, registry tools.Registry) Runner {
	runner := New(chatClient, model)
	runner.tools = registry
	return runner
}

// NewWithContextLimits creates a runner with deterministic context-window limits.
func NewWithContextLimits(chatClient llm.ChatClient, model string, limits ContextLimits) Runner {
	runner := New(chatClient, model)
	runner.contextManager = NewContextManager(limits)
	return runner
}

// NewWithToolsAndContextLimits creates a runner with tool and context-window configuration.
func NewWithToolsAndContextLimits(chatClient llm.ChatClient, model string, registry tools.Registry, limits ContextLimits) Runner {
	runner := NewWithTools(chatClient, model, registry)
	runner.contextManager = NewContextManager(limits)
	return runner
}

// NewWithToolsContextLimitsAndSummarizer creates a runner with tool, context-window, and running-summary configuration.
func NewWithToolsContextLimitsAndSummarizer(chatClient llm.ChatClient, model string, registry tools.Registry, limits ContextLimits, summarizer Summarizer) Runner {
	runner := NewWithTools(chatClient, model, registry)
	runner.contextManager = NewContextManagerWithSummarizer(limits, summarizer)
	return runner
}

// Run builds message history, calls the model, executes requested tools, and
// repeats until the model returns a final answer or the step limit is reached.
func (r Runner) Run(ctx context.Context, prompt string) (Result, error) {
	return r.RunWithHistory(ctx, nil, prompt)
}

// RunWithHistory appends a new user prompt to existing conversation history,
// then runs the same model/tool loop used by Run. It returns the complete
// updated history when the turn succeeds.
func (r Runner) RunWithHistory(ctx context.Context, history []llm.Message, prompt string) (Result, error) {
	return r.RunWithSummary(ctx, history, ConversationSummary{}, prompt)
}

// RunWithSummary appends a new user prompt to existing conversation history and
// uses the supplied running summary when building model-facing context.
func (r Runner) RunWithSummary(ctx context.Context, history []llm.Message, summary ConversationSummary, prompt string) (Result, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return Result{}, errors.New("prompt is required")
	}
	if r.chatClient == nil {
		return Result{}, errors.New("chat client is required")
	}
	if r.maxSteps <= 0 {
		r.maxSteps = defaultMaxSteps
	}

	messages := append([]llm.Message(nil), history...)
	if len(messages) == 0 {
		messages = append(messages, llm.Message{Role: llm.RoleSystem, Content: r.systemPrompt})
	}
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: prompt})
	if err := ValidatePromptSize(prompt, r.contextManager.limits); err != nil {
		return Result{}, err
	}
	toolSpecs := toolSpecsFromRegistry(r.tools)
	contextReports := []ContextReport{}
	traceEvents := []TraceEvent{}

	for step := 0; step < r.maxSteps; step++ {
		stepNumber := step + 1
		snapshot, err := r.contextManager.Build(ctx, messages, summary)
		if err != nil {
			return Result{}, fmt.Errorf("build model context: %w", err)
		}
		summary = snapshot.Summary
		contextReports = append(contextReports, snapshot.Report)
		traceEvents = append(traceEvents, TraceEvent{
			Type:            TraceEventContextBuild,
			Step:            stepNumber,
			InputMessages:   len(messages),
			OutputMessages:  len(snapshot.Messages),
			MaxMessages:     r.contextManager.limits.MaxMessages,
			ContextReport:   snapshot.Report,
			ContextMessages: append([]llm.Message(nil), snapshot.Messages...),
		})
		request := llm.NewChatRequest(r.model, snapshot.Messages...)
		request.Tools = toolSpecs
		traceEvents = append(traceEvents, TraceEvent{
			Type:           TraceEventModelCall,
			Step:           stepNumber,
			Model:          r.model,
			InputMessages:  len(messages),
			OutputMessages: len(snapshot.Messages),
			ToolCount:      len(toolSpecs),
		})

		response, err := r.chatClient.Chat(ctx, request)
		if err != nil {
			return Result{}, fmt.Errorf("chat failed: %w", err)
		}

		assistantMessage := response.Message
		assistantMessage.ToolCalls = response.ToolCalls
		messages = append(messages, assistantMessage)

		if response.IsFinalAnswer() {
			traceEvents = append(traceEvents, TraceEvent{Type: TraceEventFinalAnswer, Step: stepNumber, FinalAnswerChars: len(response.Message.Content)})
			return Result{
				Answer:         response.Message.Content,
				Messages:       messages,
				ContextReports: contextReports,
				Summary:        summary,
				TraceEvents:    traceEvents,
			}, nil
		}

		for _, toolCall := range response.ToolCalls {
			traceEvents = append(traceEvents, TraceEvent{Type: TraceEventToolCall, Step: stepNumber, ToolName: toolCall.Name, ToolCallID: toolCall.ID})
			tool, err := r.tools.Require(toolCall.Name)
			if err != nil {
				return Result{}, err
			}

			toolResult, err := tool.Execute(ctx, toolCall.Arguments)
			if err != nil {
				return Result{}, fmt.Errorf("tool %s failed: %w", toolCall.Name, err)
			}
			traceEvents = append(traceEvents, TraceEvent{Type: TraceEventToolResult, Step: stepNumber, ToolName: toolCall.Name, ToolCallID: toolCall.ID, ToolResultChars: len(toolResult)})

			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				Content:    toolResult,
				ToolCallID: toolCall.ID,
			})
		}
	}

	return Result{}, fmt.Errorf("max agent steps exceeded: %d", r.maxSteps)
}

func toolSpecsFromRegistry(registry tools.Registry) []llm.ToolSpec {
	metadata := registry.Metadata()
	if len(metadata) == 0 {
		return nil
	}

	specs := make([]llm.ToolSpec, 0, len(metadata))
	for _, item := range metadata {
		specs = append(specs, llm.ToolSpec{
			Name:        item.Name,
			Description: item.Description,
			Schema:      item.Schema,
		})
	}
	return specs
}
