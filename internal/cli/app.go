package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/zachfire9/agent-harness/internal/agent"
	"github.com/zachfire9/agent-harness/internal/config"
	"github.com/zachfire9/agent-harness/internal/llm"
	"github.com/zachfire9/agent-harness/internal/runlog"
	harnessruntime "github.com/zachfire9/agent-harness/internal/runtime"
	"github.com/zachfire9/agent-harness/internal/tools"
	"github.com/zachfire9/agent-harness/internal/vectorstore"
	"github.com/zachfire9/agent-harness/internal/version"
)

const defaultMessage = "agent-harness: staged learning CLI ready"

// App holds command dependencies so CLI behavior can be tested without real API calls.
type App struct {
	chatClient          llm.ChatClient
	model               string
	stdin               io.Reader
	contextLimits       agent.ContextLimits
	summarizer          agent.Summarizer
	summaryModel        string
	runLogDir           string
	runLogsEnabled      bool
	runLogSecretList    []string
	vectorStore         vectorstore.Store
	vectorStoreProvider string
}

// NewApp creates a CLI app with an injected chat client and model.
func NewApp(chatClient llm.ChatClient, model string) App {
	return App{chatClient: chatClient, model: model, stdin: os.Stdin, vectorStore: vectorstore.NewNoopStore(), vectorStoreProvider: config.DefaultVectorStoreProvider}
}

// NewAppWithInput creates a CLI app with an injected chat client, model, and input stream.
func NewAppWithInput(chatClient llm.ChatClient, model string, stdin io.Reader) App {
	return App{chatClient: chatClient, model: model, stdin: stdin, vectorStore: vectorstore.NewNoopStore(), vectorStoreProvider: config.DefaultVectorStoreProvider}
}

// NewAppWithInputAndContextLimits creates a CLI app with injected dependencies and context limits.
func NewAppWithInputAndContextLimits(chatClient llm.ChatClient, model string, stdin io.Reader, limits agent.ContextLimits) App {
	return App{chatClient: chatClient, model: model, stdin: stdin, contextLimits: limits, vectorStore: vectorstore.NewNoopStore(), vectorStoreProvider: config.DefaultVectorStoreProvider}
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
		summarizer:          agent.NewLLMSummarizer(summaryClient, cfg.SummaryModel),
		summaryModel:        cfg.SummaryModel,
		runLogDir:           cfg.RunLogDir,
		runLogsEnabled:      cfg.RunLogsEnabled,
		runLogSecretList:    []string{cfg.APIKey},
		vectorStore:         vectorstore.NewNoopStore(),
		vectorStoreProvider: cfg.VectorStoreProvider,
	}
}

// WithRunLogging returns a copy of the app configured for durable run/session logs.
func (a App) WithRunLogging(dir string, enabled bool, secrets []string) App {
	a.runLogDir = dir
	a.runLogsEnabled = enabled
	a.runLogSecretList = append([]string(nil), secrets...)
	return a
}

// WithVectorStore returns a copy of the app configured with a vector store dependency.
func (a App) WithVectorStore(store vectorstore.Store) App {
	if store != nil {
		a.vectorStore = store
	}
	return a
}

// Run executes the agent-harness command and returns a process-style exit code.
func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) >= 3 && args[1] == "config" && args[2] == "check" {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(stderr, "config error: %v\n", err)
			return 1
		}
		if _, err := vectorstore.New(vectorstore.Config{Provider: cfg.VectorStoreProvider}); err != nil {
			fmt.Fprintf(stderr, "config error: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "config ok\n%s\n", cfg.SafeString())
		return 0
	}
	if len(args) <= 1 || (args[1] != "ask" && args[1] != "chat") || (args[1] == "ask" && strings.TrimSpace(strings.Join(args[2:], " ")) == "") {
		return App{}.Run(args, stdout, stderr)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "config error: %v\n", err)
		return 1
	}

	store, err := vectorstore.New(vectorstore.Config{Provider: cfg.VectorStoreProvider})
	if err != nil {
		fmt.Fprintf(stderr, "config error: %v\n", err)
		return 1
	}

	app := NewAppWithConfig(llm.NewOpenAIClient(cfg.BaseURL, cfg.APIKey), llm.NewOpenAIClient(cfg.BaseURL, cfg.APIKey), cfg).WithVectorStore(store)
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
	case "daemon":
		return a.runDaemon(args[2:], stdout, stderr)
	case "init":
		return a.runInit(args[2:], stdout, stderr)
	case "service":
		return a.runService(args[2:], stdout, stderr)
	case "status":
		return a.runStatus(args[2:], stdout, stderr)
	case "tool":
		return a.runTool(args[2:], stdout, stderr)
	case "version":
		return a.runVersion(args[2:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", args[1])
		return 1
	}
}

func (a App) runVersion(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintf(stderr, "version error: unknown option: %s\n", args[0])
		return 1
	}
	fmt.Fprint(stdout, version.Info().FormatHuman())
	return 0
}

