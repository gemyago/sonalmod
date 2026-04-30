## Why

The last profile-execution change locked the standard run contract too tightly to `profileName`, leaked a new public dispatcher abstraction, and left substantial ACP stdio internals under legacy `opencode` names. It also left a functional gap for regular profiles: selecting a built-in profile currently only changes `defaultModel`, while the profile's own identity and instructions are ignored during execution.

## What Changes

- Restore the standard run HTTP contract so `model` remains available on `POST /agent-runs` and `POST /sessions/{sessionId}/agent-runs`.
- Make `profileName` optional on standard run requests instead of required.
- Define regular-run precedence so a request-level `model` overrides a selected profile's regular `defaultModel`, while ACP stdio profiles continue to ignore request-level model overrides.
- Make selected regular profiles influence built-in execution beyond model choice by applying profile identity and instructions to the built-in runner.
- Undo the UI change that serializes the model picker value into `profileName`; Sonal UI should send `model` again for standard chat runs until it has an actual profile selector.
- **BREAKING**: remove the exported `ProfileRunDispatcher` abstraction and related `httpapi.HandlerArgs` dependency from the public Go runtime surface; `agent.Runner` should own profile-aware dispatch internally.
- Rename remaining generic ACP stdio runtime concepts away from `opencode` naming, keeping `OpenCode` wording only in executable-specific leaf adapters, fixtures, or explicitly historical docs.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `agent-profile-execution-settings`: standard run request requirements, regular profile execution behavior, public runtime API boundaries, and generic ACP stdio naming rules must be corrected.

## Impact

- `runtime/internal/agentapi/openapi.yaml` and regenerated Go/TypeScript API artifacts
- `runtime/internal/agentapi/server.go` request parsing and dispatch behavior
- `runtime/agent`, `runtime/httpapi`, and `runtime/internal/agentrun.go` public/internal execution wiring
- runner-owned system prompt and agent-construction paths for selected regular profiles
- `runtime/internal/profileexec` dispatcher ownership and tests
- `apps/sonal-ui` standard chat request serialization and related tests/docs
- Remaining `runtime/internal/codinglane` files and tests that still use generic `opencode` naming
