# Daily Brief Integration Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task after Zach approves the phase breakdown.

**Goal:** Add a local-first daily brief workflow to `agent-harness` that can reproduce the current Hermes daily brief, starting with a minimal local MVP and building toward richer sources, delivery, scheduling, and optional remote deployment.

**Architecture:** Keep the daily brief deterministic and inspectable rather than fully agentic at first. Add a dedicated `daily-brief` command that collects sections, renders a message, persists rotation state, and later hands the message to a delivery adapter. Start by running on the same machine as Hermes/agent-harness; make storage and delivery interfaces swappable so a later AWS Lambda/EventBridge/SES deployment is possible without rewriting the brief logic.

**Tech Stack:** Go CLI, local JSON state, deterministic unit tests, injectable clock/HTTP/source/delivery interfaces, optional SMTP/email delivery, optional Google Docs API integration, optional external cron/systemd/GitHub Actions/AWS scheduling.

---

## Current baseline

`agent-harness` currently has:

- OpenAI-compatible model config and `ask`/`chat` commands.
- An agent runner with tool calling, context management, trace output, and JSONL run logs.
- Local workspace tools and a gated command tool.
- A future milestone list that already includes scheduler, Telegram, long-term memory, and browser/web tools.

The live Hermes daily brief currently depends on functionality that is not yet in `agent-harness`:

- Scheduled weekday/weekend execution.
- Persistent rotating state for cards and cached lookups.
- Google Docs ingestion for Vocabulary and Elements.
- Dictionary/example lookup for vocabulary.
- Wikipedia-based element extra facts and year-in-review awards data.
- Date rendering in Spanish, Italian, French, and German.
- Delivery to Telegram.

## Implementation principles

- **Local-first:** The first working version should run on the same Linux machine where this agent is running, from the repo checkout or an installed binary.
- **No remote dependency for MVP:** Do not require AWS, Docker, a database, Telegram, or Google Docs for the first useful version.
- **Deterministic before agentic:** Use deterministic Go code for date math, rotations, source parsing, and rendering. Use the LLM only later, if we decide it adds value.
- **Cost-aware scheduled runs:** Recurring daily brief runs should default to deterministic/no-model execution. Optional wording polish, classification, or summarization must route through an explicitly configured cheap/mini model profile; do not reuse the Codex/high-end development account/model for routine scheduled briefs unless Zach deliberately opts in for a specific feature. Record model/profile/token/cost metadata when model calls happen, without logging secrets or full prompts by default.
- **Swappable adapters:** Brief source, state storage, delivery, and scheduling should be interfaces so local files can become Google Docs, stdout can become email/Telegram, and local JSON can become S3/DynamoDB later.
- **Public-safe repo:** Keep real document IDs, email addresses, tokens, SMTP passwords, and delivery targets out of committed files. Use placeholders in `.env.example` and docs.
- **Test each phase:** Every implementation phase should include unit tests and a manual smoke command.

## Proposed package layout

```text
agent-harness/
  internal/
    brief/
      brief.go             # top-level orchestration
      render.go            # final message rendering
      dates.go             # birthday countdown + multilingual date words
      state.go             # local JSON state and rotation cursors
      sources.go           # source interfaces and fixture/local-file sources
      vocabulary.go        # vocabulary parsing/rotation/enrichment hooks
      elements.go          # elements parsing/rotation/enrichment hooks
      awards.go            # year-in-review rotation/cache
      delivery.go          # delivery interface + stdout implementation
    delivery/
      email.go             # later SMTP/SES sender
    google/
      docs.go              # later Google Docs client
    scheduler/
      ...                  # later local job runner, if needed
  docs/
    plans/
      daily-brief-integration-plan.md
```

Package names can be adjusted during implementation if the code reads cleaner, but keep brief-specific logic separate from the generic agent loop.

## Configuration shape

Add local config keys over time, with `.env.example` placeholders only:

```env
# Daily brief local-first config
DAILY_BRIEF_STATE_PATH=.agent-harness/daily-brief-state.json
DAILY_BRIEF_BIRTHDAY=1983-10-28
DAILY_BRIEF_TARGET_BIRTHDAY_AGE=80
DAILY_BRIEF_TIMEZONE=America/New_York

# MVP local source files
DAILY_BRIEF_VOCABULARY_SOURCE=fixtures/daily-brief/vocabulary.txt
DAILY_BRIEF_ELEMENTS_SOURCE=fixtures/daily-brief/elements.txt

# Later Google Docs sources
DAILY_BRIEF_VOCABULARY_DOC_ID=<google-doc-id>
DAILY_BRIEF_ELEMENTS_DOC_ID=<google-doc-id>
GOOGLE_CLIENT_SECRET_PATH=<local-path>
GOOGLE_TOKEN_PATH=<local-path>

# Later email delivery
DAILY_BRIEF_DELIVERY=stdout
DAILY_BRIEF_EMAIL_TO=<recipient@example.com>
DAILY_BRIEF_EMAIL_FROM=<sender@example.com>
SMTP_HOST=<smtp-host>
SMTP_PORT=587
SMTP_USERNAME=<smtp-username>
SMTP_PASSWORD=<smtp-password>

# Later optional LLM polish / model routing
DAILY_BRIEF_LLM_POLISH=false
DAILY_BRIEF_MODEL_PROFILE=cheap
DAILY_BRIEF_CHEAP_MODEL=<provider/model-for-low-cost-formatting>
DAILY_BRIEF_FALLBACK_MODEL=<provider/model-for-fallbacks>
```

## Cost policy for scheduled brief runs

Scheduled daily brief runs should be designed as the cheapest reliable path, not as miniature coding-agent sessions:

- **Default:** no model call. Date math, rotations, source parsing, caching, validation, and templated rendering stay deterministic.
- **Optional polish:** if wording polish, short summarization, or classification becomes useful, enable it explicitly with `DAILY_BRIEF_LLM_POLISH=true` and route it through `DAILY_BRIEF_MODEL_PROFILE=cheap` / `DAILY_BRIEF_CHEAP_MODEL`.
- **Fallbacks:** `DAILY_BRIEF_FALLBACK_MODEL` is for temporary cheap-provider failures or quality regressions, not the normal path.
- **High-end models:** Codex/high-end reasoning models are for development-time implementation or genuinely hard synthesis, not routine scheduled brief delivery unless Zach explicitly opts in.
- **Observability:** when a model is called, log model profile, model name, token counts, and estimated cost where available; do not log API keys, credentials, full prompts, or private source content by default.

## Phase 0 — Review and refine this plan

- **Status:** In review
- **Branch:** `plan/daily-brief-integration`
- **Pull Request:** TBD

### Objective

Agree on the phased path before implementation starts.

### Review questions

1. Should the MVP render the message without any model call, or should it optionally pass through the existing LLM client for light wording polish?
2. Which cheap model/provider should be the first target for optional daily-brief polish once model calls are introduced?
3. For the first working local run, are local text fixtures acceptable, or should Google Docs support be part of MVP?
4. Should email delivery be SMTP first, or should we target a specific provider/API later?
5. Should scheduling initially be external cron/systemd, or should `agent-harness` own a scheduler sooner?
6. Should the existing daily brief state be imported, or is it OK for `agent-harness` to start its own rotation from the beginning?

### Verification

- Zach reviews this plan and approves the phase breakdown.

---

# MVP milestone: local dry-run daily brief

The MVP should let Zach run this locally:

```powershell
go run ./cmd/agent-harness daily-brief --dry-run
```

Expected result: a complete daily brief printed to stdout, using local source files and local JSON state, with no network calls and no delivery side effects.

## Phase 1 — CLI skeleton and config

- **Status:** Pending
- **Branch:** `step-21-daily-brief-cli-skeleton`
- **Pull Request:** TBD

### Objective

Add a `daily-brief` command with `--dry-run`, config loading, injected clock support, and placeholder docs, but no real brief content yet.

### Scope

- Add command parsing for:
  - `agent-harness daily-brief --dry-run`
  - `agent-harness daily-brief --help`
- Add config fields for state path, timezone, birthday, and target birthday age.
- Add `.env.example` placeholders.
- Keep `daily-brief` independent from the agent/LLM runner for now.

### Files

