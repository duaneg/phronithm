package orchestrator

import (
	"strconv"
	"strings"

	"github.com/duaneg/phronithm/lsf-runtime/plan"
)

// The stage prompts, ported from implement.js. tools/promptparity checks
// that they render byte-identical to the JS for the same inputs.

// planRef is the plan a prompt reads: its path at a SHA.
type planRef struct{ path, sha string }

func itoa(n int) string { return strconv.Itoa(n) }

// jsonIndent is JSON.stringify(v, null, 2).
func jsonIndent(v any) string {
	b, err := plan.MarshalJS(v, "  ")
	if err != nil {
		panic(err) // the prompt types always marshal
	}
	return string(b)
}

func jsonOrNone(n int, v any) string {
	if n == 0 {
		return "none"
	}
	return jsonIndent(v)
}

const returnFields = "\nBesides `result`, return `commits`: every commit you made, branch and SHA; an empty array when none. A SHA is what `git rev-parse --short HEAD` prints after the commit, copied from that output. Nothing else: no questions or remarks outside the fields named. The return is one flat object."

// pauseClause: the stages that run in the main checkout at a wave boundary
// report the pause ref; the loop acts on it at the loop top.
func pauseClause(c *runCtx) string {
	return "Run `git show-ref --verify --quiet " + c.pauseRef + " && echo present || echo absent` and return `pauseRequested` true exactly when it prints `present`. Do not create, delete, or read the ref beyond that."
}

// commitMessage is the commit message form for the append stages: the
// subject alone on the first line, a blank line, then any trailers.
func commitMessage(subject string) string {
	return "The commit message is the subject line \"" + subject + "\", then a blank line, then any trailer lines; nothing on the subject line but the subject."
}

// ownFilesOnly: the main checkout is shared with the requester, who may
// edit it while the run is live. A stage there commits only the files it
// wrote, by name, so an edit it did not make is never swept into its commit
// or discarded.
func ownFilesOnly(what string) string {
	return "The main checkout may hold uncommitted changes that are not yours: the requester can edit it while this run is live. Commit only the files this stage wrote (" + what + "), naming them: `git add -- <paths>`, then `git commit -m <message> -- <paths>`, options before the `--`, whose pathspec keeps everything else out of the commit. If the commit fails, return an empty SHA; never retry it without the pathspec. Never run `git add -A`, `git add .`, `git add -u`, `git commit -a`, `git stash`, `git clean` or `git reset --hard` in the main checkout, and never stage, revert, or discard a change you did not make."
}

func header(c *runCtx, p planRef) string {
	decisionLog := "none"
	if c.decisionLogSha != "" {
		decisionLog = c.decisionLogPath + " @ " + c.decisionLogSha
	}
	return strings.Join([]string{
		"Feature: " + c.name,
		"Decomposition plan: " + p.path + " @ " + p.sha,
		"Architecture document: " + c.architectureDoc + " @ " + c.architectureSha.or("HEAD"),
		"Request record: " + c.requestRecord + " (the architecture brief; its requirements and success criteria)",
		"Decision log: " + decisionLog,
		"Integration branch: " + c.integrationBranch,
		"Read documents at the stated SHA (`git show <sha>:<path>`), not the working tree.",
	}, "\n")
}

// entrySemantics is what the unit entry's fields mean to a stage. Scope is
// read by the scheduler for overlap between units in flight; it is not a
// permission list, and a stage that treats it as one reports a defect that
// is not there.
const entrySemantics = "The entry's Scope line declares the files this unit writes that another unit in flight might also write; the scheduler reads it for overlap. It does not limit what the work may edit. A file the work needs to change outside the Scope line is changed, and reported as such where a stage asks for it — never treated as a question. A clause of the entry or of the architecture document that admits more than one reading, or names something that does not exist as named (a file, a test, a function), is read the plainest way consistent with the code; the unit plan records the reading and later stages hold to it."

