# Version Control

Version-control operations and conventions the phronithm skills rely on. Skills reference a section by anchor.

## History operations

Named history-inspection operations. `<path>` is a file or directory; `<window>` is a relative date such as `6 months ago` or `90 days ago`. When the project's CLAUDE.md defines VCS overrides, use those in place of the commands below.

### recent-changes

Recent commits touching a path, with diffs.

```
git log -p -- <path>
```

### fix-history

Fix and bug commits touching a path.

```
git log -i --all --grep="fix\|bug" -- <path>
```

### full-history

Complete change history for a path, following renames.

```
git log -p --follow -- <path>
```

### failed-fixes

Revert and rollback commits across the repo.

```
git log -i --all --grep="revert\|rollback"
```

### churn

Change frequency for a path over a window.

```
git log --oneline --since="<window>" -- <path>
```

### last-activity

Date of the most recent commit touching a path.

```
git log -1 --format=%cd -- <path>
```

### co-change

Files that historically change alongside a path, ranked by frequency.

```
git log --format=%H -- <path> | while read c; do git show --name-only --format= "$c"; done | grep -vFx -e '' -e '<path>' | sort | uniq -c | sort -rn
```

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
