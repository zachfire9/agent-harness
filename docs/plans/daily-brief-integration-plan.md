# Daily Brief Integration Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task after Zach approves the phase breakdown.

**Goal:** Add a local-first daily brief workflow to `agent-harness`, starting with a deliberately small MVP: an agent running on this Linux machine that sends a daily email with a customizable birthday countdown. After that works reliably, expand toward the richer Hermes daily brief content, sources, delivery options, scheduling, and optional remote deployment.

**Architecture:** Keep the MVP deterministic and inspectable rather than fully agentic. Add a dedicated `daily-brief` command that renders a birthday countdown message, can send it through an email delivery adapter, and can be run daily by a user-level background service/timer on the same machine as Hermes/agent-harness. Design storage, delivery, and scheduling as swappable interfaces so richer daily-brief sections and later AWS Lambda/EventBridge/SES deployment can be added without rewriting the core logic.

**Tech Stack:** Go CLI, deterministic unit tests, injectable clock, email delivery adapter, user-level systemd service/timer for this Linux machine, later local JSON state, later HTTP/source adapters, optional Google Docs API integration, optional external cron/GitHub Actions/AWS scheduling.

---

## Current baseline

`agent-harness` currently has:

- OpenAI-compatible model config and `ask`/`chat` commands.
- An agent runner with tool calling, context management, trace output, and JSONL run logs.
- Local workspace tools and a gated command tool.
- A future milestone list that already includes scheduler, Telegram, long-term memory, and browser/web tools.

The live Hermes daily brief currently depends on functionality that is not yet in `agent-harness`:

- Daily scheduled execution on this machine.
- Email delivery for the first MVP.
- Later persistent rotating state for cards and cached lookups.
- Later Google Docs ingestion for Vocabulary and Elements.
- Later dictionary/example lookup for vocabulary.
- Later Wikipedia-based element extra facts and year-in-review awards data.
- Later date rendering in Spanish, Italian, French, and German.
- Later Telegram delivery.

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
      email.go             # MVP SMTP/email sender, later SES/provider senders
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
# MVP birthday-countdown email config
DAILY_BRIEF_BIRTHDAY=1983-10-28
DAILY_BRIEF_TARGET_BIRTHDAY_AGE=80
DAILY_BRIEF_TIMEZONE=America/New_York
DAILY_BRIEF_DELIVERY=email
DAILY_BRIEF_EMAIL_TO=<recipient@example.com>
DAILY_BRIEF_EMAIL_FROM=<sender@example.com>
SMTP_HOST=<smtp-host>
SMTP_PORT=587
SMTP_USERNAME=<smtp-username>
SMTP_PASSWORD=<smtp-password>

# Later richer daily brief state/source config
DAILY_BRIEF_STATE_PATH=.agent-harness/daily-brief-state.json
DAILY_BRIEF_VOCABULARY_SOURCE=fixtures/daily-brief/vocabulary.txt
DAILY_BRIEF_ELEMENTS_SOURCE=fixtures/daily-brief/elements.txt

# Later Google Docs sources
DAILY_BRIEF_VOCABULARY_DOC_ID=<google-doc-id>
DAILY_BRIEF_ELEMENTS_DOC_ID=<google-doc-id>
GOOGLE_CLIENT_SECRET_PATH=<local-path>
GOOGLE_TOKEN_PATH=<local-path>

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

1. Which birthday/date/target age should be the default for the first countdown email?
2. Should MVP email delivery use SMTP first, or a provider-specific API such as Gmail/SES?
3. Should the daily run use a user-level systemd timer first, with built-in scheduler work deferred?
4. Which email recipient/sender should be configured locally on this machine without committing secrets?
5. Which cheap model/provider should be the first target for optional daily-brief polish later, if model calls are introduced?
6. Should later rich daily-brief state start fresh or import the existing Hermes cron state?

### Verification

- Zach reviews this plan and approves the phase breakdown.

---

# MVP milestone: daily birthday-countdown email agent on this machine

The scaled-down MVP should let Zach run this locally:

```bash
go run ./cmd/agent-harness daily-brief --dry-run
go run ./cmd/agent-harness daily-brief --send
```

Expected result: a deterministic birthday-countdown message can be previewed locally, sent by email, and then run once per day by a user-level background service/timer on the Linux machine this agent is running on. The MVP should not require Google Docs, Telegram, AWS, local source files, card rotations, or any model calls.

