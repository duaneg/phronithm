# Agent Protocols

Shared protocols for autonomous agents. Each agent persona references this document and
declares an **adaptation block** listing context-specific overrides. All protocols apply to
every task type (bug fix, feature, refactoring) unless a section explicitly states otherwise.

## Conventions

Throughout this document, **"escalation target"** refers to whoever the agent reports to — a
human user, a Manager agent, or another coordinator. The adaptation block in each persona
specifies the concrete target and communication mechanism.

**"Halt and report"** means: stop work, describe the situation to the escalation target via
the mechanism in the adaptation block, and do not proceed until a response is received. If no
response can be received (non-interactive mode, unresponsive target), output a summary of the
situation and terminate.

## Nested subagents

The Agent tool is available inside subagent sessions, but uncontrolled nesting drains tokens
very fast. Do not spawn subagents except where a workflow
skill you are running explicitly directs you to (e.g. a critique or review gate that requires
context separation). Never spawn a subagent to do work you can do yourself, and never let a
subagent you spawn go on to spawn its own. If a task genuinely needs fan-out you cannot perform
directly, halt and report to the escalation target rather than spawning speculatively.

## Branch creation

Before creating a feature branch, ensure the branch base is `origin/HEAD` (the remote default
branch), not the local HEAD. Run:

```
git fetch origin
git checkout -b <branch-name> origin/HEAD
```

Do not use `git checkout -b <branch-name>` without an explicit base — this branches from
whatever local HEAD is checked out, which may be ahead of `origin/HEAD` with unrelated local
commits, contaminating the PR diff.

**No force push.** Never use `git push --force` or `git push --force-with-lease`. If `git push`
is rejected (non-fast-forward), halt and report to the escalation target — the rejection means
another agent or process has pushed to the same branch, and force-pushing would destroy their
work. The escalation target will resolve the conflict.

## Pre-flight test baseline

Before making any changes to the codebase (including exploratory edits, dependency installs, or
file writes), run the test suite and record any pre-existing failures unrelated to the issue.

- If the test suite fails to complete (compilation error, missing service, or non-zero exit
  before any tests run), halt and report.
- If the test suite completes with no failures, record the clean baseline and proceed.
- If there are pre-existing failures, report them to the escalation target — they must
  acknowledge these as pre-existing before you may proceed. If acknowledgement cannot be
  obtained, list the failures and halt.

Do not treat any new failures introduced by your changes as pre-existing.

## Scope rules

Limit changes to lines required for the fix. Confine production code changes to the declared
scope (the union of all scope paths provided in the task assignment or adaptation block).

**Test files**: test cases, fixtures, and mocks may be added or modified wherever the project's
test structure requires. If a file in a test directory is also imported by production code,
treat it as production code for scope purposes.

**Shared test helpers**: if fixing a test requires modifying shared test helpers or utilities
used by tests outside the declared scope, explain why and get approval from the escalation
target before proceeding.

**Unrelated issues**: do not fix unrelated bugs, linting violations, or formatting. If a file
you must touch has pre-existing linting or formatting violations that would fail the test suite
or CI checks, fix them in a separate preceding atomic commit covering only that file's
formatting and linting — never mix formatting fixes with semantic changes. Note the separate
commit in the PR description.

**Generated and vendored files**: do not edit these. If the upstream source is external to the
repository, flag it to the escalation target. Otherwise, edit the upstream source and
regenerate.

**Cross-scope dependencies**: if the fix requires modifying a file outside the declared scope
that is imported or called by code outside the scope, explain why and get approval from the
escalation target.

**Public API impact**: if a change within the declared scope is consumed by callers outside it:
- If it does not alter the public API contract, note the broader impact in the PR description
  and proceed.
- If it alters the public API contract, halt and report — see Stop conditions.

A change alters the public API contract if it changes the signature, return type, error
conditions, or observable behaviour (beyond correcting the defect described in the issue) of any
exported function, class, endpoint, CLI interface, or configuration file format.

## Dependency approval

Before adding or updating any dependency at any point, explain what is needed and why, and get
approval from the escalation target.

## Stop conditions

Stop and report to the escalation target if:

- You receive a `type: "shutdown"` message from the Manager (team context only) — finish any
  in-flight commit (do not leave uncommitted changes), then stop immediately. Do not
  acknowledge, do not re-process your completed task, do not send a status-update. The
  shutdown message is a control signal, not a task.
- You discover a security-sensitive related bug — see Security escalation below
- You cannot run the test suite at all (missing required services, credentials, or environment)
  — describe the blocker
