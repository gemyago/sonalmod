## Important Apply Phase Constraint

## 1. Neutral profile execution errors

- [x] 1.1 Introduce a neutral internal error kind/wrapper to replace `runtime/internal/profilerun.Error` and update HTTP error mapping/tests without changing current status-code behavior.
- [x] 1.2 Update ACP-specific execution paths and any runner adapters to use the neutral error type so `runtime/internal/profilerun/error.go` has no remaining consumers.

## 2. Move standard dispatch into agent run internals

- [x] 2.1 Add standard internal agent-run helpers in `runtime/internal/agentrun.go` (or adjacent internal agent-run code) for direct runs and regular profile-backed runs using the existing built-in runner construction path.
- [x] 2.2 Move profile lookup, effective-model resolution, profile instructions, and unsupported-mode handling out of `runtime/internal/profilerun/execution_runner.go` and into the standard internal agent-run path with focused unit tests.
- [x] 2.3 Update `runtime/agent/runner.go` wiring so it delegates through the standard internal agent-run ownership path instead of constructing `runtime/internal/profilerun.ExecutionRunner`.

## 3. Keep ACP execution ACP-specific

- [x] 3.1 Preserve `acp-stdio` profile dispatch by delegating from the standard internal agent-run path into `runtime/internal/acpstdio` with unchanged request-model-ignore semantics and standard `RunResult` behavior.
- [x] 3.2 Keep ACP result mapping and session-recording coverage green after the dispatch ownership move, adding runner-level delegation tests only where they improve regression protection.

## 4. Remove profilerun and clean references

- [x] 4.1 Delete `runtime/internal/profilerun/` and migrate or rewrite its surviving tests under the standard internal agent-run area or ACP-specific area as appropriate.
- [x] 4.2 Update runtime docs, comments, and test naming so no surviving code describes `profilerun` as a generic execution layer.

## 5. Final verification

- [x] 5.1 Run `make affected-lint-test` from the repository root and resolve any lint, test, generation, or API drift failures.
- [x] 5.2 Confirm the final implementation satisfies the `agent-profile-execution-settings` delta: direct and regular profile runs use the standard internal agent-run path, ACP execution remains ACP-specific, and profile-run HTTP error mapping behavior is unchanged.
