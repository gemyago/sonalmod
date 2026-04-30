## Important Apply Phase Constraint

Applying phase must follow [orchestrate-tasks](../../orchestrate-tasks.md) instruction.

---

## 1. Restore Standard Run Request Contract

- [x] 1.1 Update `runtime/internal/agentapi/openapi.yaml` so `AgentRunRequest` carries `message` plus optional `profileName` and optional `model`, regenerate Go and Svelte API artifacts, and update request-shape tests.
- [x] 1.2 Update runtime run-request parsing and dispatch behavior so no-profile requests require `model`, regular profiles use request `model` as an override, ACP stdio profiles ignore request `model`, and add handler tests for the corrected precedence and validation rules.

## 2. Internalize Profile Dispatch

- [x] 2.1 Extend runner-owned run execution so selected regular profiles apply profile `name`, profile `instructions`, and effective model resolution internally, while `role` and `toolRefs` remain unchanged.
- [x] 2.2 Remove exported `agent.ProfileRunDispatcher` and `agent.NewProfileRunDispatcher`, move profile-aware standard-run composition behind `agent.Runner` / runner-owned internals, and update `runtime/httpapi` / `runtime/internal/agentapi` tests so public handler construction no longer depends on a dispatcher argument.
- [x] 2.3 Update bundled backend wiring in `apps/sonalmod` and any runner-owned runtime helpers so public consumers pass only the runner plus existing services while internal profile execution still supports ACP stdio session persistence.

## 3. Undo UI Contract Drift

- [x] 3.1 Regenerate the Svelte API client, revert Sonal UI standard chat submission to send `model` instead of `profileName`, and update affected chat/client tests.
- [x] 3.2 Update UI/runtime docs that currently describe model selection as `profileName`-based submission.

## 4. Finish ACP Naming Cleanup

- [x] 4.1 Delete dead OpenCode binding and launch-mapper remnants that are no longer used after the public API removal, along with their obsolete tests and fixtures.
- [x] 4.2 Rename surviving generic ACP stdio runtime concepts away from `opencode` naming, keeping `OpenCode` wording only in executable-specific leaf adapters, fixtures, or clearly historical references.

## 5. Verification

- [x] 5.1 Update runtime and app AGENTS or architecture docs if the internalized dispatch wiring or corrected regular-profile behavior changes their documented public contract.
- [x] 5.2 Run `make affected-lint-test` from the repository root and fix all lint, test, generation, or API drift failures.
- [x] 5.3 Verify the final OpenAPI and generated clients expose optional `profileName` plus request-level `model`, selected regular profiles affect built-in runner identity/instructions, the public runtime surface exports no `ProfileRunDispatcher`, and generic ACP stdio internals no longer use stale `opencode` naming.
