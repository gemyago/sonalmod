## Context

Current ACP stdio wiring still requires `agent.Runner` to construct `acpstdio.SessionRecorder` before creating the ACP profile runner. That coupling leaks ACP-specific setup details into runner construction and diverges from the runtime's established pattern of passing a single params struct into behavior-specific components.

This change keeps standard run behavior intact while narrowing responsibility boundaries: `agent.Runner` should orchestrate mode selection, and ACP internals should own ACP execution setup details (including recorder creation).

## Goals / Non-Goals

**Goals:**
- Move ACP session recorder initialization into the ACP profile runner constructor path.
- Replace multi-argument ACP profile execution entry points with a dedicated params struct.
- Keep `agent.Runner` profile-mode dispatch behavior unchanged from an API perspective.
- Preserve existing ACP stdio runtime semantics (streaming contract, ignored request-level model).

**Non-Goals:**
- Changing HTTP API schema for run requests.
- Changing profile persistence schema.
- Changing ACP protocol execution or event mapping semantics.
- Refactoring unrelated regular profile execution paths.

## Decisions

1. ACP profile runner constructor owns session recorder creation.

Rationale: Recorder lifecycle is ACP-specific and should be hidden behind ACP runtime boundaries. This reduces cross-package construction knowledge in `agent.Runner`.

Alternative considered: Keep recorder construction in `agent.Runner` and only wrap arguments in a struct. Rejected because it preserves leaked ACP setup responsibilities.

2. ACP execution entry points accept a params struct instead of positional arguments.

Rationale: The codebase convention prefers consumer-defined interfaces with concrete struct params when argument count and context grow. A struct provides explicit naming, easier extension, and lower call-site fragility.

Alternative considered: Keep positional args and rely on comments/documentation. Rejected because it does not solve readability or extensibility concerns.

3. Preserve behavior contract while changing wiring ownership.

Rationale: This refactor targets maintainability and architecture consistency, not product-level behavior. Existing tests for ACP execution outcomes should remain valid with minimal adaptation.

Alternative considered: Introduce behavior changes together with refactor. Rejected to keep blast radius small and make regressions easier to detect.

## Risks / Trade-offs

- [Constructor churn across ACP call sites] -> Update all call sites in one atomic refactor and keep compile-time coverage for missing fields.
- [Hidden behavior drift while moving recorder setup] -> Reuse existing ACP execution tests and add focused constructor/wiring tests.
- [Params struct growth over time] -> Keep struct scoped to ACP runner package and document required/optional fields clearly.

## Migration Plan

1. Introduce ACP params struct and update ACP runner signatures.
2. Move session recorder initialization into ACP runner construction.
3. Simplify `agent.Runner` ACP setup call path.
4. Update unit tests for constructors and ACP execution wiring.
5. Run repo verification (`make affected-lint-test`) during apply.

Rollback strategy: revert the refactor commit(s); no data/schema migration is involved.

## Open Questions

None.
