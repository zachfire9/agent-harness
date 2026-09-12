# Agent Harness

A small Go-based AI agent harness built one reviewable learning step at a time.

The goal of this repo is to make each agent concept understandable through small commits and pull requests. Every implementation step includes deterministic tests before it lands on `main`.

## What this project demonstrates

This is intentionally not a full production assistant. It is a learning-focused local CLI app that shows the main moving pieces behind an agent harness:

- configuration for OpenAI-compatible model providers
- a testable CLI layer
- a small LLM client abstraction
- an agent runner that owns the model/tool loop
- model-callable tools with schema metadata
- manual tool debugging without an LLM call
- interactive chat with in-memory history
- context-window management and summarization
- trace output and durable run logs
- guarded command execution
- future-ready vector-store plumbing for RAG

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
- provider-neutral vector store abstraction with a default `none` implementation for future RAG support without requiring a vector database yet

## Project layout

```text
agent-harness/
  cmd/agent-harness/
    main.go              # thin executable entrypoint
  internal/
    agent/               # agent loop, context building, summaries, trace events
    cli/                 # command parsing and user-facing CLI behavior
    config/              # environment/.env config loading and safe display
    llm/                 # provider-neutral chat types plus OpenAI-compatible client
    runlog/              # durable JSONL event writer
    tools/               # tool interface, registry, and built-in tools
    vectorstore/         # future-ready vector store abstraction
  docs/plans/
    agent-harness-learning-plan.md
  README.md
```

The `internal` packages are split so each concept can be tested in isolation. The CLI layer coordinates dependencies, but the reusable agent behavior lives below it.

## Quick start

Run the placeholder command:

```powershell
go run ./cmd/agent-harness
```

Expected output:

```text
agent-harness: staged learning CLI ready
```

Run the test suite:

```powershell
go test ./...
```

## Configuration

The app loads config from:

1. Process environment variables, when they are set.
2. A local `.env` file in the current working directory.
3. Built-in defaults for optional values.

Process environment variables take precedence over values in `.env`. Only `OPENAI_API_KEY` is required. `OPENAI_BASE_URL` and `OPENAI_MODEL` have defaults. Base URLs are normalized by trimming whitespace and a trailing slash, so `https://api.openai.com/v1/` becomes `https://api.openai.com/v1`.

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
VECTOR_STORE_PROVIDER=none
```

`.env` is listed in `.gitignore`, so local secrets stay out of git. `.env.example` is safe to commit because it contains placeholders only.

You can also set values directly in PowerShell instead of using `.env`:

```powershell
$env:OPENAI_API_KEY="your-openai-api-key-here"
$env:OPENAI_BASE_URL="https://api.openai.com/v1"
$env:OPENAI_MODEL="gpt-4.1-mini"
```

### Config check

Check local configuration without making a model call:

```powershell
go run ./cmd/agent-harness config check
```

Expected output starts with `config ok` followed by a safe config summary. It shows provider/model settings, context limits, run-log settings, and the vector-store provider, but redacts the API key.

## Commands

### Ask

Ask with the configured model:

```powershell
go run ./cmd/agent-harness ask "What is an agent?"
```

Expected output is the assistant response from your configured OpenAI-compatible model, for example:

```text
An agent is a program that uses a model to decide what to do next, optionally call tools, and continue until it can return a final answer.
```

If config is missing, the command fails before making a model call and prints a helpful config error.

### Trace an ask run

Add `--trace` to print human-readable observability details to stderr while keeping the assistant answer on stdout:

```powershell
go run ./cmd/agent-harness ask --trace "What is an agent?"
```

Trace output includes:

- model calls
- context compaction reports
- tool calls
- tool result sizes
- truncation metadata
- summary/background-summary events
- final-answer sizes

Trace intentionally avoids printing full prompt content, tool output, API keys, or other secret-bearing config values.

### Chat

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

### Tool debug command

Debug a registered tool without making a model/API call:

```powershell
go run ./cmd/agent-harness tool echo '{"message":"hello from tool debug"}'
```

Expected output:

```text
hello from tool debug
```

This path executes the same registered tool implementation that the agent loop uses, but it bypasses the LLM so tool behavior can be tested directly.

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

## Core architecture

At runtime, the app flows through a few focused layers:

1. `cmd/agent-harness/main.go` calls `cli.Run(os.Args, os.Stdout, os.Stderr)`.
2. `internal/cli` parses the command, loads config when needed, constructs dependencies, and formats user-facing output.
3. `internal/config` loads process env and `.env` values, applies defaults, normalizes provider settings, and exposes safe display output.
4. `internal/llm` defines provider-neutral request/response types and implements an OpenAI-compatible chat client.
5. `internal/tools` defines tools, schemas, and a registry used by both manual debug commands and model-driven agent runs.
6. `internal/agent` owns the model loop, message history, context-window decisions, tool execution, summarization, and trace events.
7. `internal/runlog` writes durable structured JSONL events for later inspection.
8. `internal/vectorstore` provides a provider-neutral interface for future RAG storage/retrieval without requiring a real vector database yet.

This separation keeps model/network code behind interfaces and keeps most behavior unit-testable without real API calls.

## Tool-calling flow

The core agent loop is:

```text
user prompt
  -> build model-facing messages
  -> send available tool metadata to model
  -> model returns either final text or tool calls
  -> if tool calls: lookup registered tools by name
  -> execute tools with JSON args under app-side policy
  -> append tool results as `tool` messages with matching tool_call_id
  -> call the model again with the updated messages
  -> repeat until final answer or max step limit
