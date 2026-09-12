# Agent Harness Learning Implementation Plan

> **Purpose:** Build a small Go-based AI agent harness one reviewable step at a time. Each step should teach one agent/app concept, include deterministic unit tests, and land through its own pull request after this initial plan commit.

## Working agreement

- This plan is the source of truth for the staged build.
- After this initial plan commit, every implementation step happens on its own branch.
- Branch naming format: `step-NN-short-description`, for example `step-01-go-cli-foundation`.
- Every step gets a GitHub pull request before it is merged into `main`.
- PR title format: `Step N: Short step description`, for example `Step 1: Go CLI foundation`.
- Each PR should explain:
  - what concept the step teaches
  - what code changed
  - what tests were added
  - how it was verified
- Zach reviews/approves/merges PRs before the next step is based on `main`.
- After a PR is created, add the PR link to that step in this plan.
- On the step branch, mark that step as completed once the step's implementation and tests are done so it will show as completed when the PR is merged into `main`.
- Updating the plan status/PR links should be part of the relevant step branch/PR unless Zach asks for a separate docs-only update.

## Testing standard

Every behavior introduced by a commit gets a deterministic unit test.

- Unit tests should not call real LLM APIs.
- LLM/network behavior should be tested with fakes or `httptest`.
- Real model calls are manual smoke tests only.
- Agent-loop steps should treat token growth as a design constraint: cap tool outputs early, then add explicit context-window management before longer-lived chat/session features.
- Each implementation PR should pass:

```powershell
go test ./...
```

The development rhythm should be:

```text
small concept -> failing or updated test -> minimal implementation -> passing tests -> PR -> review/merge
```

## Proposed project shape

The layout can evolve, but the intended direction is:

```text
agent-harness/
  cmd/agent-harness/
    main.go
  internal/
    agent/
    cli/
    config/
    llm/
    tools/
    trace/
  docs/
    plans/
  go.mod
  README.md
```

## Step checklist

### Step 01 — Go CLI foundation

- **Status:** Completed
- **Branch:** `step-01-go-cli-foundation`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/1
- **Concept:** A runnable Go project starts with a thin CLI entrypoint and testable internal logic.
- **Functionality:**
  - Initialize the Go module.
  - Add `cmd/agent-harness/main.go`.
  - Add a small internal command/app package instead of putting logic directly in `main.go`.
  - Add `.gitignore`.
  - Add initial `README.md` with project purpose and run/test commands.
- **Tests:**
  - Smoke test command/app construction.
  - Verify default invocation returns expected placeholder output.
- **Verification:**
  - `go test ./...`
  - `go run ./cmd/agent-harness`

### Step 02 — `ask` command skeleton

- **Status:** Completed
- **Branch:** `step-02-ask-command-skeleton`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/2
- **Concept:** Separate CLI input handling from model/agent execution.
- **Functionality:**
  - Support `agent-harness ask "What is an agent?"`.
  - Echo or structure the prompt without calling a model yet.
  - Return helpful errors for missing prompts or unknown commands.
- **Tests:**
  - `ask` requires a prompt.
  - `ask` accepts a prompt.
  - Output contains the provided prompt.
  - Unknown command returns a helpful error.
- **Verification:**
  - `go test ./...`
  - `go run ./cmd/agent-harness ask "What is an agent?"`

### Step 03 — Configuration loading

- **Status:** Completed
- **Branch:** `step-03-configuration-loading`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/3
- **Concept:** Agent behavior depends on explicit model/provider configuration.
- **Functionality:**
  - Add config package.
  - Read model settings from process environment variables and an optional local `.env` file:
    - `OPENAI_API_KEY`
    - `OPENAI_BASE_URL`
    - `OPENAI_MODEL`
  - Keep `.env` ignored by git and commit `.env.example` with placeholders only.
  - Let process environment variables override `.env` values.
  - Fail clearly when required config is missing.
- **Tests:**
  - Missing API key returns a clear error.
  - Defaults are applied for base URL/model if defaults are chosen.
  - Explicit env values override defaults.
  - Local `.env` values are loaded when process environment variables are absent.
  - Process environment variables override local `.env` values.
  - API key is not printed or exposed in normal config output.
