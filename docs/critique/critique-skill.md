# Critique Appendix: Skill / Process Document

## Axis refinements

- **Adversarial cases**: Construct a scenario where an LLM following this skill literally produces a worse outcome than one ignoring it. Pay special attention to loops and decision points — can the process get stuck or oscillate?
- **Unstated assumptions**: Would an LLM without deep relevant domain experience miss something critical? Are instructions unambiguous enough to follow without human intuition?
- **External consistency**: Does this skill follow the project's skill structure conventions? (Reference implementation: [phronithm:refactor](../../skills/refactor/SKILL.md).)

## Additional axes

7. **Actionability**: Can an LLM execute every step without ambiguity? Are decision criteria explicit and measurable, or do they require judgement the skill doesn't equip the reader to make?
8. **Termination**: Does the process converge? Are stop conditions explicit? Can it loop indefinitely under any realistic scenario?
9. **Measurability**: How do you know the skill produced a good outcome? Are success criteria stated or inferrable?

## Meta

Prose clarity is substance for skill documents — an ambiguous sentence is a functional defect. Flag unclear phrasing as a finding, not a style issue.

**Design intent**: For skill-type artefacts, verify that flagged phrasing is intentional design before classifying it as a defect. Check what the instruction is trying to accomplish before proposing rewrites — "subagent" may be deliberate Agent-tool usage, a multi-pass loop may be intentional, an apparently inverted condition may be the correct stop criterion.

When critiquing a SKILL.md, also apply [critique-phronithm](critique-phronithm.md) — it covers LLM instruction-quality dimensions that apply to all phronithm types.
