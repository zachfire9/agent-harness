# Agent Harness Learning Implementation Plan

> **Purpose:** Build a small Go-based AI agent harness one reviewable step at a time. Each step should teach one agent/app concept, include deterministic unit tests, and land through its own pull request after this initial plan commit.

## Working agreement

- This plan is the source of truth for the staged build.
- After this initial plan commit, every implementation step happens on its own branch.
- Branch naming format: `step-NN-short-description`, for example `step-01-go-cli-foundation`.
- Every step gets a GitHub pull request before it is merged into `main`.
- Each PR should explain:
  - what concept the step teaches
  - what code changed
  - what tests were added
  - how it was verified
- Zach reviews/approves/merges PRs before the next step is based on `main`.
- After a PR is created, add the PR link to that step in this plan.
- After a PR is merged, mark the step completed in this plan.
- Updating the plan status/PR links should be part of the relevant step branch/PR unless Zach asks for a separate docs-only update.

## Testing standard

Every behavior introduced by a commit gets a deterministic unit test.

- Unit tests should not call real LLM APIs.
- LLM/network behavior should be tested with fakes or `httptest`.
- Real model calls are manual smoke tests only.
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

- **Status:** In review
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

- **Status:** Pending
- **Branch:** `step-02-ask-command-skeleton`
- **Pull Request:** TBD
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

- **Status:** Pending
- **Branch:** `step-03-configuration-loading`
- **Pull Request:** TBD
- **Concept:** Agent behavior depends on explicit model/provider configuration.
- **Functionality:**
  - Add config package.
  - Read model settings from environment variables:
    - `OPENAI_API_KEY`
    - `OPENAI_BASE_URL`
    - `OPENAI_MODEL`
  - Add `.env.example`.
  - Fail clearly when required config is missing.
- **Tests:**
  - Missing API key returns a clear error.
  - Defaults are applied for base URL/model if defaults are chosen.
  - Explicit env values override defaults.
  - API key is not printed or exposed in normal config output.
- **Verification:**
  - `go test ./...`

### Step 04 — LLM client interface

- **Status:** Pending
- **Branch:** `step-04-llm-client-interface`
- **Pull Request:** TBD
- **Concept:** The agent runtime should depend on an interface, not a specific provider implementation.
- **Functionality:**
  - Add `internal/llm` package.
  - Define message types for `system`, `user`, `assistant`, and later `tool` messages.
  - Define a minimal chat client interface.
  - Add a fake client for tests.
- **Tests:**
  - Message roles/types are represented consistently.
  - Request construction preserves message ordering.
  - Fake client satisfies the interface and can return deterministic responses.
- **Verification:**
  - `go test ./...`

### Step 05 — OpenAI-compatible chat call

- **Status:** Pending
- **Branch:** `step-05-openai-compatible-chat-call`
- **Pull Request:** TBD
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

- **Status:** Pending
- **Branch:** `step-06-agent-message-orchestration`
- **Pull Request:** TBD
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

- **Status:** Pending
- **Branch:** `step-07-tool-registry-abstraction`
- **Pull Request:** TBD
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

- **Status:** Pending
- **Branch:** `step-08-echo-demo-tool`
- **Pull Request:** TBD
- **Concept:** Tool execution should be independently testable before involving the LLM.
- **Functionality:**
  - Add an `echo` tool that accepts a JSON message and returns it.
  - Optionally add a direct internal/CLI execution path for debugging tools.
- **Tests:**
  - Valid args return expected echo output.
  - Invalid JSON returns a clear error.
  - Missing required field returns a clear error.
  - Tool schema declares the required field.
- **Verification:**
  - `go test ./...`

### Step 09 — Tool-call response parsing

- **Status:** Pending
- **Branch:** `step-09-tool-call-response-parsing`
- **Pull Request:** TBD
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

### Step 10 — First agent loop with tool execution

- **Status:** Pending
- **Branch:** `step-10-agent-loop-tool-execution`
- **Pull Request:** TBD
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

### Step 11 — Workspace-safe file tools

- **Status:** Pending
- **Branch:** `step-11-workspace-file-tools`
- **Pull Request:** TBD
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

### Step 12 — Trace output for agent steps

- **Status:** Pending
- **Branch:** `step-12-trace-output`
- **Pull Request:** TBD
- **Concept:** Agent systems need observability to be understandable and debuggable.
- **Functionality:**
  - Add `--trace` flag.
  - Show model calls, tool calls, tool result summaries, errors, and final answer.
  - Avoid printing secrets.
- **Tests:**
  - Trace disabled by default.
  - Trace records model step.
  - Trace records tool call.
  - Trace records tool result summary.
  - Trace avoids leaking API key.
- **Verification:**
  - `go test ./...`
  - Manual example: `agent-harness ask --trace "Summarize this repo"`

### Step 13 — Run/session logging

- **Status:** Pending
- **Branch:** `step-13-run-session-logging`
- **Pull Request:** TBD
- **Concept:** Agent runs should be inspectable after the fact.
- **Functionality:**
  - Save run logs as JSON under a local app directory.
  - Include prompt, messages, tool calls, final answer, and errors.
  - Redact secrets.
- **Tests:**
  - Run log file is created.
  - Log JSON has expected fields.
  - Failed run logs an error.
  - Secrets are redacted.
  - Logging can be disabled if needed.
- **Verification:**
  - `go test ./...`

### Step 14 — Interactive chat mode

- **Status:** Pending
- **Branch:** `step-14-interactive-chat-mode`
- **Pull Request:** TBD
- **Concept:** A different interface can reuse the same agent runtime.
- **Functionality:**
  - Add `agent-harness chat`.
  - Maintain message history across turns.
  - Support `exit` and `quit`.
- **Tests:**
  - Chat session appends user turns.
  - `exit`/`quit` ends session.
  - Model errors do not corrupt history.
  - Multi-turn fake model test proves previous context is preserved.
- **Verification:**
  - `go test ./...`

### Step 15 — Gated command execution tool

- **Status:** Pending
- **Branch:** `step-15-gated-command-tool`
- **Pull Request:** TBD
- **Concept:** Dangerous tools require policy, confirmation, and timeouts.
- **Functionality:**
  - Add `run_command(command)` tool.
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
  - stdout/stderr are captured safely.
- **Verification:**
  - `go test ./...`

### Step 16 — Provider/config polish

- **Status:** Pending
- **Branch:** `step-16-provider-config-polish`
- **Pull Request:** TBD
- **Concept:** Provider flexibility should be explicit and easy to verify.
- **Functionality:**
  - Add `config check` command.
  - Normalize base URL handling.
  - Improve defaults and error messages.
  - Show active model in trace/config output without exposing the API key.
- **Tests:**
  - Config check succeeds with valid config.
  - Config check fails clearly with invalid config.
  - Base URL normalization works.
  - Output shows model name but not API key.
- **Verification:**
  - `go test ./...`
  - `agent-harness config check`

### Step 17 — Learning walkthrough documentation

- **Status:** Pending
- **Branch:** `step-17-learning-walkthrough-docs`
- **Pull Request:** TBD
- **Concept:** The repo should be both a working app and a learning artifact.
- **Functionality:**
  - Expand README with a walkthrough of the completed stages.
  - Explain the core architecture.
  - Explain tool-calling flow.
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
