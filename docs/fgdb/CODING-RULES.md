# FGDb coding rules

**Draft v0 — proposed by Crdb Steward from existing practice, pending Vincent's approval.**

> **FGL page.** Future Gadget Laboratories wrote this for FutureGadgetDatabases.
> It is not a Cockroach Labs document.

Use these proposed rules to review every FGDb feature PR. Copy the checklist into the PR body and cite the evidence for each applicable rule.

## Naming

**FGDB-NAME-1.** Use short, descriptive names that match nearby CockroachDB code. Follow Go casing (`SQL`, `SQLState`, `clientID`), name `context.Context` parameters `ctx`, and start exported Go doc comments with the identifier they describe. Avoid shadowing predeclared identifiers such as `len` and `error`.

**Rationale.** Familiar names make call sites and package APIs easier to read.

**How it is checked.** Review names and exported comments against the surrounding code. Sonar `go:S978` checks predeclared identifier shadowing. See the [CockroachDB style guide pointer](../style.md).

## Function size and cognitive complexity

**FGDB-SIZE-1.** Keep each function focused on one responsibility and its cognitive complexity at 15 or below. Use early returns to limit nesting, extract helpers for distinct steps, and keep FGDb Go functions to at most seven parameters. Give a meaningful name to a string literal that repeats three or more times.

**Rationale.** A short control flow makes error paths and state transitions easier to trace.

