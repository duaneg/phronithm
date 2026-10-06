# code-stage-interface — plan

Work unit `code-stage-interface` of `docs/plans/lsf-improvements-breakdown.md` @ `492b524`. Base commit `492b524`, branch `lsf-workflow`. Phases 1–3 ran compressed in one plan-mode session, so this document is the problem statement, the design and the implementation plan. Phase 4 step 0 saves it as `docs/plans/code-stage-interface-plan.md`.

## 1. Problem statement

### Context

The Go orchestrator (`lsf-runtime/orchestrator`, unit `orchestrator-port-loop`) runs every stage as an agent call, including four that only move text the orchestrator already holds: load (copy the plan and decision-log blocks out of git), decision log (transcribe the requester's decisions into YAML), ledger append (append rendered Markdown), completion report (copy rendered Markdown onto the unit branch). Agents doing these damaged the text (register P-C1 to P-C4) and returned SHAs that do not resolve (P-C8, ledger). The substrate spike ran the same four stages as code by parsing values back out of the prompt text (`workflows/lsf/spike/driver.js`, `CODE`), which is its behavioural reference but not a design: this unit passes the values directly.

### Unit entry, verbatim

```
### code-stage-interface
- **Type**: refactor
- **Risk**: high — moves four stages out of agents and fixes the interface `code-merge-cleanup` builds on; P-C1 to P-C4 rest on it
- **Clarity**: exploratory — one open decision: the interface's shape (an extension of `orchestrator.Runtime` or typed stage calls)
- **Iteration**: write-once
- **Effort**: large
- **Scope**: `lsf-runtime/orchestrator`, `lsf-runtime/runtime`, `lsf-runtime/tools/promptparity`, `docs/design/lsf-workflow.md::model-tiers`
- **Publishes**: the orchestrator's code-stage interface, including each call's `CallResult` visible to the orchestrator
- **Depends on**: orchestrator-port-loop (logical, the ported orchestrator, `orchestrator.Runtime` and `tools/promptparity`); runtime-core (logical, `rt.Step`)
- **External gates**: `lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`)
- **Done condition**: Load, decision log, ledger append and completion report run as code through the interface; the ported orchestrator has no agent prompt for them. Tests inject at each of these stages the faults P-C1 to P-C4 and their Appendix B probes record — a placeholder, truncation, overwritten or deleted rows, an append's own rows altered or its heading dropped, a dropped line or fence, an invented `clears` entry — and a rejected commit, and show none is committed or accepted silently. A commit SHA these stages record resolves, and a fabricated one is rejected. The interface returns each agent call's `CallResult` (flags, denials) to the orchestrator with its value. `tools/promptparity` passes with only these four stages' labels removed from the JavaScript comparison. The tests pass.
- **Description**: Replaces `mechanical-stage-integrity`, with `code-merge-cleanup`. The spike's code stages parse their inputs out of prompt text (results, Build estimate); the interface passes them as values. Code stages journal through `rt.Step`, which reruns a failed step with `interrupted` set. The JavaScript orchestrator keeps its prompts until `script-retirement`. Load, ledger and completion report completed as model calls in the port's end-to-end run; the unit targets the integrity faults P-C1 to P-C4 record, not failures. `CallResult` reaches the orchestrator here because the interface is the one seam, and `runtime-signals` needs it for ledger rows. Split from merge and cleanup so the critical path does not wait on `retained-artefacts-design` (requester decision, 2026-10-06). **Flag:** four surfaces exceed the surface bound; the spike's code for each stage is the behavioural reference, and the promptparity change is a label exclusion, so the excess is not context. `orchestrator-port-loop` wrote three surfaces with six mid-implementation decisions and finished with context comfortable.
```

Standing obligations naming the unit:
- **Register dispositions**: the report disposes of P-C1, P-C2, P-C3, P-C4 and P-C8 (fabricated ledger SHAs).
- **Design of record**: Scope has `lsf-workflow.md::model-tiers`; the model-tiers paragraph is amended in the same commit as the behaviour change it describes.

Programme-wide, carried as before:
- **Frozen JavaScript**: `workflows/` is not edited.
- **`go.mod` stdlib-only**.

Headless pin does not apply: the unit has no `claude-code-headless` gate, and no paid run is planned.

### Decisions confirmed with the requester (2026-10-06)

- **Interface: extend `orchestrator.Runtime`** with generic primitives (`Call`, `Step`, `Repo`, `Worktree`). The four stages are typed Go functions in the orchestrator, run inside `Step`. Typed per-stage calls behind a `CodeStages` interface were rejected: the interface would widen with every code stage (merge and cleanup are next), and orchtest would need a fake per method.
- **Commits: porcelain plus verification.** Each committing stage writes the file and runs `git commit -- <path>`, so project hooks run as they did for the agents. It then reads the commit back and checks it. A mismatch stops the run and names the commit. A rejected commit restores the file. The completion report commits in its runtime worktree, as the spike did. Plumbing (a commit built and checked before any ref moves) was rejected: hooks would not run, and the main-checkout stages would have to resync the index and working tree after moving a checked-out branch.
- **Load is journaled** through `rt.Step` like the other stages. On a resume it replays the plan and decision texts and the architecture SHA it saw, so later prompts hash the same and replay too. This is how today's agent load already behaves. Refinement after critique: the HEAD branch and the pause ref are re-read live after every load and ledger step, replayed or not, and override the journaled value (§2.4). Otherwise a relaunch would replay a stale off-branch HEAD or a cleared pause, and the remedy for the `precondition` stop or the pause could never take effect.
- **Seeds do not stand in for code stages.** A seed naming one is logged and ignored (divergence).

### Success criteria

Target `proven` unless stated.

| # | Criterion | Target |
|---|---|---|
| S1 | Load, decision log, ledger append and completion report run as code through the interface. The orchestrator package has no prompt builder or schema for them (`copyPrompt`, `decisionLogPrompt`, `ledgerPrompt`, `completionScribePrompt` and their schemas deleted). A test shows a whole launch over orchtest makes no agent call labelled with these four prefixes. | proven |
| S2 | **Fault tests**, each through the stage function against a temporary git repo. <br>• Load: a placeholder and a truncated block (no closing fence), injected into the git read, end as an `invalid-plan` stop. On an untouched read, the text `LoadGraph` receives equals `git cat-file blob` byte for byte. P-C1's moved line and rewritten characters need a transcription to arise, and code has none: this criterion proves the absence of a transcription, not detection of a parse-valid alteration. <br>• Decision log: an invented `clears` entry, a dropped closing fence, a dropped line, a shortened decision text (injected by a pre-commit hook). <br>• Ledger: an overwritten earlier row, a deleted earlier row, its own rows altered, its heading dropped, a deleted wave section (hook). <br>• Completion report: a dropped line, an altered phrase (hook). <br>Each committing-stage fault ends as a stop naming the reason and the commit; the orchestrator uses no value from it. A relaunch does not adopt the bad commit: it stops again until the requester replaces it (C1). <br>• Every committing stage: a rejected commit (hook exits 1) leaves no commit, the file restored, and `git status --porcelain -- <path>` empty. | proven |
| S3 | **SHAs**: every SHA these stages record is checked by the stage before it is returned. A fresh commit must be a commit whose parent is the pre-commit tip, whose changes are exactly the stage's path, which is the branch tip, and whose blob equals the intended bytes. An adopted or deduplicated SHA must resolve, be an ancestor of the branch, and be intact (§2.3). A fabricated SHA (injected into `rev-parse` output, in the shape of P-C8's fabricated tails) is rejected with a stop, for each committing stage. | proven |
| S4 | **`CallResult`**: `orchestrator.Runtime.Call` returns `lsfrt.CallResult`. Every agent call site gets it with the value, including a null return and a `Parallel` branch. A test with a stub returning flags and denials shows them at the call site for a plain call, a null call and a parallel reviewer. The journal records denials as well as flags, so a replayed call returns both (runtime test). This unit consumes neither at the sites; `runtime-signals` does. | proven |
| S5 | **Resume**: a step interrupted after its commit and before its record (intent left) reruns with `interrupted=true`. It adopts the commit only if the commit passes the full check against its own parent (§2.4, adoption), and makes no second commit. Tests cover the ledger, the decision log and the completion report. A completed step replays without touching git. A relaunch after an off-branch `precondition` stop, with HEAD corrected, and after a cleared pause, proceeds. | proven |
| S6 | **Main-checkout safety**: the ledger and decision-log stages refuse, as a stop, when HEAD is not the integration branch, or when their path has uncommitted changes they did not make. They commit only their path; a test shows another staged or unstaged requester change survives the commit untouched. | proven |
| S7 | `go run ./tools/promptparity` passes (also under `go test`). The comparison changes only by excluding JS calls labelled `load`, `load:*`, `decision-log`, `ledger:*` and `completion-report:*` (with any `#n`); each is answered from the Go step's recorded value. Its coverage checks (`prefixes`, `labels`) count Go steps as well as calls, so the same label shapes must still be reached. The `unreachable` allowlist is unchanged. | proven |
| S8 | **On the real runtime**: `TestOnRuntimeLabelsUniqueAndReplay` passes with steps journaled (step values scripted through `rt.Step`). A new `TestOnRuntimeCodeStages` runs `lsfrt.Run` with the fake `claude`, a real repository and real code stages: decisions, load, one unit, its completion report on a real branch, ledger appends, ending `complete`. A relaunch replays every call and step (no spawn, no new commit). | proven |
| S9 | Divergences recorded in `orchestrator/doc.go` (D18 onward). The model-tiers paragraph of `docs/design/lsf-workflow.md` says these four stages are code, amended in steps 2 and 3, each in the commit that moves its stages. | evidenced |
| S10 | `go vet ./...` and `go test -race ./...` pass from `lsf-runtime/`; `go.mod` unchanged; `git diff --stat 492b524 -- workflows/` empty. | proven (tests), evidenced (diffs) |