// longGates: a gate past the Bash tool's limit moves to the background, and
// an agent that ends its turn waiting returns with the gate unrun
// (postmortem §D). The status file sits under the git directory, outside
// the work tree.
const longGates = "A gate can outlast the Bash tool's time limit, which moves the command to the background; ending your turn while it runs returns this stage with the gate unrun. Run each gate detached unless you know it finishes well inside a minute, one at a time, numbering the gates 1, 2, … in the order you report them, with `<command>` the gate's full command line: `D=$(git rev-parse --absolute-git-dir); rm -f \"$D\"/lsf-gate-<n>.*; ( { <command>; } > \"$D/lsf-gate-<n>.log\" 2>&1; echo $? > \"$D/lsf-gate-<n>.rc\" ) </dev/null >/dev/null 2>&1 & echo $! > \"$D/lsf-gate-<n>.pid\"`. Then wait for it with the Bash tool's timeout set to 600000 ms: `D=$(git rev-parse --absolute-git-dir); P=$(cat \"$D/lsf-gate-<n>.pid\"); timeout 540 sh -c \"until [ -f '$D/lsf-gate-<n>.rc' ] || ! kill -0 $P 2>/dev/null; do sleep 15; done\"`. Exit status 124 means the gate is still running: make the same call again. Otherwise read `lsf-gate-<n>.rc`: 0 is `green`, any other value `red`, with the summary from the end of `lsf-gate-<n>.log`; no `.rc` file means the gate died, and it is `not-run`. Report a gate `not-run`, with why in its summary, only when it could not be completed; the run stops on a gate not run, and the requester decides."

// fixTail is the fix stage's subject-specific instructions.
func fixTail(u *plan.Unit, subject string, gates []gateRun, decl *plan.Declaration) string {
	if subject == "plan" {
		return "The plan is docs/plans/" + u.ID + "-plan.md on this branch. Repair the findings; do not shorten the plan to length."
	}
	if u.Type == "design" {
		return "Run the tests the fix touches as you go, then the project's gates once at the end; a fix that breaks a test is not applied."
	}
	if decl != nil {
		return "Run the tests the fix touches as you go; a fix that breaks a test is not applied. " + declaredGatesClause(decl)
	}
	cmds := []string{}
	for _, g := range gates {
		cmds = append(cmds, "- `"+g.Command+"`")
	}
	return "Run the tests the fix touches as you go; a fix that breaks a test is not applied. Then run the project's gates once at your final HEAD — every gate the implement stage ran, below, with its command unchanged, not only those the fix touches:\n" +
		strings.Join(cmds, "\n") + "\nand report each under `gates` with its command, status, and a one-line summary; the next review round reads these, not the implement stage's. " + longGates
}

// declaredGatesClause replaces a code stage's gate instructions when the
// plan declares its gates (D27).
func declaredGatesClause(decl *plan.Declaration) string {
	cmds := []string{}
	for _, g := range decl.Gates {
		cmds = append(cmds, "- "+g.Name+": `"+g.Command+"`")
	}
	return "The runtime runs the project's declared gates on a clean checkout of the commit you return, so they see only what is committed, and the review stages read its results:\n" + strings.Join(cmds, "\n") +
		"\nDo not run them as a final check (running tests while you work is your own business), and return `gates` as an empty array."
}

// gatesRun is the gate runs the reviewers read instead of re-running the
// suite (which in a compiled project cost more of their wall-clock than
// the review itself): the stage that ran them, and the runs.
type gatesRun struct {
	by    string
	gates []gateRun
}

