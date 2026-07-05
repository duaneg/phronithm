# VCS History Operations

Named history-inspection operations used by [phronithm:diagnose](../skills/diagnose/SKILL.md), [phronithm:impact-analysis](../skills/impact-analysis/SKILL.md), and [phronithm:refactor](../skills/refactor/SKILL.md). Skills reference an operation by name; the commands below implement each against git. When the project's CLAUDE.md defines VCS overrides, use those in place of the commands below.

`<path>` is a file or directory; `<window>` is a relative date such as `6 months ago` or `90 days ago`.

## recent-changes

Recent commits touching a path, with diffs.

```
git log -p -- <path>
```

## fix-history

Fix and bug commits touching a path.

```
git log -i --all --grep="fix\|bug" -- <path>
```

## full-history

Complete change history for a path, following renames.

```
git log -p --follow -- <path>
```

## failed-fixes

Revert and rollback commits across the repo.

```
git log -i --all --grep="revert\|rollback"
```

## churn

Change frequency for a path over a window.

```
git log --oneline --since="<window>" -- <path>
```

## last-activity

Date of the most recent commit touching a path.

```
git log -1 --format=%cd -- <path>
```

## co-change

Files that historically change alongside a path, ranked by frequency.

```
git log --format=%H -- <path> | while read c; do git show --name-only --format= "$c"; done | grep -vFx -e '' -e '<path>' | sort | uniq -c | sort -rn
```
