package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/zachfire9/agent-harness/internal/agent"
	"github.com/zachfire9/agent-harness/internal/config"
	"github.com/zachfire9/agent-harness/internal/llm"
	"github.com/zachfire9/agent-harness/internal/tools"
)

const defaultMessage = "agent-harness: staged learning CLI ready"

// App holds command dependencies so CLI behavior can be tested without real API calls.
type App struct {
	chatClient    llm.ChatClient
	model         string
	stdin         io.Reader
	contextLimits agent.ContextLimits
	summarizer    agent.Summarizer
	summaryModel  string
}

// NewApp creates a CLI app with an injected chat client and model.
func NewApp(chatClient llm.ChatClient, model string) App {
	return App{chatClient: chatClient, model: model, stdin: os.Stdin}
}

// NewAppWithInput creates a CLI app with an injected chat client, model, and input stream.
func NewAppWithInput(chatClient llm.ChatClient, model string, stdin io.Reader) App {
	return App{chatClient: chatClient, model: model, stdin: stdin}
}

// NewAppWithInputAndContextLimits creates a CLI app with injected dependencies and context limits.
func NewAppWithInputAndContextLimits(chatClient llm.ChatClient, model string, stdin io.Reader, limits agent.ContextLimits) App {
	return App{chatClient: chatClient, model: model, stdin: stdin, contextLimits: limits}
}

// NewAppWithConfig creates a CLI app from loaded configuration.
func NewAppWithConfig(chatClient llm.ChatClient, summaryClient llm.ChatClient, cfg config.Config) App {
	return App{
		chatClient: chatClient,
		model:      cfg.Model,
		stdin:      os.Stdin,
		contextLimits: agent.ContextLimits{
			MaxMessages:             cfg.MaxContextMessages,
			MaxMessageChars:         cfg.MaxMessageChars,
			MaxToolResultChars:      cfg.MaxToolResultChars,
			MaxSummaryChars:         cfg.MaxSummaryChars,
			SummaryMaxInputMessages: cfg.SummaryInputMessages,
		},
		summarizer:   agent.NewLLMSummarizer(summaryClient, cfg.SummaryModel),
		summaryModel: cfg.SummaryModel,
	}
}

// Run executes the agent-harness command and returns a process-style exit code.
func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) <= 1 || (args[1] != "ask" && args[1] != "chat") || (args[1] == "ask" && strings.TrimSpace(strings.Join(args[2:], " ")) == "") {
		return App{}.Run(args, stdout, stderr)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "config error: %v\n", err)
		return 1
	}

	app := NewAppWithConfig(llm.NewOpenAIClient(cfg.BaseURL, cfg.APIKey), llm.NewOpenAIClient(cfg.BaseURL, cfg.APIKey), cfg)
	return app.Run(args, stdout, stderr)
}

