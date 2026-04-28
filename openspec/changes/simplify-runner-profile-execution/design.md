## Context

The runtime already has profile-backed standard runs, but the current code path is over-layered:

- `agent.Runner` stores `profileRuns *profileexec.Dispatcher`.
- `agent.NewRunner` only creates that dispatcher when `RunnerArgs.AgentProfilesService` is non-nil.
- `Runner.Run` delegates every profile-backed request to the dispatcher.
- The dispatcher loads the profile, then calls a `regularRunner` adapter back into `Runner` for regular profile execution.
- The same dispatcher also owns ACP stdio result mapping and session recording.

This creates two mismatches with the intended architecture. First, `AgentProfilesService` is optional even though HTTP profile management and profile-backed execution are now part of the runtime contract. Second, regular profile execution takes an unnecessary detour through a generic dispatcher and wrapper even though the built-in runner already owns model resolution, system prompt construction, tools, and session execution.

## Goals / Non-Goals

**Goals:**

- Require `AgentProfilesService` in `agent.NewRunner`.
- Make `agent.Runner.Run` the single owner of standard run dispatch for direct, regular-profile, and ACP-profile requests.
- Execute regular profiles directly through runner-owned built-in runner helpers.
- Move ACP stdio execution, result-to-session-event mapping, and ACP session recording behind ACP-specific internal code.
- Preserve the existing standard HTTP request rules: no `profileName` requires `model`; regular profiles allow request `model` override; ACP stdio ignores request `model`.
- Preserve public API simplicity: embedders still pass a runner to `httpapi.NewHandler`, not a profile dispatcher.

**Non-Goals:**

- Changing the HTTP run request schema.
- Changing agent profile CRUD schema or persistence.
- Changing ACP stdio protocol semantics.
- Adding runtime behavior for profile `role` or `toolRefs`.
- Reintroducing any public OpenCode-specific endpoints or services.

## Decisions

1. `RunnerArgs.AgentProfilesService` becomes required.

   Rationale: profile-backed run selection is not an optional plugin path anymore. Keeping the service optional forces `Runner.Run` to carry a dead configuration branch that can only fail at request time. Failing at construction gives embedders a clear wiring error and makes the runner's behavior deterministic.

   Alternative considered: keep optional construction and return "profile execution unavailable" only when `profileName` is provided. Rejected because it preserves the false optionality the user identified.

2. `Runner` stores the profiles service directly and removes the generic dispatcher field.

   Implementation direction:

   - Add an unexported `profiles AgentProfilesService` field to `Runner`.
   - Remove `profileRuns *profileexec.Dispatcher`.
   - Add a small runner-owned helper for direct built-in execution, for example `runBuiltIn(ctx, params, agentName, profileInstructions)`.
   - In `Run`, trim `params.ProfileName` and `params.Model`.
   - If `profileName` is empty, require `model` and call the built-in helper with `defaultRunnerAgentName` and no profile instructions.
   - If `profileName` is present, load the profile through `r.profiles.Get`.
   - For omitted or `regular` mode, resolve effective model as request `model` override or profile `defaultModel`, then call the same built-in helper with profile `Name` and `Instructions`.
   - For `acp-stdio`, delegate to an ACP-specific runner/helper.
   - For unsupported modes, return the same error classification expected by HTTP error mapping.

   Alternative considered: leave `profileexec.Dispatcher` in place but move only regular execution into the runner. Rejected because the dispatcher would still be a generic orchestration layer without a clear responsibility.

3. Generic profile execution errors move out of `profileexec`.

   Rationale: `profileexec` should not remain as a generic package if the only retained non-regular execution mode is ACP-specific. HTTP error mapping still needs stable validation, not-found, unsupported, and execution classifications, but those classifications are not ACP-specific.

   Implementation direction:

   - Move `ErrorKind` and the wrapper error type to a neutral internal location that both `agent.Runner` and `internal/agentapi` can import, such as `runtime/internal`.
   - Update `AgentAPIServer.writeAgentRunError` to inspect the neutral internal error type instead of `profileexec.Error`.
   - Keep external behavior unchanged: validation/unsupported map to `400`, missing profiles map to `404`, execution failures map to `500`.

   Alternative considered: expose profile run errors from the public `agent` package. Rejected because HTTP internals can import runtime internal packages and there is no reason to widen the public contract.