```

Important details:

- Tool schemas guide the model, but app-side validation remains authoritative.
- Unknown tools, invalid args, tool failures, and max-step exhaustion return controlled errors.
- Tool result messages preserve the model-requested `tool_call_id` so the follow-up model call can connect observations to the original request.
- The manual `tool` command uses the same registered implementations, which makes debugging tools easier before involving the model.

## Message history vs model context

The harness separates stored conversation state from what gets sent to the model.

- **Message history** is the raw user/assistant/tool conversation the app has accumulated.
- **Model context** is the subset and/or summary that fits the configured limits for the next model call.

The context manager:

- preserves the system prompt, original user goal, and latest user message
- keeps recent messages for continuity
- summarizes older omitted messages with `AGENT_SUMMARY_MODEL`
- caps regular message content with `AGENT_MAX_MESSAGE_CHARS`
- caps tool results separately with `AGENT_MAX_TOOL_RESULT_CHARS`
- caps inserted summaries with `AGENT_MAX_SUMMARY_CHARS`
- batches summary updates with `AGENT_SUMMARY_MAX_INPUT_MESSAGES`
- rejects an oversized latest user prompt instead of silently truncating it
- emits context warnings when hard truncation still happens

Interactive chat can proactively start a background summary update after a response when the next turn is likely to need compaction. If the next user message arrives before the summary is ready, chat waits for that prepared summary before making the next main model call.

## Run logs vs trace output

Trace output and run logs solve related but different problems.

- `--trace` is opt-in, human-readable, and sent to stderr during a run.
- Run/session logs are durable JSONL files under `AGENT_RUN_LOG_DIR`.

Run logs can include run starts, errors, final stored messages, model-context snapshots, context reports, trace events, and machine-readable truncation records. Configured secrets such as `OPENAI_API_KEY` are redacted before events are written. Disable these files with `AGENT_RUN_LOGS_ENABLED=false` when you only want stdout/stderr output.

## Vector store / future RAG support

Vector store support is intentionally future-ready but disabled by default.

`internal/vectorstore` defines provider-neutral document upsert and similarity-search interfaces. `VECTOR_STORE_PROVIDER=none` selects a no-op implementation for this first app iteration, so the rest of the agent can depend on the abstraction without requiring credentials, network calls, or vendor SDKs.

Future real providers could include:

- pgvector
- Qdrant
- Pinecone
- Weaviate
- Chroma

## Test strategy

The test suite is designed to keep the learning project deterministic:

- CLI behavior is tested by injecting fake clients and in-memory stdin/stdout/stderr buffers.
- LLM behavior is tested with fakes or `httptest`; unit tests do not call real model APIs.
- Tool behavior is tested directly through each tool and through the shared registry/CLI paths.
- Agent-loop behavior is tested with fake model responses that request tools or return final answers.
- Context management is tested with synthetic histories to verify summarization, truncation, and latest-prompt protection.
- Config tests use temporary working directories and environment overrides.
- Run-log tests write to temporary directories and verify JSONL events plus secret redaction.
- Vector-store tests use the default no-op implementation and unsupported-provider errors; no test requires a real vector database.

Primary verification command:

```powershell
go test ./...
```

Manual smoke tests are useful for checking real provider integration, but they are not required for deterministic unit-test coverage.

## Learning walkthrough

The staged plan built the project in these phases:

1. **CLI foundation:** create a thin executable and testable internal CLI package.
2. **Prompt input:** add `ask` command parsing before any real model integration.
3. **Configuration:** load provider settings from env or `.env` without committing secrets.
4. **LLM abstraction:** define chat messages, requests, responses, and a fake client.
5. **OpenAI-compatible client:** implement real chat/completions calls behind the interface.
6. **Agent orchestration:** move message construction and model calls into `internal/agent`.
7. **Tool registry:** represent tools as named, schema-described functions.
8. **Demo tool:** add `echo` so tool execution is testable without an LLM.
9. **Manual tool debug:** expose registered tools through `agent-harness tool`.
10. **Tool-call parsing:** parse model responses that request tools.
11. **Agent tool loop:** execute model-requested tools, append observations, and re-call the model.
12. **Interactive chat:** preserve in-memory history across user turns.
13. **Workspace file tools:** add sandboxed file listing, reading, and searching.
14. **Context management:** separate raw history from model-facing context with summaries and limits.
15. **Trace output:** make model/tool/context decisions visible without leaking content or secrets.
16. **Run/session logging:** persist structured events for post-run inspection.
17. **Gated command tool:** allow a tiny exact command allowlist with explicit confirmation and timeout.
18. **Vector-store abstraction:** prepare for future RAG without choosing a vendor.
19. **Provider/config polish:** add `config check`, normalize base URLs, and make safe config output more useful.
20. **Walkthrough documentation:** turn the completed repo into a readable learning artifact.

See [`docs/plans/agent-harness-learning-plan.md`](docs/plans/agent-harness-learning-plan.md) for the detailed checklist and PR links for each step.

## Deferred ideas

These are intentionally not part of the first learning sequence:

- Web UI
- Telegram bot interface
- Scheduler/cron jobs
- GitHub issue/PR tools
- Multi-agent delegation
- Long-term memory
- Browser/web search tools

They are good follow-up milestones once the basic local CLI agent harness is understandable and reviewable.