- **Verification:**
  - `go test ./...`

### Step 04 — LLM client interface

- **Status:** Completed
- **Branch:** `step-04-llm-client-interface`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/4
- **Concept:** The agent runtime should depend on an interface, not a specific provider implementation.
- **Functionality:**
  - Add `internal/llm` package.
  - Define message types for `system`, `user`, `assistant`, and later `tool` messages.
  - Define a minimal chat client interface.
  - Add a fake client for tests.
- **Tests:**
  - Message roles/types are represented consistently.
  - Request construction preserves message ordering.
  - Request construction copies messages to avoid caller mutation surprises.
  - Fake client satisfies the interface and can return deterministic responses.
  - Fake client can return configured errors.
- **Verification:**
  - `go test ./...`

### Step 05 — OpenAI-compatible chat call

- **Status:** Completed
- **Branch:** `step-05-openai-compatible-chat-call`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/5
- **Concept:** A non-tool agent begins as a structured chat request to a model API.
- **Functionality:**
  - Implement an OpenAI-compatible chat/completions client.
  - Wire `ask` to send system + user messages.
  - Print the assistant response.
  - Keep real API calls out of unit tests.
- **Tests:**
  - HTTP request payload is correct using `httptest`.
  - Response parsing handles assistant text.
  - API errors return useful Go errors.
  - No test requires a real API key or external network.
- **Verification:**
  - `go test ./...`
  - Optional manual smoke test with real env vars.

### Step 06 — Agent message orchestration

- **Status:** Completed
- **Branch:** `step-06-agent-message-orchestration`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/6
- **Concept:** Agents maintain state by appending messages to a conversation history.
- **Functionality:**
  - Add `internal/agent` package.
  - Move orchestration out of the CLI layer.
  - Build explicit message history: system -> user -> assistant.
  - Add a debug-friendly result object if useful.
- **Tests:**
  - Initial messages include system prompt then user prompt.
  - Agent returns assistant response from a fake LLM.
  - Model errors bubble up clearly.
- **Verification:**
  - `go test ./...`
  - Existing `ask` command still works.

### Step 07 — Tool registry abstraction

- **Status:** Completed
- **Branch:** `step-07-tool-registry-abstraction`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/7
- **Concept:** Tools are named, schema-described functions that the harness controls.
- **Functionality:**
  - Add `Tool` interface.
  - Add registry for registering and looking up tools by name.
  - Expose tool metadata and JSON schema.
- **Tests:**
  - Register a tool.
  - Reject duplicate tool names.
  - Lookup known tool.
  - Unknown tool returns helpful error.
  - Schema metadata is exposed correctly.
- **Verification:**
  - `go test ./...`

### Step 08 — Echo demo tool

- **Status:** Completed
- **Branch:** `step-08-echo-demo-tool`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/8
- **Concept:** Tool execution should be independently testable before involving the LLM.
- **Functionality:**
  - Add an `echo` tool that accepts a JSON message and returns it.
- **Tests:**
  - Valid args return expected echo output.
  - Invalid JSON returns a clear error.
  - Missing required field returns a clear error.
  - Tool schema declares the required field.
- **Verification:**
  - `go test ./...`

### Step 09 — Manual tool debug command

- **Status:** Completed
- **Branch:** `step-09-manual-tool-debug-command`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/9
- **Concept:** The harness should be able to execute registered tools directly before the LLM drives them.
- **Functionality:**
  - Add `agent-harness tool <tool-name> <json-args>`.
  - Build the same built-in tool registry that the agent loop will use later.
  - Look up the requested tool by name and execute it with raw JSON args.
  - Print successful tool output to stdout.
  - Print controlled errors for unknown tools, missing args, invalid JSON, and tool execution failures.
  - Keep this path LLM-free so tools can be debugged without model calls or API keys.
- **Tests:**
  - `tool echo {"message":"hi"}` prints `hi`.
  - Missing tool name returns a clear usage error.
  - Unknown tool returns a controlled error.
  - Invalid JSON returns a clear error before or during tool execution.
  - Tool execution errors return a clear non-zero CLI result.