### Candidate approaches

**A. Extend `orchestrator.Runtime` with generic primitives; typed stage functions inside `Step` (chosen).** Builds on `lsfrt.Runtime.Call` and `.Step` (`runtime/agent.go`, `runtime/journal.go`), the worktree creation in `runtime/worktree.go` (`cwdFor`), the orchtest handler-by-label pattern, and the spike's `CODE` stages as the behavioural reference.

**B. A typed `CodeStages` interface** (one method per stage), implemented over git in its own package and faked in orchtest. Rejected as above.

**C. Label dispatch inside the runtime** (the spike: `agent()` routes known labels to code that parses the prompt). Rejected: it is the prompt-parsing the unit exists to remove.

### Exploration findings

- `orchestrator/runtime.go`: `Runtime{Agent, Parallel, Log, Args}`; `orch.agent` applies seeds and the D10 `#n` label counter; `call[T]` = agent + `decode[T]`.
- Call sites: `loop.go` `loadPlan` (labels `load`, `load:w<n>`, `load:termination`), `scribeLedger` (`ledger:w<n>`, `ledger:carried`, `#n` repeats), the decision-log block in `Run`; `unit.go:344-370` completion report (`completion-report:<unit>`, isolation worktree, D17 check). 15 `call[T]`/`o.agent` sites in total.
- `lsfrt.Runtime.Step(ctx, label, input, fn(ctx, interrupted))`: replays a record with the same label+input hash; otherwise writes an intent, runs fn, records the value (null included) and removes the intent; an fn error keeps the intent and is returned as is (not a stop). `noteLabel` warns on a reused label, so steps must share the orchestrator's `#n` counter.
- `cwdFor` creates `<state>/wt/<key>` on `lsf/<plan8>/<refSafe(key)>` from the integration branch, reuses it on resume, and serialises worktree git. It is unexported and returns no branch.
- `plan`: `LoadGraph(planMarkdown, decisionsMarkdown)` extracts the blocks itself (`ExtractBlock`); `ParseDecisions` reports duplicate ids; the YAML subset accepts double-quoted `\" \\ \n \t`; `LedgerHeading`, `RenderLedger`, `LedgerLabel`, `NullReturnSignal`, `NoCommitSignal` (kind `agent-null`).
- orchtest default handlers for `load`, `ledger`, `decision-log` and `completion-report` don't read the prompt, so they serve steps unchanged. 17 assertions in `run_test.go` read these stages' prompts; they move to the step input.
- promptparity: Go runs on orchtest and records `Returns` by final label; node replays `implement.js` answering each label from that map; calls, logs and outcome are compared.
- `integration_test.go` serves the fake `claude` from the stub's prompts and returns, with SHAs that are not real. Real code stages need real SHAs, so that test scripts its step values through `rt.Step`, and a new test covers real git.
- `ownFilesOnly`, `commitMessage` and `shortSha` stay: direct-edits, merge, re-plan and termination still use them.

