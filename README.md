# Agent Harness

A small Go-based AI agent harness built one reviewable learning step at a time.

The goal of this repo is to make each agent concept understandable through small commits and pull requests. Every implementation step should include deterministic unit tests before code lands on `main`.

## Current status

Step 01 establishes the project foundation:

- Go module
- thin CLI entrypoint at `cmd/agent-harness/main.go`
- testable CLI package under `internal/cli`
- initial test coverage for default invocation

## Run

```powershell
go run ./cmd/agent-harness
```

Expected output:

```text
agent-harness: staged learning CLI ready
```

Ask command skeleton:

```powershell
go run ./cmd/agent-harness ask "What is an agent?"
```

Expected output:

```text
Prompt: What is an agent?
```

This step only structures the prompt locally; it does not call an LLM yet.

## Test

```powershell
go test ./...
```

## Plan

See [`docs/plans/agent-harness-learning-plan.md`](docs/plans/agent-harness-learning-plan.md) for the staged implementation plan.