- **Verification:**
  - `go test ./...`
  - Manual example: `agent-harness tool echo '{"message":"hello from tool debug"}'`

### Step 10 — Tool-call response parsing

- **Status:** Completed
- **Branch:** `step-10-tool-call-response-parsing`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/10
- **Concept:** The model requests tools; the harness parses those requests and remains in control.
- **Functionality:**
  - Parse OpenAI-compatible tool-call responses.
  - Distinguish final assistant text from tool calls.
  - Support one or more tool calls in a response.
- **Tests:**
  - Final-answer response parses correctly.
  - Single tool call parses correctly.
  - Multiple tool calls parse correctly.
  - Malformed tool args are handled safely.
  - Empty response returns a useful error.
- **Verification:**
  - `go test ./...`

### Step 11 — First agent loop with tool execution

- **Status:** Completed
- **Branch:** `step-11-agent-loop-tool-execution`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/11
- **Concept:** The core agent loop is model call -> tool execution -> observation -> repeat -> final answer.
- **Functionality:**
  - Send messages to the model.
  - Execute requested tools.
  - Append tool results as tool messages.
  - Repeat until final answer or max step limit.
  - Add a default max step limit, such as 8.
- **Tests:**
  - Final answer without tool calls returns immediately.
  - Requested tool executes and result is sent back to the fake model.
  - Max step limit prevents infinite loops.
  - Unknown tool returns a controlled error.
  - Tool execution error is handled consistently.
- **Verification:**
  - `go test ./...`

### Step 12 — Interactive chat mode

- **Status:** Completed
- **Branch:** `step-12-interactive-chat-mode`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/12
- **Concept:** Interactive agents preserve conversation history across user turns, while each turn can still use the existing agent/tool loop internally.
- **Functionality:**
  - Add `agent-harness chat`.
  - Read user input in a prompt loop, such as `You:`, and print assistant replies, such as `Agent:`.
  - Maintain in-memory message history for the lifetime of the chat process.
  - Reuse the same agent runtime/tool registry used by `ask` so each chat turn can still run tool loops before producing a final answer.
  - Support `/exit`, `exit`, `quit`, EOF, and interrupt-friendly shutdown.
  - Keep the first version in-memory only; persisted named sessions can be a later step.
- **Tests:**
  - Chat command starts a prompt loop with injected input/output streams.
  - A second user turn includes earlier user/assistant context in the fake model request.
  - `exit`, `quit`, `/exit`, and EOF end the session cleanly.
  - Model errors print a controlled error and allow the session to continue or exit without corrupting history.
  - Tool calls still execute inside a chat turn through the existing registry path.
- **Verification:**
  - `go test ./...`
  - Manual example: `agent-harness chat`, then ask `What is an agent?` followed by `Can you give a simpler example?`

### Step 13 — Workspace-safe file tools

- **Status:** Completed
- **Branch:** `step-13-workspace-file-tools`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/13
- **Concept:** Useful tools need sandboxing, output limits, and predictable errors.
- **Functionality:**
  - Add `list_files(path)`.
  - Add `read_file(path)`.
  - Add `search_files(query, path)`.
  - Restrict all paths to a configured workspace root.
  - Cap tool output size.
- **Tests:**
  - Can list files inside workspace.
  - Can read file inside workspace.
  - Rejects `../outside.txt`.
  - Rejects absolute paths outside workspace.
  - Search finds matching files.
  - Output truncation works.
  - Missing file returns useful error.
- **Verification:**
  - `go test ./...`
  - Manual example: `agent-harness ask "What files are in this project?"`

### Step 14 — Context window management