4. ACP stdio profile execution gets an ACP-specific boundary.

   Rationale: ACP stdio has genuinely different mechanics: command mapping, subprocess launch, protocol result mapping, synthetic session events, and manual session recording. Those concerns should be isolated from regular built-in runner construction, not mixed into a generic profile dispatcher.

   Implementation direction:

   - Move or rename `profileexec` ACP-only files into an ACP-focused internal package or into `runtime/internal/codinglane`.
   - Keep `codinglane.ACPStdioExecutor` as the subprocess/protocol executor.
   - Add an ACP profile run helper that accepts the resolved `AgentProfile`, request user/session/message data, and returns `*internal.RunResult`.
   - The helper maps `AgentProfile.executionSettings` and message text into `ACPStdioExecutorRequest`, executes it, converts updates/results/errors into `internal.SessionEvent` values, records the user and agent events through session storage, and returns a standard `RunResult`.
   - `Runner.Run` should only decide that the selected profile is ACP stdio and delegate to this helper; it should not own ACP protocol result mapping.

   Alternative considered: keep ACP mapping in `profileexec` but delete regular dispatch. Rejected because the remaining package name would still imply generic profile execution and keep the layering ambiguity.

5. Tests should verify behavior at the runner boundary, not the removed wrapper boundary.

   Rationale: the behavior users depend on is `agent.Runner.Run` and the HTTP mapping around it. Unit tests that primarily prove `Dispatcher` calls `regularRunner` are testing the layer being removed.

   Implementation direction:

   - Update runner constructor tests to require both providers and profiles services.
   - Move regular profile tests from `profileexec.Dispatcher` to `runtime/agent` runner tests.
   - Keep ACP result mapping/session recording tests with the ACP-specific package after the move.
   - Update `internal/agentapi` tests that assert profile error mapping to use the neutral internal error type.
   - Update all `NewRunner` test fixtures to provide a profiles service.

   Alternative considered: keep dispatcher tests as compatibility tests. Rejected because the dispatcher should not survive as a generic unit.

## Risks / Trade-offs

- [More test fixtures need a profiles service even when they only exercise direct model runs] -> Add a small local stub or helper in affected tests so the required dependency is explicit and cheap.
- [Moving error types can create noisy import churn] -> Keep the replacement internal error type small and mechanically equivalent to the current `profileexec.Error`.
- [ACP session recording could regress during package moves] -> Move tests with the code and add one runner-level ACP delegation test that verifies session events are still returned through the standard run contract.
- [Runner.Run becomes longer if all dispatch code is inlined] -> Keep only orchestration in `Run`; extract unexported helpers for profile loading, regular built-in execution, and ACP delegation.
- [Package naming could introduce another vague abstraction] -> Prefer names that describe the surviving behavior, such as ACP stdio profile execution, over generic terms like dispatcher, wrapper, or profileexec.

## Migration Plan

1. Make `AgentProfilesService` required in `NewRunner` and update all construction sites/tests.
2. Add runner-owned helpers for direct built-in and regular-profile built-in execution.
3. Move generic profile error classification to a neutral internal location and update HTTP error mapping.
4. Move ACP stdio result mapping and session recording out of `profileexec` into ACP-specific internals.
5. Replace `profileexec.Dispatcher` usage in `Runner.Run` with direct runner logic plus ACP delegation.
6. Delete the generic dispatcher and regular-run adapter types.
7. Update `runtime/AGENTS.md` and any affected architecture docs.
8. Run `make affected-lint-test` from the repository root.

Rollback is a normal code rollback. The project is pre-release, and no runtime compatibility shim is required for constructing `Runner` without `AgentProfilesService`.

## Open Questions

None.
