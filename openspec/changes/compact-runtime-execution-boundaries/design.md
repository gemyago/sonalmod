## Context

The current runtime execution flow is structurally correct but harder to reason about than it needs to be:

- `runtime/agent/runner.go` wires a separate `ProfileExecutionRunner` from `runtime/internal/profileexec.go`.
- `runtime/internal/agentrun.go` already owns the built-in runner construction path: model resolution, tools, prompt assembly, session handling, and low-level runner creation.
- `runtime/internal/acpstdio` owns ACP-profile orchestration and session recording, but `runtime/internal/codinglane` owns the ACP request mapper, executor types, subprocess client, and protocol mechanics for the same execution mode.
- `runtime/internal/agentexec_error.go` provides useful internal error classification, but `runtime/internal/agentapi/server.go` currently treats wrapped error text as part of the public `400` response detail for some profile-selection failures.

The requested change is a boundary compaction, not a feature expansion. Standard run behavior must stay the same, but the ownership model should become obvious from the package layout and the public HTTP contract should stop reflecting incidental internal error strings.

## Goals / Non-Goals

**Goals:**

- Make `runtime/internal/agentrun.go` the single internal owner of direct-run and regular-profile dispatch.
- Remove `runtime/internal/profileexec.go` as a separate orchestration layer.
- Consolidate generic ACP stdio mechanics under `runtime/internal/acpstdio`.
- Preserve the current execution semantics for direct runs, regular profiles, and `acp-stdio` profiles.
- Keep stable classification for validation, not-found, unsupported, and execution failures while preventing low-level wrapped errors from becoming part of the public HTTP detail contract.
- Rewrite tests and docs so they describe the surviving boundaries instead of the removed split.

**Non-Goals:**

- Changing request or response schemas for agent runs.
- Changing persisted profile schema or supported execution modes.
- Changing ACP stdio protocol behavior, launch settings semantics, or session replay semantics.
- Introducing new public runtime abstractions.
- Reworking unrelated internal package boundaries outside agent run execution and ACP stdio.

## Decisions

1. Standard run dispatch moves into `agentrun.go`.

   Rationale: direct runs and regular profile runs already depend on the same built-in machinery owned by `runtime/internal/agentrun.go`. Keeping profile lookup and mode branching in `profileexec.go` preserves an extra mental model without isolating truly different behavior.

   Implementation direction:
   - Move profile lookup, effective-model resolution, selected-profile instruction handling, and ACP-mode branching into the internal agent-run area.
   - Remove `ProfileExecutionRunner` as a separate runtime concept. If a small coordinator type still helps, it should live in `agentrun.go` as part of the standard agent-run implementation rather than in a sibling file whose only job is dispatch.
   - Keep `runtime/agent/runner.go` thin: it wires dependencies and delegates `Run` into the standard internal agent-run path.

   Alternative considered: keep `profileexec.go` but treat it as “close enough” because it is already in `runtime/internal`. Rejected because the user-visible maintenance problem is not package export level, it is that the built-in execution path still appears to have two owners.

2. ACP stdio internals are consolidated into `runtime/internal/acpstdio`.

   Rationale: `acpstdio` and `codinglane` currently describe the same concern from two different sides. Every ACP change crosses the package boundary even though request mapping, executor launch, result translation, and session recording are all part of one execution mode.

   Implementation direction:
   - Move generic ACP request mapping, executor request/result types, subprocess launch logic, and related helpers from `runtime/internal/codinglane` into `runtime/internal/acpstdio`.
   - Keep OpenCode-specific naming only at the executable-specific leaf adapter level when a type is explicitly about the OpenCode binary; generic ACP concepts should live in `acpstdio`.
   - Update imports so the rest of the runtime depends on one ACP-focused internal boundary.

   Alternative considered: keep `codinglane` as a low-level transport/mechanics package beneath `acpstdio`. Rejected because it adds indirection without providing a reusable abstraction beyond ACP stdio itself.

3. Error classification stays internal, but public problem details become stable and sanitized.

   Rationale: the runtime still needs a compact way to classify profile-selection and execution failures across `agentrun`, `acpstdio`, and `agentapi`. The issue is not the existence of a classified internal error, but the fact that wrapped internal error text is currently reused as public HTTP detail.

   Implementation direction:
   - Keep or replace `AgentExecError` with an equivalent internal classification mechanism carrying validation, not-found, unsupported, and execution categories.
   - Treat the classifier as internal transport only. `runtime/internal/agentapi/server.go` maps error kinds to stable problem details instead of forwarding arbitrary wrapped messages.
   - Log the full wrapped error internally so debugging fidelity is preserved without expanding the HTTP contract.

   Alternative considered: remove classified execution errors entirely and infer HTTP status directly from raw wrapped errors. Rejected because that would make handler behavior brittle and couple transport mapping to dependency-specific error shapes.

4. Tests move from wrapper coverage to boundary coverage.

   Rationale: the regression risk is not “does `profileexec.go` exist,” but whether the runtime still dispatches the three execution paths correctly and preserves public error behavior after the cleanup.

   Implementation direction:
   - Move `profileexec`-focused tests into the standard agent-run test area.
   - Move `codinglane` tests into `acpstdio` tests, keeping leaf-client coverage where subprocess/protocol behavior is exercised.
   - Keep HTTP handler tests for stable `400`, `404`, and `500` problem details.

   Alternative considered: preserve the current file/package-oriented tests and only adjust imports. Rejected because the test suite would continue encoding the old architecture after the implementation has changed.

## Risks / Trade-offs

- [The cleanup could just replace one wrapper with a renamed wrapper] -> Anchor dispatch in `agentrun.go` and avoid introducing a new sibling file/package with the same orchestration role.
- [ACP package consolidation could become a large mechanical rename] -> Keep the move scoped to generic ACP stdio code and leave only truly executable-specific naming at leaf adapters.
- [Sanitizing public errors could reduce debugging clarity for clients] -> Preserve internal logs and error wrapping while keeping the HTTP detail surface intentionally small and stable.
- [Moving tests could create temporary blind spots] -> Preserve behavioral coverage first, then delete old files only after equivalent tests exist in the surviving boundaries.

## Migration Plan

1. Move standard run dispatch logic from `profileexec.go` into the internal agent-run implementation.
2. Update `runtime/agent/runner.go` wiring to depend on the compacted internal agent-run path instead of a separate profile executor.
3. Consolidate generic ACP stdio code from `runtime/internal/codinglane` into `runtime/internal/acpstdio`.
4. Tighten `agentapi` error mapping so public problem details no longer depend on wrapped internal error strings.
5. Migrate or rewrite tests to match the new ownership boundaries.
6. Remove obsolete files and references (`profileexec.go`, `codinglane` package remnants, stale comments/docs).
7. Run repository verification and confirm the OpenSpec capability delta is satisfied.

Rollback is a standard code rollback. No schema, storage, or API migration is required.

## Open Questions

None.