- **Status:** Completed
- **Branch:** `step-14-context-window-management`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/14
- **Concept:** Stored run history and model context are different; agents need token-aware rules for deciding what gets sent back to the model.
- **Functionality:**
  - Add a context manager that builds the next model request from full run history.
  - Always preserve the system prompt, original user goal, and latest user message.
  - Keep the most recent messages needed for continuity.
  - Enforce configurable message/tool-output character limits before calling the model.
  - Add an LLM-backed running summary for older messages that are compacted out of the raw model context.
  - Configure the summary model separately from the main model with `AGENT_SUMMARY_MODEL`, defaulting to `gpt-4.1-mini`.
  - Configure `AGENT_MAX_SUMMARY_CHARS` for the inserted summary and `AGENT_SUMMARY_MAX_INPUT_MESSAGES` for per-summary-call batching; default summary input batch size is `10` with no app-enforced upper cap beyond requiring a positive integer.
  - In interactive chat, proactively start a background goroutine after each response when the next user turn is likely to require summarization; if the next user message arrives before the job completes, wait for the prepared summary before the next main LLM call.
  - Return a clear error instead of silently truncating the latest user prompt when it exceeds `AGENT_MAX_MESSAGE_CHARS`.
  - Log every remaining hard truncation, including tool-result and inserted-summary truncation, as an explicit context warning with kept, omitted, and original character counts.
  - Keep full raw history separate from model-facing summarized context.
- **Tests:**
  - Short histories are sent unchanged.
  - System prompt, original user goal, and latest user message are preserved when trimming is required.
  - Recent assistant/tool messages are kept preferentially.
  - Older compacted messages are incorporated into a running summary via the configured summary model.
  - Summary updates are batched by `AGENT_SUMMARY_MAX_INPUT_MESSAGES`.
  - Background summary jobs are started only when the next chat turn is likely to exceed limits.
  - Oversized latest user prompts return a clear error instead of being silently truncated.
  - Oversized tool results are truncated with an explicit notice.
  - Summary and tool-result truncation events are logged clearly enough to monitor frequency and tune limits later.
  - The context manager reports omitted, summarized, and truncated context for future trace/logging.
- **Verification:**
  - `go test ./...`

### Step 15 — Trace output for agent steps

- **Status:** Completed
- **Branch:** `step-15-trace-output`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/15
- **Concept:** Agent systems need observability to be understandable and debuggable, including visibility into context compaction, running-summary, and hard-truncation decisions.
- **Functionality:**
  - Add `--trace` flag.
  - Show model calls, tool calls, tool result summaries, errors, and final answer.
  - Render `ContextReport` details before model calls when trace is enabled, including input/output message counts, max message limits, omitted message counts, summarized message counts, whether a summary was inserted, summary model update counts, and any truncation records.
  - Show summary-specific trace details, including which summary model is configured and whether interactive chat used an already-prepared background summary or waited for a pending summary job.
  - Keep the always-visible Step 14 `context warning` output for hard truncation, but make trace output the richer explanation of why the truncation happened.
  - Avoid printing secrets or full oversized content in trace output.
- **Tests:**
  - Trace disabled by default.
  - Trace records model step.
  - Trace records tool call.
  - Trace records tool result summary.
  - Trace records context compaction reports from `ContextReport`.
  - Trace records hard truncation details without dumping full truncated content.
  - Trace records summary/background-summary events.
  - Trace avoids leaking API key.
- **Verification:**
  - `go test ./...`
  - Manual example: `agent-harness ask --trace "Summarize this repo"`

### Step 16 — Run/session logging

- **Status:** Completed
- **Branch:** `step-16-run-session-logging`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/16
- **Concept:** Agent runs should be inspectable after the fact without forcing every stored detail back into model context.
- **Functionality:**
  - Add durable structured run/session logs using Go's standard `log/slog` or a small app-specific JSONL event writer built around `slog`-style structured events.
  - Save full run logs as JSON/JSONL under a local app directory.
  - Include prompt, stored messages, model-context snapshots, `ContextReport` data, context compaction notices, truncation warnings, summary updates, tool calls, final answer, and errors.
  - Persist hard truncation events as machine-readable records so frequency can be measured over time, with fields for role/source, message index, kept characters, omitted characters, and original characters.
  - Keep trace output human-facing and opt-in; keep run/session logging durable and structured.
  - Redact secrets.
- **Tests:**
  - Run log file is created.
  - Log JSON has expected fields.
  - Context compaction and truncation events are written as structured log records.
  - Failed run logs an error.
  - Secrets are redacted.
  - Logging can be disabled if needed.
- **Verification:**
  - `go test ./...`

