# Static Analysis — Reference

Supporting reference for the `phronithm:static-analysis` skill. The skill body holds the workflow; this file holds the shared vocabulary and formats that consumer skills and hand-edited config depend on. Tool selection, curated configs, and precedence live in `configs/static-analysis/` (see its README) and are not duplicated here.

## Finding categorisation

Each normalised finding carries one `category`, used by consumer skills to group and triage. Map a tool's native rule to the closest category; when none fits, use `logic`.

- **null-reference** — null/None dereferences, optional mishandling
- **resource-lifecycle** — unclosed files, connections, streams
- **type-safety** — type mismatches, incorrect signatures
- **security** — injection, insecure crypto, hardcoded secrets, unsafe C functions
- **concurrency** — races, deadlocks
- **memory** — buffer overflow, use-after-free, leaks (C/C++)
- **undefined-behaviour** — integer overflow, uninitialised reads (C/C++)
- **error-handling** — empty catches, swallowed errors
- **logic** — unused variables, unreachable code, complexity; default for unmapped rules

Representative native-rule mappings: spotbugs `NP_*`→null-reference, `MT_*`→concurrency, `OBL_*`/`OS_*`→resource-lifecycle, `SQL_*`/`XSS_*`→security; ruff `S*`→security, `SIM115`→resource-lifecycle, `B904`/`TRY*`→error-handling; clang-tidy `clang-analyzer-security.*`→security, `bugprone-use-after-move`→memory, `concurrency-*`→concurrency; gcc `-Wnull-dereference`→null-reference, `-Wstringop-overflow`→memory, `-Wconversion`→type-safety.

## Exclusions file

`.claude/static-analysis-exclusions.yaml` is hand-maintained. The skill loads and applies it; there is no command that edits it for you — add entries by hand and keep them under review. Three scopes:

```yaml
# disable a rule everywhere
rules:
  - rule: "ruff:E501"
    reason: "Project uses a 120-char limit"
    type: intentional            # false-positive | accepted-risk | intentional

# skip whole files, or specific rules within them
files:
  - path: "src/generated/**"
    skip: true
    reason: "Generated code"
    type: intentional
  - path: "tests/**"
    exclude_rules: ["clang-tidy:readability-function-cognitive-complexity"]
    reason: "Test complexity acceptable"
    type: intentional

# silence one specific finding
findings:
  - file: "src/legacy/auth.c"
    line: 42
    rule: "clang-tidy:clang-analyzer-security.insecureAPI.strcpy"
    reason: "Refactor scheduled"
    type: accepted-risk
    review_by: "2026-09-01"       # optional; skill flags entries past this date
```

`type` classifies the suppression (`false-positive` = tool is wrong; `accepted-risk` = real but deferred; `intentional` = not a problem here). `review_by` turns accepted risk into a time-bound commitment; the skill reports how many active exclusions are past their review date.

## JSON output (`--format=json`)

Consumed by `phronithm:review`. Shape:

```json
{
  "summary": { "files_analyzed": 47, "errors": 23, "warnings": 156, "stale_exclusions": 3 },
  "findings": [
    {
      "file": "src/auth.c", "line": 42, "column": 15,
      "rule": "clang-tidy:clang-analyzer-security.insecureAPI.strcpy",
      "severity": "error", "category": "security",
      "message": "Use of strcpy is insecure",
      "fix": { "available": true, "diff": "--- a/src/auth.c\n+++ ...", "description": "Use strncpy with bounds" }
    }
  ]
}
```

## Design rationale

- **Strict by default**: better to surface everything and suppress deliberately than to miss a real defect. The exclusions file is what makes strictness liveable.
- **Suggest, never auto-apply**: static analysis misreads context; the engineer reviews and applies each change.
- **Hand-edited exclusions with review dates**: the failure mode is silent accumulation of normalised bad practice. A visible stale count is the cheapest guard against drift.
