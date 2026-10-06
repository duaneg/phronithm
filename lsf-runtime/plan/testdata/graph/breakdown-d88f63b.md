# lsf-improvements-breakdown.md @ d88f63b, plan block only

```yaml lsf-plan
units:
  - id: problem-register
    type: foundation
    complete: true
    dependencies: []
    scope: [docs/design/lsf-problem-register.md]
    publishes: [problem ids, generality classes, topic tags]
    gates:
      - name: archipeligo-records
        kind: data
        pin: "archipeligo-evolved c220c62"
      - name: cartomancer-records
        kind: data
        pin: "quality-bar-interiors-signals.md; transcripts from 2026-09-10"
  - id: relay-probe
    type: design
    complete: true
    dependencies: []
    scope: [docs/design/lsf-workflow.md::constraints]
    publishes: [relay behaviour]
    gates:
      - name: claude-code-workflow-tool
        kind: environment
        pin: "2.1.289"
  - id: substrate-options
    type: design
    complete: true
    dependencies: [problem-register, relay-probe, gate-results-integrity]
    scope: [docs/design/lsf-substrate.md]
    publishes: [selected option, interim script policy]
    gates:
      - name: requester-verdict
        kind: organisational
  - id: substrate-spike
    type: design
    complete: true
    dependencies: [substrate-options]
    scope: [docs/design/lsf-substrate.md, workflows/lsf/spike]
    publishes: [confirmed substrate]
    gates:
      - name: claude-code-workflow-tool
        kind: environment
        pin: "2.1.289"
      - name: claude-code-headless
        kind: environment
        pin: "2.1.289"
      - name: node
        kind: environment
        pin: "v26.9.0"
  - id: arch-pin-refresh
    type: bug-fix
    complete: true
    dependencies: []
    scope: [workflows/lsf/implement.js, workflows/lsf/scheduling.js, workflows/lsf/schema.js, workflows/lsf/test/implement.test.js, workflows/lsf/test/scheduling.test.js, workflows/lsf-implement.js, docs/design/lsf-workflow.md::wave-loop]
    publishes: [merge-stage architectureSha and merges sha, gate amendments]
    gates:
      - name: node
        kind: environment
        pin: "v26.9.0"
  - id: gate-results-integrity
    type: bug-fix
    complete: true
    dependencies: [arch-pin-refresh]
    scope: [workflows/lsf/implement.js, workflows/lsf/schema.js, workflows/lsf/test/implement.test.js, workflows/lsf-implement.js, docs/design/lsf-workflow.md::lifecycle-mapping, skills/lsf-drive/SKILL.md::relaunch]
    publishes: [fix return shape, gate-not-run stop, long-gate recipe, P-D3 residual]
    gates:
      - name: node
        kind: environment
        pin: "v26.9.0"
  - id: plan-preflight-check
    type: feature
    complete: false
    dependencies: [orchestrator-port-plan]
    scope: [lsf-runtime/validate, skills/breakdown/SKILL.md::quality-check, skills/lsf-drive/SKILL.md::startup]
    publishes: [plan-validation command]
    gates:
      - name: lsf-toolchain
        kind: environment
        pin: "1.27.1"
  - id: significant-precision-sample
    type: design
    complete: false
    dependencies: []
    scope: [docs/investigations/lsf-significant-precision.md]
    publishes: [precision estimate]
    gates:
      - name: archipeligo-records
        kind: data
        pin: "archipeligo-evolved c220c62"
      - name: cartomancer-records
        kind: data
        pin: "quality-bar-interiors-signals.md"
  - id: empirical-criteria-design
    type: design
    complete: false
    dependencies: [problem-register]
    scope: [docs/design/lsf-empirical-work.md::criteria]
    publishes: [criteria requirements]
    gates:
      - name: archipeligo-records
        kind: data
        pin: "archipeligo-evolved c220c62"
      - name: cartomancer-records
        kind: data
        pin: "quality-bar-interiors brief and plans"
      - name: requester-verdict
        kind: organisational
  - id: empirical-unit-design
    type: design
    complete: false
    dependencies: [problem-register, empirical-criteria-design, gate-results-integrity]
    scope: [docs/design/lsf-empirical-work.md::units]
    publishes: [empirical work plan shape]
    gates:
      - name: archipeligo-records
        kind: data
        pin: "archipeligo-evolved c220c62"
      - name: requester-verdict
        kind: organisational
  - id: retained-artefacts-design
    type: design
    complete: false
    dependencies: [problem-register]
    scope: [docs/design/lsf-empirical-work.md::retained-artefacts]
    publishes: [retained-artefact policy]
    gates:
      - name: archipeligo-records
        kind: data
        pin: "archipeligo-evolved c220c62"
      - name: archipeligo-runs
        kind: data
        pin: "~/src/archipeligo-runs, 80 GB on 2026-10-04"
      - name: requester-verdict
        kind: organisational
  - id: findings-consumer-design
    type: design
    complete: false
    dependencies: [significant-precision-sample, problem-register]
    scope: [docs/design/lsf-findings.md]
    publishes: [findings consumer decision]
    gates:
      - name: requester-verdict
        kind: organisational
  - id: drive-decision-design
    type: design
    complete: false
    dependencies: [problem-register, substrate-options]
    scope: [docs/design/lsf-session-role.md]
    publishes: [session role]
    gates:
      - name: requester-verdict
        kind: organisational
  - id: s2-permissions-design
    type: design
    complete: false
    dependencies: [substrate-spike, runtime-core]
    scope: [docs/design/lsf-runtime.md::permissions]
    publishes: [per-stage permission configuration]
    gates:
      - name: claude-code-headless
        kind: environment
        pin: "2.1.289"
      - name: requester-verdict
        kind: organisational
  - id: runtime-language-design
    type: design
    complete: true
    dependencies: [substrate-spike]
    scope: [docs/design/lsf-runtime.md::language, docs/investigations/runtime-language-probes, lsf-runtime/go.mod, lsf-runtime/plan/doc.go, lsf-runtime/runtime/doc.go, lsf-runtime/orchestrator/doc.go, lsf-runtime/cli/main.go, lsf-runtime/testdata/claude]
    publishes: [language, toolchain pin and source layout]
    gates:
      - name: requester-verdict
        kind: organisational
  - id: runtime-core
    type: foundation
    complete: true
    dependencies: [substrate-spike, runtime-language-design]
    scope: [lsf-runtime/runtime, lsf-runtime/testdata, docs/design/lsf-runtime.md::runtime, CLAUDE.md]
    publishes: [host runtime, journal, stage roles as configuration, call result, step API, worktree isolation]
    gates:
      - name: lsf-toolchain
        kind: environment
        pin: "1.27.1"
      - name: claude-code-headless
        kind: environment
        pin: "2.1.289"
  - id: orchestrator-port-plan
    type: foundation
    complete: false
    dependencies: [runtime-language-design]
    scope: [lsf-runtime/plan, lsf-runtime/tools]
    publishes: [plan graph, schema and scheduling rules]
    gates:
      - name: lsf-toolchain
        kind: environment
        pin: "1.27.1"
  - id: orchestrator-port-loop
    type: foundation
    complete: false
    dependencies: [orchestrator-port-plan, runtime-core]
    scope: [lsf-runtime/orchestrator, lsf-runtime/tools]
    publishes: [orchestrator run(runtime)]
    gates:
      - name: lsf-toolchain
        kind: environment
        pin: "1.27.1"
      - name: claude-code-headless
        kind: environment
        pin: "2.1.289"
  - id: code-stage-interface
    type: refactor
    complete: false
    dependencies: [orchestrator-port-loop, runtime-core, arch-pin-refresh, retained-artefacts-design]
    scope: [lsf-runtime/orchestrator, lsf-runtime/runtime, docs/design/lsf-workflow.md::model-tiers]
    publishes: [code-stage interface]
    gates:
      - name: lsf-toolchain
        kind: environment
        pin: "1.27.1"
  - id: runtime-signals
    type: feature
    complete: false
    dependencies: [code-stage-interface, runtime-core]
    scope: [lsf-runtime/plan, lsf-runtime/runtime, docs/design/lsf-workflow.md::escalation]
    publishes: [driver-event signal kinds]
    gates:
      - name: lsf-toolchain
        kind: environment
        pin: "1.27.1"
  - id: declared-gates
    type: feature
    complete: false
    dependencies: [runtime-signals]
    scope: [lsf-runtime/plan, lsf-runtime/runtime, docs/plan-graph.md, skills/breakdown/SKILL.md::graph, docs/design/lsf-workflow.md::lifecycle-mapping]
    publishes: [declared gate-command field]
    gates:
      - name: lsf-toolchain
        kind: environment
        pin: "1.27.1"
  - id: runtime-cli-drive
    type: integration
    complete: false
    dependencies: [declared-gates, s2-permissions-design, plan-preflight-check, relay-probe, runtime-core]
    scope: [lsf-runtime/cli, skills/lsf-drive/SKILL.md::launch, skills/lsf-drive/SKILL.md::relaunch, skills/lsf-drive/SKILL.md::startup, docs/design/lsf-runtime.md::cli]
    publishes: [runtime CLI]
    gates:
      - name: claude-code-headless
        kind: environment
        pin: "2.1.289"
      - name: lsf-toolchain
        kind: environment
        pin: "1.27.1"
  - id: runtime-distribution
    type: integration
    complete: false
    dependencies: [runtime-cli-drive, runtime-language-design]
    scope: [lsf-runtime/dist, .github/workflows/lsf-runtime.yml, skills/lsf-drive/SKILL.md::launch, docs/design/lsf-runtime.md::distribution]
    publishes: [runtime launcher, release build]
    gates:
      - name: github-actions
        kind: environment
      - name: lsf-toolchain
        kind: environment
        pin: "1.27.1"
      - name: requester-verdict
        kind: organisational
  - id: script-retirement
    type: refactor
    complete: false
    dependencies: [runtime-cli-drive, runtime-distribution]
    scope: [workflows, docs/design/lsf-workflow.md::constraints, CLAUDE.md]
    publishes: []
    gates: []
  - id: empirical-script-support
    type: feature
    complete: false
    dependencies: [empirical-unit-design, script-retirement]
    scope: [lsf-runtime/plan, lsf-runtime/orchestrator, docs/design/lsf-workflow.md::wave-loop]
    publishes: [empirical plan-block fields]
    gates:
      - name: lsf-toolchain
        kind: environment
        pin: "1.27.1"
  - id: findings-consumer-impl
    type: feature
    complete: false
    dependencies: [findings-consumer-design, empirical-script-support]
    scope: [lsf-runtime/plan, lsf-runtime/orchestrator, docs/design/lsf-workflow.md::escalation]
    publishes: [finding identity]
    gates:
      - name: lsf-toolchain
        kind: environment
        pin: "1.27.1"
  - id: long-running-design
    type: design
    complete: false
    dependencies: [substrate-spike, runtime-core, empirical-unit-design, retained-artefacts-design, gate-results-integrity]
    scope: [docs/design/lsf-long-running.md]
    publishes: [long-running work decision]
    gates: []
  - id: long-running-impl
    type: feature
    complete: false
    dependencies: [long-running-design, findings-consumer-impl]
    scope: [lsf-runtime/plan, lsf-runtime/orchestrator, lsf-runtime/runtime, docs/design/lsf-workflow.md::wave-loop, docs/design/lsf-workflow.md::lifecycle-mapping]
    publishes: []
    gates:
      - name: lsf-toolchain
        kind: environment
        pin: "1.27.1"
  - id: empirical-criteria-impl
    type: feature
    complete: false
    dependencies: [empirical-criteria-design]
    scope: [skills/large-scale-feature/SKILL.md::phase-1, skills/large-scale-feature/SKILL.md::phase-2]
    publishes: []
    gates: []
  - id: phase2-gate-discipline
    type: feature
    complete: false
    dependencies: [problem-register, empirical-criteria-impl]
    scope: [skills/large-scale-feature/SKILL.md::phase-2, docs/critique-gate.md]
    publishes: []
    gates:
      - name: requester-verdict
        kind: organisational
  - id: empirical-unit-impl
    type: feature
    complete: false
    dependencies: [empirical-unit-design, retained-artefacts-design, empirical-script-support, plan-preflight-check, declared-gates]
    scope: [skills/breakdown/SKILL.md, docs/plan-graph.md]
    publishes: []
    gates: []
  - id: programme-close-out
    type: feature
    complete: false
    dependencies: [findings-consumer-design, empirical-criteria-design]
    scope: [skills/large-scale-feature/SKILL.md::termination, skills/lsf-drive/SKILL.md::termination]
    publishes: []
    gates:
      - name: archipeligo-records
        kind: data
        pin: "archipeligo-evolved c220c62"
  - id: drive-presentation-impl
    type: feature
    complete: false
    dependencies: [drive-decision-design]
    scope: [skills/lsf-drive/SKILL.md::present, skills/lsf-drive/SKILL.md::decide, docs/design/lsf-workflow.md::architecture]
    publishes: []
    gates: []
  - id: drive-mechanics
    type: feature
    complete: false
    dependencies: [substrate-spike, drive-decision-design, drive-presentation-impl, runtime-cli-drive, runtime-distribution, plan-preflight-check, programme-close-out]
    scope: [skills/lsf-drive/SKILL.md]
    publishes: []
    gates: []
  - id: shakedown-fixture
    type: integration
    complete: false
    dependencies: [long-running-impl, empirical-unit-impl, drive-mechanics]
    scope: [lsf-runtime/fixtures/shakedown, docs/design/lsf-workflow.md::status, docs/design/lsf-workflow.md::testing]
    publishes: [shakedown fixture plan]
    gates:
      - name: claude-code-headless
        kind: environment
        pin: "2.1.289"
  - id: programme-close
    type: integration
    complete: false
    dependencies: [shakedown-fixture, phase2-gate-discipline, programme-close-out, empirical-criteria-impl]
    scope: [docs/design/lsf-problem-register.md, docs/design/lsf-improvements.md, docs/design/lsf-workflow.md::status, CLAUDE.md]
    publishes: []
    gates: []
execution_order: [problem-register, relay-probe, arch-pin-refresh, gate-results-integrity, substrate-options, significant-precision-sample, empirical-criteria-design, retained-artefacts-design, substrate-spike, empirical-unit-design, findings-consumer-design, drive-decision-design, runtime-language-design, runtime-core, s2-permissions-design, orchestrator-port-plan, plan-preflight-check, orchestrator-port-loop, code-stage-interface, runtime-signals, declared-gates, runtime-cli-drive, runtime-distribution, script-retirement, empirical-script-support, findings-consumer-impl, long-running-design, long-running-impl, empirical-criteria-impl, phase2-gate-discipline, empirical-unit-impl, programme-close-out, drive-presentation-impl, drive-mechanics, shakedown-fixture, programme-close]
```
