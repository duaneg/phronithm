// Spike only (substrate-spike, S2): a host runtime for run(runtime) in
// ../implement.js. agent() is a headless `claude -p` call; the mechanical
// stages (load, ledger, completion report, merge, cleanup) are code here,
// dispatched on the call's label, so implement.js runs unchanged.
//
// usage: node driver.js <state-dir>
//   <state-dir>/config.json: { repo, args, gateCommands, faults, concurrency }
// Re-running the same command resumes: a call whose label and prompt hash
// match a cached return is not re-run; code steps are idempotent instead.
// A file named KILL in the state dir stops the run before the next call.
import { spawn, execFileSync } from 'node:child_process';
import { createHash, randomUUID } from 'node:crypto';
import { appendFileSync, existsSync, mkdirSync, readFileSync, writeFileSync, rmSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { run } from '../implement.js';
import { ledgerHeading } from '../scheduling.js';
import { extractBlock, parseDecisions } from '../graph.js';
import { validate } from './validate.js';

const stateDir = resolve(process.argv[2] ?? '');
const config = JSON.parse(readFileSync(join(stateDir, 'config.json'), 'utf8'));
const repo = resolve(config.repo);
const args = config.args;
const faults = config.faults ?? []; // [{ at: <label prefix>, kind, once?, ... }]
const gateCommands = config.gateCommands ?? [];
for (const d of ['calls', 'wt']) mkdirSync(join(stateDir, d), { recursive: true });

// The orchestrator's path defaults, which the code stages need.
const ctx = {
  name: args.name,
  integrationBranch: args.integrationBranch,
  architectureDoc: args.architectureDoc,
  planPath: args.planPath ?? `docs/plans/${args.name}-breakdown.md`,
  decisionLogPath: args.decisionLogPath ?? `docs/plans/${args.name}-decisions.md`,
  ledgerPath: args.ledgerPath ?? `docs/plans/${args.name}-signals/${args.planSha}.md`,
  archivePath: args.archivePath ?? `docs/plans/${args.name}-archive.md`,
  pauseRef: args.pauseRef ?? `refs/lsf/${args.name}/pause`,
};

// ---------------------------------------------------------------- utilities

const now = () => new Date().toISOString();
const log = (msg) => { const line = `${now()} ${msg}`; console.log(line); appendFileSync(join(stateDir, 'log.txt'), `${line}\n`); };
const event = (e) => appendFileSync(join(stateDir, 'telemetry.jsonl'), `${JSON.stringify({ at: now(), ...e })}\n`);
const setStatus = (s) => writeFileSync(join(stateDir, 'status.json'), JSON.stringify({ pid: process.pid, at: now(), ...s }, null, 2));
const sha256 = (s) => createHash('sha256').update(s).digest('hex');
const slug = (label) => label.replace(/[^A-Za-z0-9._-]+/g, '_');
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

function git(cwd, ...a) {
  return execFileSync('git', a, { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trimEnd();
}
function gitOk(cwd, ...a) {
  try { return { ok: true, out: git(cwd, ...a) }; } catch (e) { return { ok: false, out: `${e.stdout ?? ''}${e.stderr ?? ''}` }; }
}
const pauseRequested = () => gitOk(repo, 'show-ref', '--verify', '--quiet', ctx.pauseRef).ok;
// A `once` fault stays fired across restarts (state dir), so a resume after
// an injected kill does not kill again.
const firedPath = join(stateDir, 'faults-fired.json');
const fired = new Set(existsSync(firedPath) ? JSON.parse(readFileSync(firedPath, 'utf8')) : []);
const faultId = (f) => `${f.at}/${f.kind}`;
const fault = (label, kind) => faults.find((f) => label.startsWith(f.at) && f.kind === kind && !(f.once && fired.has(faultId(f))));
const fire = (f) => {
  if (f) { fired.add(faultId(f)); writeFileSync(firedPath, JSON.stringify([...fired])); event({ kind: 'fault-injected', fault: f.kind, at: f.at }); }
  return f;
};
function killSelf(why) {
  log(`FAULT: SIGKILL self (${why})`);
  writeFileSync(join(stateDir, 'killed-at.txt'), why);
  process.kill(process.pid, 'SIGKILL');
}

// Children of a killed driver keep running (run 4: the implement agent
// outlived a SIGKILLed driver and raced its resumed successor in the same
// worktree). Live children are recorded; a launch first terminates any left
// by its predecessor, matched by pid and session id.
const childrenPath = join(stateDir, 'children.json');
const children = new Map();
const saveChildren = () => writeFileSync(childrenPath, JSON.stringify([...children.entries()]));
function reapOrphans() {
  if (!existsSync(childrenPath)) return;
  for (const [pid, sessionId] of JSON.parse(readFileSync(childrenPath, 'utf8'))) {
    let cmd = '';
    try { cmd = readFileSync(`/proc/${pid}/cmdline`, 'utf8'); } catch { continue; }
    if (!cmd.includes(sessionId)) continue;
    process.kill(pid, 'SIGTERM');
    event({ kind: 'orphan-terminated', pid, sessionId });
    log(`terminated orphaned call pid ${pid} (session ${sessionId}) left by a previous launch`);
  }
  writeFileSync(childrenPath, '[]');
}

class KillError extends Error {}
const checkKill = () => { if (existsSync(join(stateDir, 'KILL'))) throw new KillError('KILL file present'); };

// A concurrency cap over headless calls.
let active = 0;
const waiters = [];
async function acquire() {
  while (active >= (config.concurrency ?? 4)) await new Promise((r) => waiters.push(r));
  active += 1;
}
function release() { active -= 1; waiters.shift()?.(); }

// ---------------------------------------------------------------- worktrees

const branchPrefix = `lsf/${String(args.planSha).slice(0, 8)}`;
function ensureWorktree(key, base = args.integrationBranch) {
  const path = join(stateDir, 'wt', key);
  const branch = `${branchPrefix}/${key}`;
  if (!existsSync(path)) git(repo, 'worktree', 'add', '-B', branch, path, base);
  return { path, branch };
}

// ---------------------------------------------------------------- declared gates

// The driver runs the declared gate commands itself, on a clean detached
// checkout of the reported commit, so uncommitted changes in the agent's
// worktree cannot turn a gate green. The agent's report is kept for
// comparison only.
function runGatesAt(sha, key) {
  const path = join(stateDir, 'wt', `gates_${key}`);
  gitOk(repo, 'worktree', 'remove', '--force', path);
  git(repo, 'worktree', 'add', '--detach', path, sha);
  try { return runGates(path); } finally { gitOk(repo, 'worktree', 'remove', '--force', path); }
}
function runGates(cwd) {
  return gateCommands.map((command) => {
    const r = gitless(command, cwd);
    return { command, status: r.code === 0 ? 'green' : 'red', summary: r.tail };
  });
}
function gitless(command, cwd) {
  try {
    const out = execFileSync('sh', ['-c', command], { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'], timeout: 30 * 60 * 1000 });
    return { code: 0, tail: out.trim().split('\n').slice(-1)[0] ?? '' };
  } catch (e) {
    return { code: e.status ?? 1, tail: `${e.stdout ?? ''}${e.stderr ?? ''}`.trim().split('\n').slice(-1)[0] ?? '' };
  }
}

// ---------------------------------------------------------------- headless calls

// The orchestrator's prompts require compound shell commands (`&&`, pipes,
// variable assignments, `$(…)`), which per-command prefix allow rules
// cannot express (run 1: every implement Bash call denied). Bash is granted
// whole; deny rules, matched per subcommand, carry the restrictions.
const DENY_COMMON = ['Bash(git push*)', 'Bash(git worktree *)', 'Bash(git branch -D*)', 'Bash(git branch -d*)', 'Bash(git config*)', 'Bash(git update-ref*)', 'Bash(rm -rf*)',
  'Edit(.git/**)', 'Write(.git/**)', 'Edit(.claude/**)', 'Write(.claude/**)'];
const ROLE_TOOLS = {
  critique: { tools: 'Read,Grep,Glob,Skill,Bash', allow: ['Skill', 'Read', 'Grep', 'Glob', 'Bash'], deny: [...DENY_COMMON, 'Bash(git commit*)', 'Bash(git merge*)', 'Bash(git rebase*)'] },
  worker: { tools: 'default', allow: ['Read', 'Edit', 'Write', 'Glob', 'Grep', 'Skill', 'Bash'], deny: DENY_COMMON },
};
const roleOf = (opts) => (opts.agentType === 'phronithm:critic' || /^(critique-|lens-review|review-gate)/.test(opts.label) ? 'critique' : 'worker');

// A prompt that names a skill is sent as that skill's slash command, which
// loads the skill body deterministically. Headless calls given the prompt
// alone ran the skill in 2 of 4 critique-code calls (run 2 and replays);
// Workflow agents in 395 of 397.
const SKILL_BY_LABEL = [[/^(critique-plan|critique-code|critique-design)/, 'phronithm:critique'], [/^lens-review/, 'phronithm:review'], [/^(replan|decision-replan)/, 'phronithm:breakdown']];
const withSkill = (label, prompt) => {
  const skill = SKILL_BY_LABEL.find(([re]) => re.test(label))?.[1];
  return skill ? `/${skill} ${prompt}` : prompt;
};

function headless(prompt, opts, cwd, key, attempt) {
  const role = ROLE_TOOLS[roleOf(opts)];
  prompt = withSkill(opts.label, prompt);
  const sessionId = randomUUID();
  const a = ['-p', prompt, '--output-format', 'stream-json', '--verbose', '--session-id', sessionId,
    '--model', opts.model ?? 'opus', ...(opts.effort ? ['--effort', opts.effort] : []),
    '--strict-mcp-config', '--permission-mode', 'dontAsk', '--tools', role.tools,
    '--allowedTools', ...role.allow, '--disallowedTools', ...role.deny, ...(fire(fault(opts.label, 'deny'))?.rules ?? []),
    '--json-schema', JSON.stringify(opts.schema)];
  const transcript = join(stateDir, 'calls', `${key}.a${attempt}.jsonl`);
  const failInject = fire(fault(opts.label, 'fail-call'));
  if (failInject) return Promise.resolve({ code: 1, result: { is_error: true, api_error_status: 529, subtype: 'error_injected' }, sessionId, transcript });
  return new Promise((res) => {
    const p = spawn(process.env.LSF_CLAUDE_BIN ?? 'claude', a, { cwd, stdio: ['ignore', 'pipe', 'pipe'] });
    event({ kind: 'spawn', label: opts.label, pid: p.pid, sessionId, cwd });
    children.set(p.pid, sessionId); saveChildren();
    const killMid = fire(fault(opts.label, 'kill-mid-stage'));
    if (killMid) setTimeout(() => killSelf(`mid-stage ${opts.label}, child pid ${p.pid}`), killMid.afterMs ?? 15000);
    let buf = '';
    let result = null;
    let stderr = '';
    p.stdout.on('data', (d) => {
      appendFileSync(transcript, d);
      buf += d;
      let i;
      while ((i = buf.indexOf('\n')) >= 0) {
        const line = buf.slice(0, i); buf = buf.slice(i + 1);
        try { const e = JSON.parse(line); if (e.type === 'result') result = e; } catch {}
      }
    });
    p.stderr.on('data', (d) => { stderr += d; });
    p.on('close', (code) => { children.delete(p.pid); saveChildren(); }).on('close', (code) => res({ code, result, sessionId, transcript, stderr: stderr.slice(-500) }));
  });
}

const TRANSIENT_BACKOFF_MS = [5000, 20000, 60000];
async function callWithRetry(prompt, opts, cwd, key) {
  for (let attempt = 0; ; attempt++) {
    const t0 = Date.now();
    const r = await headless(prompt, opts, cwd, key, attempt);
    const res = r.result ?? {};
    const so = res.structured_output ?? null;
    const errors = so ? validate(opts.schema, so) : ['no structured_output'];
    event({
      kind: 'call', label: opts.label, attempt, code: r.code, secs: (Date.now() - t0) / 1000, sessionId: r.sessionId,
      model: opts.model, effort: opts.effort, subtype: res.subtype, is_error: res.is_error, api_error_status: res.api_error_status,
      cost: res.total_cost_usd, usage: res.usage && { in: res.usage.input_tokens, cc: res.usage.cache_creation_input_tokens, cr: res.usage.cache_read_input_tokens, out: res.usage.output_tokens },
      denials: (res.permission_denials ?? []).map((d) => `${d.tool_name}: ${JSON.stringify(d.tool_input).slice(0, 160)}`),
      valid: errors.length === 0, errors: errors.slice(0, 3), stderr: r.code ? r.stderr : undefined,
    });
    if (errors.length === 0) return so;
    const transient = r.code !== 0 || res.is_error || res.api_error_status;
    // A missing or invalid structured output after a clean exit is retried once.
    const limit = transient ? TRANSIENT_BACKOFF_MS.length : 1;
    if (attempt >= limit) {
      log(`${opts.label}: no valid return after ${attempt + 1} attempt(s) (${transient ? 'transient' : errors[0]}); null`);
      return null;
    }
    const wait = transient ? TRANSIENT_BACKOFF_MS[attempt] : 0;
    log(`${opts.label}: attempt ${attempt + 1} failed (${transient ? `exit ${r.code}, ${res.api_error_status ?? res.subtype}` : errors[0]}); retrying in ${wait / 1000}s`);
    await sleep(wait);
  }
}

// A returned SHA is checked against git before the orchestrator uses it:
// result.sha must be the reported branch's tip; each commits[] entry must
// exist. A mismatch is corrected from git and recorded.
function verifyShas(label, ret) {
  const r = ret.result ?? {};
  if (typeof r.branch === 'string' && r.branch && 'sha' in r) {
    const tip = gitOk(repo, 'rev-parse', '--verify', `${r.branch}^{commit}`);
    if (!tip.ok) { event({ kind: 'sha-check', label, verdict: 'branch-missing', branch: r.branch }); return null; }
    const claimed = r.sha ? gitOk(repo, 'rev-parse', '--verify', `${r.sha}^{commit}`) : { ok: false };
    if (!claimed.ok || claimed.out !== tip.out) {
      event({ kind: 'sha-check', label, verdict: 'corrected', claimed: r.sha, actual: tip.out.slice(0, 12) });
      log(`${label}: returned SHA ${r.sha || '(empty)'} is not ${r.branch}'s tip; corrected to ${tip.out.slice(0, 12)}`);
      r.sha = tip.out.slice(0, 12);
    }
  }
  const commits = ret.commits ?? [];
  const real = commits.filter((c) => c.sha && gitOk(repo, 'cat-file', '-e', `${c.sha}^{commit}`).ok);
  if (real.length !== commits.length) {
    event({ kind: 'sha-check', label, verdict: 'commits-dropped', dropped: commits.filter((c) => !real.includes(c)).map((c) => c.sha) });
    ret.commits = real;
  }
  for (const k of ['planSha']) {
    if (typeof r[k] === 'string' && r[k] && !gitOk(repo, 'cat-file', '-e', `${r[k]}^{commit}`).ok) {
      event({ kind: 'sha-check', label, verdict: 'invalid', field: k, claimed: r[k] });
      return null;
    }
  }
  return ret;
}

function injectReturnFaults(label, ret) {
  if (!ret) return ret;
  if (fire(fault(label, 'fabricate-sha'))) {
    const s = ret.result.sha ?? '';
    ret.result.sha = s ? `${s.slice(0, -3)}${s.endsWith('fff') ? '000' : 'fff'}` : 'deadbeef';
    ret.commits = [...(ret.commits ?? []), { branch: ret.result.branch ?? '', sha: 'deadbeefcafe' }];
  }
  if (fire(fault(label, 'drop-gate')) && ret.result.gates?.length) ret.result.gates = ret.result.gates.slice(0, -1);
  return ret;
}

// ---------------------------------------------------------------- code stages

const between = (text, open, close) => {
  const i = text.indexOf(open);
  const j = text.indexOf(close, i + open.length);
  if (i < 0 || j < 0) throw new Error(`markers ${open}…${close} not found`);
  return text.slice(i + open.length, j);
};
const fencedBlock = (markdown, info) => {
  const lines = markdown.split('\n');
  const start = lines.findIndex((l) => l.trim() === `\`\`\`${info}`);
  if (start < 0) return '';
  const end = lines.findIndex((l, i) => i > start && l.trim() === '```');
  return lines.slice(start, end < 0 ? lines.length : end + 1).join('\n');
};
const commitSubject = (prompt) => prompt.match(/subject line "([^"]+)"/)?.[1] ?? 'lsf: commit';

// Commit exactly `paths`, by pathspec; returns the new HEAD or '' when git
// made no commit (a rejected hook, nothing to commit).
function commitPaths(cwd, paths, message) {
  const before = git(cwd, 'rev-parse', 'HEAD');
  const add = gitOk(cwd, 'add', '--', ...paths);
  const c = add.ok && gitOk(cwd, 'commit', '-q', '-m', message, '--', ...paths);
  const after = git(cwd, 'rev-parse', 'HEAD');
  if (!c?.ok || after === before) {
    event({ kind: 'commit-failed', paths, out: (c?.out ?? add.out).slice(0, 300) });
    gitOk(cwd, 'reset', '-q', '--', ...paths); // unstage only what this step staged
    return '';
  }
  return after;
}

// The YAML subset's double-quoted scalar: escapes for `"`, `\`, newline, tab.
const yq = (x) => `"${String(x).replace(/\\/g, '\\\\').replace(/"/g, '\\"').replace(/\n/g, '\\n').replace(/\t/g, '\\t')}"`;
const decisionKey = (d) => JSON.stringify([d.id, d.question, d.decision, d.clears ?? []]);

const CODE = {
  // Decision log: rendered from the decision objects in args, appended inside
  // the fenced block, and parsed back before the commit: the committed log
  // must hold exactly its prior decisions plus these (P-C4: an agent once
  // added a `clears` entry no decision made).
  'decision-log'(prompt) {
    const path = ctx.decisionLogPath;
    const fresh = args.decisions.map((d) => ({ ...d, clears: d.clears ?? [] }));
    const prior = gitOk(repo, 'show', `HEAD:${path}`);
    const text = prior.ok ? `${prior.out}\n` : `# Decisions: ${ctx.name}\n\n\`\`\`yaml lsf-decisions\ndecisions:\n\`\`\`\n`;
    const before = parseDecisions(extractBlock(text, 'lsf-decisions').body);
    if (before.problems.length) throw new Error(`decision log: existing block invalid: ${before.problems[0].statement}`);
    const have = new Set(before.decisions.map(decisionKey));
    const todo = fresh.filter((d) => !have.has(decisionKey(d)));
    if (!todo.length) {
      const sha = git(repo, 'log', '-1', '--format=%h', '--abbrev=12', '--', path);
      return { result: { path, sha }, commits: [] };
    }
    const date = new Date().toISOString().slice(0, 10);
    const entries = todo.map((d) => [`  - id: ${yq(d.id)}`, `    date: ${date}`, `    question: ${yq(d.question)}`, `    decision: ${yq(d.decision)}`,
      ...(d.clears.length ? [`    clears: [${d.clears.map(yq).join(', ')}]`] : [])].join('\n')).join('\n');
    const lines = text.split('\n');
    const open = lines.findIndex((l) => l.trim() === '```yaml lsf-decisions');
    const close = lines.findIndex((l, i) => i > open && l.trim() === '```');
    const next = [...lines.slice(0, close), entries, ...lines.slice(close)].join('\n');
    const after = parseDecisions(extractBlock(next, 'lsf-decisions').body);
    const want = [...before.decisions, ...todo].map(decisionKey);
    if (after.problems.length || JSON.stringify(after.decisions.map(decisionKey)) !== JSON.stringify(want)) {
      throw new Error(`decision log: rendered block does not parse back to the decisions given (${after.problems[0]?.statement ?? 'content differs'})`);
    }
    mkdirSync(join(repo, path, '..'), { recursive: true });
    writeFileSync(join(repo, path), next);
    const sha = commitPaths(repo, [path], commitSubject(prompt));
    return { result: { path, sha: sha.slice(0, 12) }, commits: sha ? [{ branch: args.integrationBranch, sha: sha.slice(0, 12) }] : [] };
  },

  // Copy stage: the blocks are read from git, not transcribed.
  load(prompt) {
    const sha = prompt.match(/Copy stage\. Run `git show ([0-9a-f]+):/)[1];
    const plan = git(repo, 'show', `${sha}:${ctx.planPath}`);
    const dsha = prompt.match(/git show ([0-9a-f]+):\S*decisions/)?.[1];
    return {
      result: {
        planBlock: fencedBlock(plan, 'yaml lsf-plan'),
        decisionsBlock: dsha ? fencedBlock(git(repo, 'show', `${dsha}:${ctx.decisionLogPath}`), 'yaml lsf-decisions') : '',
        headBranch: git(repo, 'rev-parse', '--abbrev-ref', 'HEAD'),
        architectureSha: git(repo, 'rev-list', '-1', args.integrationBranch, '--', args.architectureDoc),
        pauseRequested: pauseRequested(),
      },
      commits: [],
    };
  },

  // Append stage: the new content is the committed content plus the block,
  // checked byte for byte after the commit. A block already at the end of
  // the committed file is not appended again (resume after a kill between
  // the commit and its record).
  ledger(prompt, label) {
    const block = between(prompt, '<<<LEDGER\n', 'LEDGER>>>');
    const path = ctx.ledgerPath;
    const prior = gitOk(repo, 'show', `HEAD:${path}`);
    const old = prior.ok ? `${prior.out}\n` : `${ledgerHeading(ctx.name)}`;
    if (prior.ok && `${prior.out}\n`.endsWith(`\n${block}`)) {
      const sha = git(repo, 'log', '-1', '--format=%h', '--', path);
      event({ kind: 'idempotent-skip', label, path, sha });
      return { result: { path, sha, pauseRequested: pauseRequested() }, commits: [] };
    }
    const next = `${old}\n${block}`;
    mkdirSync(join(repo, path, '..'), { recursive: true });
    writeFileSync(join(repo, path), next);
    const sha = commitPaths(repo, [path], commitSubject(prompt));
    if (sha && fire(fault(label, 'kill-after-commit'))) killSelf(`after ledger commit ${sha.slice(0, 12)}, before record`);
    if (sha && git(repo, 'show', `${sha}:${path}`) !== next.trimEnd()) throw new Error(`ledger: committed ${path} differs from the rendered content`);
    return { result: { path, sha: sha.slice(0, 12), pauseRequested: pauseRequested() }, commits: sha ? [{ branch: args.integrationBranch, sha: sha.slice(0, 12) }] : [] };
  },

  'completion-report'(prompt, label) {
    const base = prompt.match(/git reset --hard (\S+) && git clean/)[1];
    const text = between(prompt, '<<<REPORT\n', 'REPORT>>>');
    const path = prompt.match(/write the Markdown between the markers below to (\S+?), every/i)[1];
    const { path: wt, branch } = ensureWorktree(slug(label), base);
    // Already committed on top of this base with this text: reuse it.
    const tip = gitOk(repo, 'rev-parse', '--short=12', branch);
    const parent = gitOk(repo, 'rev-parse', `${branch}^`);
    const shown = gitOk(repo, 'show', `${branch}:${path}`);
    if (tip.ok && parent.ok && parent.out === git(repo, 'rev-parse', base) && shown.ok && shown.out === text.trimEnd()) {
      event({ kind: 'idempotent-skip', label, sha: tip.out });
      return { result: { branch, sha: tip.out, path }, commits: [] };
    }
    git(wt, 'reset', '-q', '--hard', base);
    mkdirSync(join(wt, path, '..'), { recursive: true });
    writeFileSync(join(wt, path), text);
    const sha = commitPaths(wt, [path], commitSubject(prompt));
    if (sha && git(wt, 'show', `${sha}:${path}`) !== text.trimEnd()) throw new Error('completion report: committed text differs');
    return { result: { branch, sha: sha.slice(0, 12), path }, commits: sha ? [{ branch, sha: sha.slice(0, 12) }] : [] };
  },

  // Merge in plan order with the declared gates after each; a conflict is
  // aborted and reported unresolvable (the spike does not call a model for
  // conflict resolution). Then the plan bookkeeping, in one commit.
  merge(prompt) {
    const name = ctx.name;
    const wave = Number(prompt.match(/Wave (\d+) merge order:/)[1]);
    const order = [...prompt.split('merge order:\n')[1].matchAll(/^- (\S+): branch (\S+) \(completion report (\S+)\)$/gm)].map((m) => ({ unit: m[1], branch: m[2], report: m[3] }));
    const merges = [];
    for (const m of order) {
      // Already merged (resume after a kill past this point): report it.
      if (gitOk(repo, 'merge-base', '--is-ancestor', m.branch, args.integrationBranch).ok) {
        const sha = git(repo, 'log', '--merges', '-1', '--format=%h', '--abbrev=12', `--grep=^merge(${name}): ${m.unit}$`, args.integrationBranch);
        event({ kind: 'idempotent-skip', label: `merge:${m.unit}`, sha });
        merges.push({ unit: m.unit, status: 'clean', files: [], sha, already: true });
        continue;
      }
      const pre = git(repo, 'rev-parse', 'HEAD');
      const r = gitOk(repo, 'merge', '--no-ff', '-m', `merge(${name}): ${m.unit}`, m.branch);
      if (!r.ok) {
        const files = gitOk(repo, 'diff', '--name-only', '--diff-filter=U').out.split('\n').filter(Boolean);
        gitOk(repo, 'merge', '--abort');
        merges.push({ unit: m.unit, status: 'unresolvable', files, sha: '' });
        continue;
      }
      const gates = runGates(repo);
      const failed = gates.filter((g) => g.status !== 'green');
      if (failed.length) {
        git(repo, 'reset', '--merge', pre);
        merges.push({ unit: m.unit, status: 'regressed', files: failed.map((g) => g.command), sha: '' });
        continue;
      }
      merges.push({ unit: m.unit, status: 'clean', files: git(repo, 'diff', '--name-only', `${pre}..HEAD`).split('\n').filter(Boolean), sha: git(repo, 'rev-parse', '--short=12', 'HEAD') });
    }
    let plan = git(repo, 'show', `HEAD:${ctx.planPath}`);
    // A unit whose entry is already the completion stub is recorded.
    const done = merges.filter((x) => x.status === 'clean' && !plan.includes(`### ${x.unit} — complete at `));
    for (const x of merges) delete x.already;
    let planSha = git(repo, 'log', '-1', '--format=%H', '--', ctx.planPath);
    if (done.length) {
      const archivePath = join(repo, ctx.archivePath);
      let archive = existsSync(archivePath) ? readFileSync(archivePath, 'utf8') : `# Archive: ${name}\n\n## Work Units\n`;
      const date = new Date().toISOString().slice(0, 10);
      for (const x of done) {
        const m = order.find((o) => o.unit === x.unit);
        const re = new RegExp(`^### ${x.unit}\\n[\\s\\S]*?(?=^##)`, 'm');
        const entry = plan.match(re)?.[0];
        if (!entry) throw new Error(`merge: no entry for ${x.unit}`);
        archive = `${archive.trimEnd()}\n\n${entry.trimEnd()}\n`;
        plan = plan.replace(re, `### ${x.unit} — complete at ${x.sha}; entry archived to ${ctx.archivePath}; report ${m.report}\n\n`);
        plan = plan.replace(/(^## Plan Maintenance\n[\s\S]*?)(?=^## )/m, (s) => `${s.trimEnd()}\n\n### ${date} — ${x.unit} complete\nReport: ${m.report} @ ${x.sha}. Ledger: ${ctx.ledgerPath} wave ${wave}.\n\n`);
        plan = plan.replace(new RegExp(`(  - id: ${x.unit}\\n(?:    .*\\n)*?    complete: )false`), '$1true');
      }
      writeFileSync(join(repo, ctx.planPath), `${plan.trimEnd()}\n`);
      mkdirSync(join(archivePath, '..'), { recursive: true });
      writeFileSync(archivePath, archive);
      const sha = commitPaths(repo, [ctx.planPath, ctx.archivePath], `docs(${name}): record wave ${wave} completions`);
      if (!sha) throw new Error('merge: plan bookkeeping commit failed');
      planSha = sha;
    }
    return {
      result: {
        integrationBranch: args.integrationBranch, merges, planSha: planSha.slice(0, 12), pauseRequested: pauseRequested(),
        architectureSha: git(repo, 'rev-list', '-1', args.integrationBranch, '--', args.architectureDoc),
      },
      commits: [],
    };
  },

  // Remove the driver's worktrees whose branch is merged; never --force, so a
  // worktree holding untracked or ignored-but-present changes is kept.
  cleanup() {
    const merged = new Set(git(repo, 'branch', '--merged', args.integrationBranch, '--format=%(refname:short)').split('\n'));
    let removed = 0;
    const kept = [];
    const list = git(repo, 'worktree', 'list', '--porcelain').split('\n\n');
    for (const w of list) {
      const path = w.match(/^worktree (.*)$/m)?.[1];
      const branch = w.match(/^branch refs\/heads\/(.*)$/m)?.[1];
      if (!path?.startsWith(join(stateDir, 'wt')) || !branch?.startsWith(branchPrefix)) continue;
      if (!merged.has(branch)) { kept.push(branch); continue; }
      // `git worktree remove` without --force still deletes ignored files
      // (run 3); a worktree holding any is kept.
      const ignored = gitOk(path, 'status', '--porcelain', '--ignored').out.split('\n').filter((l) => l.startsWith('!!'));
      if (ignored.length) { kept.push(branch); event({ kind: 'cleanup-kept', branch, why: `ignored files: ${ignored.slice(0, 5).join(', ')}` }); continue; }
      const r = gitOk(repo, 'worktree', 'remove', path);
      if (!r.ok) { kept.push(branch); event({ kind: 'cleanup-kept', branch, why: r.out.slice(0, 200) }); continue; }
      gitOk(repo, 'branch', '-d', branch);
      removed += 1;
    }
    gitOk(repo, 'worktree', 'prune');
    return { result: { removed, kept }, commits: [] };
  },
};
const codeStage = (label) => Object.keys(CODE).find((k) => label === k || label.startsWith(`${k}:`));

// ---------------------------------------------------------------- runtime

const cachePath = (key) => join(stateDir, 'calls', `${key}.json`);

async function agent(prompt, opts) {
  checkKill();
  const label = opts.label;
  const key = slug(label);
  const stage = codeStage(label);
  const journaled = stage && stage !== 'load'; // load is a pure read; it re-runs to see the pause ref
  const hash = sha256(`${prompt}\n${JSON.stringify(opts.schema)}\n${opts.model}/${opts.effort}`);
  // Code steps are journaled like calls: a completed step replays its
  // recorded return (run 4: re-executing a completed completion-report step
  // after a resume met a branch cleanup had deleted). An intent written
  // before the step marks one interrupted mid-way; only that one re-executes,
  // and its own effect check (ledger block present, report committed) keeps
  // it from acting twice.
  if (journaled && existsSync(cachePath(key))) {
    const c = JSON.parse(readFileSync(cachePath(key), 'utf8'));
    if (c.hash === hash) { event({ kind: 'code-replayed', label }); return c.ret; }
  }
  if (stage) {
    const intent = join(stateDir, 'calls', `${key}.intent`);
    if (journaled && existsSync(intent) && readFileSync(intent, 'utf8') === hash) event({ kind: 'code-interrupted-rerun', label });
    if (journaled) writeFileSync(intent, hash);
    const plant = fire(fault(label, 'plant-untracked'));
    if (plant) for (const f of plant.files) { mkdirSync(join(stateDir, 'wt', f, '..'), { recursive: true }); writeFileSync(join(stateDir, 'wt', f), 'retained artefact\n'); }
    const edit = fire(fault(label, 'requester-edit'));
    if (edit) appendFileSync(join(repo, edit.file), edit.text);
    const t0 = Date.now();
    const ret = CODE[stage](prompt, label);
    event({ kind: 'code', label, secs: (Date.now() - t0) / 1000, sha: ret.result.sha ?? ret.result.planSha });
    if (journaled) writeFileSync(cachePath(key), JSON.stringify({ label, hash, ret }, null, 2));
    return ret;
  }
  if (existsSync(cachePath(key))) {
    const c = JSON.parse(readFileSync(cachePath(key), 'utf8'));
    if (c.hash === hash) { event({ kind: 'cache-hit', label }); log(`${label}: cached return`); return c.ret; }
    event({ kind: 'cache-stale', label });
  }
  await acquire();
  let ret;
  try {
    checkKill();
    const cwd = opts.isolation === 'worktree' ? ensureWorktree(key).path : repo;
    log(`${label}: spawn (${opts.model}/${opts.effort ?? '-'}) in ${cwd === repo ? 'main checkout' : cwd}`);
    ret = injectReturnFaults(label, await callWithRetry(prompt, opts, cwd, key));
    if (ret) ret = verifyShas(label, ret);
    if (ret && /^(implement|fix-code):/.test(label) && gateCommands.length && !(ret.result.gateProbe ?? []).some((g) => g.drifted)) {
      const reported = ret.result.gates ?? [];
      const dirty = git(cwd, 'status', '--porcelain');
      if (dirty) event({ kind: 'uncommitted-at-return', label, files: dirty.split('\n').slice(0, 10) });
      const own = runGatesAt(ret.result.sha || 'HEAD', key);
      event({ kind: 'gates', label, reported: reported.map((g) => `${g.command}=${g.status}`), driver: own.map((g) => `${g.command}=${g.status}`) });
      const missing = gateCommands.filter((c) => !reported.some((g) => g.command.includes(c)));
      if (missing.length) log(`${label}: agent did not report declared gate(s) ${missing.join(', ')}; the driver ran them`);
      ret.result.gates = own;
    }
  } finally {
    release();
  }
  if (ret) writeFileSync(cachePath(key), JSON.stringify({ label, hash, ret }, null, 2));
  return ret;
}

async function parallel(thunks) {
  return Promise.all(thunks.map((t) => t().catch((e) => {
    if (e instanceof KillError) throw e;
    log(`parallel branch threw: ${e.stack}`);
    return null;
  })));
}

reapOrphans();
setStatus({ state: 'running' });
log(`driver start: repo ${repo}, plan ${args.planSha}`);
try {
  const outcome = await run({ agent, parallel, log: (m) => log(`[run] ${m}`), args });
  writeFileSync(join(stateDir, 'outcome.json'), JSON.stringify(outcome, null, 2));
  setStatus({ state: outcome.status });
  log(`driver end: ${outcome.status}`);
} catch (e) {
  setStatus({ state: e instanceof KillError ? 'killed' : 'crashed', error: String(e.stack ?? e) });
  log(`driver ${e instanceof KillError ? 'stopped by KILL file' : `crashed: ${e.stack}`}`);
  process.exitCode = 2;
}
