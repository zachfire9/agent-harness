# Agent Instructions

These instructions apply to the entire repository.

## Security-first development

- Prefer security best practices over convenience when designing or implementing features.
- Follow OWASP guidance where applicable, especially for authentication, authorization, secret handling, input validation, logging, dependency management, and secure defaults.
- Use the minimum access needed for each feature. Request the narrowest practical OAuth scopes, API permissions, filesystem access, network access, and runtime privileges.
- Treat OAuth tokens, API keys, passwords, client secrets, refresh tokens, session cookies, and private keys as secrets.
- Never commit secrets, real credentials, private tokens, private keys, or user-specific sensitive config.
- Never print secrets in CLI output, status files, logs, traces, errors, test snapshots, docs, PR bodies, or examples.
- Redact sensitive values in diagnostics. Prefer showing safe metadata such as account email, scope names, token expiry, config path, or connected/disconnected state.
- Store sensitive local runtime material only in explicitly secret locations with restrictive permissions, not in normal config files or source-controlled paths.
- Provide a clear revoke/delete path for connected accounts and other credentials.

## Least-privilege integration design

- Start integrations with read-only or send-only scopes when possible instead of broad account access.
- Make any escalation of permissions explicit in code, docs, tests, and PR descriptions.
- Prefer per-instance credentials over global credentials unless there is a deliberate reason to share them.
- Keep safe local/debug adapters available for tests and smoke checks so external credentials are not required for normal verification.

## Implementation expectations

- Add deterministic tests for security-sensitive behavior, including permission validation, redaction, unsafe path rejection, and failure modes.
- Keep public examples placeholder-only. Use fake emails, fake tokens, fake client IDs, and fake paths unless the user explicitly provides real values for local use.
- Validate untrusted input at boundaries and fail closed with clear, non-secret error messages.
- Avoid broad filesystem writes. Runtime state should stay under the configured instance home unless a feature explicitly documents and validates otherwise.
- Document security-relevant tradeoffs and limitations in the feature docs when introducing credentials, network delivery, OAuth, or external account access.
