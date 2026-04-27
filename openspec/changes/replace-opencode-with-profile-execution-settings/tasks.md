## 1. Profile Execution Settings Domain

Ownership: `runtime/internal/agentprofiles/**` and public profile aliases in `runtime/agent/agent_profiles.go`. This group must not touch HTTP handlers, OpenAPI generated code, or ACP subprocess code.

- [ ] 1.1 Add mode-aware execution settings domain types where omitted mode normalizes to `regular`, explicit `regular` is accepted, and explicit `acp-stdio` is accepted.
- [ ] 1.2 Implement validation so regular profiles require `defaultModel`, while `acp-stdio` profiles require command settings and do not require `defaultModel`.
- [ ] 1.3 Model ACP stdio command settings with command, args, and optional cwd only; do not add a separate transport field.
- [ ] 1.4 Update file-backed profile read/write to persist the new shape and read existing model-only frontmatter as regular execution settings.
- [ ] 1.5 Update database-backed profile JSON persistence and migration structs for the new mode-aware execution settings.
- [ ] 1.6 Add domain, file service, and database service tests covering omitted mode, explicit regular mode, `acp-stdio`, invalid modes, invalid commands, and legacy model-only reads.

## 2. Runtime HTTP Contract

Ownership: `runtime/internal/agentapi/openapi.yaml`, generated `runtime/internal/agentapi/api.gen.go`, profile/request mappers, and agentapi handler tests. This group owns the public HTTP shape but not execution dispatch internals.

- [ ] 2.1 Update profile schemas so `executionSettings.mode` is optional, defaults semantically to `regular`, and accepts only `regular` or `acp-stdio`.
- [ ] 2.2 Update ACP stdio profile schema fields to include agent command, args, and optional cwd, with no transport property.
- [ ] 2.3 Update `AgentRunRequest` to require `profileName` and remove request-level `model`.
- [ ] 2.4 Remove `/opencode-bindings`, `/opencode-bindings/{bindingName}`, `/opencode-launches`, and all OpenCode binding/launch schemas from `openapi.yaml`.
- [ ] 2.5 Regenerate runtime OpenAPI Go code with `go generate ./internal/agentapi` from the `runtime/` module root.
- [ ] 2.6 Update profile mappers, request parsing, and agentapi tests for optional mode defaulting, `acp-stdio` settings, required `profileName`, and removed OpenCode routes.

## 3. Regular Profile Run Dispatch

Ownership: standard run parameters, `runtime/internal/agentapi` run handler integration, background-runner wiring, and regular-run tests. This group must keep ACP execution behind an interface or stub and must not implement the ACP subprocess flow.

- [ ] 3.1 Update runtime run params and builders so standard runs carry `profileName` and no longer carry request-level model selection.
- [ ] 3.2 Add a profile-aware dispatcher interface below HTTP handling that resolves the selected profile before executing a run.
- [ ] 3.3 Route omitted-mode and explicit `regular` profiles through the existing built-in runner using the profile's `defaultModel`.
- [ ] 3.4 Return validation or not-found errors for missing `profileName`, missing profile, unsupported mode, and regular profiles without model.
- [ ] 3.5 Add regular dispatch tests for start run, continue run, missing profile selection, missing selected profile, and default-mode behavior.

## 4. ACP Stdio Execution

Ownership: internal ACP execution under a generic package such as `runtime/internal/acpstdio` or equivalent, plus dispatcher adapter code needed to plug ACP stdio into the runner-style contract. This group must not reintroduce public OpenCode endpoints, public OpenCode constructors, or generic internal types named after OpenCode.

- [ ] 4.1 Rename surviving generic ACP execution package/files/types away from OpenCode-specific names before adapting behavior.
- [ ] 4.2 Adapt the ACP launch request mapper to consume profile `acp-stdio` execution settings instead of saved OpenCode bindings.
- [ ] 4.3 Convert ACP prompt results, updates, and protocol failures into standard runtime session events for SSE streaming.
- [ ] 4.4 Persist ACP stdio run events through the common session storage path so `GET /sessions/{sessionId}` can replay them.
- [ ] 4.5 Add ACP stdio execution tests for successful launch, command validation failure, protocol failure, stream output mapping, and session read-back.
- [ ] 4.6 Verify remaining OpenCode names are limited to leaf executable-specific code, fixtures, or historical docs.

## 5. Public Surface Cleanup And App Wiring

Ownership: exported runtime packages, runtime HTTP constructor args, bundled backend wiring, obsolete OpenCode persistence, and AGENTS/public-contract docs. This group removes public leakage after the replacement path exists.

- [ ] 5.1 Remove exported OpenCode binding and launcher aliases, constructors, and public tests from `runtime/agent`.
- [ ] 5.2 Remove OpenCode binding service and launcher requirements from `runtime/httpapi.HandlerArgs` and its tests.
- [ ] 5.3 Remove OpenCode binding service construction, launcher construction, and binding auto-migration from `apps/sonalmod/internal/runtime.go` and related tests.
- [ ] 5.4 Delete obsolete OpenCode binding persistence code and tests when no longer referenced by ACP stdio execution.
- [ ] 5.5 Update `runtime/AGENTS.md`, `apps/sonalmod/AGENTS.md`, and any public-contract docs that mention OpenCode endpoints, binding services, or launcher dependencies.

## 6. UI Generated Client

Ownership: `apps/sonal-ui/src/lib/agentapi/agentapi.generated.ts` and only the minimal client/type call sites needed after API generation. This group must not change UI screens unless generated types require it.

- [ ] 6.1 Regenerate the Svelte OpenAPI TypeScript client with `make -C apps/sonal-ui generate-api` from the repository root.
- [ ] 6.2 Update client/type usage for required `profileName`, removed request-level `model`, and new profile execution settings if existing TypeScript references break.
- [ ] 6.3 Run `make -C apps/sonal-ui check-api` and targeted UI tests or type checks needed for the generated client change.

## 7. Cross-Cut Verification

Ownership: final integration verification only. This group should not add new feature code except fixes required by verification failures.

- [ ] 7.1 Verify every requirement in `specs/agent-profile-execution-settings/spec.md` is covered by tests or explicit implementation checks.
- [ ] 7.2 Run targeted runtime tests after all runtime groups land and fix failures.
- [ ] 7.3 Run targeted bundled backend tests after app wiring changes land and fix failures.
- [ ] 7.4 Run repo level checks, fix issues and re-run the task until all checks are green.