## Phase 1 — CLI skeleton and config

- **Status:** Pending
- **Branch:** `step-21-daily-brief-cli-skeleton`
- **Pull Request:** TBD

### Objective

Add a `daily-brief` command with `--dry-run`, config loading, injected clock support, and the first real MVP content: a customizable birthday countdown message. No model calls, Google Docs, rotations, or rich content yet.

### Scope

- Add command parsing for:
  - `agent-harness daily-brief --dry-run`
  - `agent-harness daily-brief --send`
  - `agent-harness daily-brief --help`
- Add config fields for timezone, birthday, target birthday age, delivery mode, and email placeholders.
- Render a plain-text birthday countdown message, for example days remaining until the configured target birthday.
- Add `.env.example` placeholders without committing real email addresses or SMTP credentials.
- Keep `daily-brief` independent from the agent/LLM runner for now.

### Files

- Modify: `internal/cli/app.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Create: `internal/brief/brief.go`
- Create: `internal/brief/brief_test.go`
- Create: `internal/brief/dates.go`
- Create: `internal/brief/dates_test.go`
- Modify: `.env.example`
- Modify: `README.md`

### Tests

- CLI accepts `daily-brief --dry-run`, `daily-brief --send`, and `daily-brief --help`.
- Missing optional daily-brief config falls back to safe defaults.
- Invalid timezone fails clearly.
- Invalid birthday format fails clearly.
- Known-date countdown test cases are deterministic.
- Leap-year/date-boundary cases are covered.
- Existing commands still work.

### Verification

```bash
go test ./...
go run ./cmd/agent-harness daily-brief --dry-run
```


## Phase 2 — Email delivery and local background service on this machine

- **Status:** Pending
- **Branch:** `step-22-agent-harness-background-service`
- **Pull Request:** TBD

### Objective

Make `agent-harness` installable and runnable on the Linux machine this agent currently runs on, with email delivery for the birthday-countdown MVP plus clear install, start, daily timer, update, status, and log/error verification instructions.

### Scope

- Add a `Sender` interface plus stdout and SMTP/email sender implementations for the MVP countdown message.
- Add documented Linux user-service install path for this machine, using systemd user services by default so root is not required.
- Build/install the `agent-harness` binary to a stable user-owned path such as `~/.local/bin/agent-harness`.
- Add systemd user service and timer examples that run `agent-harness daily-brief --send` once per day.
- Add explicit startup commands:
  - `systemctl --user daemon-reload`
  - `systemctl --user enable --now agent-harness-daily-brief.timer`
  - optional `loginctl enable-linger $USER` if the service must survive logout/reboot and Zach approves the user-level persistence behavior.
- Add explicit update commands for future changes:
  - `git -C /home/alf/projects/agent-harness pull --ff-only`
  - `go test ./...`
  - `go build -o ~/.local/bin/agent-harness ./cmd/agent-harness`
  - `systemctl --user restart agent-harness-daily-brief.service`
- Add verification commands that Alf can run from this environment:
  - `systemctl --user status agent-harness-daily-brief.timer --no-pager`
  - `systemctl --user status agent-harness-daily-brief.service --no-pager`
  - `systemctl --user is-active agent-harness-daily-brief.timer`
  - `journalctl --user -u agent-harness-daily-brief.service -n 100 --no-pager`
  - a future app-level health/status command, such as `agent-harness status` or `agent-harness jobs list`, once implemented.
- Document how to distinguish healthy, stopped, failed, restart-looping, and config-error states from `systemctl`/`journalctl` output.
- Keep secrets and local machine paths out of committed defaults; committed docs may use this machine's repo path as an example because the requested first deployment target is this agent host.

### Files

- Create: `internal/brief/delivery.go`
- Create: `internal/brief/delivery_test.go`
- Create: `docs/local-background-service.md`
- Maybe create: `deploy/systemd/agent-harness-daily-brief.service.example`
- Maybe create: `deploy/systemd/agent-harness-daily-brief.timer.example`
- Modify: `README.md`
- Modify: `.gitignore` if runtime directories such as `.agent-harness/` need to stay local-only
- Maybe modify: `internal/cli/app.go` if a minimal `status` command or long-running placeholder command is needed for verification

### Tests

- Email sender uses fake SMTP/server in tests and does not send real email during unit tests.
- Delivery failure exits non-zero and is visible in logs.
- Service/timer examples contain the expected installed binary path and `daily-brief --send` command.
- Install/update docs include test/build/restart/status/log commands.
- Any added `status` command has deterministic tests and exits non-zero on invalid config.
- Existing CLI commands continue to work.

### Verification

```bash
go test ./...
go build -o ~/.local/bin/agent-harness ./cmd/agent-harness
systemctl --user daemon-reload
systemctl --user status agent-harness-daily-brief.timer --no-pager
systemctl --user status agent-harness-daily-brief.service --no-pager
journalctl --user -u agent-harness-daily-brief.service -n 100 --no-pager
```

Manual acceptance for this phase: Alf can verify from this environment whether the daily birthday-countdown email timer is enabled/running, whether the last send succeeded or errored, and can follow documented update steps after future plan PRs merge.

## Phase 3 — Additional deterministic date section

- **Status:** Pending
- **Branch:** `step-23-daily-brief-dates`
- **Pull Request:** TBD

### Objective

Extend the MVP countdown with optional deterministic date presentation: Spanish/Italian/French/German date lines. The birthday countdown itself already exists in the MVP.

### Scope

- Reuse the injected clock/date helper from the MVP countdown.
- Render weekday/month/day words in four languages.
- Keep this optional formatting deterministic and independent from LLMs.

### Files

- Modify: `internal/brief/dates.go`
- Modify: `internal/brief/dates_test.go`
- Modify: `internal/brief/brief.go`
- Modify: `README.md`

### Tests

- Spanish, Italian, French, and German date line snapshots for known dates.
- Timezone boundary case around UTC vs Eastern date.
- Existing birthday countdown tests keep passing.

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

Add the first post-MVP rich daily-brief content using local text files for vocabulary and element cards.

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
- Final rich-content output contains date, vocabulary, and knowledge-refresh sections.

### Verification

```powershell
go test ./...
go run ./cmd/agent-harness daily-brief --dry-run
```

---

# Delivery milestone: harden local-machine delivery

This milestone keeps execution on the current machine and improves reliability after the MVP email/timer path exists.

## Phase 6 — Rich daily brief delivery safeguards

- **Status:** Pending
- **Branch:** `step-26-daily-brief-email-delivery`
- **Pull Request:** TBD

### Objective

Harden delivery behavior before richer sections start depending on stateful rotations or external sources.

### Scope

- Ensure `--dry-run` remains the default safe path for all richer content.
- Ensure state only advances after successful delivery when stateful sections are enabled.
- Add subject/body rendering conventions for richer brief sections.
- Redact SMTP credentials in logs and errors.
- Document how failed sends appear in `journalctl` on this machine.

### Files

- Modify: `internal/brief/delivery.go`
- Modify: `internal/brief/delivery_test.go`
- Create or modify: `internal/delivery/email.go` if not already created in Phase 2
- Create or modify: `internal/delivery/email_test.go` if not already created in Phase 2
- Modify: `internal/config/config.go`
- Modify: `README.md`

### Tests

- Dry-run prints only and does not send.
- Fake sender receives expected rich-content subject/body.
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

Recommended post-MVP approach for reliability: static data file first, optional refresh later.

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

- Which birthday/date/target age should be the default for the first countdown email.
- Whether MVP email delivery should use SMTP, Gmail API, SES, or another provider.
- Whether to enable user-level linger for this machine so the timer survives logout/reboot.
- Whether later rich daily-brief state should start fresh or import the existing Hermes cron state.
- Whether to keep the later rich daily-brief exact wording/format or make deterministic formatting the source of truth.
- Whether to add the built-in scheduler before or after Google Docs/enrichment.
- Which cheap provider/model should scheduled daily-brief polish use first, if optional polish is enabled.

## Suggested first approved scope

If Zach approves, start with these two PRs only:

1. **Phase 1:** CLI skeleton/config plus deterministic birthday-countdown rendering.
2. **Phase 2:** Email delivery plus user-level systemd service/timer on this machine.

That gives the scaled-down MVP: a verifiable local agent running on this machine that sends a daily birthday-countdown email, without Google Docs, Telegram, AWS, card rotations, rich sources, or model calls. After that works, add stateful/richer daily brief content incrementally.
