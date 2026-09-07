package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/zachfire9/agent-harness/internal/agent"
	"github.com/zachfire9/agent-harness/internal/config"
	"github.com/zachfire9/agent-harness/internal/llm"
	"github.com/zachfire9/agent-harness/internal/tools"
)

const defaultMessage = "agent-harness: staged learning CLI ready"

// App holds command dependencies so CLI behavior can be tested without real API calls.
type App struct {
	chatClient llm.ChatClient
	model      string
}

// NewApp creates a CLI app with an injected chat client and model.
func NewApp(chatClient llm.ChatClient, model string) App {
	return App{chatClient: chatClient, model: model}
}

// Run executes the agent-harness command and returns a process-style exit code.
func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) <= 1 || args[1] != "ask" || strings.TrimSpace(strings.Join(args[2:], " ")) == "" {
		return App{}.Run(args, stdout, stderr)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "config error: %v\n", err)
		return 1
	}

	app := NewApp(llm.NewOpenAIClient(cfg.BaseURL, cfg.APIKey), cfg.Model)
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
	case "tool":
		return a.runTool(args[2:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", args[1])
		return 1
	}
}

func (a App) runAsk(promptArgs []string, stdout io.Writer, stderr io.Writer) int {
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
	runner := agent.NewWithTools(a.chatClient, a.model, registry)
	result, err := runner.Run(context.Background(), prompt)
	if err != nil {
		fmt.Fprintf(stderr, "ask failed: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, result.Answer)
	return 0
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
	if err := registry.Register(tools.NewEchoTool()); err != nil {
		return tools.Registry{}, err
	}
	return registry, nil
}
