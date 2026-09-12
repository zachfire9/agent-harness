# Agent Harness

A small Go-based AI agent harness built one reviewable learning step at a time.

The goal of this repo is to make each agent concept understandable through small commits and pull requests. Every implementation step should include deterministic unit tests before code lands on `main`.

## Current status

The project currently has:

- Go module and thin CLI entrypoint at `cmd/agent-harness/main.go`
- testable CLI package under `internal/cli`
- `ask` command for sending prompts to a configured model
- config loading from process environment variables or a local `.env` file
- minimal LLM package under `internal/llm` with message types, chat client interface, fake client, OpenAI-compatible chat/completions client, and tool-call response parsing
- `internal/agent` runner that builds message history, sends available tool metadata, executes model-requested tools, appends tool results, and repeats until a final answer
- `internal/tools` registry for named, schema-described tools
- `echo` demo tool for deterministic tool-execution tests
- workspace-safe `list_files`, `read_file`, and `search_files` tools with path sandboxing and output caps
- gated `run_command` tool with an exact-command allowlist, explicit confirmation field, timeout, and structured stdout/stderr capture
- deterministic context-window management that preserves the system prompt and original user goal, keeps recent history, summarizes older omitted messages with a separately configured summary model, and truncates oversized messages before model calls
- `tool` debug command for manually executing registered tools without an LLM/API call
- `ask` wired through the agent runner to print the assistant response
- `chat` command for an in-memory interactive conversation that preserves history across turns while still allowing tool calls inside each turn
- opt-in `--trace` output for model calls, context compaction reports, tool calls/results, and final-answer summaries without dumping full prompt/tool content
- durable JSONL run/session logs under the configured local run-log directory, with structured context/truncation records and secret redaction

## Run

```powershell
go run ./cmd/agent-harness
```

Expected output:

```text
agent-harness: staged learning CLI ready
```

Ask with the configured model:

```powershell
go run ./cmd/agent-harness ask "What is an agent?"
```

Add `--trace` to print human-readable observability details to stderr while keeping the assistant answer on stdout:

```powershell
go run ./cmd/agent-harness ask --trace "What is an agent?"
```

Trace output includes model calls, context compaction reports, tool calls, tool result sizes, truncation metadata, and final-answer sizes. It intentionally avoids printing full prompt content, tool output, API keys, or other secret-bearing config values.

Expected output is the assistant response from your configured OpenAI-compatible model, for example:

```text
An agent is a program that uses a model to decide what to do next, optionally call tools, and continue until it can return a final answer.
```

If config is missing, the command fails before making a model call and prints a helpful config error.

Start an interactive chat session with the configured model:

```powershell
go run ./cmd/agent-harness chat
```

Example session:

```text
You: What is an agent?
Agent: An agent is a program that uses a model to decide what to do next, optionally call tools, and continue until it can return a final answer.
You: Can you give a simpler example?
Agent: A simple agent might answer a question, call a calculator tool if math is needed, then use that result in its final answer.
You: /exit
Goodbye.
```

The first chat implementation keeps history only in memory for the lifetime of the process. Use `/exit`, `exit`, `quit`, or EOF to end the session.

Debug a registered tool without making a model/API call:

```powershell
go run ./cmd/agent-harness tool echo '{"message":"hello from tool debug"}'
```

Expected output:

```text
hello from tool debug
```

This path executes the same registered tool implementation that the agent loop will use later, but it bypasses the LLM so tool behavior can be tested directly.

Debug workspace file tools from the current working directory:

```powershell
go run ./cmd/agent-harness tool list_files '{"path":"."}'
go run ./cmd/agent-harness tool read_file '{"path":"README.md"}'
go run ./cmd/agent-harness tool search_files '{"query":"agent","path":"."}'
```

The workspace root is the directory where you run `agent-harness`. File tools reject `..` traversal and absolute paths outside that workspace, skip sensitive local files such as `.env` and `.git`, and cap long outputs with an explicit truncation notice.

Debug the gated command tool with an exact allowlisted command and explicit confirmation:

```powershell
go run ./cmd/agent-harness tool run_command '{"command":"pwd","confirm":true}'
```

The initial allowlist is intentionally small: `pwd`, `git status`, `git status --short`, `git diff`, and `go test ./...`. The tool exposes that allowlist to the model as a JSON-schema enum, supports optional per-command descriptions for custom/local software, rejects commands outside that exact allowlist, rejects calls without `confirm:true`, runs without a shell, enforces a timeout, and returns JSON containing `command`, `exit_code`, `stdout`, and `stderr`.

