## Context

The runtime already keeps protocol-specific and storage-specific details inside `runtime/internal`, but `runtime/agent/runner.go` still contains most of the branching for direct runs, regular profile runs, and `acp-stdio` profile runs. That leaves the public runner owning profile lookup, mode dispatch, effective model selection, and ACP delegation details even though those concerns are implementation internals rather than public contract.

The requested refactor is architectural, not behavioral. The public `agent.Runner` contract, HTTP API semantics, and existing run-mode behavior should stay the same. What changes is ownership: execution-path resolution should move behind an internal runner boundary so the public layer remains a thin orchestrator over dependency wiring and exported helpers such as `ModelsLocator`, `AutoMigrate`, `ReadSession`, and `ListSessions`.

## Goals / Non-Goals

**Goals:**

- Move direct, regular-profile, and `acp-stdio` execution-path logic out of public `runtime/agent/runner.go`.
- Introduce a dedicated internal runner component that owns profile lookup, mode dispatch, effective-model resolution, and path-specific execution setup.
- Keep the public `agent.Runner.Run` method minimal and stable from an embedder perspective.
- Preserve current run behavior and error classifications for direct runs, regular profiles, and `acp-stdio` profiles.
- Make the public-vs-internal responsibility split obvious in docs and tests.

**Non-Goals:**

- Changing the HTTP request/response schema for runs.
- Changing profile persistence schema or execution mode values.
- Changing ACP stdio protocol execution, result mapping, or session recording semantics.
- Refactoring session listing, model listing, or storage migration APIs beyond what is needed to compose the new internal runner.

## Decisions

1. Compose a dedicated internal execution runner during `agent.NewRunner`.

Rationale: the public constructor already wires the dependencies needed for run execution: model resolution, system prompt fragments, session storage, profile service, and ACP runner. Creating one internal execution runner from those dependencies lets `agent.Runner` delegate `Run` without continuing to own execution-path branching.

Implementation direction:

- Add a new internal runner type in `runtime/internal` or another runtime-internal package with a focused `Run(ctx, params)` entry point.
- The internal runner accepts the built-in runner factory, profile service, ACP profile runner, default app/agent names, system prompt fragments, and tools provider through a params struct.
- The public `Runner` stores that internal runner as an unexported dependency and calls it from `Run`.

Alternative considered: keep the current helpers in `agent.Runner` and only move some branches into additional private methods. Rejected because it keeps the public package as the owner of execution-path behavior, which is the boundary the user wants to change.

2. Keep public `agent.Runner` logic limited to public-surface orchestration.

Rationale: the public package should expose a tight contract and avoid accumulating implementation policy. `Run` may still perform minimal request normalization if needed, but it should not load profiles, branch on execution mode, or prepare ACP-specific requests.

Implementation direction:

- `agent.Runner.Run` delegates the run request into the internal execution runner.
- `agent.Runner` continues to own constructor-time wiring, `ModelsLocator`, `AutoMigrate`, `ReadSession`, and `ListSessions`.
- Existing exported types (`RunParams`, `RunResult`, `AgentRunner`) remain unchanged.

Alternative considered: move the entire public `Runner` implementation into `runtime/internal` and make the exported type a thin wrapper. Rejected for this change because it increases file churn without improving the contract beyond delegating `Run`.

3. Centralize path resolution inside the internal runner, not inside ACP-specific code.

Rationale: ACP-specific code should stay ACP-specific. The decision about whether a run is direct, regular-profile, or ACP-profile is generic runtime orchestration and belongs in one internal coordinator. Once the path is chosen, the coordinator should delegate to the built-in runner path or ACP runner path.

Implementation direction:

- Internal runner resolves `profileName` and request-level `model`.
- No-profile and regular-profile paths share a built-in execution helper that prepares `internal.NewAgentRunnerParams`.
- `acp-stdio` selection delegates to `acpstdio.ACPProfileRunner`.
- Unsupported or missing profile cases continue to use the existing profile-run error kinds so HTTP mapping stays stable.

Alternative considered: move all branching into `internal/acpstdio` plus existing built-in runner helpers. Rejected because path resolution would remain split across packages instead of being owned by one internal runtime coordinator.

4. Tests should assert the boundary, not just behavior.

Rationale: behavior-only tests are necessary but not sufficient for this refactor. The failure mode here is architectural regression, where the public runner quietly grows execution-path logic again while behavior remains green.

Implementation direction:

- Add internal runner tests for direct, regular-profile, missing-profile, unsupported-mode, and ACP-delegated runs.
- Simplify public runner tests so they verify delegation/wiring and exported helper behavior rather than path-specific branching.
- Update docs (`runtime/AGENTS.md` and related architecture text if needed) to describe the public runner as a thin orchestrator over internal execution paths.

Alternative considered: rely only on existing runner behavior tests. Rejected because they would not make ownership drift visible.

## Risks / Trade-offs

- [New internal abstraction duplicates existing low-level runner concepts] -> Keep the new internal runner focused on path orchestration only; reuse `internal.AgentRunnerFactory` for built-in execution and `acpstdio.ACPProfileRunner` for ACP execution.
- [Refactor churn could scatter logic across too many internal files] -> Prefer one clearly named internal coordinator plus small helper functions instead of multiple vague dispatcher/router layers.
- [Behavior drift during ownership move] -> Preserve current scenarios and error kinds in tests before simplifying public-runner coverage.
- [Public runner could still retain hidden policy via helper methods] -> Remove or shrink public helpers that exist only for execution-path branching after the internal runner is introduced.

## Migration Plan

1. Introduce the internal execution runner and move direct/profile/ACP path branching into it.
2. Update `agent.NewRunner` to compose the internal execution runner from existing dependencies.
3. Simplify `agent.Runner.Run` to delegate to the internal runner and remove now-redundant public helpers/fields.
4. Update tests to cover the new internal boundary plus unchanged public behavior.
5. Update runtime docs that describe runner responsibility boundaries.
6. Run repository verification during apply (`make affected-lint-test`).

Rollback is a normal code rollback. No persistence, API, or migration compatibility work is required.

## Open Questions

None.
