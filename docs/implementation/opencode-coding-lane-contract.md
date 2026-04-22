# OpenCode Coding Lane Contract

This document defines the runtime API contract and service boundary for the OpenCode coding lane introduced in Phase 3 Plan 03-03.

## Runtime Endpoints

The runtime HTTP API exposes the following OpenCode endpoints (mounted under the runtime API base path):

- `GET /opencode-bindings` - list saved OpenCode bindings.
- `POST /opencode-bindings` - create a saved OpenCode binding.
- `GET /opencode-bindings/{bindingName}` - fetch one binding.
- `PUT /opencode-bindings/{bindingName}` - update mutable binding fields (`cwd`, `agentCommand`, `launchOptions`).
- `DELETE /opencode-bindings/{bindingName}` - delete a binding.
- `POST /opencode-launches` - launch an OpenCode coding run from saved selectors and a prompt.

## Persisted Boundary

General agent profile data and OpenCode connection defaults are intentionally split:

- General profile (`agent-profiles`): role, instructions, tool references, default model.
- OpenCode binding (`opencode-bindings`): profile reference, command/args, working directory, transport defaults.

This separation prevents OpenCode process details from leaking into the general profile schema, while still allowing deterministic launch composition.

## Launch Contract

`POST /opencode-launches` accepts:

- `profileName` (required selector)
- `bindingName` (optional selector; if omitted, launcher resolves by profile)
- `prompt` (required user prompt)

The launch handler:

1. Requires authenticated runtime API context.
2. Performs explicit selector and prompt validation.
3. Delegates to the launcher service that resolves saved profile and binding configuration.
4. Maps launcher error kinds to deterministic HTTP problem-details responses.

## Validated ACP Subset

The implementation uses only the validated OpenCode ACP subset documented in [opencode-acp-capability-map.md](./opencode-acp-capability-map.md):

- `initialize`
- `session/new`
- `session/prompt`
- `session/update` notifications consumed from the prompt stream

## Deferred ACP Features

The following ACP features remain deferred in this contract:

- `session/cancel`
- `session/close`
- `session/list`
- `session/load`
- resumable/multi-step session control beyond the launch flow

These deferred items are intentionally excluded from current runtime API behavior.