## Configuration

Step 03 adds configuration loading for future model calls. The app loads config from:

1. Process environment variables, when they are set.
2. A local `.env` file in the current working directory.
3. Built-in defaults for optional values.

Process environment variables take precedence over values in `.env`. Only `OPENAI_API_KEY` is required. `OPENAI_BASE_URL` and `OPENAI_MODEL` have defaults.

Context-window limits are optional and deterministic:

- `AGENT_MAX_CONTEXT_MESSAGES` caps how many messages are sent to the model.
- `AGENT_MAX_MESSAGE_CHARS` caps non-tool message content.
- `AGENT_MAX_TOOL_RESULT_CHARS` caps tool-result content separately so large tool outputs cannot crowd out the conversation.
- `AGENT_SUMMARY_MODEL` selects the separately configured model used to update the running summary; it defaults to `gpt-4.1-mini`.
- `AGENT_MAX_SUMMARY_CHARS` caps the inserted summary message.
- `AGENT_SUMMARY_MAX_INPUT_MESSAGES` controls how many newly compacted messages are sent to one summary-model update; it defaults to `10` and accepts any positive integer without an app-enforced upper cap.
- `AGENT_RUN_LOGS_ENABLED` enables or disables durable JSONL run/session logs; it defaults to `true`.
- `AGENT_RUN_LOG_DIR` controls where run/session logs are written; it defaults to `.agent-harness/runs`.

The context manager preserves the system prompt and original user goal, keeps the most recent remaining messages, updates a running summary for older omitted messages, and inserts that summary into the next model request. Interactive chat proactively starts summary updates in a background goroutine after a response when the next user turn is likely to exceed the message cap; if the next user message arrives while that update is still running, the chat waits for the prepared summary before making the next main model call. Oversized latest user prompts are rejected with a clear error instead of being silently truncated. Any remaining hard truncation, such as capped tool results or capped inserted summaries, is logged to stderr as a `context warning` with kept/omitted/original character counts so we can monitor frequency and tune the limits later.

Run/session logs are durable JSONL files under `AGENT_RUN_LOG_DIR`. They record run starts, errors, final stored messages, model-context snapshots, context reports, trace events, and machine-readable `context.truncation` events. Configured secrets such as `OPENAI_API_KEY` are redacted before events are written. Disable these files with `AGENT_RUN_LOGS_ENABLED=false` when you only want stdout/stderr output.

### Create an OpenAI API key

Your ChatGPT Plus subscription does not cover API usage. API requests are billed separately through the OpenAI Platform.

1. Go to [platform.openai.com/api-keys](https://platform.openai.com/api-keys).
2. Sign in or create an OpenAI Platform account.
3. Create a new secret key.
4. Copy the key once and store it in your local `.env` file.
5. In the Platform billing/settings area, add billing and set a usage limit before making real model calls.

Do not commit API keys, tokens, passwords, or real connection strings.

### Local `.env` file

Create your local config by copying the committed example file:

```powershell
Copy-Item .env.example .env
```

Then edit `.env` and replace the placeholder API key:

```text
OPENAI_API_KEY=your-openai-api-key-here
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_MODEL=gpt-4.1-mini
AGENT_SUMMARY_MODEL=gpt-4.1-mini
AGENT_MAX_CONTEXT_MESSAGES=40
AGENT_MAX_MESSAGE_CHARS=8000
AGENT_MAX_TOOL_RESULT_CHARS=4000
AGENT_MAX_SUMMARY_CHARS=2000
AGENT_SUMMARY_MAX_INPUT_MESSAGES=10
AGENT_RUN_LOGS_ENABLED=true
AGENT_RUN_LOG_DIR=.agent-harness/runs
```

`.env` is listed in `.gitignore`, so local secrets stay out of git. `.env.example` is safe to commit because it contains placeholders only.

You can also set values directly in PowerShell instead of using `.env`:

```powershell
$env:OPENAI_API_KEY="your-openai-api-key-here"
$env:OPENAI_BASE_URL="https://api.openai.com/v1"
$env:OPENAI_MODEL="gpt-4.1-mini"
```

## Test

```powershell
go test ./...
```

## Plan

See [`docs/plans/agent-harness-learning-plan.md`](docs/plans/agent-harness-learning-plan.md) for the staged implementation plan.
