## Context

The runtime already has agent profiles, but execution still has two separate public paths:

- regular runs use `POST /agent-runs` and an optional request-level `model`
- OpenCode runs use `/opencode-bindings` and `/opencode-launches`, with OpenCode service and launcher dependencies exported through `runtime/agent` and required by `runtime/httpapi`

That makes OpenCode a product API concept instead of an internal external-agent implementation. The corrected design is: clients select an agent profile, and the profile's `executionSettings` declare how that profile runs.

## Goals / Non-Goals

**Goals:**

- Make `AgentProfile.executionSettings` the single public execution contract.
- Support two execution modes: omitted or `regular` for the built-in runner, and `acp-stdio` for an external ACP-compatible stdio agent.
- Route standard session run endpoints through the selected profile.
- Remove OpenCode binding and launch endpoints, persistence, and exported public aliases.
- Preserve profile CRUD as the public place where execution defaults are managed.
- Keep ACP stdio process handling generic and internal to the runtime.
- Confine OpenCode-specific names to leaf command adapters or test fixtures that truly depend on the OpenCode executable.

**Non-Goals:**

- Add a generic remote backend registry or plugin system.
- Preserve `/opencode-*` endpoint compatibility.
- Preserve separate OpenCode binding records as a supported persistence model.
- Add non-stdio ACP modes in this change.
- Redesign provider/model CRUD beyond using regular profile settings.

## Decisions

1. Use optional `executionSettings.mode` as the discriminator.

   Rationale: the user's intended public model is profile-centric. A mode on the existing execution settings avoids introducing a parallel `customBackend` field and keeps regular model execution and ACP stdio execution mutually exclusive. The mode is optional so existing and simple profiles stay terse; omitted mode is normalized as `regular`.

   Alternative considered: add `customBackend` beside `executionSettings`. Rejected because it keeps two execution concepts on the profile and repeats the design drift this change is meant to correct.

2. Model regular and ACP stdio settings as mode-specific shapes.

   Rationale: `regular` needs a `defaultModel`; `acp-stdio` needs command/process settings. Forcing every mode through one flat struct either leaves irrelevant fields or makes validation ambiguous. Encoding stdio in the mode removes a separate `transport` field until another ACP transport exists.

   Proposed API shape:

   ```json
   {
     "executionSettings": {
       "defaultModel": "openai/gpt-4.1"
     }
   }
   ```

   ```json
   {
     "executionSettings": {
       "mode": "acp-stdio",
       "agentCommand": {
         "command": "opencode",
         "args": ["acp"]
       },
       "cwd": "/workspace"
     }
   }
   ```

   Alternative considered: keep `defaultModel` required for every profile. Rejected because ACP stdio agents can own their model selection through command arguments or their own configuration.

3. Make standard run requests require `profileName` and remove request-level model selection.

   Rationale: a run should be a request to execute a profile. If callers can still override the model directly on the run request, the profile is no longer the source of truth for execution settings.

   Alternative considered: keep `model` as an override for regular profiles. Rejected for this correction because it weakens the profile-only contract; a future explicit override can be added if product requirements justify it.

4. Put profile-aware dispatch below the HTTP handler.

   Rationale: `runtime/internal/agentapi` should validate HTTP input and stream results, not know ACP process details. A profile-aware execution component can implement the existing runner-style contract: resolve profile, run regular profiles through the built-in runner with `defaultModel`, and run `acp-stdio` profiles through an internal ACP executor.

   Alternative considered: branch directly in `StartAgentRun` and `ContinueAgentRun`. Rejected because it couples the OpenAPI handler to external-agent internals and makes session behavior harder to keep consistent.

5. Keep session semantics common across regular and ACP stdio execution.

   Rationale: clients use the standard session run and session read endpoints. ACP stdio runs must therefore emit `sessionBound`, stream `agent` or `error` events, and leave readable session history just like regular runs.

   Implementation direction: ACP stdio execution should adapt ACP prompt results/updates into `runtime/internal.SessionEvent` values and persist them through the same session storage path used by regular runs. If direct reuse is awkward, extract a small internal session recorder interface rather than exporting storage details.

6. Rename surviving internal OpenCode concepts to generic ACP stdio concepts.

   Rationale: the API and runtime architecture should speak in terms of ACP stdio. Internal code may still launch the `opencode` command, but dispatcher, profile execution, persistence, handler wiring, and exported package names should not expose OpenCode-specific resources. Prefer names such as `acpstdio`, `ACPStdioExecutor`, `ACPCommand`, and `ProfileExecutionDispatcher` for surviving generic pieces.

   Alternative considered: keep all internal names as OpenCode because OpenCode is the first ACP stdio implementation. Rejected because internal names shape future architecture; it is acceptable only for leaf adapter code or tests that specifically invoke the OpenCode executable.

## Risks / Trade-offs

- [Existing OpenCode API clients break] -> Treat as acceptable pre-release breakage and document the migration to profile `executionSettings.mode=acp-stdio` plus standard run endpoints.
- [Existing persisted profiles lack `mode`] -> Normalize existing `defaultModel`/`model` persistence as `regular` during read when practical; new regular writes may omit mode while ACP stdio writes must include `mode: acp-stdio`.
- [ACP stdio session history can drift from regular history] -> Add tests for run streaming and subsequent session read replay for ACP stdio profiles.
- [ACP output does not map cleanly to existing stream events] -> Start with text/result/error mapping and keep raw ACP payloads internal unless the public stream contract is deliberately extended.
- [Command execution is sensitive] -> Keep validation strict, disallow control characters and empty args, support only `stdio`, and keep subprocess execution behind persisted profile configuration rather than arbitrary per-run request input.

## Migration Plan

1. Extend profile execution settings domain/API with optional `mode`, default `regular`, and `acp-stdio` validation.
2. Update file and database profile persistence to read/write the new execution settings shape.
3. Change standard run request schema from request-level `model` to required `profileName`.
4. Introduce profile-aware execution dispatch and wire it into the standard runner path.
5. Adapt and rename the current ACP launch code so generic internals consume profile `acp-stdio` settings instead of OpenCode bindings.
6. Delete `/opencode-bindings`, `/opencode-launches`, binding services, launcher aliases, and bundled backend wiring.
7. Verify remaining OpenCode names are limited to leaf executable-specific code, fixtures, or historical docs.
8. Regenerate Go and TypeScript OpenAPI artifacts and update affected tests/docs.

Rollback is a code rollback. Because this is pre-release and explicitly breaking, no compatibility adapter for removed `/opencode-*` endpoints is required.

## Open Questions

- Should ACP stdio settings include a user-visible label for UI display, or is the profile display name sufficient?
- Should the first implementation expose ACP update payloads as text-only stream events, or add a generic metadata field to `AgentStreamEvent`?
