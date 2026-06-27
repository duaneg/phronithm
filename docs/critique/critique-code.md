# Critique Appendix: Code

## Axis refinements

- **Adversarial cases**: Construct inputs, states, or timing conditions that break this code. Focus on boundary values, error paths, concurrent access, and resource exhaustion.
- **Unstated assumptions**: What invariants, preconditions, or environmental requirements does this code depend on but not check or document? What happens when they're violated?
- **External consistency**: Does this code follow project conventions, language idioms, and framework patterns? Is error handling consistent with surrounding code?

## Additional axes

7. **Correctness**: Does the code do what it claims? Are edge cases handled? Is the logic sound?
8. **Security**: OWASP top 10 and context-specific concerns. Input validation at trust boundaries. Secrets handling.
9. **Error handling**: Are all error paths handled? Are errors propagated with sufficient context? Are resources cleaned up on failure?
10. **Testability**: Can this code be tested in isolation? Are dependencies injectable? Is state observable?

## Meta

Do not suggest style or formatting changes unless they affect correctness or clarity at trust boundaries. Focus on substance.