- Modify: `internal/cli/app.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Create: `internal/brief/brief.go`
- Create: `internal/brief/brief_test.go`
- Modify: `.env.example`
- Modify: `README.md`

### Tests

- CLI accepts `daily-brief --dry-run`.
- Missing optional daily-brief config falls back to safe defaults.
- Invalid timezone fails clearly.
- Invalid birthday format fails clearly.
- Existing commands still work.

### Verification

```powershell
go test ./...
go run ./cmd/agent-harness daily-brief --dry-run
```


## Phase 2 — Local background service on this machine

- **Status:** Pending
- **Branch:** `step-22-agent-harness-background-service`
- **Pull Request:** TBD

### Objective

Make `agent-harness` installable and runnable as a long-lived background process on the Linux machine this agent currently runs on, with clear install, start, update, status, and log/error verification instructions.

### Scope

- Add documented Linux user-service install path for this machine, using systemd user services by default so root is not required.
- Build/install the `agent-harness` binary to a stable user-owned path such as `~/.local/bin/agent-harness`.
- Add a service unit example for running an `agent-harness` background command once one exists, with an initial placeholder/no-op or status-capable command if needed.
- Add explicit startup commands:
  - `systemctl --user daemon-reload`
  - `systemctl --user enable --now agent-harness.service`
  - optional `loginctl enable-linger $USER` if the service must survive logout/reboot and Zach approves the user-level persistence behavior.
- Add explicit update commands for future changes:
  - `git -C /home/alf/projects/agent-harness pull --ff-only`
  - `go test ./...`
  - `go build -o ~/.local/bin/agent-harness ./cmd/agent-harness`
  - `systemctl --user restart agent-harness.service`
- Add verification commands that Alf can run from this environment:
  - `systemctl --user status agent-harness.service --no-pager`
  - `systemctl --user is-active agent-harness.service`
  - `journalctl --user -u agent-harness.service -n 100 --no-pager`
  - a future app-level health/status command, such as `agent-harness status` or `agent-harness jobs list`, once implemented.
- Document how to distinguish healthy, stopped, failed, restart-looping, and config-error states from `systemctl`/`journalctl` output.
- Keep secrets and local machine paths out of committed defaults; committed docs may use this machine's repo path as an example because the requested first deployment target is this agent host.

### Files

- Create: `docs/local-background-service.md`
- Maybe create: `deploy/systemd/agent-harness.service.example`
- Modify: `README.md`
- Modify: `.gitignore` if runtime directories such as `.agent-harness/` need to stay local-only
- Maybe modify: `internal/cli/app.go` if a minimal `status` command or long-running placeholder command is needed for verification

### Tests

- Service unit example contains the expected installed binary path or documented placeholder.
- Install/update docs include test/build/restart/status/log commands.
- Any added `status` command has deterministic tests and exits non-zero on invalid config.
- Existing CLI commands continue to work.

### Verification

```bash
go test ./...
go build -o ~/.local/bin/agent-harness ./cmd/agent-harness
systemctl --user daemon-reload
systemctl --user status agent-harness.service --no-pager
journalctl --user -u agent-harness.service -n 100 --no-pager
```

Manual acceptance for this phase: Alf can verify from this environment whether the app service is running, stopped, or erroring, and can follow documented update steps after future plan PRs merge.

## Phase 3 — Deterministic date and birthday section

- **Status:** Pending
- **Branch:** `step-23-daily-brief-dates`
- **Pull Request:** TBD

### Objective

Render the first part of the brief deterministically: birthday countdown plus Spanish/Italian/French/German date lines.

### Scope

- Add injected clock/date helper.
- Compute days until Oct 28, 2063 from the configured timezone.
- Render weekday/month/day words in four languages.
- Avoid LLM dependence for date wording.

### Files

- Create: `internal/brief/dates.go`
- Create: `internal/brief/dates_test.go`
- Modify: `internal/brief/brief.go`
- Modify: `README.md`

### Tests

- Known-date countdown test cases.
- Leap-year/date-boundary cases.
- Spanish, Italian, French, and German date line snapshots for known dates.
- Timezone boundary case around UTC vs Eastern date.

### Verification

```powershell
go test ./...
go run ./cmd/agent-harness daily-brief --dry-run
```

## Phase 4 — Local JSON state and rotation helpers

- **Status:** Pending
- **Branch:** `step-24-daily-brief-state`
- **Pull Request:** TBD

### Objective

Add persistent local state and reusable rotation helpers without any network or Google dependency.

### Scope

- Store state in `DAILY_BRIEF_STATE_PATH`.
- Support dry-run mode that does not write state.
- Add source cycle keys and cursors.
- Add atomic write via temp file + rename.
- Add cache namespaces for later definitions/examples/awards.

### Files

- Create: `internal/brief/state.go`
- Create: `internal/brief/state_test.go`
- Modify: `internal/brief/brief.go`
- Modify: `.gitignore` if needed for `.agent-harness/`
- Modify: `README.md`

### Tests

- Missing state starts with defaults.
- Corrupt state recovers with a controlled warning or error, depending on final design.
- Dry-run leaves state unchanged.
- Non-dry-run advances selected cursors.
- Rotation wraps at the end.
- Atomic write produces valid JSON.

### Verification

```powershell
go test ./...
go run ./cmd/agent-harness daily-brief --dry-run
go run ./cmd/agent-harness daily-brief --state-path .agent-harness/test-daily-brief-state.json
```

## Phase 5 — Local source files for Vocabulary and Elements

- **Status:** Pending
- **Branch:** `step-25-daily-brief-local-sources`
- **Pull Request:** TBD

### Objective

Render a complete no-network MVP brief using local text files for vocabulary and element cards.

### Scope

- Add source interfaces.
- Add local text-file source implementation.
- Add test fixtures for vocabulary and elements.
- Parse Elements in the current heading/fact style:
  - heading like `1 - Hydrogen`
  - following lines as facts
- Parse Vocabulary one word per non-empty line, bottom-up.
- Render a vocabulary section with the word and placeholder definition/example text until enrichment is added.
- Render an Elements section with up to five facts.

### Files

- Create: `internal/brief/sources.go`
- Create: `internal/brief/sources_test.go`
- Create: `internal/brief/vocabulary.go`
- Create: `internal/brief/vocabulary_test.go`
- Create: `internal/brief/elements.go`
- Create: `internal/brief/elements_test.go`
- Create: `internal/brief/testdata/vocabulary.txt`
- Create: `internal/brief/testdata/elements.txt`
- Modify: `internal/brief/brief.go`
- Modify: `.env.example`
- Modify: `README.md`

### Tests

- Vocabulary reads bottom-up.
- Duplicate vocabulary words dedupe case-insensitively.
- Elements parse heading/facts correctly.
- Elements rotate top-down.
- Empty source files fail with a helpful error.
- Final MVP output contains date, vocabulary, and knowledge-refresh sections.

### Verification

```powershell
go test ./...
go run ./cmd/agent-harness daily-brief --dry-run
```

---

# Delivery milestone: local machine sends the brief

This milestone keeps execution on the current machine. Scheduling can be external at first.

## Phase 6 — Email delivery adapter

- **Status:** Pending
- **Branch:** `step-26-daily-brief-email-delivery`
- **Pull Request:** TBD

### Objective

Allow the local command to send the rendered brief by email, while keeping stdout/dry-run as the default safe path.

### Scope

- Add a `Sender` interface.
- Add stdout sender.
- Add SMTP sender or provider-specific email sender after review.
- Add `--send` flag.
- Do not advance state if delivery fails.
- Keep secrets out of traces/run logs.

### Files

- Create: `internal/brief/delivery.go`
- Create: `internal/brief/delivery_test.go`
- Create or modify: `internal/delivery/email.go`
- Create or modify: `internal/delivery/email_test.go`
- Modify: `internal/config/config.go`
- Modify: `.env.example`
- Modify: `README.md`

### Tests

- Dry-run prints only and does not send.
- Fake sender receives expected subject/body.
- Missing email config fails clearly when `--send` is used.
- Delivery failure does not advance state.
- Delivery success advances state.
- Secrets are not included in error output.

### Verification

```powershell
go test ./...
go run ./cmd/agent-harness daily-brief --dry-run
go run ./cmd/agent-harness daily-brief --send
```

## Phase 7 — External local scheduling docs

- **Status:** Pending
- **Branch:** `step-27-daily-brief-local-scheduling-docs`
- **Pull Request:** TBD

### Objective

Document how to run the brief every day on the same machine before building a scheduler into `agent-harness`.

### Scope

- Document Linux cron examples.
- Document systemd timer option if useful.
- Document Windows Task Scheduler/PowerShell equivalent for Zach's local machine if he wants to run it there.
- Include weekday 8am and weekend noon examples.
- Include log-file guidance.

### Files

- Modify: `README.md`
- Maybe create: `docs/daily-brief-local-scheduling.md`

### Tests

- Documentation command snippets are syntax-checked where practical.
- Existing tests still pass.

### Verification

```powershell
go test ./...
```

---

# Source/enrichment milestone: match the current Hermes content

This milestone adds the dynamic data currently handled by `daily_knowledge_refresh.py`.

## Phase 8 — Vocabulary definitions and example sentences

- **Status:** Pending
- **Branch:** `step-28-daily-brief-vocabulary-enrichment`
- **Pull Request:** TBD

### Objective

Replace placeholder vocabulary text with real definitions and actual example sentences.

### Scope

- Add HTTP client abstraction with timeout and user agent.
- Add Wiktionary definition lookup.
- Add dictionaryapi.dev fallback.
- Add Tatoeba or similar example-sentence fallback.
- Cache definitions/examples in state.
- Gracefully render fallback text if providers fail.

### Files

- Modify: `internal/brief/vocabulary.go`
- Modify: `internal/brief/vocabulary_test.go`
- Create: `internal/brief/http.go`
- Create: `internal/brief/http_test.go`
- Modify: `internal/brief/state.go`

### Tests

- Definition provider success.
- Definition fallback provider success.
- Example sentence uses the target word.
- Cache hit avoids HTTP calls.
- Provider timeout/error returns fallback text.
- Output caps long definitions/examples.

### Verification

```powershell
go test ./...
go run ./cmd/agent-harness daily-brief --dry-run
```

## Phase 9 — Year-in-review awards section

- **Status:** Pending
- **Branch:** `step-29-daily-brief-awards-section`
- **Pull Request:** TBD

### Objective

Add the rotating year-in-review section with Best Picture, Super Bowl winner, and NBA champion.

### Scope

Choose one data strategy during implementation review:

1. **Static curated data file** committed to repo fixtures for reliability.
2. **Wikipedia fetch/cache** matching the current Hermes script.
3. **Hybrid:** committed seed data plus optional refresh command.

Recommended MVP for reliability: static data file first, optional refresh later.

### Files

- Create: `internal/brief/awards.go`
- Create: `internal/brief/awards_test.go`
- Create: `internal/brief/testdata/awards.json`
- Modify: `internal/brief/state.go`
- Modify: `internal/brief/brief.go`
- Modify: `README.md`

### Tests

- Starts at configured first year.
- Advances one year per successful run.
- Wraps after previous calendar year.
- Returns expected awards for known years.
- Handles missing data clearly.

### Verification

```powershell
go test ./...
go run ./cmd/agent-harness daily-brief --dry-run
```

## Phase 10 — Element extra fact enrichment

- **Status:** Pending
- **Branch:** `step-30-daily-brief-element-extra-fact`
- **Pull Request:** TBD

### Objective

Add one outside fact to the Elements card, matching the current brief behavior.

### Scope

- Fetch Wikipedia summary for the selected element.
- Select a concise sentence not already present in doc facts.
- Cache if useful.
- Gracefully omit the extra fact if lookup fails.

### Files

- Modify: `internal/brief/elements.go`
- Modify: `internal/brief/elements_test.go`
- Modify: `internal/brief/http.go`

### Tests

- Fetch success adds `Extra:` line.
- Existing duplicate fact is not repeated.
- Network/provider failure keeps the brief usable.
- Long summary sentence is capped or skipped.

### Verification

```powershell
go test ./...
go run ./cmd/agent-harness daily-brief --dry-run
```

## Phase 11 — Google Docs source adapter

- **Status:** Pending
- **Branch:** `step-31-daily-brief-google-docs-sources`
- **Pull Request:** TBD

### Objective

Read Vocabulary and Elements directly from Google Docs instead of local files.

### Scope

- Add a `DocumentSource` interface if not already present.
- Add Google Docs client using configured OAuth token/client-secret paths.
- Keep local files as a fallback/testable option.
- Do not commit real doc IDs or token paths.
- Add config validation and docs.

### Files

- Create: `internal/google/docs.go`
- Create: `internal/google/docs_test.go`
- Modify: `internal/brief/sources.go`
- Modify: `internal/config/config.go`
- Modify: `.env.example`
- Modify: `README.md`

### Tests

- Google Docs response fixture parses expected text.
- Auth/config errors are clear and secret-safe.
- Local source mode still works without Google credentials.
- Source selection from config is deterministic.

### Verification

```powershell
go test ./...
go run ./cmd/agent-harness daily-brief --dry-run
```

---

# Agent/scheduler/platform milestone: build beyond MVP

These phases can be reordered based on what feels most useful after the local email version works.

## Phase 12 — Scheduled-run cost controls and optional LLM polish mode

- **Status:** Pending
- **Branch:** `step-32-daily-brief-cost-aware-llm-polish`
- **Pull Request:** TBD

### Objective

Define how the daily brief chooses no-model, cheap-model, and fallback-model paths, then optionally pass deterministic brief output through a low-cost LLM for light wording polish while keeping deterministic rendering as the default.

### Scope

- Add config or flag: `--polish` / `DAILY_BRIEF_LLM_POLISH=true`.
- Add a brief-specific model profile, for example `DAILY_BRIEF_MODEL_PROFILE=cheap`.
- Allow a cheap model override such as `DAILY_BRIEF_CHEAP_MODEL=<provider/model>` for low-risk formatting/summarization.
- Allow a fallback model override such as `DAILY_BRIEF_FALLBACK_MODEL=<provider/model>` for temporary provider failures or quality regressions.
- Reuse the existing OpenAI-compatible client/config plumbing where possible, but do not hard-code the same Codex/high-end account used for development work.
- Default to zero model calls unless `--polish` or equivalent config is enabled.
- Prompt must prohibit adding outside facts.
- Track/log model choice and rough token usage/cost metadata when available, without logging secrets or full prompts by default.
- Tests use fake LLM client.

### Tests

- Default path does not call LLM.
- Cheap-model polish path calls fake client with deterministic context and configured model profile.
- Missing cheap-model config fails clearly only when polish is enabled.
- Fallback model is used only under the selected failure policy.
- LLM failures fall back or fail according to chosen policy.
- Prompt includes “do not add outside facts” constraint.
- Trace/run-log metadata records model/profile without exposing API keys or raw credentials.

## Phase 13 — Built-in scheduler/job runner

- **Status:** Pending
- **Branch:** `step-33-scheduler-run-due`
- **Pull Request:** TBD

### Objective

Add generic scheduled job support to `agent-harness`, using daily brief as the first real job type.

### Scope

- Store job definitions separately from run logs.
- Add `jobs list`, `jobs run`, and `jobs run-due` commands.
- Use injected clock for due-job tests.
- Prevent recursive scheduled job creation.
- Keep external cron as a supported simpler option.

### Tests

- Valid/invalid schedule parsing.
- Due-job selection with injected clock.
- Disabled jobs do not run.
- Job failure records state/logs without stopping other jobs.

## Phase 14 — Telegram delivery adapter

- **Status:** Pending
- **Branch:** `step-34-daily-brief-telegram-delivery`
- **Pull Request:** TBD

### Objective

Add Telegram delivery as an alternative to email/stdout.

### Scope

- Add outbound-only send adapter first.
- Later bot interface can support inbound messages separately.
- Config supports bot token and chat/thread target.
- Redact tokens in logs.

### Tests

- Fake Telegram server receives expected payload.
- Missing token/target fails clearly.
- Delivery errors do not advance state.

## Phase 15 — Remote deployment option

- **Status:** Pending
- **Branch:** `step-35-daily-brief-remote-deployment-plan`
- **Pull Request:** TBD

### Objective

Document and optionally implement a remote deployment path after local execution is proven.

### Recommended AWS shape

```text
EventBridge Scheduler -> Lambda Go binary -> SES/email
                              |
                              +-> S3 or DynamoDB state