## 2. Design

### 2.1 Interface

```go
// orchestrator/runtime.go
type Runtime interface {
	Call(ctx context.Context, prompt string, o lsfrt.Opts) (lsfrt.CallResult, error)   // replaces Agent
	Parallel(ctx context.Context, thunks []func(context.Context) (json.RawMessage, error)) ([]json.RawMessage, error)
	Step(ctx context.Context, label string, input []byte, fn func(ctx context.Context, interrupted bool) (json.RawMessage, error)) (json.RawMessage, error)
	Repo() string                                                              // the main checkout
	Worktree(ctx context.Context, label string) (path, branch string, err error) // a step's own worktree
	Log(msg string)
	Args() json.RawMessage
}
```

`*lsfrt.Runtime` gains:
- `Repo() string` (the config's repo).
- `Worktree(ctx, label)`: `cwdFor` with `Opts{Label: label, Isolation: "worktree"}`. `cwdFor` is changed to return the branch too. The key is `journalKey(label)`, so `completion-report:A` gets the same worktree and branch its agent call had.

`Body` is unchanged.

Agent calls:
- `orch.agent` returns `(lsfrt.CallResult, label, error)`.
- `call[T]` returns `(*ret[T], lsfrt.CallResult, error)`; existing sites take the result as `_`.
- A seed returns `CallResult{Value: seed}`.
- A `Parallel` thunk stores its `CallResult` in a slice the caller owns, indexed by branch, and returns `cr.Value`. The gate+cleanup pair and the reviewers do this.

Code steps:
- `orch.step(ctx, label, input any, fn) (json.RawMessage, string, error)` applies the same `#n` counter as `agent` (labels stay unique across calls and steps).
- It marshals `input` with `plan.MarshalJS` and calls `rt.Step`. A seed for the label is logged (`<label>: args.seed ignored; code stage`) and not used.
- `step[T]` decodes like `call[T]`.
- Errors from `rt.Step`:
  - a `*lsfrt.StopError` propagates;
  - an error while `ctx` is done (cancellation reached git) is returned as `&lsfrt.StopError{Reason: lsfrt.StopCancelled}`, not as a stage fault;
  - any other error is the stage's fault, returned to the caller as `stageErr` with the reason.
- `lsfrt` journal records gain `Denials`; `replayCall` returns them with `Flags` (S4).

### 2.2 Stage values

Each step's value has the agent-return shape: `{"result": {...}, "commits": []}`. The existing result types are reused, so `decode`, the orchtest default handlers and the promptparity replay carry over unchanged.

| Stage | Input (journal hash) | Value `result` |
|---|---|---|
| load | `{sha, planPath, decisionLogSha, decisionLogPath, integrationBranch, architectureDoc, pauseRef}` | `copyResult`. `planBlock` and `decisionsBlock` hold the **whole documents** (`git cat-file blob <sha>:<path>`); `LoadGraph` extracts the blocks, so no second extractor exists to diverge from it. Also `headBranch` (`git symbolic-ref --short -q HEAD`, else `HEAD`), `architectureSha` (`git rev-list -1 <integ> -- <doc>`) and `pauseRequested` (`git show-ref --verify --quiet <pauseRef>`). |
| decision-log | `{path, name, integrationBranch, decisions, subject}` | `scribeResult{path, sha}` |
| ledger | `{path, name, integrationBranch, block, subject, pauseRef}` (block = `plan.RenderLedger(fresh, wave)`) | `scribeResult{path, sha, pauseRequested}` |
| completion-report | `{unit, base, path, text, subject}` (base = the unit's current branch) | `completionResult{branch, sha, path}` |

SHAs are full (40 hex).

### 2.3 Git helper (`orchestrator/git.go`)

`type gitRunner interface { run(ctx, dir string, args ...string) ([]byte, error) }` (stdout; stderr folded into the error). The production runner execs `git` with `GIT_TERMINAL_PROMPT=0` under `ctx`. It has a 30-minute bound, not the worktree code's 5 minutes, because a `commit` runs project hooks, which may run tests. `orch.git` holds it. `Run` sets it when `rt.Repo() != ""`; under the orchtest stub (no repo) it is nil, the steps are scripted, and the live reads below are skipped. Tests swap in a decorator that rewrites the output of chosen commands: this is how faults reach the load read and how fabricated `rev-parse` output is injected.

Helpers:
- `blob(rev, path) (bytes, ok)`
- `head(dir)`
- `commitPath(dir, path, subject, pre) (sha, error)`

`commitPath`:
1. `git add -- path` (a new file needs it).
2. `git commit -q -m <message> -- path`. The message is the JS subject, a blank line, and two trailers: `Lsf-Step: <final label>` and `Lsf-Content: sha256:<hex>`, the digest of the bytes the stage wrote. The digest makes any stage commit self-checking, in any later launch, whatever its label. Labels recur across launches (`ledger:w1`, `completion-report:A`), so a label alone cannot identify an attempt. A `commit-msg` hook that demands other trailers rejects the commit, which is a visible stop.
3. On failure, `git reset -q -- path` unstages it, and the caller restores the file's prior bytes, or removes a new file. The path's `git status` is then empty.
4. `sha = rev-parse HEAD`, then `verifyCommit`.

`verifyCommit(dir, branch, sha, pre, path, want)` errors unless all of these hold:
- `git cat-file -t sha` = `commit`;
- `sha^` = `pre` (or no parent when `pre` = "");
- `git diff-tree --root --no-commit-id --name-only -r sha` = `[path]`;
- `rev-parse refs/heads/<branch>` = `sha`;
- `blob(sha, path)` = `want`, byte for byte.

A failure after the commit landed leaves the commit (the run never rewrites the integration branch) and names it in the error.

**Intact.** A commit is *intact* if it carries `Lsf-Content` and its blob at `path` hashes to it. A hook, or a later amend that alters the bytes, breaks this. A history rewrite that copies the trailer onto other content breaks it too (a squash, for instance). That case is a false stop, recoverable as below.

**Guard.** It runs before a main-checkout stage writes. It finds the last first-parent commit touching `path` on the integration branch (`git log -1 --first-parent --format=%H <integ> -- path`). If that commit carries `Lsf-Content` and is not intact, the stage stops, naming it: an earlier attempt left a bad commit, whatever its label or input. A requester's repair must be a *new* commit without an `Lsf-Content` trailer: an `--amend` or `--fixup` keeps the trailer and stops again. The stop's statement says so. With that commit in place, the next launch proceeds on the repaired text. The guard sees only the last commit touching the path. An agent commit (direct edits) landing after a bad stage commit would hide it. Agents no longer write these two paths, so this is accepted.

**Adoption** is how a stage reuses its own earlier commit instead of making a second. It walks the first-parent commits touching `path` (at most 50), newest first. It adopts the first one that satisfies all of these:
- it is intact;
- its `Lsf-Step` label is this step's label;
- its blob equals the stage's own rendering from that commit's parent's blob. For the decision log, that means it parses to the parent's decisions plus this step's, in order. The entry date is free, but it is covered by the digest;
- its effect survives at the branch tip. For the ledger, the tip's blob has the candidate's appended block in it. For the decision log, the tip's block parses to include this step's decisions. A commit the requester has since reverted is not adopted.

An adopted SHA, and the SHA the decision-log dedup returns, need not be the branch tip. They are checked as follows: the SHA resolves to a commit, it is an ancestor of the integration branch (`merge-base --is-ancestor`), and it is intact (adoption) or touches the path (dedup). A fresh commit gets the full `verifyCommit`.

This identifies the attempt by content, not by label or input hash. A same-label commit from an earlier launch, with other rows, never matches. A requester edit made after the stage's commit does not hide it. Adoption is tried on an interrupted rerun (ledger, decision log). The completion report always tries it, against its own branch's tip, with `pre = base`.

### 2.4 The stages (`orchestrator/stages.go`)

- **load**: reads only.
  - Journaled: the plan text at `sha`, the decision log at `decisionLogSha` (when set), and `architectureSha`. A missing blob is an error (a stage fault).
  - Live, after the step (replayed or not), when `orch.git` is set: `headBranch` and `pauseRequested` are re-read and override the value. A relaunch after the requester fixes HEAD or clears the pause therefore sees the fix.
  - Faults in the read reach `LoadGraph` and become `invalid-plan` stops: a placeholder has no block, and a truncated block has no closing fence.
  - P-C1's faults need a transcription to arise, and code has none. The load tests show two things. Placeholder and truncation, injected into the read, stop. On an untouched read, the value `LoadGraph` receives equals `git cat-file blob` byte for byte. A moved line or rewritten characters that still parse are undetectable by any reader of the same bytes, and the report says so.
- **decision-log** (main checkout; `date` is today, taken inside the step and not part of the input hash, so a relaunch across midnight still replays):
  1. Precheck. HEAD must be on `refs/heads/<integ>`. `git status --porcelain -- path` must be empty, unless `interrupted` and the working file parses to the prior decisions plus this step's. Then run the guard (§2.3).
  2. If `interrupted`, try adoption (§2.3).
  3. Prior. `blob(HEAD, path)`, else the spike's new-file skeleton. Its block must parse (`ParseDecisions`), or it is an error: a broken log is never appended to.
  4. Dedup. Decisions whose `[id, question, decision, clears]` are already present are dropped. If none remain, the log already says what was asked: return the SHA of the last commit touching the path. That covers a requester's repair commit.
  5. Render. Entries are double-quoted scalars (spike `yq`); `clears` is a flow sequence, omitted when empty. They are inserted before the block's closing fence.
  6. Parse back. The new text must parse to exactly prior + new in order, or it is an error and nothing is written. A duplicate id with other content fails here.
  7. Write, `commitPath`, `verifyCommit` (`want` = the rendered text), and parse the committed blob back to the same decisions.
- **ledger** (main checkout):
  1. Precheck as for the decision log (the interrupted exemption: the working file equals the rendering), then run the guard.
  2. If `interrupted`, try adoption (§2.3). There is no adoption otherwise. The prior's integrity rests on the guard: an earlier stage commit must be intact, and text the requester committed is theirs.
  3. `next = TrimRight(prior, "\n") + "\n\n" + block`, where a new file's prior is `LedgerHeading(name)`.
  4. Write, `commitPath`, `verifyCommit` (`want = next`).
  5. `pauseRequested` is read live after the step, as for load.
- **completion-report** (its own worktree, from `rt.Worktree(label)`; branch `lsf/<plan8>/<key>`, used by no one else):
  1. If the branch tip is intact, its parent is `base`, and its blob equals `text`, reuse it with no commit. This covers an interrupted step and a branch left by an earlier launch.
  2. Otherwise run `git reset -q --hard base` and `git clean -fdq` in the worktree, then write, `commitPath` and verify against the worktree's branch.
  3. A bad tip from an earlier attempt is discarded by the reset, never built on. That is safe only because the branch is the step's own.
  4. Git here runs in the step's worktree, concurrently with other units' stages. That is already the case for the agent stages' commits. Branch refs differ, and git's ref and object writes take their own locks, so `wtSem` (which serialises `worktree add`) is not taken. This rests on git's own locking and is not tested here.
- **Live reads** (HEAD and pause) apply only to these code stages. A git error in a live read is a stage fault (a stop). Pause is `show-ref` exit status: absent is not an error. Merge, re-plan and termination are still agents, and report `pauseRequested` from their own live read. Under orchtest (`orch.git` nil) the scripted values stand. The live reads are therefore tested on real repositories only, for both load and ledger.

### 2.5 Orchestrator wiring

- **`loadPlan`**: `step[copyResult]`. A stage fault is logged (`<label>: <reason>`) and stops with `stageFault(at{stage: "load"}, reason)`. The `cp == nil` path is kept for a scripted null.
- **Decision log in `Run`**: `step[scribeResult]`. A fault becomes `stopped("decision-log", stageFault)`. The `committed`, null and D17-like checks are unchanged for scripted values.
- **`scribeLedger`**: `step[scribeResult]`. A fault sets `ledgerFault = stageFault(at{stage: "ledger", path})`, logged as now. Everything downstream is unchanged.
- **`runUnit` completion report**: `step[completionResult]`. A fault appends `stageFault(at{stage: "completion-report", unit})` and aborts. The D17 check stays.
- `stageFault(at, reason)` is a signal with kind `agent-null` (a stop kind; `plan` is outside Scope), source `structural`, statement `"<stage> failed: <reason>"`, and referent the path, else the unit, else the stage. Whether code-stage faults get their own kind is noted for `runtime-signals`.
- Dead code is deleted: `copyPrompt`, `decisionLogPrompt`, `ledgerPrompt`, `completionScribePrompt`, `copySchema`, `decisionLogSchema`, `ledgerSchema`, `completionSchema`, and the `ledgerSchema` assertion in `runtime_test.go`. The tier lookups for these stages go too.

### 2.6 orchtest

- `Stub.Call`: the old `Agent` body. A handler may return an `lsfrt.CallResult` to set flags and denials.
- `Stub.Step`: records a `Call{Label, Prefix, Unit, Input, Step: true}` with the same repeated-label check, and the value in `Returns`. It answers from the handler for the prefix; fn is never run. A handler returning a non-stop `error` makes `Step` return it, which the orchestrator treats as the stage fault (how `Run`-level fault tests script one).
- `Repo()` returns "". `Worktree` returns an error.
- `Stub.Input(label)` returns the step input.
- Tests that matched a stage's prompt now decode the input and match its `block`, `text`, `decisions` or `sha`.

### 2.7 promptparity

- `runGo` keeps steps out of `r.Calls`, but keeps their `Returns`, and records their labels in a new `r.Steps`.
- The coverage checks (`prefixes`, `labels`, `main.go:373-427`) count `r.Calls` and `r.Steps` together, so `ledger:w0`, `ledger:carried`, `load:w1`, `load:termination` and the four prefixes are still required to be reached.
- `unitStages` keeps `completion-report`; its JS calls are dropped before lanes are compared.
- The JS replay answers every label from `Returns`, unchanged.
- Before comparison, JS calls whose base label (before `#`) is `load`, starts `load:`, is `decision-log`, starts `ledger:` or starts `completion-report:` are dropped.
- The lane order comparison drops them on both sides.
- The header comment states the exclusion.

### 2.8 Divergences (`orchestrator/doc.go`)

- **D18.** Load, decision log, ledger append and completion report are journaled code steps. Their values keep the agent-return shape, and `planBlock`/`decisionsBlock` hold whole documents. A seed naming one is logged and ignored. Load is journaled, so a resume replays the texts and the architecture SHA it saw. The HEAD branch and pause are re-read live. The decision log drops a decision already present with identical content. Code commits carry an `Lsf-Step: <label>` trailer. A state dir journaled before this unit holds `call` records for these labels, which `Step` treats as stale and reruns; such a state dir is not resumable.
- **D19.** A code stage's failure is an `agent-null`-kind stop whose statement gives the reason:
  - a git error;
  - HEAD off the integration branch;
  - the path dirty with changes not the stage's;
  - a broken existing decision block;
  - a rejected commit (the file is restored and unstaged);
  - an earlier stage commit that is not intact (the guard). A project whose hooks reformat these files gets this stop on every launch until it exempts the paths from the hook; the bytes must be exact.
  - a commit that fails verification (the commit stays, named).

  JS gets these only as a null or empty-SHA return, without the reason.

### 2.9 Risks and unknowns

| Risk | Resolution | When |
|---|---|---|
| A pre-commit hook's `git add` under `git commit -- path` may not reach the commit (git re-reads the temporary index after the hook; unverified on the installed git). | Step 1 probe in a scratch repo. If it does not, hook-based content faults are injected by the runner decorator at `commit` instead (it rewrites the file and runs `git add` before delegating). | before step 2 |
| A relaunch with *changed* input after a kill between the commit and its record is not `interrupted` (the intent hash differs). The ledger then appends its new rows after the committed ones, which may repeat rows. | Visible in the ledger, not silent damage. Recorded in D19. | accepted |
| Projects whose hooks reformat files (end-of-file or whitespace fixers) fail verification and stop. | Intended (the bytes must be exact). `RenderLedger` and the decision renderer emit no trailing whitespace and end in one newline. Recorded as a discovery. | after |
| A requester edit to the same path between the precheck and the commit. | Verification catches a content change; the window is milliseconds. | accepted |
| `git status --porcelain` treats an ignored path as clean. | The ledger and decision paths are tracked docs; an ignored path makes `git add` fail, which is a stop. | accepted |
| Out-of-Scope doc: `docs/design/lsf-runtime.md` Interface lists the runtime's methods. | Add `Repo` and `Worktree` lines; recorded as a Scope deviation. | step 1 |

No third-party API is involved. The git behaviours relied on (`commit -- <path>` commits only those paths, `diff-tree --name-only -r`, `cat-file -t`, `symbolic-ref -q`) are verified by the tests themselves against the installed git.

### 2.10 Critique record

Critique gate (`phronithm:critic`, sonnet, critique-design), three rounds:

- **Round 1** raised 1 Critical: resume and dedup adopting unverified commits. It also raised 8 Significant findings:
  - promptparity coverage counted calls only;
  - per-step promptparity ordering;
  - a rejected commit left its path staged;
  - load fault claims;
  - stale journaled HEAD and pause;
  - denials not journaled;
  - model-tiers commit timing;
  - the ledger's interrupted check.

  All were addressed: strict adoption, the §2.7 coverage change, promptparity in steps 2–3, unstaging, the S2 load restatement, live reads, journaled denials, and per-step tier amendments. Ten minor findings were addressed or accepted in the text.
- **Round 2** raised 3 Critical findings: label-keyed adoption gave false stops across launches; the integration test broke at step 2; a bad commit from an earlier attempt with other input was built on. It also raised 4 Significant findings: decision-log repair deadlock, date in the hash, a requester edit hiding the stage commit, and live-read coverage. All were addressed: the `Lsf-Content` digest trailer, and the intact, guard and content-based adoption definitions; the wrapper moved to step 2; the date left out of the hash; adoption walks history; the live-read bullet.
- **Round 3**: no Critical findings (converged). Six Significant or Minor points were folded in:
  - intact = digest only, with squash false stops recoverable;
  - the repair must be a new trailer-free commit;
  - adoption requires its effect to survive at the tip;
  - the SHA contract for adopted and deduplicated SHAs;
  - live-read errors;
  - reformatting hooks and changed-input relaunches recorded.

  One point is accepted as stated: the guard blind spot for agent commits.

## 3. Implementation plan

Each step: implement, `gofmt`, `go vet ./...`, `go test -race ./...` from `lsf-runtime/`, commit, then spawn the critique gate (`phronithm:critic`, model sonnet, critique-code) on the step's diff and continue.

### Step 0 — save the plan
Save this document as `docs/plans/code-stage-interface-plan.md`; commit.

### Step 1 — the seam
- **Probe**: in a scratch repo, check whether a pre-commit hook that rewrites the file and runs `git add` changes the commit made by `git commit -- path`. Record the result. If it does not, hook-shaped content faults are injected by the runner decorator at `commit` (rewrite the file and `git add` before delegating). Rejected commits stay real hooks either way. The report states which rows use real hooks.
- **`lsfrt`**: `Repo()`; `Worktree(ctx, label)`; `cwdFor` returns `(path, branch, err)`; journal records keep `Denials` and replays return them.
  - Tests: a step's worktree is created on `lsf/<plan8>/<key>` and reused; missing args is a `runtime-error` stop; a replayed call returns its flags and denials.
- **Orchestrator**:
  - Change the interface (§2.1): `orch.agent` and `call[T]` carry `CallResult`; the 15 sites are updated.
  - `Parallel` thunks capture `CallResult`.
  - Add `orch.step` and `step[T]` (cancellation → `StopCancelled`).
- **orchtest**: `Call`, `Step`, `Repo`, `Worktree`, `Input`; `Call.Step` marks a step; a handler error from `Step`.
- **promptparity**: `r.Steps` and coverage over calls and steps (§2.7). No exclusion is needed yet, since nothing is a step.
- **Docs**: the `lsf-runtime.md` Interface lines.
- **Acceptance**:
  - All existing tests and promptparity pass with behaviour unchanged (stages still agents).
  - New test `TestCallResultReachesCallSites`: a plain call, a null call and a parallel reviewer show the flags and denials (S4).
  - `step` shares the label counter: a test repeats a label across a call and a step.

### Step 2 — git helper; load and decision log as code
- `git.go` (§2.3), with tests of `verifyCommit` on a temporary repo: each of its five checks fails on a crafted commit, including the root-commit case.
- `stages.go`: load and the decision log (§2.4), with adoption. The orchestrator fixtures build temporary repos.
- Wire `loadPlan` (with the live HEAD and pause reads) and the decision log in `Run` (§2.5). Delete `copyPrompt`, `decisionLogPrompt` and their schemas. Rewrite the `run_test.go` assertions that read those prompts.
- promptparity: exclude `load`, `load:*` and `decision-log` JS calls (§2.7).
- `TestOnRuntimeLabelsUniqueAndReplay`: wrap `*lsfrt.Runtime` in a test type whose `Step` calls `rt.Step` with an fn returning the scripted value. Its fake `planSha` would otherwise fail a real load. The journal and replay assertions count steps too.
- Model tiers: amend the paragraph to say load and the decision log are code.
- **Fault tests**:
  - Load: placeholder and truncation through the decorator end `invalid-plan`. A clean read hands `LoadGraph` the blob bytes. A missing SHA ends `load failed: …`. A replayed load with HEAD since corrected, and a pause since cleared, sees the live values.
  - Decision log, through a hook: an invented `clears`, a dropped fence, a dropped line and a shortened text. Each fails verification and stops, naming the commit. A relaunch stops again (guard), with the same input or other decisions. After a requester repair commit, it proceeds, and a repair that already holds the decisions returns its SHA.
  - A rejected commit leaves no commit, the file restored and its `git status` empty.
  - A fabricated SHA through the decorator is rejected.
  - HEAD off the integration branch, and a dirty path, are refused.
  - Another requester change, staged and unstaged, survives.
  - Dedup is applied; a duplicate id with different content stops.
  - An interrupted rerun after a good commit adopts it and makes no second commit, including after midnight (date outside the hash) and after a requester commit on top.
- **Acceptance**: S2, S3, S5 and S6 for these stages; promptparity passes.

### Step 3 — ledger and completion report as code
- `stages.go`: ledger and completion report (§2.4).
- Wire `scribeLedger` and `runUnit` (§2.5). Delete `ledgerPrompt`, `completionScribePrompt` and their schemas, and the `ledgerSchema` test. Rewrite the remaining prompt assertions.
- promptparity: also exclude `ledger:*` and `completion-report:*`.
- Model tiers: the paragraph's final form (all four stages code; the mechanical tier covers worktree cleanup only).
- **Fault tests**:
  - Ledger, through a hook: an overwritten earlier row, a deleted earlier row, own rows altered, the heading dropped and a deleted wave section. Each stops, naming the commit. A relaunch with any rows or label stops again (guard) until a requester repair commit.
  - Ledger: a rejected commit, a fabricated SHA, an off-branch HEAD and a dirty path.
  - Ledger kill-after-commit: the intent is left, and a rerun with `interrupted` adopts the commit, making no second one. Two scribes in one wave (`ledger:w2`, `ledger:w2#2`) are not confused, nor is an earlier launch's intact `ledger:w1` commit with other rows: no false stop, and a normal append follows. A requester commit to the ledger after the stage's commit does not cause a second append.
  - Ledger live pause: a replayed ledger step reports the pause ref as it stands now (real repository).
  - Completion report, through a hook: a dropped line and an altered phrase. A rejected commit and a fabricated SHA. A rerun reuses a matching tip. A stale or bad tip is reset and recommitted.
  - Through `Run` on orchtest: a scripted ledger fault stops the run `ledger`, and a completion-report fault aborts the unit.
  - A whole orchtest launch makes no call with the four prefixes.
- **Acceptance**: S1, S2, S3, S5 and S6 complete; S7 (promptparity) passes.

### Step 4 — the real runtime
- The fake `claude` gains a second table form for the new test: an ordered list of `{contains, return}`. The first entry whose substring the prompt contains answers. No match answers the existing error result (`api_error_status` 400, exit 1). The prompt-hash form stays for the existing test.
- New `TestOnRuntimeCodeStages`, run in a temporary repo. Setup:
  - The plan is committed (one unit `A`), and its SHA (40 hex) is `args.planSha`.
  - An `A` work branch has one commit.
  - Agent returns name real refs: the implement and fix branch is the `A` work branch. The merge stays an agent and the fake performs no merge (verifying merges is `code-merge-cleanup`'s). Its return reports `A` clean with `planSha` set to a pre-made commit on a side branch whose plan marks `A` complete. Re-plan and termination return that SHA too, so `load:w1` and `load:termination` read a real commit.
  - `args.decisions` holds one decision.
- Assertions:
  - The run ends `complete`.
  - The decision log, ledger and completion report commits exist with exact content and the trailer.
  - No call has these four labels.
  - A relaunch replays every call and step: no spawn, no new commit, `step-replayed` events counted.
- **Acceptance**: S8.

### Step 5 — records
- `doc.go` D18 and D19; the `orchestrator` package comment lines that say every stage is an agent.
- Full `go vet`, `go test -race`; promptparity.
- `git diff --stat 492b524 -- workflows/` is empty and `go.mod` unchanged.
- The implementation summary.
- **Acceptance**: S9, S10.

Dependencies are linear (1 → 2 → 3 → 4 → 5); steps 2 and 3 touch the same files, so they are not parallelised.

## Remaining phases
This plan was generated by the phronithm:feature skill. After implementing the steps above,
complete Phase 5 (Review) as defined in the phronithm:feature skill.

## Implementation-summary fields
The implementation-summary must be saved to: docs/plans/code-stage-interface-implementation.md
It must include:
- Working branch
- Reference to this plan file (filename or path)
- Files modified (list of paths changed during implementation)
- Per-step record: decisions taken, deviations from plan, surprises encountered
- Per-step critique subagent findings: severity-tagged with dispositions (addressed before exit / noted for Phase 5)
- Success criteria verification: each criterion from Phase 1 with pass/fail status, its target and achieved rigour level with evidence, and — when achieved is below target — an explicit upgrade path. Rigour ladder: `proven ▸ evidenced ▸ conjectured` (or the project override); rung definitions are in the problem-statement / Phase 1 step 2.
- Handoff footer for Phase 5

## Work-unit context
This plan implements work unit code-stage-interface of docs/plans/lsf-improvements-breakdown.md.
Unit entry, verbatim (Phase 4 re-verifies the external gates against these
pins before implementing, recording the outcome in the implementation-summary): see §1 "Unit entry, verbatim" above, and the standing obligations listed after it (Register dispositions: P-C1, P-C2, P-C3, P-C4, P-C8 fabricated ledger SHAs; Design of record: `lsf-workflow.md::model-tiers`; programme-wide Frozen JavaScript and `go.mod` stdlib-only). External gate: `lsf-toolchain` (environment, Go 1.27.1, pinned in `lsf-runtime/go.mod`).
After Phase 5, produce the completion-report (contents per the
phronithm:feature skill's Produces) at docs/plans/code-stage-interface-completion.md,
then present this continuation prompt to the requester:
Run `/phronithm:breakdown` in re-plan mode with docs/plans/lsf-improvements-breakdown.md and docs/plans/code-stage-interface-completion.md before starting any dependent unit.
