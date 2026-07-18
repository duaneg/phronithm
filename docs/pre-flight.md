# Pre-flight

Run `git status`. If the working directory is dirty (modified, staged, or untracked files relevant to the project), **stop** — do not begin the workflow. Present the user with options:

- Commit the current changes first
- Stash them (`git stash`)
- Confirm the dirty state is intentional and proceed (record a note explaining why)

Wait for the user to choose before continuing.

Record the current HEAD as the **base commit**: `git rev-parse HEAD`. This is the reference point for commit ranges produced by the workflow (e.g. `base..HEAD` in the Produces output).

When the workflow targets a directory other than the primary working directory (an *additional* working directory), use **absolute paths** in shell commands and file tools. The shell cwd can reset to the primary working directory between calls, silently breaking relative paths.

## Pre-supplied dirty-tree decision

The dirty-tree halt above requires a live user answer, which a non-interactive caller (a sub-agent dispatch, a batch run) cannot supply — it cannot reach the user to ask. A skill invoking this check may declare an optional input for its own answer, conventionally named **on-dirty-tree**, with the same three values as the halt's options: `commit` / `stash` / `proceed`. When the caller supplies one, take that action directly instead of presenting the options and waiting — for `proceed`, still record the note explaining why, exactly as the interactive path would. The halt still fires and blocks by default when no such input is supplied.