func renderGates(gates []gateRun) string {
	if len(gates) == 0 {
		return "- none reported"
	}
	out := []string{}
	for _, g := range gates {
		line := "- `" + g.Command + "` — " + g.Status
		if g.Summary != "" {
			line += ": " + g.Summary
		}
		if g.Tail != "" {
			fence := "```"
			for strings.Contains(g.Tail, fence) {
				fence += "`"
			}
			tail := strings.ReplaceAll(strings.TrimRight(g.Tail, "\n"), "\n", "\n  ")
			line += "\n\n  " + fence + "\n  " + tail + "\n  " + fence
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// completionPath is where a unit's completion report goes; the report is
// rendered from the implement and review returns, and a copy stage commits
// it on the unit branch.
func completionPath(u *plan.Unit) string {
	return "docs/plans/" + u.ID + "-completion.md"
}

// amendedClause: the pin has already moved to the merged text, so a unit's
// own change to the document is judged against the text it was merged onto
// and cannot become its own baseline. Its merge commit against the first
// parent leaves the requester's commits out.
func amendedClause(c *runCtx, merged []mergedUnit, prevArch optStr) string {
	if prevArch.Equal(c.architectureSha) {
		return ""
	}
	doc := c.architectureDoc
	fallback := "no baseline either; judge its change from its completion report"
	if prevArch.v != "" {
		fallback = "`git diff " + prevArch.v + " " + c.architectureSha.v + " -- " + doc + "` holds its change among any others, judged against the document @ " + prevArch.v + "; attributing them is yours"
	}
	diffs := []string{}
	for _, m := range merged {
		d := "merge SHA not reported; " + fallback
		if m.mergeSha != "" {
			d = "`git diff " + m.mergeSha + "^1 " + m.mergeSha + " -- " + doc + "`"
		}
		diffs = append(diffs, "- "+m.unit+" ("+m.typ+"): "+d)
	}
	from := prevArch.v
	if from == "" {
		from = "an unrecorded pin"
	}
	return "\nThe architecture document changed during this wave, from " + from + " to the SHA above. Each merged unit's own change to it is listed below; an empty diff means the unit did not touch it, and changes outside these diffs are the requester's own and are not judged. Judge each unit's change against the text it was merged onto (the diff's first commit), which includes changes merged before it this wave, reading that unit's completion report first. A change that records the decision a design unit was charged to settle, that settles a question the document left open to the unit that made it, or that corrects wording, goes under `amendments`: the unit, the section, and one clause on what changed. A change that alters what a section decided beyond that is an invalidation, all five fields filled: `section`; `decision`, what the section decided before the change; `premise`, what that decision rested on; `evidence`, the merge SHA and the lines of the diff; `amendment`, the text the unit wrote. Report each change once: an invalidation for a unit's change is not reported again as a premise invalidation of the amended text.\n" + strings.Join(diffs, "\n") + "\n"
}

func mergedUnits(merged []mergedUnit) []string {
	out := []string{}
	for _, m := range merged {
		out = append(out, m.unit)
	}
	return out
}

func mergedReports(merged []mergedUnit) []string {
	out := []string{}
	for _, m := range merged {
		out = append(out, m.completionPath)
	}
	return out
}

// maintenanceForm is the compact Plan Maintenance entry a re-plan writes,
// and the Risk Registry archive rule: the plan records what changed and
// where, never restated statements. heading is the text after the date.
func maintenanceForm(c *runCtx, heading string) string {
	return "Record the pass in `## Plan Maintenance` as one entry: the heading `### <date, date -I> — " + heading + "`, then one line per amendment in the form `<kind> <unit> — <one clause>`, and nothing else — no restated items, reports, or decisions; the ledger holds those. A Risk Registry entry this pass closes — its risk retired by a completion or an amendment — moves to " + c.archivePath + " under `## Closed risks`, cited by id and one clause; a risk that stands keeps its id and one clause. Parallel Streams, Critical Path, and Dependency Graph sections are not maintained: the script computes readiness and the wave plan from the ```yaml lsf-plan block."
}

// replanBoundary is what the re-plan writes, and where an edit anywhere
// else goes instead.
func replanBoundary(subject string) string {
	return "The re-plan writes two documents: the plan, and under `document-text` the architecture document. " + subject + " whose remedy is text elsewhere — a comment or docstring, a figure in a completion report or the debt register, a guide sentence, a register row to file — is not made by the re-plan: where a remaining unit's Scope covers the file, amend that unit's done condition (`amended`); where none does and the edit changes no behaviour and no test, return it under `directEdits` (path, what to change) with the verdict `amended` — a scribe stage makes and commits it after this re-plan and the ledger records each; where it changes behaviour or a test, add a unit (`unit-added`)."
}

// stopItem is a stop signal as the re-plan prompt lists it.
type stopItem struct {
	Kind      string `json:"kind"`
	Unit      string `json:"unit,omitempty"`
	Stage     string `json:"stage"`
	Statement string `json:"statement"`
	Referent  string `json:"referent"`
}

// stopsBlock tells the re-plan which signals the run stops on.
func stopsBlock(stops []*plan.Signal) string {
	if len(stops) == 0 {
		return ""
	}
	items := []stopItem{}
	for _, s := range stops {
		items = append(items, stopItem{s.Kind, s.Unit, s.Stage, s.Statement, s.Referent})
	}
	return "\nThe run stops after this re-plan on the signals below; they go to the requester. Do not amend the plan for them — no edge, scope, or order change on their account — and leave every unit they name as it is: its branch is unmerged and it stays incomplete. Absorb the merged units' reports as above.\n" + jsonIndent(items) + "\n"
}

func worktree(base string) string {
	return "You are running in a git worktree of the repository. Before anything else run `git reset --hard " + base + " && git clean -fd` so the worktree matches that branch exactly, with no leftover untracked files from an earlier attempt. Make all changes and commits here; never touch other worktrees or the main checkout. Report your branch (`git rev-parse --abbrev-ref HEAD`) and HEAD SHA in `result`."
}

// unitBlock gives the unit entry and the obligations naming the unit by
// extraction command rather than a read of the whole plan: the plan is
// large and the stage needs one section of it. The prompt embeds the SHA,
// so the commands are stable across a wave.
func unitBlock(p planRef, u *plan.Unit) string {
	return "Work unit: " + u.ID + " (type: " + u.Type + ")\nUnit entry: the `### " + u.ID + "` section under `## Work Units` in " + p.path + " @ " + p.sha + ". Read it first, with this command verbatim, and take it as requirement text:\n`git show " + p.sha + ":" + p.path + " | sed -n '/^### " + u.ID + "$/,/^##/p'`\nStanding obligations naming the unit, with this command:\n`git show " + p.sha + ":" + p.path + " | sed -n '/^## Standing Obligations/,/^## /p' | grep -n '" + u.ID + "'`\nDo not read the rest of the plan; the entry and the obligations are what this stage needs from it. " + entrySemantics
}

func unitPlanPrompt(c *runCtx, p planRef, u *plan.Unit) string {
	return header(c, p) + "\n" + worktree(c.integrationBranch) + "\n\n" + unitBlock(p, u) + "\n\nWrite the implementation plan for this unit. The architecture document is the primary design input; the unit's done condition, adopted verbatim, is a success criterion; each standing obligation naming the unit is a requirement. Do not re-explore what the architecture document and unit entry settle. Keep it within the unit's size budget: about six steps, at most one open design decision. The plan is read whole by every critique round and by the implement stage; about " + itoa(unitPlanTargetKB) + " KB is the target. Sections, in order:\n\n1. **Goal** — one sentence: what the unit delivers. The unit entry, its done condition, and the standing obligations naming the unit travel with every stage prompt; cite them, do not copy them.\n2. **Context** — the architecture sections relied on, by heading.\n3. **Readings** — only the clauses of the entry or the architecture document that admitted more than one reading, or named something that does not exist as named, each with the reading taken and the evidence for it, one or two lines each. A clause with one plain reading is not listed. Return the same list in `readings`. Take the reading; do not raise a question and do not stop.\n4. **Design** — the decisions the plan makes and why, citing the architecture sections they rest on rather than restating them; the one open design decision at most.\n5. **Steps** — about six, each a heading and one paragraph: what it changes and how it is verified, in an order that keeps the project's gates green after every step.\n6. **Verification** — how the done condition will be evaluated exactly as written, under the readings above.\n\nThe plan and the unit entry are all the implement stage works from; the plan needs nothing else. It carries no \"Remaining phases\" section and mandates no implementation summary document; this workflow's own stages run the plan single-shot, review the result, and write the unit's completion report. Write the whole file to docs/plans/" + u.ID + "-plan.md in one call and commit it. Do not trim it to length afterwards: length past the target is at most a Minor finding for the critique round, never a defect for this stage to repair. Return branch, SHA, path, the file's size in bytes (one `wc -c`, for the record), and the readings; not the plan text.\n" + returnFields
}

func planCritiquePrompt(c *runCtx, p planRef, u *plan.Unit, branch string, round int) string {
	return header(c, p) + "\n\n" + unitBlock(p, u) + "\n\nCritique the implementation plan at docs/plans/" + u.ID + "-plan.md on branch " + branch + " (`git show " + branch + ":docs/plans/" + u.ID + "-plan.md`) with type critique-design. This is round " + itoa(round) + " of the critique gate. Pass bar: no Critical findings. Report Significant and Minor findings; they are recorded, not fixed. The plan's Readings section records how it read clauses that admitted more than one reading; a reading you judge wrong is a finding at the severity its consequence warrants, not a separate remark. Set `systemic` when the findings indicate a structural defect that iteration will not resolve. Continue through to completion — do not stop to wait for instruction.\n" + returnFields
}

func fixPrompt(c *runCtx, p planRef, u *plan.Unit, branch, subject string, criticals []critical, round int, gates []gateRun, decl *plan.Declaration) string {
	return header(c, p) + "\n" + worktree(branch) + "\n\n" + unitBlock(p, u) + "\n\nCritique gate round " + itoa(round) + " found these Critical findings against the " + subject + ":\n\n" + jsonIndent(criticals) + "\n\nFix each finding that is repairable within the " + subject + ", then commit. A finding you decline to fix — because the repair would change the unit entry, the architecture document, or the request record, or because you cannot repair it — goes under `notApplied` with the reason. The run stops on a declined finding and the requester decides, so decline only when the repair is genuinely outside the " + subject + ". " + fixTail(u, subject, gates, decl) + "\n" + returnFields
}

func implementPrompt(c *runCtx, p planRef, u *plan.Unit, planBranch string, decl *plan.Declaration) string {
	gates := "Run the project's gates — the test suite and whatever else the project's own instructions name (linters, type checks) — once at your final HEAD and report each under `gates` with its command, status, and a one-line summary; the review stages take this as the test result and do not re-run the suite. " + longGates
	if decl != nil && u.Type != "design" {
		gates = declaredGatesClause(decl)
	}
	return header(c, p) + "\n" + worktree(planBranch) + "\n\n" + unitBlock(p, u) + "\n\nImplementation plan: docs/plans/" + u.ID + "-plan.md on this branch. Hold to its Readings section.\n\nFirst re-probe every external gate in the unit entry against its plan-time pin: probe environment gates mechanically (binaries and versions, fonts, services, accounts, data sets); organisational and release gates are already cleared per the decision log and need no probe. Report each under `gateProbe`. If any gate has drifted, implement nothing — return the probe results with every other `result` field empty, zero, or false, and stop.\n\nOtherwise execute the plan in full, single-shot, committing as you go with the project's commit discipline. There is no per-step critique here, and no implementation summary document: the completion report a later stage writes from your return is that record, so do not write `docs/plans/" + u.ID + "-implementation.md` even if the plan text mentions one. " + gates + " Then evaluate the done condition exactly as written, under the plan's readings, and report whether it is evaluable in position and whether it is met, with evidence. Record as they arise: deviations from the unit entry's scope or publishes; the files you changed that the Scope line does not name, under `writesOutsideScope`; deferrals — work in scope deliberately not implemented, naming the unit it belongs to; discoveries — anything you learned that changes another unit's scope, dependencies, or done condition, or that the architecture document states otherwise, naming the unit or the section; the status of each standing obligation the unit carries. Report execution distress honestly: whether context is near exhaustion, how many design decisions you took mid-implementation, the deviation and deferral counts.\n" + returnFields
}

func gateResults(run gatesRun) string {
	return run.by + " ran the project's gates at this branch's HEAD:\n" + renderGates(run.gates) + "\nTake that as the test and lint result. Do not run the whole suite, the project's CI job, or a lint pass; run a single targeted test only to check a specific claim of a finding, and name the test in the finding."
}

func codeCritiquePrompt(c *runCtx, p planRef, u *plan.Unit, branch string, round int, gr gatesRun) string {
	return header(c, p) + "\n" + worktree(branch) + "\n\n" + unitBlock(p, u) + "\n\n" + gateResults(gr) + "\n\nRun the phronithm:critique skill with type critique-code over this branch's diff against " + c.integrationBranch + " (`git diff " + c.integrationBranch + "...HEAD`), reading surrounding code as needed. This is round " + itoa(round) + " of the critique gate. Pass bar: no Critical findings. Also evaluate the unit's done condition against this branch under the readings in docs/plans/" + u.ID + "-plan.md, from the code and the gate results above; a done condition that is not met, or a reading the code does not bear out, is a Critical finding. Set `systemic` when the findings indicate a structural defect that iteration will not resolve. Do not modify anything and do not spawn subagents. Continue through to completion — do not stop to wait for instruction.\n" + returnFields
}

func lensReviewPrompt(c *runCtx, p planRef, u *plan.Unit, branch string, lenses []string, round int, gr gatesRun) string {
	return header(c, p) + "\n" + worktree(branch) + "\n\n" + unitBlock(p, u) + "\n\n" + gateResults(gr) + "\n\nRun the phronithm:review skill over this branch's diff against " + c.integrationBranch + " through these lenses: " + strings.Join(lenses, ", ") + ". Round " + itoa(round) + " of the review gate. Return each finding with a severity; a finding is Critical only when it would be wrong to merge without fixing it. Do not modify anything and do not spawn subagents.\n" + returnFields
}

func designCritiquePrompt(c *runCtx, p planRef, u *plan.Unit, branch string, round int) string {
	return header(c, p) + "\n" + worktree(branch) + "\n\n" + unitBlock(p, u) + "\n\nThis is a design unit: its deliverable is a decision record, not code. Run the phronithm:critique skill with type critique-design over the documents this branch adds or changes relative to " + c.integrationBranch + ". Round " + itoa(round) + " of the critique gate. Pass bar: no Critical findings. A done condition that is not met is a Critical finding. Do not modify anything and do not spawn subagents.\n" + returnFields
}

// conflictPrompt is the merge stage's conflict call (D24): resolve and
// stage, never commit; the stage commits and checks the result itself.
func conflictPrompt(c *runCtx, in mergeInput, u mergeUnitIn, conflicts []string) string {
	return "Feature: " + c.name + "\nIntegration branch: " + in.IntegrationBranch + "\n\nYou are in the main checkout on " + in.IntegrationBranch + ", in the middle of `git merge` of unit " + u.Unit + " (commit " + u.Sha + ", its completion report " + u.Report + "). The merge stopped on conflicts in these files:\n" + jsonIndent(conflicts) + "\n\nResolve them only when the resolution is mechanical — both sides' changes apply without judgement about intent — editing each file to hold both and staging it with `git add -- <path>`. Touch no other path. Never commit, and never run `git merge --abort`, `git reset`, `git stash`, `git checkout` or `git clean`: the stage commits a resolution itself, checks it, and aborts the merge if you report it unresolvable. If any file needs judgement, report `unresolvable`, leave the files as they are, and say why in `statement`. Return the status, the conflicted files, and one sentence.\n" + returnFields
}

func gatePrompt(c *runCtx, p planRef, wave int, merged []mergedUnit, prevArch optStr) string {
	return header(c, p) + "\n\nWave " + itoa(wave) + " of Phase 3 has merged these units into " + c.integrationBranch + ": " + strings.Join(mergedUnits(merged), ", ") + ". Their completion reports are at " + strings.Join(mergedReports(merged), ", ") + ".\n\nAnswer one question: do this wave's results invalidate the architecture document at " + c.architectureDoc + " @ " + c.architectureSha.or("HEAD") + " — does any decision the document makes rest on a premise the merged code or the completion reports show to be false? Read the reports, then the merged code the reports and the document point at.\n" + amendedClause(c, merged, prevArch) + "\nFor each such case return an invalidation with all five fields: `section` (the heading), `decision` (what the section decides, quoted or closely paraphrased), `premise` (what the decision rests on), `evidence` (file and line, test, or report line showing the premise false), and `amendment` (what the section should now say). An invalidation stops the run and is put to the requester with the amendment you propose, so fill every field; a case where you cannot say what the section should now say is a note, not an invalidation. A discrepancy that changes no decision — a stale line number, a signature widened compatibly, a count that does not match — is a note at most.\n\nReturn `amendments`: a merged unit's own change to the architecture document that moves no decision, as described above when the document moved; an empty array when it did not. Return under `notes` what the re-plan or the requester should know that is not an invalidation: an open decision whose trigger has fired and which no unit owns; a discrepancy between a document and the code; a deviation or deferral the reports disclose that bears on a unit not yet started. Each note names a referent.\n\nDo not re-evaluate the units' done conditions or review findings: the unit stages did that at the bar this workflow sets (Critical findings fixed, Significant and Minor recorded), and the completion reports carry the outcome. Do not report this workflow's own process as a defect. Do not modify anything.\n" + returnFields
}

// declaredGatesFrozen tells a re-plan the gate block is pinned (D27).
func declaredGatesFrozen(decl *plan.Declaration) string {
	if decl == nil {
		return ""
	}
	return "Do not edit the plan's ```yaml " + plan.GatesTag + " block; a change to it stops the run.\n"
}

func replanPrompt(c *runCtx, p planRef, wave int, merged []mergedUnit, conflicts []plan.Merge, items []replanItem, stops []*plan.Signal, decl *plan.Declaration) string {
	return header(c, p) + "\n\nYou are in the main checkout on " + c.integrationBranch + ". Run the phronithm:breakdown skill in re-plan mode over " + p.path + " with the completion reports of wave " + itoa(wave) + ": " + strings.Join(mergedReports(merged), ", ") + ". Diff their deviations, deferrals, and discoveries against every remaining unit's scope, dependencies, and done condition; amend the plan; re-run the quality check over the remaining units, examining the size budget first where a completed unit showed distress. " + maintenanceForm(c, "re-plan wave "+itoa(wave)+" (<item ids>)") + " Treat each merge conflict below between units the plan claimed disjoint as a plan defect: record it and add the missing edge or scope correction. A file a unit wrote outside its Scope line is a Scope correction where a later unit shares the file.\n\nItems this wave, each with an id — deferrals, discoveries, claimed-disjoint conflicts, the gate's notes, and distress. Each is yours to absorb: amend the plan (a Scope or Publishes line, a done condition, an edge, the execution order, a unit added or removed), or show the plan as it stands already answers it. Return a verdict for every id: `amended`, `no-change` (with what in the plan answers it), `document-text`, or `cannot` (with why the plan cannot absorb it — the answer changes a decision in the architecture document or the request record, or needs the requester). An id you do not answer is treated as `cannot` and stops the run.\n\n`document-text` is for a gate note whose only remedy is wording in the architecture document and that moves no decision — a resolved unknown still marked open, a measured figure replacing an estimate, a named accessor or signature the code settled. Make that correction in the document, commit it, and put the section and the new wording in the verdict statement. A correction that would change what a section decides is not text; that is `cannot`.\n\n" + replanBoundary("An item") + "\n" + jsonOrNone(len(items), items) + "\n\nMerge conflicts: " + jsonOrNone(len(conflicts), conflicts) + "\n" + stopsBlock(stops) + "\nEvery amendment to a unit's type, dependencies, scope, publishes, gates, or completion, and to the execution order, is made in both the unit's entry and the plan's ```yaml lsf-plan block, which must agree; the script reads the block. Commit the amended plan on " + c.integrationBranch + ". " + ownFilesOnly(p.path+", and "+c.architectureDoc+" if you made a `document-text` correction") + " Return the new plan SHA; each amendment with its kind (" + strings.Join(plan.AmendmentKinds, ", ") + "), unit, and statement; a verdict per item id; `directEdits` (an empty array when none); and every remaining unit over its size budget with its disposition: `extracted` (a preceding design unit now carries the excess), `flagged` (the excess is wall-clock, not context, and the unit carries the rationale — breakdown's size budget discharges it), or `unrepaired`. A flag already on the unit from an earlier pass stands unless the unit's scope has grown since; report it as `flagged`. Do not answer whether the architecture document still stands — the gate did. Do not select or execute any unit.\n" + declaredGatesFrozen(decl) + returnFields
}

func decisionReplanPrompt(c *runCtx, p planRef, decisions []decisionItem) string {
	ids := []string{}
	for _, d := range decisions {
		ids = append(ids, d.ID)
	}
	return header(c, p) + "\n\nYou are in the main checkout on " + c.integrationBranch + ". The requester has answered the questions the last run stopped on; the answers are in the decision log at the SHA above. Run the phronithm:breakdown skill in re-plan mode over " + p.path + " with these decisions as the input: apply each to the plan (a Scope or Publishes line, a done condition, an edge, the execution order, a unit added or removed), and re-run the quality check over the remaining units. " + maintenanceForm(c, "decisions "+strings.Join(ids, ", ")) + " Return a verdict for every decision id: `amended`, `no-change` (with what in the plan already reflects it), `document-text` (the decision only corrects wording in the architecture document; make the correction, commit it, and state the section), or `cannot` (with why the plan cannot carry it out — it changes a decision in the architecture document or the request record, or is not a plan matter). An id you do not answer is treated as `cannot` and stops the run. Report any remaining unit over its size budget with its disposition — `extracted`, `flagged` (a flag with wall-clock rationale, standing or new), or `unrepaired`.\n\n" + replanBoundary("A decision") + "\n" + jsonIndent(decisions) + "\n\nEvery amendment to a unit's type, dependencies, scope, publishes, gates, or completion, and to the execution order, is made in both the unit's entry and the plan's ```yaml lsf-plan block, which must agree; the script reads the block. Commit on " + c.integrationBranch + ". " + ownFilesOnly(p.path+", and "+c.architectureDoc+" if you made a `document-text` correction") + " Return the new plan SHA (the SHA given when nothing changed), each amendment with its kind (" + strings.Join(plan.AmendmentKinds, ", ") + "), unit, and statement, a verdict per decision id, and `directEdits` (an empty array when none). Do not select or execute any unit.\n" + returnFields
}

func terminationPrompt(c *runCtx, p planRef) string {
	return header(c, p) + "\n\nYou are in the main checkout on " + c.integrationBranch + ". Every work unit in " + p.path + " is complete. Read its Meta-Tasks and Standing Obligations sections. Turn each substantial meta-task (integration testing, tuning infrastructure, anything needing its own implementation plan) into a work unit entry of type meta with a verifiable done condition, scope, dependencies, and gates, following phronithm:breakdown's unit conventions; append these to the plan's Work Units, to its execution order, and to its ```yaml lsf-plan block (entry and block must agree; the script reads the block), record the amendment in Plan Maintenance, and commit. " + ownFilesOnly(p.path) + " List each small meta-task (documentation updates and the like) under `directEdits` with the path and what to change; do not make those edits. Return the new plan SHA and the ids of the appended units. If no meta-task remains, return the plan SHA given, unchanged.\n" + returnFields
}

func directEditsPrompt(c *runCtx, p planRef, what string, edits []directEdit) string {
	return header(c, p) + "\n\nYou are in the main checkout on " + c.integrationBranch + ". Make these small edits, which the " + what + " returned as outside the plan; commit them with the project's commit discipline, and return the path of the plan and the final commit SHA. " + ownFilesOnly("the paths the edits below name") + "\n\n" + jsonIndent(edits) + "\n" + returnFields
}

func obligationsPrompt(c *runCtx, p planRef) string {
	return header(c, p) + "\n\nEvery work unit and meta-task in " + p.path + " is complete. For each standing obligation in the plan's Standing Obligations section, return its disposition with the evidence, from the completion reports and " + c.integrationBranch + ": `discharged` — every carrying unit did what the obligation asks, cited; `carried` — the obligation's own text places its discharge outside this plan, and `carrier` names the file and row or section where it is now recorded outside this plan (a debt-register row, another plan's Standing Obligations); `undischarged` — neither. An obligation recorded nowhere outside this plan is `undischarged` whatever its text says. Do not modify anything.\n" + returnFields
}