### Step 17 — Gated command execution tool

- **Status:** Completed
- **Branch:** `step-17-gated-command-tool`
- **Pull Request:** https://github.com/zachfire9/agent-harness/pull/17
- **Concept:** Dangerous tools require policy, confirmation, and timeouts.
- **Functionality:**
  - Add `run_command(command)` tool.
  - Add a small `AllowedCommand` descriptor for future custom/local commands, including an optional description.
  - Expose the allowed commands to the model as a JSON-schema enum so it can choose valid commands deliberately.
  - Include per-command descriptions in the schema when provided, while still allowing common commands with no custom description.
  - Start with an allowlist of safe commands such as:
    - `go test ./...`
    - `git status`
    - `git diff`
    - `pwd`
  - Require confirmation before execution.
  - Enforce a timeout.
  - Capture stdout/stderr safely.
- **Tests:**
  - Allowed command executes.
  - Blocked command is rejected.
  - Timeout is enforced.
  - Denied confirmation does not execute.
  - Command schema exposes the allowlist as an enum.
  - Optional descriptions for custom commands are included in model-facing schema.
  - Agent-facing code can still omit descriptions for obvious/common commands.
  - stdout/stderr are captured safely.
- **Verification:**
  - `go test ./...`

### Step 18 — Vector store abstraction for future RAG support

- **Status:** Completed
- **Branch:** `step-18-vector-store-abstraction`
- **Pull Request:** TBD
- **Concept:** RAG-ready applications isolate vector retrieval behind an interface before committing to a specific vector database provider.
- **Functionality:**
  - Add an `internal/vectorstore` package with a small provider-neutral interface for future document upserts and similarity search.
  - Add a no-op or in-memory implementation used by default so the first app iteration does not require a real vector database.
  - Add a small factory that selects the implementation from config, starting with `none` and failing clearly for unsupported providers.
  - Add future-facing config placeholders such as `VECTOR_STORE_PROVIDER=none` without requiring credentials or network access.
  - Document future provider candidates such as pgvector, Qdrant, Pinecone, Weaviate, and Chroma without adding their SDKs yet.
- **Tests:**
  - Default config selects the no-op/in-memory vector store.
  - Unsupported providers return a controlled error.
  - Agent-facing code can depend on the vector store interface instead of a concrete vendor.
  - No test requires a real vector database or external network.
- **Verification:**
  - `go test ./...`

### Step 19 — Provider/config polish

- **Status:** Pending
- **Branch:** `step-19-provider-config-polish`
- **Pull Request:** TBD
- **Concept:** Provider flexibility should be explicit and easy to verify.
- **Functionality:**
  - Add `config check` command.
  - Normalize base URL handling.
  - Improve defaults and error messages.
  - Show active model in trace/config output without exposing the API key.
  - Show configured context/message limits without exposing secrets.
  - Show configured vector store provider without requiring vector DB credentials when the provider is `none`.
- **Tests:**
  - Config check succeeds with valid config.
  - Config check fails clearly with invalid config.
  - Base URL normalization works.
  - Output shows model name but not API key.
  - Output shows context limits.
  - Output shows vector store provider.
- **Verification:**
  - `go test ./...`
  - `agent-harness config check`

### Step 20 — Learning walkthrough documentation

- **Status:** Pending
- **Branch:** `step-20-learning-walkthrough-docs`
- **Pull Request:** TBD
- **Concept:** The repo should be both a working app and a learning artifact.
- **Functionality:**
  - Expand README with a walkthrough of the completed stages.
  - Explain the core architecture.
  - Explain tool-calling flow.
  - Explain message history vs model context.
  - Explain test strategy and how to run examples.
- **Tests/verification:**
  - `go test ./...`
  - Manually verify documented commands still match the app.
- **Verification:**
  - README is accurate and current.

## Deferred ideas

These are intentionally not part of the first learning sequence:

- Web UI.
- Telegram bot interface.
- Scheduler/cron jobs.
- GitHub issue/PR tools.
- Multi-agent delegation.
- Long-term memory.
- Browser/web search tools.

They are good follow-up milestones once the basic local CLI agent harness is understandable and reviewable.

