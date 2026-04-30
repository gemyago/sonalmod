## Context

The current runtime execution path is layered in a way that makes the ownership boundary harder to follow than it needs to be:

- `runtime/agent/runner.go` wires `runtime/internal/profilerun.ExecutionRunner`.
- `runtime/internal/profilerun/execution_runner.go` trims request inputs, loads the selected profile, branches on execution mode, and then delegates regular execution back into the standard built-in runner path.
- The built-in runner path already lives in `runtime/internal/agentrun.go`, where model resolution, system prompt construction, tools, session handling, and low-level agent runner creation are owned.
- Only `acp-stdio` execution actually needs mode-specific mechanics such as ACP request mapping, subprocess execution, result translation, and session recording, and those already live under `runtime/internal/acpstdio`.

This makes `profilerun` a mostly generic wrapper over the standard agent run path instead of a meaningful abstraction. The requested refactor is architectural, not behavioral: the runtime should keep the same external run semantics while making the standard internal agent run path the clear owner of direct and regular profile execution.

## Goals / Non-Goals

**Goals:**
- Remove `runtime/internal/profilerun` as a standalone generic profile execution package.
- Move direct-run and regular-profile orchestration into the standard internal agent run path centered on `runtime/internal/agentrun.go`.
- Keep only ACP-specific execution, event mapping, and session recording under `runtime/internal/acpstdio`.
- Preserve current run semantics for direct runs, regular profiles, and `acp-stdio` profiles.
- Preserve stable error classification for HTTP error mapping after `profilerun` is removed.
- Make tests and runtime docs describe the new ownership boundary clearly.

**Non-Goals:**
- Changing HTTP request or response schemas for agent runs.
- Changing profile persistence schema or supported execution mode values.
- Changing ACP stdio protocol behavior or request-to-command mapping semantics.
- Introducing new profile behavior beyond ownership cleanup.

## Decisions

1. Standard agent-run internals own direct and regular profile execution.

   Rationale: direct runs and regular profile runs already share the same built-in mechanics: effective model resolution, prompt assembly, tools wiring, session handling, and low-level runner construction. Keeping those branches outside the standard agent-run implementation adds an unnecessary indirection layer without isolating any distinct behavior.

   Implementation direction:
   - Move the direct/regular profile branching logic out of `runtime/internal/profilerun`.
   - Re-home that responsibility into the standard internal agent-run implementation in `runtime/internal`, alongside `agentrun` logic.
   - Reuse the same built-in helper path for:
     - direct runs without `profileName`
     - regular profiles with request-level model override
     - regular profiles using the profile `defaultModel`
   - Any surviving helper type for this orchestration belongs in the standard internal agent-run area, not in a separate `profilerun` package.

   Alternative considered: inline all profile branching directly into the public `runtime/agent/runner.go` layer. Rejected because the public runner should stay thin and the standard internal agent-run path is the better ownership boundary.

2. ACP stdio remains the only mode-specific execution boundary.

   Rationale: ACP stdio has genuinely different execution mechanics and is the only part of profile execution that benefits from a dedicated mode-specific internal package. Standard agent-run internals should decide that a run is ACP-backed, but should not absorb ACP protocol translation, subprocess execution, or synthetic session recording.

   Implementation direction:
   - Keep ACP request mapping, ACP executor invocation, result-to-session-event translation, and ACP session recording in `runtime/internal/acpstdio`.
   - The standard internal agent-run path should only resolve the selected profile, recognize `acp-stdio`, and delegate the run request into ACP-specific internals.
   - Request-level `model` continues to be ignored for `acp-stdio` profiles.

   Alternative considered: move ACP execution logic into the standard agent-run implementation as part of the cleanup. Rejected because that would collapse a real mode-specific boundary while removing a fake one.

3. Generic profile execution errors move out of `profilerun`.

   Rationale: `runtime/internal/agentapi` still needs stable error categories such as validation, not-found, unsupported, and execution after `profilerun` is removed. Leaving `profilerun` behind only to host shared errors would preserve the retired abstraction in a smaller form.

   Implementation direction:
   - Introduce or reuse a neutral internal error type for profile-selection and execution dispatch failures.
   - Update the standard agent-run path, ACP-specific execution, and HTTP error mapping to use that neutral error type instead of `profilerun.Error`.
   - Preserve existing HTTP behavior: validation and unsupported map to `400`, missing profile maps to `404`, execution failures map to `500`.

   Alternative considered: keep a minimal `profilerun` package only for shared errors. Rejected because the package name would continue to describe a responsibility that no longer exists.

4. Tests and docs shift from package-level wrapper coverage to ownership-boundary coverage.

   Rationale: the important regression risk is not the existence of `profilerun`; it is whether direct runs, regular profile runs, ACP-backed runs, and error mapping still behave correctly after the package is removed. Tests and docs should therefore describe the surviving boundaries rather than the deleted wrapper.

   Implementation direction:
   - Move or rewrite `runtime/internal/profilerun` tests under the standard internal agent-run area and ACP-specific tests as appropriate.
   - Keep focused coverage for direct runs, regular profile model resolution, missing profiles, unsupported modes, ACP delegation, and error classification.
   - Update runtime docs to describe the standard internal agent-run path as the owner of direct and regular profile execution.

   Alternative considered: preserve wrapper-oriented tests as compatibility coverage. Rejected because it would keep the deleted structure alive in the test suite and documentation.

## Risks / Trade-offs

- [The cleanup could accidentally replace one vague wrapper with another internal wrapper] -> Keep the responsibility anchored to the standard internal agent-run path and avoid introducing a new generic package with similar scope.
- [Error-type migration could create noisy import churn] -> Keep the replacement error classification small and mechanically compatible with current HTTP mapping expectations.
- [ACP session recording could regress during boundary cleanup] -> Preserve ACP-specific tests and keep session recording ownership entirely in `runtime/internal/acpstdio`.
- [Too much logic might move back into the public runner while removing `profilerun`] -> Use the standard internal agent-run area as the target ownership boundary and keep public runner changes limited to wiring/delegation.

## Migration Plan

1. Introduce a neutral internal error type for profile execution and update HTTP error mapping to use it.
2. Move direct-run and regular-profile orchestration from `runtime/internal/profilerun` into the standard internal agent-run path.
3. Keep ACP-specific execution delegated into `runtime/internal/acpstdio` with unchanged runtime semantics.
4. Update public runner wiring to depend on the new standard internal ownership path instead of `runtime/internal/profilerun`.
5. Move or rewrite unit tests so they cover the standard internal agent-run path and ACP boundaries.
6. Remove `runtime/internal/profilerun`.
7. Update runtime docs that describe execution-path ownership.

Rollback is a normal code rollback. No API, storage, or schema migration is required.

## Open Questions

None.