```

### Scope

- Keep local runner as the primary path.
- Add remote state interface only when needed:
  - local JSON for local mode
  - S3 object or DynamoDB item for Lambda mode
- Add SES sender if email delivery should be AWS-native.
- Add SAM template only after the local command is stable.

### Tests

- State store interface works with local fake.
- AWS-specific code behind adapters/fakes.
- SAM template validation where available.

---

## Open decisions before implementation

- Whether MVP should use local text sources or Google Docs immediately.
- Whether email should use SMTP, Gmail API, SES, or another provider.
- Whether state should start fresh or import the existing Hermes cron state.
- Whether to keep the current exact wording/format or make deterministic formatting the source of truth.
- Whether to add the built-in scheduler before or after Google Docs/enrichment.
- Which cheap provider/model should scheduled daily-brief polish use first, if optional polish is enabled.

## Suggested first approved scope

If Zach approves, start with these five PRs only:

1. **Phase 1:** CLI skeleton and config.
2. **Phase 2:** Local background service on this machine.
3. **Phase 3:** Deterministic date and birthday section.
4. **Phase 4:** Local JSON state and rotation helpers.
5. **Phase 5:** Local source files for Vocabulary and Elements.

That gives a real local MVP plus a documented, verifiable background runtime on this machine without network calls, secrets, delivery risk, or AWS. After that works, add email delivery and external scheduling.
