# Agent Harness

A small Go-based AI agent harness built one reviewable learning step at a time.

The goal of this repo is to make each agent concept understandable through small commits and pull requests. Every implementation step should include deterministic unit tests before code lands on `main`.

## Current status

The project currently has:

- Go module and thin CLI entrypoint at `cmd/agent-harness/main.go`
- testable CLI package under `internal/cli`
- `ask` command for sending prompts to a configured model
- config loading from process environment variables or a local `.env` file
- minimal LLM package under `internal/llm` with message types, chat client interface, fake client, and OpenAI-compatible chat/completions client
- `internal/agent` runner that builds system/user/assistant message history for one model turn
- `internal/tools` registry for named, schema-described tools
- `echo` demo tool for deterministic tool-execution tests
- `tool` debug command for manually executing registered tools without an LLM/API call
- `ask` wired through the agent runner to print the assistant response

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

Expected output is the assistant response from your configured OpenAI-compatible model, for example:

```text
An agent is a program that uses a model to decide what to do next, optionally call tools, and continue until it can return a final answer.
```

If config is missing, the command fails before making a model call and prints a helpful config error.

Debug a registered tool without making a model/API call:

```powershell
go run ./cmd/agent-harness tool echo '{"message":"hello from tool debug"}'
```

Expected output:

```text
hello from tool debug
```

This path executes the same registered tool implementation that the agent loop will use later, but it bypasses the LLM so tool behavior can be tested directly.

## Configuration

Step 03 adds configuration loading for future model calls. The app loads config from:

1. Process environment variables, when they are set.
2. A local `.env` file in the current working directory.
3. Built-in defaults for optional values.

Process environment variables take precedence over values in `.env`. Only `OPENAI_API_KEY` is required. `OPENAI_BASE_URL` and `OPENAI_MODEL` have defaults.

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
