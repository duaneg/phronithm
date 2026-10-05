# Recorded streams

`claude -p --output-format stream-json --verbose` output from Claude Code 2.1.289, recorded 2026-10-06 (runtime-core step 0), haiku/low, prompt on stdin, `--strict-mcp-config`.

| File | Call |
|---|---|
| `ok.jsonl` | plain structured-output call; carries a real `rate_limit_event` |
| `permanent.jsonl` | `--model claude-nonexistent-9`: assistant `error: model_not_found`, result `is_error`, 404 |
| `skill-loaded.jsonl` | prompt prefixed `/phronithm:critique`; the command is in init's `slash_commands` |
| `skill-tool.jsonl` | no prefix; the model loaded `phronithm:critique` with the Skill tool |
| `skill-absent.jsonl` | prompt prefixed `/phronithm:nonexistent-skill`; not in `slash_commands`, sent as plain text, valid output |

Trimmed: `system` events other than `init` dropped; `init` reduced to the fields listed in the step-0 script; strings over 400 characters cut and marked `…[trimmed]`; paths replaced with `/work/repo` and `/home/user`. Other values unchanged.
