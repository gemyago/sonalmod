## Context

The archived `replace-opencode-with-profile-execution-settings` change successfully moved ACP stdio execution behind agent profiles, but it also encoded three decisions that do not match the intended product contract:

- `AgentRunRequest` now requires `profileName` and removed request-level `model`
- Sonal UI sends the current model picker value as `profileName`
- the public Go surface exposes `ProfileRunDispatcher` and requires callers to pass it into `runtime/httpapi.NewHandler`

There is also still a large amount of generic ACP stdio code and dead binding/persistence code under `opencode` names. Some of that naming is acceptable only at the executable-specific leaf adapter boundary, but much of it is now either misleading generic runtime plumbing or stale code that should be deleted.

There is a second gap in the regular built-in execution path: when a request selects a regular profile, the runtime currently loads that profile only to read `executionSettings.defaultModel`. The profile's own `name`, `instructions`, `role`, and `toolRefs` do not currently influence built-in execution. For this correction, the minimum intended behavior is to make selected regular profiles contribute agent identity and instructions; `role` and `toolRefs` stay out of scope unless a later change gives them concrete runtime behavior.

This correction must preserve the good parts of the previous change: profile execution settings stay supported, ACP stdio continues to run through the standard SSE/session flow, and the public `/opencode-*` endpoints remain removed.

## Goals / Non-Goals

**Goals:**

- Restore the standard run request contract so `model` remains available and `profileName` becomes optional.
- Define a clear precedence rule: for regular execution, request-level `model` overrides a selected profile's `defaultModel`.
- Make selected regular profiles influence built-in execution through profile name and instructions, not just `defaultModel`.
- Keep profile-aware dispatch internal to runtime implementation so public consumers pass a runner, not a dispatcher abstraction.
- Revert Sonal UI to sending request-level `model` until a real profile-selection UI exists.
- Finish the generic ACP stdio naming cleanup by renaming or deleting non-leaf `opencode` runtime concepts.

**Non-Goals:**

- Reintroduce public `/opencode-*` endpoints or public OpenCode binding services.
- Remove profile execution settings or ACP stdio support from saved profiles.
- Build a new profile-selection UI in this change.
- Define runtime behavior for `role` or `toolRefs`; they remain persisted schema fields but are not newly wired in this correction.
- Change ACP stdio protocol semantics beyond what is needed to support the corrected run contract.
- Rename executable-specific leaf adapters that explicitly target the OpenCode binary unless they become generic in this change.

## Decisions

1. Standard run requests will carry `message` plus optional `profileName` and optional `model`.

   Rationale: profiles are execution defaults and alternate backends, not mandatory selectors for every regular run. The request-level model must remain the direct way to choose a built-in model, including overriding a regular profile's stored default.

   Effective resolution rules:
   - no `profileName` + `model` => built-in runner with request `model`
   - regular `profileName` + no `model` => built-in runner with profile `defaultModel`
   - regular `profileName` + `model` => built-in runner with request `model`
   - `acp-stdio` `profileName` => ACP stdio execution; request `model` is accepted but ignored
   - no `profileName` + no `model` => validation error

   Alternative considered: make `model` and `profileName` mutually exclusive. Rejected because it prevents the useful and explicit "profile as default, request as override" behavior.

2. Selected regular profiles will shape built-in runner identity and instructions.

   Rationale: a selected regular profile should be more than a model lookup record. The saved profile already carries `name`, `instructions`, `role`, and `toolRefs`; today only `defaultModel` is used. The minimum correction is to make built-in execution use the profile's technical `name` as the internal agent name and append profile `instructions` to the system prompt chain.

   Implementation direction:
   - when a regular profile is selected, built-in execution resolves the effective model using the precedence rules above
   - the built-in runner uses the saved profile `name` as the agent identity passed to agent construction
   - the built-in runner appends the saved profile `instructions` as a profile-scoped system prompt fragment
   - `role` and `toolRefs` remain persisted but do not gain runtime behavior in this correction

   Alternative considered: continue using regular profiles only for model lookup. Rejected because it makes profile selection semantically hollow and contradicts the purpose of persisted profile instructions.