- The fix requires changing the public API contract, database schema, or external interface —
  explain why (note: correcting error conditions that are themselves the defect does not
  constitute an API change)
- You cannot reproduce the reported failure, or the failure you reproduce does not match the
  reported symptom — describe what you tried and the discrepancy
- The test you expect to fail before the fix is already passing — describe why this may be
- The intended behaviour is unclear — state your interpretation and ask for confirmation

## Root cause protocol

Applies only after the reported failure has been successfully reproduced.

If during investigation you discover a bug that may be the root cause of the issue — that is,
fixing it would prevent the reported failure without additional code changes — stop and describe
both bugs to the escalation target, then wait for a decision on which to address. All scope
constraints still apply when fixing the root cause — approval of which bug to address does not
grant permission to modify files outside the declared scope.

If the root-cause bug is security-sensitive, apply the Security escalation protocol first.

If fixing the discovered bug would not by itself eliminate the reported failure, proceed with
the surface fix and note the pattern in a comment on the issue.

## Security escalation

For security-sensitive related bugs (unvalidated input, privilege escalation, data exposure):

Halt immediately. Before reporting to the escalation target:

1. Enumerate every call that interacted with GitHub in this session — including direct API
   calls, `gh` CLI commands, and any MCP or other tool-mediated GitHub interactions. For each
   call, list the tool or command used and the full content sent. Identify which calls included
   information about this bug; if any did, report them and ask that they be reviewed and deleted.
2. Write your findings only to this session's conversational output — do not write them to any
   file, log, external service, or GitHub surface.

Then report to the escalation target and do not continue until you have explicit confirmation.

For all other non-root-cause related bugs: note them in a comment on the issue but do not fix
them.

## Red-green test discipline

Assess whether the reported behaviour can be covered by a meaningful automated test. A
meaningful test is one that would fail on the unfixed code for the right reason — not merely a
test that exercises adjacent code or asserts trivially true properties.

Cases where no meaningful test is possible include: prose or configuration changes, race
conditions, environment-specific crashes, and fixes whose correctness cannot be observed at the
unit or integration level. If no meaningful test is possible, document why in the PR description
— do not write a placeholder test.

If a meaningful test is possible:

1. Search the test suite for tests covering this behaviour. Restore any that were deleted,
   disabled, skipped, marked as expected-failure, had their assertion weakened, had their
   expected value updated to match the defective behaviour, or were relocated outside the test
   runner's discovery scope to suppress the failure. If no pre-existing test covers this
   behaviour, write a new test.
2. Before applying the fix, run the test and confirm it fails. If it passes on unfixed code,
   the test does not reproduce the defect — revise it before proceeding.

## Done criteria

The fix is complete when:

- The test suite passes (pre-existing failures acknowledged in the pre-flight step are excluded;
  test tiers that cannot run in the current environment are excluded — note which tiers passed
  and which could not)
- Red-green test discipline has been satisfied (see above)
- A PR is opened with a description stating: your interpretation of the expected behaviour, what
  the fix changes, how to verify it, and any broader impact on callers noted during
  implementation

## Commit discipline

**Atomic commits**: Each commit should address exactly one purpose — one bug fix, one
refactoring step, one feature increment, or one issue. A reviewer should be able to describe
the commit's intent in one sentence. Do not split a single logical change across commits unless
the parts are independently meaningful.

**Multi-issue batches**: When a task covers multiple issues that share a file scope, commit each
issue's changes separately, in the order completed. Each commit message references only its own
issue. When multiple issues touch the same file, complete and commit one issue's changes before
starting the next — do not interleave edits across issues. If a commit breaks tests, fix or
revert before proceeding to the next issue.

**Commit messages**: Write for someone reading the history without access to the conversation.
State what changed and why. For bug fixes, include the bug, root cause, and fix. For
non-obvious changes, include the reasoning.

**Commit cadence**: Commit intermediate work rather than waiting for completion — prefer
recoverable state over lost work. Commit before switching to a different concern.

If investigation, annotation, or documentation changes are substantial, commit them separately
from the functional change.

## PR and issue discipline

Use `Closes #N` in the PR body so GitHub closes the issue automatically when the PR is merged
to the default branch.

Do not use closing keywords (Closes, Fixes, Resolves, etc.) in commit messages or issue
comments — commit keywords close on push to the default branch (not only on merge), and comment
keywords have no auto-close effect.

Add a comment to the issue linking the PR and summarising the change.

If you cannot open the PR or post the issue comment, output the text so it can be submitted
manually.
