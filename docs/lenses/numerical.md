# Lens: Numerical

Floating-point and numeric code — accuracy, stability, special values, and cross-platform reproducibility. The detailed checklist the [critique-maths](../critique/critique-maths.md) appendix defers to for numerical artefacts.

Each concern below names what to **flag** in the code, with the fix. The prose is the reasoning; the [Red flags](#red-flags) section is the greppable distillation — apply both.

**Larger than a typical lens by design.** The domain spans three loosely-coupled failure families (algorithmic, representation, environment) that cross-reference heavily — compensated summation only holds if the compiler doesn't reassociate. Jump to the relevant concern; you rarely need all of it.

**Defers to**: [general](general.md) (gross correctness), [data-structures](data-structures.md) (floats as map keys), [phronithm:concurrency](../../skills/concurrency/SKILL.md) (locking around shared accumulators — but FP non-associativity of parallel reductions is covered here), [error-handling](error-handling.md) (handling a NaN or FP exception once detected). [critique-maths](../critique/critique-maths.md) flags *where* the float/real gap matters and defers here for the analysis of how it bites — the boundary is depth, not topic.

**Tool support**: [phronithm:static-analysis](../../skills/static-analysis/SKILL.md) can surface compiler FP flags and UBSan `float-cast-overflow`. Runtime trapping (`feenableexcept`, numpy `seterr`) and roundoff estimators (Verrou, CADNA) are in [Testing & detection](#testing--detection) below.

## Concerns

### Cancellation and loss of significance

Flag subtraction of nearly-equal **computed** values: the subtraction is exact, but it strips the leading digits and exposes the rounding error the operands already carried, amplifying it. (Subtracting exactly-known values is benign.)

- **Quadratic formula**: `(-b ± sqrt(b*b - 4*a*c))/(2*a)` loses the root where `-b` and `±sqrt` nearly cancel. Use `q = -(b + sign(b)*sqrt(disc))/2`, then `x1 = q/a`, `x2 = c/q`.
- **Variance as `E[x²] - E[x]²`** (one-pass `Σx`, `Σx²` then subtract): catastrophic for large mean / small variance, can yield *negative* variance. Use Welford's online update or two-pass.
- **Naive numerical differentiation** `(f(x+h)-f(x))/h`: shrinking `h` toward 0 makes the numerator cancel — error is worst at the smallest `h`, not best. Flag a hard-coded tiny `h`; prefer complex-step `Im(f(x+ih))/h` (analytic `f`) or autodiff.
- **Signatures and fixes**: `1-cos(x)` → `2*sin(x/2)²`; `exp(x)-1` → `expm1`; `log(1+x)` → `log1p`; `(1+r)**n - 1` for small `r` → `expm1`; `sqrt(x+1)-sqrt(x)` → `1/(sqrt(x+1)+sqrt(x))`.

### Summation and accumulation

Flag a naive `s += x` accumulator over many (≳10⁴) or mixed-magnitude terms: error grows with `n`, and small later terms are absorbed below the partial sum's ULP.

- **Remedies in increasing cost**: pairwise/tree summation (error `O(log n)`, numpy's default); Kahan compensated summation (`O(1)` dominant term); Neumaier when terms exceed the running sum in magnitude. A higher-precision accumulator (binary32 inputs → binary64 accumulator) is often the simplest fix; flag binary32 accumulators in long sums/dot products.
- **Absorption / swamping**: `big + small` with `small < ulp(big)` silently drops `small` — accumulating a tiny increment into a large baseline (`x += dt*v` over a long simulation). Compensate, or track displacement from a reference.
- **Non-associativity**: `(a+b)+c ≠ a+(b+c)`. Results depend on evaluation order, so SIMD/parallel reductions are not bit-reproducible (see [Reproducibility & environment](#reproducibility--environment)). Treat algebraically-equal rewrites (`a*b - a*c` → `a*(b-c)`) as behaviour-changing.

### Overflow / underflow in intermediates

Flag intermediates that can overflow to ±Inf or underflow to 0 even when the inputs and the final result are in range.

- **Norms / magnitude**: `sqrt(a*a + b*b)` overflows when either term exceeds `sqrt(MAX)`; use `hypot`, or scaled accumulation for vector norms.
- **Softmax / log-sum-exp**: `log(Σ exp(xᵢ))` overflows once any `xᵢ` is large; shift by the max: `a + log(Σ exp(xᵢ - a))`.
- **Products of probabilities / likelihoods** underflow to 0 → `log(0) = -∞`; work in log-space (`Σ log pᵢ`, `logaddexp`).
- **Factorials / Gamma / binomials** overflow long before their ratios do; use `lgamma`/`lbeta` and exponentiate the in-range final result. Reorder `a*b/c` to `a*(b/c)` when `a*b` overflows.

### Conditioning vs stability

Decide whether a large error is the **problem's** fault or the **algorithm's** — they are orthogonal (`forward error ≲ condition number × backward error`: conditioning is the problem's input-sensitivity, stability the algorithm's injected error).

- **Ill-conditioned input** (near-singular matrix, near-multiple roots) → large error is *conditioning*, not a bug; reformulate or regularise, don't reach for a "more careful" algorithm. **Well-conditioned input, large error** → the algorithm is *unstable*: a real bug.
- Flag a comment blaming "floating-point error" on an obviously ill-conditioned problem. Ask whether `κ`/`rcond` is known and monitored.
- **Algorithm selection**: Gaussian elimination needs pivoting (flag hand-written LU/GE with no pivot search). Least squares via normal equations `AᵀA x = Aᵀb` squares the condition number — prefer QR (or SVD if rank-deficient); flag explicit `A.T @ A`. A failed Cholesky is a useful signal the matrix isn't actually SPD — don't paper over it with a diagonal fudge. Prefer `solve(A, b)` over `inv(A) @ b`.

### Comparison and tolerance

- **No `==`/`!=` on computed floats**: `if (x == 0.1)`, `while (t != 1.0)`, float loop counters. Iterate with integers; compare with tolerance.
- **The combined test**: `|a-b| ≤ max(rel·max(|a|,|b|), abs)` — relative for large magnitudes, an absolute floor near zero. Absolute-only fails across magnitudes; relative-only has the **near-zero trap** (collapses to near-exact equality when both operands approach 0, and is degenerate at `a=b=0`).
- **Comparison to zero always needs an explicit `abs`** chosen from the problem's natural zero-scale (the relative term vanishes when the reference is 0).
- **Epsilon scales with accumulated error** ≈ `(#rounding ops)·u`, not a magic constant. `FLT_EPSILON`/`1e-15` as a post-computation tolerance is far too tight.
- **numpy `isclose` is asymmetric** (`|a-b| ≤ atol + rtol·|b|`, `b` the reference) — put the expected value second; `math.isclose` is symmetric with `abs_tol=0` (so it fails near-zero unless you set it). ULP comparison is rigorous for same-sign finite values but ill-defined across zero / with NaN/Inf.

### Special values

- **NaN**: `x == NaN` is *always* false — `x != x` is the portable NaN test; use `isnan`. NaN routes through `else` branches (both `<` and `>=` are false), so single-comparison min/max/clamp mishandle it. One NaN **poisons an entire reduction** (`sum`/`mean`/`dot`) — reductions over external data need a NaN policy (`nansum`, masking, guard). `fmin`/`fmax` return the non-NaN operand; IEEE-2019 `minimum`/`maximum` propagate NaN — know which yours is. Sorting NaN violates strict-weak-ordering (UB in `qsort`/`std::sort`); NaN keys are unretrievable in hash maps.
- **Inf**: from overflow or `x/0` (`0/0` is NaN, not Inf). `Inf-Inf`, `Inf/Inf`, `0*Inf`, `Inf%x` all → NaN — an overflow far upstream surfaces as a NaN that decouples symptom from cause. Prefer overflow-safe primitives (`hypot`, `logaddexp`) or explicit saturation.
- **Signed zero**: `-0.0 == 0.0` is true but `1/+0 = +Inf` vs `1/-0 = -Inf`; the sign carries direction-of-approach and matters at branch cuts (complex `sqrt`/`log`, `atan2`). `x + 0.0` normalises `-0.0` to `+0.0`. Test the sign with `signbit`, not `x < 0`. (Java `compareTo`/`equals` distinguish ±0 while `==` doesn't.)

### Representation and type choice

- **Decimals are not exactly representable**: `0.1 + 0.2 == 0.3` is false. Any `==` on decimal-literal arithmetic is suspect.
- **Non-uniform spacing**: 1 ULP grows with magnitude (≈2.2e-16 near 1.0, ≈2.0 near 1e16) — a fixed absolute tolerance is wrong across scales.
- **The 2^53 integer trap**: doubles represent every integer only up to `2^53` (binary32: `2^24`). 64-bit IDs, epoch-nanosecond timestamps, or BIGINTs through a `double` silently lose precision (`2^53 + 1 == 2^53`). Flag `(double)int64` and JSON numeric IDs.
- **Narrowing**: binary64→binary32 drops to ~7 digits and can turn a stable computation unstable; flag accumulators narrowed for memory. In ML, fp16 overflows easily (needs loss-scaling); bf16 keeps fp32's exponent range but ~3 mantissa digits — keep reductions/master weights in fp32.
- **Money and time are never binary floats**: use integer minor units, a decimal type (`Decimal`/`BigDecimal`/SQL `NUMERIC`), or exact rationals; specify the rounding mode explicitly; verify that split/allocation conserves the total. Flag `float`/`double`/`real` in money or large-counter schemas.

### Reproducibility and environment

The same source can produce different — or wrong — results across hardware, OS, compiler, libraries, build flags, and runtime mode. **Bit-for-bit FP reproducibility across platforms is never the default; it requires deliberate, tested, documented effort.**

- **Fast-math** (`-ffast-math`, `-Ofast`, MSVC `/fp:fast`) is a bundle that breaks IEEE semantics: `-ffinite-math-only` lets the optimiser *delete your `isnan`/`isinf` guards*; `-fno-signed-zeros` breaks branch-cut/`copysign` logic; `-fassociative-math` reassociates sums (silently collapsing Kahan/compensated summation to the naive result) and enables vectorised reductions that change order. MSVC `/fp:fast` can make `NaN == NaN` true and `Inf - Inf` zero.
- **The process-global hazard**: on x86, linking anything built with `-ffast-math`/`-Ofast` pulls in `crtfastmath.o`, a static constructor that sets FTZ/DAZ in MXCSR *before `main()`*. **Merely loading a shared library built with fast-math flushes subnormals to zero in unrelated, IEEE-conformant code.** Signature: "numerics changed after adding/upgrading a dependency, no source change."
- **FMA contraction**: compilers may fuse `a*b+c` to one rounding (`-ffp-contract`); defaults differ — GCC `fast`, Clang `on`, MSVC off (pre-VS2022 on). More accurate, but `a*a - b*b` can fuse to a non-zero/negative result when `a==b`, and discriminants can go negative and break `sqrt`. For exact cancellation or sign predicates, set `-ffp-contract=off` and call `fma` explicitly where you want it.
- **x87 vs SSE2**: 32-bit x86 / `-mfpmath=387` evaluates intermediates at 80-bit and rounds back at unpredictable spill points → double-rounding, debug-vs-release and machine-to-machine differences (`FLT_EVAL_METHOD`; `long double` width is non-portable). Build for SSE2 (`-msse2 -mfpmath=sse`, default on x86-64).
- **Rounding mode**: `fesetround` is ignored unless `#pragma STDC FENV_ACCESS ON` (GCC/Clang `-frounding-math`, MSVC `/fp:strict`) is in effect — the optimiser otherwise constant-folds at round-to-nearest. Pair any change with save/restore.
- **libm / CPU / SIMD**: transcendentals (`sin`/`exp`/`log`/`pow`) are not required to be correctly rounded — glibc vs musl vs Apple vs MSVC, different glibc versions, and scalar-vs-vectorised (`libmvec`/SVML) paths differ in the last ULP. Don't assert bit-exact transcendentals across platforms.
- **Parallelism**: float `+` is non-associative, so any reduction whose order depends on thread count, tiling, or scheduling (OpenMP `reduction`, `std::reduce`, BLAS `gemm`, GPU `atomicAdd`) is run-to-run non-deterministic. Pin thread count and BLAS reproducibility mode (`MKL_CBWR`), use ordered/deterministic reductions, and test with tolerances.
- **Language runtimes**: numba `fastmath=True` (LLVM fast-math), `strictfp` history (Java <17), numpy reduction accumulator `dtype`/axis ordering. Rust scalar FP is strict; fast-math is opt-in only.

### Testing & detection

- **Exact-equality float tests are wrong** unless the value is exactly representable and produced by exact ops (small integers, powers of two — there `==` is *preferred*, don't add slop). Otherwise use a justified tolerance with both `rtol` and `atol` (comparison to zero always needs `atol`), or ULP distance against a correctly-rounded reference. `unittest.assertAlmostEqual` is absolute/scale-blind — prefer `pytest.approx`/`math.isclose`/`numpy.isclose` with explicit tolerances.
- **Property / metamorphic tests** survive roundoff where golden values don't: inverse identities (`exp(log(x))≈x`), symmetry, monotonicity, conservation (softmax sums to 1). Use generators (Hypothesis) that deliberately include NaN/±Inf/±0/subnormals.
- **Oracle**: compute in high/arbitrary precision (mpmath, MPFR/`gmpy2`, exact `Fraction`) and bound the error; or differentially test against an independent implementation (SciPy/LAPACK) or fp32-vs-fp64. For linear algebra, check the residual `‖Ax-b‖` (backward error) not `‖x-x_true‖`.
- **Make failures loud at the origin**: `feenableexcept(FE_INVALID|FE_DIVBYZERO|FE_OVERFLOW)` turns the first NaN/div-0/overflow into `SIGFPE`; numpy `seterr(all='raise')` / `errstate`; UBSan `-fsanitize=float-cast-overflow` for the UB of out-of-range float→int casts; valgrind/MSan for uninitialised-read NaNs. (Leave `FE_INEXACT`/`FE_UNDERFLOW` off — they fire constantly.)
- **Roundoff estimation / repair**: Verrou (drop-in Monte-Carlo arithmetic in valgrind) or CADNA estimate the number of correct significant digits and localise instability; Herbie rewrites an expression for accuracy.
- **Reproducibility tests**: vary thread count, optimisation level, FMA, and BLAS backend; large divergence signals either genuine instability or an unsafe fast-math reassociation.

## Red flags

- `== `/`!=` on a computed float; `while (x != ...)`; float loop counters or step accumulation.
- `x == NaN` / `x != NAN` (always false/true — use `isnan`); a reduction over external data with no NaN policy.
- Literal quadratic formula; `b*b - 4*a*c`; `sqrt(x*x + y*y)` (want `hypot`); `log(sum(exp(...)))` without max-subtraction.
- `exp(x) - 1`, `log(1 + x)`, `(1+r)**n - 1`, `1 - cos(x)` (want `expm1`/`log1p`/half-angle).
- `sum_x2`/`E[x²]-E[x]²` variance; `A.T @ A` / normal-equations least squares; explicit `inv(A)` then multiply; hand-written GE/LU with no pivot search.
- `float` (binary32) accumulator in a long sum/dot product; tolerance compared as a single global `EPSILON` against `fabs(a-b)`; relative tolerance with no absolute floor; numpy `isclose` with expected value first.
- `(int)(x * 100)` / `float→int` casts (truncation + UB on overflow); narrow-int multiply assigned to a wider type; `int64` IDs or timestamps through `double`; `float`/`double` money fields; `0.1`/`0.2`/`0.3` literals expected exact.
- Build files with `-ffast-math`, `-Ofast`, `-funsafe-math-optimizations`, `/fp:fast`, `-ffp-contract=fast`, `-ffinite-math-only`; a dependency built with fast-math (`crtfastmath.o` in the link); 32-bit x86 / `-mfpmath=387` / `long double`.
- `_mm_setcsr`/`_MM_SET_FLUSH_ZERO_MODE`/`fesetround`/`_controlfp` without save-restore, or `fesetround` without `#pragma STDC FENV_ACCESS ON`; FP-state changes in `__attribute__((constructor))`.
- Parallel reductions / `std::reduce` / GPU `atomicAdd(float)` / unpinned BLAS where reproducibility is expected; tests asserting bitwise equality across thread counts or platforms.
- Custom numerical routine with only golden-value tests — no high-precision oracle, no property/metamorphic tests, no special-value cases.