**How it is checked.** Sonar `go:S3776` checks cognitive complexity and `go:S1192` checks repeated string literals. Review function scope and parameter count. See [Testing](testing.md#how-to-add-a-test).

## Error handling

**FGDB-ERR-1.** Handle every error. Return it, add actionable context with `github.com/cockroachdb/errors.Wrap` or `Wrapf`, or recover explicitly when the operation can continue safely. Preserve the cause and any existing SQLSTATE. Use `pgerror.New` or `pgerror.Newf` with an appropriate `pgcode` for new user-facing SQL errors, and `pgerror.Wrap` or `Wrapf` when attaching a SQLSTATE to an existing error. Keep credentials and private details out of user-facing messages.

**Rationale.** Callers need stable SQLSTATEs and enough context to diagnose failures.

**How it is checked.** Review error propagation, including cleanup failures, and assert error causes or SQLSTATEs in tests where callers depend on them. See [OSS error boundaries](architecture/BOUNDARIES.md#how-a-missing-feature-fails) and [pgerror](../../pkg/sql/pgwire/pgerror).

## No panics in SQL/KV request paths

**FGDB-PANIC-1.** Return errors for invalid input, recoverable failures, and unexpected state in SQL and KV request paths. Reserve `panic` for programmer invariants where the surrounding upstream code already uses that pattern and user input cannot reach the panic.

**Rationale.** A failed request should surface an error while the node continues serving requests.

**How it is checked.** Review the paths from SQL and KV entry points to assertions and panics. Exercise invalid inputs and failure paths in targeted tests.

## Bounded resources

**FGDB-RES-1.** Account for SQL request allocations with the existing memory monitor and bound accounts, handle exhausted budgets, and release accounts on every exit. Bound loops over input, retries, buffers, and goroutine counts. Pass context through blocking operations, set timeouts for waits and I/O, and ensure spawned work exits on cancellation.

**Rationale.** One request or stalled dependency must have a predictable limit on the work it can consume.

**How it is checked.** Review budgets, retry bounds, cancellation, and cleanup. Test limit and timeout paths. See [SQL memory accounting](../../pkg/sql/rowexec/aggregator.go) and [multipart upload limits](../../pkg/cmd/fgdb-backup/store_s3.go).

## Gated or off-by-default features

**FGDB-GATE-1.** Put new FGDb database features behind a `SystemVisible` cluster setting named `fgdb.<feature>.enabled`, default `false`, using `settings.RegisterBoolSetting`. Check the setting before FGDb-specific work. When off, take the existing OSS path with the same results, errors, SQLSTATE, and job state. Test both states and follow the proposed release-compatibility and rollback requirements for activation.

**Rationale.** Existing workloads retain OSS behavior until an operator enables the feature on a compatible cluster.

**How it is checked.** Review the setting and entry points. Run gate-off parity and gate-on conformance tests, including activation checks where applicable. See [version gating](version-gating.md) and [gate unit tests](../../pkg/fgdb/gate/gate_test.go).

## Cleanroom/licensing

**FGDB-LICENSE-1.** License new FGDb implementation and tests under Apache-2.0. Never read, copy, modify, or import anything under `pkg/ccl/`, or use CockroachDB Enterprise or CSL source. For cleanroom work, implement from the approved spec and public references and record provenance. Preserve notices and record edits when touching existing vendored code.

**Rationale.** License and provenance records let reviewers verify independent implementation and protect the OSS distribution boundary.

**How it is checked.** Review headers, provenance, and approved spec handoffs. The FGDB checks workflow runs the boundary script on its configured paths. See [Cleanroom](CLEANROOM.md), [FGDB checks](../../.github/workflows/fgdb-checks.yml), and the [Porcupine vendor record](../../pkg/fgdbtest/third_party/porcupine/README.md).

## Test layout

**FGDB-TEST-1.** Put Go unit tests beside the code in `*_test.go` files. Put SQL logictests under `pkg/sql/logictest/testdata/logic_test/`. When adding, removing, or reconfiguring logictests, regenerate the corresponding `generated_test.go` files with `dev generate testlogic`. Include gate-off cases and enabled behavior for gated changes. Keep harness code out of the production binary and mark its Bazel targets `testonly = True`.

**Rationale.** Tests close to their implementation and wired into the existing suite make coverage discoverable and runnable.

**How it is checked.** Review test placement and generated registrations. Run focused tests and the relevant FGDb tier, and record any skips. See [Testing](testing.md) and [generated logictest registrations](../../pkg/sql/logictest/tests/local/generated_test.go).

## Feature docs

**FGDB-DOC-1.** Each feature PR adds `docs/fgdb/features/<feature>.md` using the feature template. Link each spec requirement to the implementing code and to the tests in a spec → code → test cross-reference table. Record the commands or CI runs, outcomes, and gaps in the test-evidence section. Link back to the feature page from relevant code doc comments.

**Rationale.** Reviewers can trace each promised behavior to its implementation and verify the evidence.

**How it is checked.** Review the feature page, cross-reference rows, code links, and test evidence in the PR. See the [feature documentation template](features/README.md).

## Sonar gate

**FGDB-SONAR-1.** Require zero new Sonar issues against the `main` baseline in the FGL scope and a passing quality gate. Supply an explicit comparator for JavaScript array sorts and preserve the intended ordering.

**Rationale.** The baseline keeps new findings visible as FGDb changes accumulate.

**How it is checked.** Review the Sonar new-issue count and quality gate for the PR against `main`. Sonar `javascript:S2871` checks array sort comparators. See the [Sonar configuration](../../sonar-project.properties) for the existing vendored complexity exceptions.

## Comments policy

**FGDB-COMMENT-1.** Keep existing comments verbatim unless the same change removes their subject. Use new comments to explain intent, invariants, or constraints that the code alone cannot convey.

**Rationale.** Preserving context helps reviewers and future changes respect the assumptions behind the code.

**How it is checked.** Review removed or reworded comment lines in the diff and confirm that each subject was removed in the same change.

## PR checklist

- [ ] `FGDB-NAME-1` Names and exported comments follow local conventions.
- [ ] `FGDB-SIZE-1` Functions stay within the complexity and parameter limits.
- [ ] `FGDB-ERR-1` Errors are handled with appropriate context and SQLSTATEs.
- [ ] `FGDB-PANIC-1` SQL and KV request failures return errors.
- [ ] `FGDB-RES-1` Resource use, retries, and waits are bounded.
- [ ] `FGDB-GATE-1` Feature gates default off and both states are tested.
- [ ] `FGDB-LICENSE-1` Licensing, source boundaries, and provenance are verified.
- [ ] `FGDB-TEST-1` Tests follow the suite layout and registrations are current.
- [ ] `FGDB-DOC-1` Feature docs link the spec, code, tests, and evidence.
- [ ] `FGDB-SONAR-1` The quality gate passes with zero new issues against `main`.
- [ ] `FGDB-COMMENT-1` Comment edits are limited to subjects removed in this change.
