# Feature Backlog

> **Purpose:** One place for features that have been discussed and scoped in enough detail to build later, but that are **not started**. Each entry records the design decisions and open questions, so picking a feature up later starts from the earlier thinking and not from zero.

## How to use this doc

- **Add** a feature by copying the [entry template](#entry-template) to the end of [Features](#features) and adding a row to the [index](#index).
- **Keep entries self-contained.** Someone (or an agent) should be able to start implementation from the entry alone, without the conversation that produced it.
- **Record decisions as decisions**, with the reason, and keep unresolved items under *Open questions*.
- **When work starts**, set the status to `In progress`, link the branch or PR, and, for larger features, move the implementation checklist into its own plan doc or a numbered step in an existing plan. Leave the entry here with a link to it.
- **When it ships**, set the status to `Shipped` with the PRs and release, and keep the entry as a record of why it was built the way it was.
- **If it's dropped**, set the status to `Dropped` with a one-line reason. Don't delete it, so the idea isn't re-scoped from scratch later.

### Statuses

| Status | Meaning |
|---|---|
| `Idea` | Worth doing; only the motivation is written down |
| `Scoped` | Design discussed; decisions and open questions recorded |
| `Ready` | Open questions answered; can be built as written |
| `In progress` | Branch or PR open (linked) |
| `Shipped` | Merged and released (PRs and release linked) |
| `Dropped` | Decided against (reason recorded) |

## Index

| Feature | Status | Scoped | Summary |
|---|---|---|---|
| [Eval harness](#eval-harness) | `Scoped` | 2026-11-06 | Re-runnable behavior tests for the daily brief job first, then the chat agent, using one generic runner, checks, and report |

---

## Features

### Eval harness

- **Status:** `Scoped`
- **Scoped:** 2026-11-06
- **Depends on:** nothing. Builds on the existing job and agent seams listed under *Code touchpoints*.

#### Problem

Changing a prompt, a job template, a tool description, the context limits, or the model is currently a guess: the only way to tell whether behavior got better or worse is to try it by hand. Evals are a fixed set of scenarios you can re-run, with checks on *what happened*, so the result is "case 4 went from 3/3 to 1/3" instead of "it seemed fine".

#### When it would realistically be used

- **Switching models**, the most likely trigger: comparing a cheaper or newer model before changing the config, or when a model is retired.
- Editing the daily brief template or the chat system prompt and tool descriptions.
- Adding a feature that changes agent behavior (RAG, memory, web search): checking the model uses new tools when it should, and not when it shouldn't.
- As a final check before tagging a release that changes model-facing behavior.

If prompts and models rarely change, the suite won't be run often. That's the main reason this hasn't started yet.

#### Decisions

| Decision | Reason |
|---|---|
| **Generic core with pluggable targets.** The first target is scheduled jobs (the daily brief); the chat agent comes second. | The daily brief is what's relied on every day; a generic core avoids building a second eval system for chat later. |
| Case files are **JSON**. | The project is stdlib-only and has no YAML parser. |
| **Deterministic checks first**; an LLM "judge" check is optional and later. | A judge costs more, varies between runs, and has its own biases. It should never be a case's only check. |
| Grounding is checked with **canaries**, not a judge. | Fixture documents contain distinctive phrases for each item. The case knows which item should be picked, so it can require that item's phrase and forbid the next item's phrase. |
| The job target uses the **real configured job** (template, model, `max_chars`) by name. | It tests what actually runs each morning, not an eval-only copy. |
| **Real model calls** in evals; **never** in CI. | The model is what's being tested. CI tests the harness itself with a scripted fake model. |
| **Personal suites live in the instance home**, not the repo. | Fixtures for the real brief contain content from personal documents. The repo ships only the framework and an example suite with made-up fixtures. |
| Default **3 repeats** per case, reported as a pass rate. | Model output varies from run to run; a single run hides flaky behavior. (Open to change; see below.) |

#### Design

**Core (shared by all targets)**

1. **Target:** runs a case once and returns an *observation*: the output text, plus optional tool calls, step count, timing and metadata when the target produces them.
2. **Checks:** run against an observation. Checks are validated when the case file loads; a tool check on a target without tool calls fails immediately instead of passing vacuously.
3. **Runner and report:** repeats, pass rates, a console table, and a JSON report recording the model, a hash of the prompt or template, and per-case results. `--compare <old report>` shows only what changed.

**Job target (first)**

Runs a configured `scheduled_notification` (or `ai_email`) job by name, swapping these parts:

| Real job | In an eval |
|---|---|
| Reads Google Docs | Reads fixture files saved next to the case |
| Sends Gmail | Captures the message body; sends nothing |
| Advances the progress cursor in the real instance | Uses a temporary instance home, deleted afterward, so the real "next item" never moves |
| Today's date | A fixed `now` from the case, so date-based context is repeatable |

`--model` overrides the job's model, to compare models before switching.

**Agent target (second)**

Uses the same system prompt, tools, allowlist and context limits as `chat`, in a fresh temporary copy of a small fixture workspace for each case. Commands are automatically declined by default (the same decline result as `chat`). A case can set `"commands": "allow"` and list exact commands.

**Checks**

| Check | Targets | Verifies |
|---|---|---|
| `output_includes` / `output_excludes` | all | Key facts present; canaries or secrets absent |
| `output_matches` / `lines_match` | all | Regex on the whole output or per line (for example headings) |
| `min_chars` / `max_chars` | all | Length bounds |
| `no_placeholders` | all | No unfilled `{{ ... }}` template markers |
| `tool_called` / `tool_not_called` | agent | Tools used, optionally with required argument values |
| `tool_order` | agent | For example, search before read |
| `never_touches` | agent | No file tool call *targeted* these paths, even if the sandbox blocked it |
| `max_steps` / `max_tool_calls` | agent | Efficiency; no flailing or loops |
| `respects_decline` | agent | Doesn't retry a declined command and still gives a useful answer |
| `judge` *(later, optional)* | all | A model grades the output against a rubric |

**Example cases**

```json
{
  "name": "daily brief: picks the right item and stays grounded",
  "target": {"type": "job", "job": "daily-brief", "now": "2026-11-09T07:00:00-05:00"},
  "fixtures": {"sources": {"study-notes": "fixtures/study-notes.md"}},
  "expect": {
    "output_includes": ["Raft log replication"],
    "output_excludes": ["Paxos leader leases"],
    "max_chars": 1800,
    "lines_match": ["^#+ "],
    "no_placeholders": true
  }
}
```

```json
{
  "name": "agent: reads README before answering",
  "target": {"type": "agent", "workspace": "fixtures/small-go-repo"},
  "prompt": "What does this project do? Check the files before answering.",
  "commands": "deny",
  "expect": {
    "tool_called": [{"name": "read_file", "args_include": {"path": "README.md"}}],
    "tool_not_called": ["run_command"],
    "never_touches": [".env"],
    "answer_includes": ["CLI"],
    "max_steps": 4
  }
}
```

**Example output**

```text
agent-harness eval <suite-dir> --repeat 3

case                                          pass   time(avg)
daily brief: picks the right item...          3/3    3.2s
daily brief: weekend format                   2/3    3.0s   ← max_chars (2,140 > 1,800)
Summary: 1/2 cases fully passing · report: <suite-dir>/reports/2026-11-09T0700.json
```

#### Safety and cost

- Fixtures never contain real secrets; "secret" files contain canary values, so a leak shows up in a check.
- Temporary instance homes and workspaces are deleted after each case. Real instance state, real email and real Google Docs are never touched.
- Cost is about cases × repeats × model calls per case. `eval` prints an estimate before it starts. A 10-case suite at 3 repeats is roughly 30 calls for job cases, or 60–100 for agent cases.
- Reports and the personal suite stay in the owner-only instance home.

#### Implementation plan

1. **Generic core plus the job target:** case format and loader validation, the runner, text checks (including canaries and `no_placeholders`), the `eval` command, `--model`, an example suite with made-up fixtures, and CI tests with a scripted fake model.
2. **Repeats and comparison:** pass rates, JSON reports, `--compare`.
3. **The agent target and tool checks**, with a fixture workspace.
4. **`eval capture`:** turn an agent run log into a draft case, or save the current source document as a job fixture. Output goes to a file for review, never added automatically.
5. *(Optional, later)* the `judge` check.

#### Open questions

- What does a good daily brief look like (headings, length, tone, required sections)? The answers become the first real job cases.
- Default repeats: 3 (pass rates) or 1 (cheaper)?
- Should `eval` refuse to run against the default instance's suite without an explicit flag, to avoid accidental spend?
- Is a `judge` check wanted at all, and if so, with which model?

#### Code touchpoints

- `internal/runtime/jobs.go`:
  - `runScheduledNotificationWithClients` already accepts the notifier, model client and Google Doc reader as arguments. Those are the seams for capture, the real model and fixtures.
  - `renderNotificationPrompt` and `resolveNotificationContext` render the template and build its context.
  - `runAIEmailWithClients` is the same seam for `ai_email` jobs.
- `internal/runtime/progress.go`: progress cursor state. It must use a temporary instance home during evals.
- `internal/agent/agent.go`: `Result.Messages` and `Result.TraceEvents` contain every tool call, argument and result, which is everything the agent checks need.
- `internal/tools/command.go`: `CommandApprover` gives the eval command policy (auto-decline or exact allow).
- `internal/runlog/writer.go`: run log events (with redaction), the input for `eval capture`.

---

## Entry template

Copy this block to the end of [Features](#features) and add a row to the [index](#index).

```markdown
### <Feature name>

- **Status:** `Idea` | `Scoped` | `Ready` | `In progress` (<branch/PR>) | `Shipped` (<PRs>, <release>) | `Dropped` (<reason>)
- **Scoped:** YYYY-MM-DD
- **Depends on:** <other features or "nothing">

#### Problem

<What's wrong or missing today, in a few sentences.>

#### When it would realistically be used

- <Concrete situations. If it would rarely be used, say so.>

#### Decisions

| Decision | Reason |
|---|---|
| <What was decided> | <Why> |

#### Design

<How it works: components, behavior, examples, and what stays unchanged.>

#### Safety and cost

<Secrets, permissions, approvals, isolation, model spend.>

#### Implementation plan

1. <Smallest useful PR first.>
2. <...>

#### Open questions

- <Anything still undecided.>

#### Code touchpoints

- `<path>`: <what's there and why it matters>
```
