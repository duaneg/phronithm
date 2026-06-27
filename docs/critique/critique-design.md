# Critique Appendix: Design Document

## Axis refinements

- **Adversarial cases**: Construct a plausible requirement change or operational scenario that invalidates this design. How brittle is it?
- **Unstated assumptions**: What constraints about the deployment environment, team capabilities, existing system state, or future requirements does this design assume without stating?
- **Internal consistency**: After each structural decision that changes the document's shape (e.g. switching from monolithic to per-file layout), check that all prior sections remain consistent with the new structure. Incremental design decisions cause consistency drift in earlier sections.
- **External consistency**: Does this design align with the project's architecture and established patterns? Does it introduce new patterns where existing ones would suffice?

## Additional axes

7. **Feasibility**: Can this design actually be implemented with the available tools, skills, and constraints? Are there hidden complexities?
8. **Tradeoff quality**: Are alternatives identified? Are tradeoffs explicit and justified, or hand-waved?
9. **Incrementality**: Can this design be implemented and validated incrementally, or does it require a big-bang delivery?
10. **Code path coverage**: Does this design cover all code paths in the target component? If the design names specific functions, structs, or modules, verify it addresses all modes, branches, and configurations those targets support — not just the ones mentioned in the brief. A design that covers sparse mode but not dense mode (or vice versa) is incomplete.

## Meta

Focus on whether the design makes good decisions and communicates them clearly. Flag missing justification for architectural choices — "we chose X" without "because Y, and not Z because W" is a gap.