func (a App) runInit(args []string, stdout io.Writer, stderr io.Writer) int {
	opts, err := parseRuntimeOptions(args)
	if err != nil {
		fmt.Fprintf(stderr, "init error: %v\n", err)
		return 1
	}
	paths, err := resolveRuntimePaths(opts)
	if err != nil {
		fmt.Fprintf(stderr, "init error: %v\n", err)
		return 1
	}
	if err := harnessruntime.InitInstance(paths); err != nil {
		fmt.Fprintf(stderr, "init error: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "initialized instance %s at %s\n", opts.instance, paths.Home)
	return 0
}

func (a App) runDaemon(args []string, stdout io.Writer, stderr io.Writer) int {
	opts, err := parseRuntimeOptions(args)
	if err != nil {
		fmt.Fprintf(stderr, "daemon error: %v\n", err)
		return 1
	}
	paths, err := resolveRuntimePaths(opts)
	if err != nil {
		fmt.Fprintf(stderr, "daemon error: %v\n", err)
		return 1
	}
	if err := harnessruntime.InitInstance(paths); err != nil {
		fmt.Fprintf(stderr, "daemon error: %v\n", err)
		return 1
	}
	metadata := version.Info()
	cfg, err := harnessruntime.ReadRuntimeConfig(paths)
	if err != nil {
		fmt.Fprintf(stderr, "daemon error: %v\n", err)
		return 1
	}
	if opts.test {
		now := time.Now().UTC()
		if err := harnessruntime.WriteHeartbeatWithRuntimeConfig(paths, opts.instance, now, now, metadata, cfg); err != nil {
			fmt.Fprintf(stderr, "daemon error: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "test heartbeat written for instance %s\n", opts.instance)
		return 0
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := harnessruntime.RunDaemon(ctx, paths, opts.instance, metadata, time.Minute); err != nil {
		fmt.Fprintf(stderr, "daemon error: %v\n", err)
		return 1
	}
	return 0
}

func (a App) runService(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "install" {
		fmt.Fprintln(stderr, "service error: unsupported service command")
		return 1
	}
	configHome, err := parseServiceInstallOptions(args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "service error: %v\n", err)
		return 1
	}
	if configHome == "" {
		configHome, err = defaultConfigHome()
		if err != nil {
			fmt.Fprintf(stderr, "service error: %v\n", err)
			return 1
		}
	}
	unitPath, err := harnessruntime.InstallSystemdUserUnit(configHome)
	if err != nil {
		fmt.Fprintf(stderr, "service error: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "installed systemd user service template: %s\n", unitPath)
	fmt.Fprintln(stdout, "next steps:")
	fmt.Fprintln(stdout, "  systemctl --user daemon-reload")
	fmt.Fprintln(stdout, "  systemctl --user enable --now agent-harness@default.service")
	return 0
}

func parseServiceInstallOptions(args []string) (string, error) {
	var configHome string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config-home":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", fmt.Errorf("--config-home requires a value")
			}
			configHome = args[i+1]
			i++
		default:
			return "", fmt.Errorf("unknown option: %s", args[i])
		}
	}
	return configHome, nil
}

func defaultConfigHome() (string, error) {
	if configHome := os.Getenv("XDG_CONFIG_HOME"); configHome != "" {
		return configHome, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config"), nil
}

func (a App) runStatus(args []string, stdout io.Writer, stderr io.Writer) int {
	opts, err := parseRuntimeOptions(args)
	if err != nil {
		fmt.Fprintf(stderr, "status error: %v\n", err)
		return 1
	}
	paths, err := resolveRuntimePaths(opts)
	if err != nil {
		fmt.Fprintf(stderr, "status error: %v\n", err)
		return 1
	}
	status, err := harnessruntime.ReadStatus(paths.StatusPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(stderr, "status error: %v\n", err)
			return 1
		}
		status = harnessruntime.Status{Instance: opts.instance, Status: harnessruntime.StateUnknown}
	}
	jobs, jobsErr := harnessruntime.ReadJobs(paths.JobsPath)
	if jobsErr != nil && !errors.Is(jobsErr, os.ErrNotExist) {
		fmt.Fprintf(stderr, "status error: %v\n", jobsErr)
		return 1
	}
	if len(jobs.Jobs) > 0 {
		status.Jobs = jobs.Jobs
	}
	if opts.json {
		data, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "status error: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, string(data))
		return 0
	}
	writeHumanStatus(stdout, status, opts.instance, paths)
	return 0
}

type runtimeOptions struct {
	instance string
	home     string
	json     bool
	test     bool
}

func parseRuntimeOptions(args []string) (runtimeOptions, error) {
	opts := runtimeOptions{instance: "default"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--instance":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return runtimeOptions{}, fmt.Errorf("--instance requires a value")
			}
			opts.instance = args[i+1]
			i++
		case "--home":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return runtimeOptions{}, fmt.Errorf("--home requires a value")
			}
			opts.home = args[i+1]
			i++
		case "--json":
			opts.json = true
		case "--test":
			opts.test = true
		default:
			return runtimeOptions{}, fmt.Errorf("unknown option: %s", args[i])
		}
	}
	if err := harnessruntime.ValidateInstanceName(opts.instance); err != nil {
		return runtimeOptions{}, err
	}
	return opts, nil
}