// Run executes the command using the app's configured dependencies.
func (a App) Run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) <= 1 {
		fmt.Fprintln(stdout, defaultMessage)
		return 0
	}

	switch args[1] {
	case "ask":
		return a.runAsk(args[2:], stdout, stderr)
	case "chat":
		return a.runChat(args[2:], stdout, stderr)
	case "tool":
		return a.runTool(args[2:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", args[1])
		return 1
	}
}

func (a App) runAsk(promptArgs []string, stdout io.Writer, stderr io.Writer) int {
	traceEnabled, promptArgs := parseTraceFlag(promptArgs)
	prompt := strings.TrimSpace(strings.Join(promptArgs, " "))
	if prompt == "" {
		fmt.Fprintln(stderr, "ask requires a prompt")
		return 1
	}
	if a.chatClient == nil {
		fmt.Fprintln(stderr, "ask requires a chat client")
		return 1
	}

	registry, err := builtInTools()
	if err != nil {
		fmt.Fprintf(stderr, "tool registry error: %v\n", err)
		return 1
	}
	runner := agent.NewWithToolsContextLimitsAndSummarizer(a.chatClient, a.model, registry, a.contextLimits, a.summarizer)
	result, err := runner.Run(context.Background(), prompt)
	if err != nil {
		fmt.Fprintf(stderr, "ask failed: %v\n", err)
		return 1
	}

	writeContextWarnings(stderr, result.ContextReports)
	if traceEnabled {
		writeTraceConfig(stderr, a)
		writeTrace(stderr, result.TraceEvents)
	}
	fmt.Fprintln(stdout, result.Answer)
	return 0
}

func (a App) runChat(args []string, stdout io.Writer, stderr io.Writer) int {
	traceEnabled, _ := parseTraceFlag(args)
	if a.chatClient == nil {
		fmt.Fprintln(stderr, "chat requires a chat client")
		return 1
	}

	stdin := a.stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	registry, err := builtInTools()
	if err != nil {
		fmt.Fprintf(stderr, "tool registry error: %v\n", err)
		return 1
	}
	runner := agent.NewWithToolsContextLimitsAndSummarizer(a.chatClient, a.model, registry, a.contextLimits, a.summarizer)
	scanner := bufio.NewScanner(stdin)
	var history []llm.Message
	var summary agent.ConversationSummary
	var pendingSummary *summaryJob

	for {
		fmt.Fprint(stdout, "You: ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				fmt.Fprintf(stderr, "chat input failed: %v\n", err)
				return 1
			}
			fmt.Fprintln(stdout, "Goodbye.")
			return 0
		}

		prompt := strings.TrimSpace(scanner.Text())
		if shouldExitChat(prompt) {
			fmt.Fprintln(stdout, "Goodbye.")
			return 0
		}
		if prompt == "" {
			continue
		}

		if pendingSummary != nil {
			if traceEnabled {
				writeTraceSummaryJob(stderr, "waiting")
			}
			updatedSummary, err := pendingSummary.Wait(context.Background())
			if err == nil {
				summary = updatedSummary
				if traceEnabled {
					writeTraceSummaryJob(stderr, "ready")
				}
			} else if traceEnabled {
				writeTraceSummaryJob(stderr, "failed")
			}
			pendingSummary = nil
		}

		result, err := runner.RunWithSummary(context.Background(), history, summary, prompt)
		if err != nil {
			fmt.Fprintf(stderr, "chat turn failed: %v\n", err)
			continue
		}

		history = result.Messages
		summary = result.Summary
		writeContextWarnings(stderr, result.ContextReports)
		if traceEnabled {
			writeTraceConfig(stderr, a)
			writeTrace(stderr, result.TraceEvents)
		}
		fmt.Fprintf(stdout, "Agent: %s\n", result.Answer)
		pendingSummary = startSummaryJob(context.Background(), history, summary, a.contextLimits, a.summarizer)
		if traceEnabled && pendingSummary != nil {
			writeTraceSummaryJob(stderr, "started")
		}
	}
}

func shouldExitChat(input string) bool {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "/exit", "exit", "quit":
		return true
	default:
		return false
	}
}

func (a App) runTool(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) < 2 || strings.TrimSpace(args[0]) == "" || strings.TrimSpace(strings.Join(args[1:], " ")) == "" {
		fmt.Fprintln(stderr, "tool requires a tool name and JSON args")
		return 1
	}

	registry, err := builtInTools()
	if err != nil {
		fmt.Fprintf(stderr, "tool registry error: %v\n", err)
		return 1
	}

	tool, err := registry.Require(args[0])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	argsJSON := strings.Join(args[1:], " ")
	result, err := tool.Execute(context.Background(), json.RawMessage(argsJSON))
	if err != nil {
		fmt.Fprintf(stderr, "tool failed: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, result)
	return 0
}

func builtInTools() (tools.Registry, error) {
	registry := tools.NewRegistry()
	workspaceRoot, err := os.Getwd()
	if err != nil {
		return tools.Registry{}, fmt.Errorf("resolve workspace root: %w", err)
	}
	for _, tool := range []tools.Tool{
		tools.NewEchoTool(),
		tools.NewListFilesTool(workspaceRoot, 0),
		tools.NewReadFileTool(workspaceRoot, 0),
		tools.NewSearchFilesTool(workspaceRoot, 0),
	} {
		if err := registry.Register(tool); err != nil {
			return tools.Registry{}, err
		}
	}
	return registry, nil
}