3. Profile-aware dispatch will be owned by `agent.Runner`, not part of the public API.

   Rationale: the public runtime contract should expose the runner and HTTP handler, not the mechanics of how profile-based execution is resolved. The existing exported `ProfileRunDispatcher` duplicates internal concepts and leaks implementation structure to embedders. This also aligns with the intent that the runner itself is fully responsible for how a run is executed.

   Implementation direction:
   - remove exported `agent.ProfileRunDispatcher` and `agent.NewProfileRunDispatcher`
   - remove `httpapi.HandlerArgs.ProfileRunDispatcher`
   - move profile-aware standard-run resolution behind `agent.Runner.Run` / runner-owned internals rather than treating it as a peer dependency passed into `httpapi`
   - let handler code pass request data to the runner plus existing services, without knowing whether execution becomes built-in or ACP stdio

   Alternative considered: keep the exported dispatcher but hide it in docs. Rejected because the user-facing public API remains wrong even if it is undocumented.

4. Sonal UI will revert to model-based submission until it has an actual profile picker.

   Rationale: the existing UI control is a model picker, not a profile selector. Serializing its value as `profileName` is a contract mismatch and produces misleading client behavior.

   Implementation direction:
   - generated TypeScript request types regain optional `model`
   - `Chat.svelte` sends `model: selectedModel`
   - tests and wireframe/docs revert to model-based submission language

   Alternative considered: reinterpret model IDs as implicit profile names. Rejected because provider/model IDs and saved profile names are different namespaces.

5. Request-level `model` will be ignored, not rejected, for `acp-stdio` profiles.

   Rationale: request-level `model` belongs to the built-in runner contract. Rejecting it for ACP stdio profiles would make mixed clients more brittle without giving a real benefit, while silently changing ACP behavior would be worse. Accepting the field and ignoring it keeps the request shape simple and predictable.

   Alternative considered: reject request-level `model` whenever `profileName` selects `acp-stdio`. Rejected because it complicates clients and creates a mode-specific payload rule for a field that is irrelevant to ACP stdio execution.

6. Remaining generic `opencode` runtime concepts will be renamed or deleted.

   Rationale: after the public OpenCode surface removal, generic runtime plumbing should not retain OpenCode-specific names. Dead binding services and launch-mapper code should be deleted; surviving generic ACP stdio types should use ACP-oriented names. Only executable-specific leaf adapters may still mention OpenCode.

   Alternative considered: leave dead `opencode`-named code in place because it is internal. Rejected because it increases maintenance cost and obscures which parts of the runtime are still generic vs historical leftovers.

## Risks / Trade-offs

- [Request contract becomes more permissive and therefore easier to misuse] -> Document and test the effective precedence rules explicitly, including the invalid "missing both model and profile" case.
- [Runner-owned profile execution may require widening internal run params] -> Keep the public contract centered on a single runner abstraction and cover the added profile-aware cases with focused runtime tests.
- [Applying profile instructions may change built-in agent behavior more than expected] -> Limit the correction to agent name plus appended instructions, and explicitly leave `role`/`toolRefs` untouched in this change.
- [UI and server contract may temporarily diverge during implementation] -> Regenerate OpenAPI artifacts and update UI request tests in the same change wave as the OpenAPI change.
- [Renaming/deleting `opencode` internals may break tests or stale references] -> Treat the cleanup as explicit task slices with repo-root verification after each slice.

## Migration Plan

1. Update the OpenAPI contract and server request parsing to restore optional `profileName` and request-level `model`.
2. Move profile-aware execution back behind runner-owned internals and make selected regular profiles apply profile name plus instructions.
3. Revert Sonal UI request serialization and tests to use `model`.
4. Delete stale OpenCode binding remnants and rename surviving generic ACP stdio types/files.
5. Re-run repo-wide verification and update docs/specs to match the corrected contract.

Rollback is a standard code rollback. This is still pre-release, so no compatibility shim is required for the brief `profileName`-only contract that was introduced by the archived change.

## Open Questions

- Should a later follow-up give `role` and `toolRefs` concrete runtime behavior, or should those fields be removed from the profile schema if they remain metadata-only?
