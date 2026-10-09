# Recorded streams

`claude -p --output-format stream-json --verbose` output from Claude Code 2.1.289 (the `claude-code-headless` pin; the versioned binary called directly), recorded 2026-10-06 in runtime-core step 0. Haiku/low, prompt on stdin, `--strict-mcp-config`, `--permission-mode dontAsk`.

| File | Call |
|---|---|
| `ok.jsonl` | plain structured-output call; carries a real `rate_limit_event` |
| `permanent.jsonl` | `--model claude-nonexistent-9`: assistant `error: model_not_found`, result `is_error`, 404 |
| `skill-loaded.jsonl` | prompt prefixed `/phronithm:critique`; the command is in init's `slash_commands`. The expansion is in the session transcript on disk, not on stdout |
| `skill-tool.jsonl` | no prefix; the model loaded `phronithm:critique` with the Skill tool |
| `skill-absent.jsonl` | no prefix; no Skill call; valid output |
| `skill-unknown.jsonl` | prompt prefixed `/phronithm:nonexistent-skill`; not in `slash_commands`, sent as plain text; valid output |
| `auth-401.jsonl` | recorded from **2.1.294** on 2026-10-09 against a local server answering every request 401 (`authentication_error`, "OAuth token has expired"), with `ANTHROPIC_BASE_URL` set to it, a placeholder `ANTHROPIC_AUTH_TOKEN` and an empty `HOME`: assistant `error: authentication_failed`, result `is_error`, 401. The CLI's ten `api_retry` system events before it are dropped by the trimming below |

Trimming:

- `system` events other than `init` are dropped.
- `init` is reduced to `type`, `subtype`, `session_id`, `claude_code_version`, `model`, `permissionMode`, `tools`, `skills`, `slash_commands` and `mcp_servers`.
- Strings over 400 characters are cut and marked `…[trimmed]`.
- Paths are replaced with `/work/repo` and `/home/user`.

Other values are unchanged.
