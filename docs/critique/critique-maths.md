# Critique Appendix: Mathematics (Proofs, Certificates, Computation)

Applies to mathematical artefacts: formal proofs (Lean, Coq, Agda), informal proofs and derivations, computational certificates, and numerical or exact-arithmetic code that establishes a mathematical claim.

## Axis refinements

- **Completeness**: Are all cases in a case analysis exhaustive and disjoint? Are boundary and degenerate cases handled (empty set, zero, dimension 1, the trivial group)? Are all hypotheses used — an unused hypothesis suggests either an over-strong statement or an incomplete proof.
- **Adversarial cases**: Construct a scenario where the result fails — weaken a hypothesis and find a counterexample, or identify the sharpest point of the argument. For computational proofs, identify inputs where the certificate search could silently produce a wrong answer. For numerical code, the [numerical](../lenses/numerical.md) lens enumerates the failure-inducing inputs (cancellation, overflow, ill-conditioning).
- **Unstated assumptions**: Hidden mathematical hypotheses — finiteness, characteristic, measurability, well-ordering. In formal proofs: axiom usage beyond the expected set, universe polymorphism issues, implicit coercions that change mathematical meaning. In computational work: implicit conventions (indexing origin, normalisation, sign) that affect correctness if mismatched. In numerical code: implicit assumptions about input conditioning, representability of intermediate values, or precision requirements that are neither checked nor documented.
- **External consistency**: Does notation and terminology follow the source paper or field conventions? For formal proofs: does it follow the proof assistant's library conventions (e.g. Mathlib naming and style)? Are definitions mathematically equivalent to the standard ones, or do they diverge — and is the divergence documented?

## Additional axes

7. **Logical soundness**: Are all deductive steps valid? Identify gaps — steps that require a lemma not cited, unstated intermediate results, or reasoning that confuses sufficient and necessary conditions. For formal proofs: flag `sorry`, `native_decide`, trusted oracles, and `Decidable` instances that bypass kernel checking. For informal reasoning: flag hand-waving ("clearly", "it is easy to see") at non-trivial steps.
8. **Encoding fidelity**: Does the representation faithfully capture the intended object — does the *thing computed or proved* correspond to the *thing claimed*, not a near-miss variant? Check the formal or computational definition against the source paper's, and that the established result answers the stated theorem rather than a weaker or adjacent one. (Index, sign, and normalisation convention mismatches are covered under Unstated assumptions and External consistency.) For floating-point code, finite-precision arithmetic is itself an encoding of real arithmetic — identify where the gap matters; the [numerical](../lenses/numerical.md) lens is the checklist for how it bites.
9. **Proof strategy**: Is the approach well-chosen for the claim? Is there a simpler or more direct path? Is the level of generality appropriate — proving too much is expensive, proving too little leaves a gap. For computational proofs: is the verification architecture sound (independent backends, sufficient precision, correct reduction)?
10. **Verification independence**: For results established by computation, are verification paths genuinely independent (different arithmetic backends, different modelling layers)? Does each path actually check the mathematical claim, or just an encoding of it? Is the trusted base explicit — what must you trust for the result to hold?

## Meta

Focus on mathematical content and correctness, not presentation or prose style.

Distinguish formal rigour (kernel-checked) from informal rigour (human-assessed). Where a proof assistant's kernel has accepted a proof, do not second-guess steps the kernel verified — focus critique on whether the *statement* proved is the statement *claimed*, and whether the axioms used are acceptable.

For artefacts spanning formal and computational modes, the bridge between them is the highest-risk surface — prioritise encoding fidelity and verification independence there.

This appendix is deliberately general — it flags *where* to look, not the full taxonomy of each failure. For depth, defer rather than inline: for floating-point and numerical code, the [numerical](../lenses/numerical.md) review lens (referenced from the adversarial and encoding-fidelity axes above); for proof-assistant idioms (Mathlib conventions, tactic style) and field-specific conventions, the target project's documentation and style guides.
