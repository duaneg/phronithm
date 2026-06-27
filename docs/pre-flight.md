# Pre-flight

Run `git status`. If the working directory is dirty (modified, staged, or untracked files relevant to the project), **stop** — do not begin the workflow. Present the user with options:

- Commit the current changes first
- Stash them (`git stash`)
- Confirm the dirty state is intentional and proceed (record a note explaining why)

Wait for the user to choose before continuing.

Record the current HEAD as the **base commit**: `git rev-parse HEAD`. This is the reference point for commit ranges produced by the workflow (e.g. `base..HEAD` in the Produces output).

When the workflow targets a directory other than the primary working directory (an *additional* working directory), use **absolute paths** in shell commands and file tools. The shell cwd can reset to the primary working directory between calls, silently breaking relative paths.
