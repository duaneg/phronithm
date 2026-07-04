# Critique Appendix: Phronithm (LLM Instruction Document)

Applies to any document loaded as LLM context: SKILL.md files, CLAUDE.md sections, lens documents, prompts, and similar instruction artefacts.

## Axis refinements

- **Adversarial cases**: Construct a scenario where an LLM following this document literally produces a worse outcome than one ignoring it. Focus on: conditional instructions with ambiguous triggers, negations without positive restatement, and instructions that conflict with LLM default behaviour.
- **Unstated assumptions**: Would an LLM without this project's context know when this document applies?
- **External consistency**: Does this document contradict other phronithms in scope? Are cross-references accurate?

## Additional axes

7. **Obedience failure modes**: Check for instruction patterns LLMs reliably under-follow:
   - Hedged imperatives ("consider", "you might", "it can be useful to") where compliance is expected — restate as direct instructions
   - Long enumerations — items beyond position 3–4 receive less attention; restructure or flatten
   - Negations without positive restatement ("don't do X" without "do Y instead") — add the positive form
   - Critical instructions buried mid-paragraph — move to section start or use a call-out

8. **Salience**: Are the most important constraints visible without reading the full document?
   - Does each section's opening sentence state the key point?
   - Is emphasis (bold, IMPORTANT:) used sparingly, reserved for genuinely critical things?
   - In enumerations, are the highest-priority items first?

9. **Triggering clarity**: Can the LLM deterministically identify when this document applies? Is the scope boundary explicit? Are trigger conditions explicit, or do they rely on situational judgement the document doesn't provide? Are conditional instructions stated with checkable conditions rather than judgement calls?

10. **Token efficiency**: Phronithms are loaded on every invocation — cost multiplies with use frequency. Check for:
   - Redundancy with the base critique template (restating universal axes already covered there)
   - Throat-clearing before the first actionable instruction
   - Examples added to unambiguous instructions (examples reduce ambiguity; where none exists, they add cost without value)
   - Constraints repeated for emphasis rather than restructured for salience
   - The named patterns in the [docs-style](../lenses/docs-style.md) lens: rationale restatement, design-history aside, cross-reference justification, announcing honesty, explained metaphor

   **Concision trap**: Cuts motivated by token efficiency can remove load-bearing content. Before flagging text as redundant, verify it is genuinely decorative. Cross-check against axes 7 and 8: does removal weaken a high-risk instruction (obedience failure mode), reduce visibility of a key constraint (salience), drop a diagnostic signal, or delete an explicit prohibition? If yes, the text is not redundant — it is misplaced emphasis. Restructure for salience rather than delete. Concision that removes load-bearing content is a functional defect.

## Meta

Imprecise language is a functional defect — an ambiguous instruction resolves unpredictably. Flag unclear phrasing as a finding, not a style issue. When proposing rewrites, prefer active, direct, affirmative language.