func resolveRuntimePaths(opts runtimeOptions) (harnessruntime.Paths, error) {
	if opts.home != "" {
		return harnessruntime.PathsForHome(opts.home), nil
	}
	dataRoot := os.Getenv("XDG_DATA_HOME")
	if dataRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return harnessruntime.Paths{}, err
		}
		dataRoot = filepath.Join(home, ".local", "share")
	}
	return harnessruntime.LinuxPaths(dataRoot, opts.instance)
}

func writeHumanStatus(stdout io.Writer, status harnessruntime.Status, instance string, paths harnessruntime.Paths) {
	state := status.Status
	if state == "" {
		state = harnessruntime.StateUnknown
	}
	fmt.Fprintf(stdout, "instance: %s\n", instance)
	fmt.Fprintf(stdout, "status: %s\n", state)
	if status.Version != "" {
		fmt.Fprintf(stdout, "version: %s\n", status.Version)
	}
	if status.Commit != "" {
		fmt.Fprintf(stdout, "commit: %s\n", status.Commit)
	}
	if status.BuildDate != "" {
		fmt.Fprintf(stdout, "build_date: %s\n", status.BuildDate)
	}
	if status.Dirty != "" {
		fmt.Fprintf(stdout, "dirty: %s\n", status.Dirty)
	}
	if status.PID != 0 {
		fmt.Fprintf(stdout, "pid: %d\n", status.PID)
	}
	if len(status.Jobs) > 0 {
		fmt.Fprintln(stdout, "jobs:")
		for name, job := range status.Jobs {
			fmt.Fprintf(stdout, "  %s: %s next_run_at=%s\n", name, job.Status, job.NextRunAt.Format(time.RFC3339))
		}
	}
	fmt.Fprintf(stdout, "home: %s\n", paths.Home)
	fmt.Fprintf(stdout, "status_file: %s\n", paths.StatusPath)
	fmt.Fprintln(stdout, "service_status_hint:")
	fmt.Fprintf(stdout, "  systemctl --user status agent-harness@%s.service --no-pager\n", instance)
	fmt.Fprintln(stdout, "logs_hint:")
	fmt.Fprintf(stdout, "  journalctl --user -u agent-harness@%s.service -n 100 --no-pager\n", instance)
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

	logger, err := a.newRunLogger()
	if err != nil {
		fmt.Fprintf(stderr, "run log error: %v\n", err)
		return 1
	}
	defer logger.Close()
	writeRunStart(logger, prompt)

	registry, err := builtInTools()
	if err != nil {
		fmt.Fprintf(stderr, "tool registry error: %v\n", err)
		return 1
	}
	runner := agent.NewWithToolsContextLimitsSummarizerAndVectorStore(a.chatClient, a.model, registry, a.contextLimits, a.summarizer, a.vectorStore)
	result, err := runner.Run(context.Background(), prompt)
	if err != nil {
		writeRunError(logger, err)
		fmt.Fprintf(stderr, "ask failed: %v\n", err)
		return 1
	}

	writeRunSuccess(logger, result)
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
	logger, err := a.newRunLogger()
	if err != nil {
		fmt.Fprintf(stderr, "run log error: %v\n", err)
		return 1
	}
	defer logger.Close()
	_ = logger.Write(runlog.Event{Type: "session.start"})
	runner := agent.NewWithToolsContextLimitsSummarizerAndVectorStore(a.chatClient, a.model, registry, a.contextLimits, a.summarizer, a.vectorStore)
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
			writeRunStart(logger, prompt)
			writeRunError(logger, err)
			fmt.Fprintf(stderr, "chat turn failed: %v\n", err)
			continue
		}

		writeRunStart(logger, prompt)
		writeRunSuccess(logger, result)
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
		tools.NewRunCommandTool(defaultAllowedCommands(), 0),
	} {
		if err := registry.Register(tool); err != nil {
			return tools.Registry{}, err
		}
	}
	return registry, nil
}

func defaultAllowedCommands() []tools.AllowedCommand {
	return []tools.AllowedCommand{
		{Command: "pwd", Description: "Print the current working directory"},
		{Command: "git status", Description: "Show repository status"},
		{Command: "git status --short", Description: "Show concise repository status"},
		{Command: "git diff", Description: "Show unstaged changes"},
		{Command: "go test ./...", Description: "Run all Go tests in the module"},
	}
}
