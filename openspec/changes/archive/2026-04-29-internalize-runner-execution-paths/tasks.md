## 1. Internal Execution Runner

- [x] 1.1 Introduce a dedicated internal execution runner that accepts runner-owned dependencies through a params struct and owns direct-run, regular-profile, and `acp-stdio` path selection.
- [x] 1.2 Move profile lookup, execution-mode dispatch, effective-model resolution, and built-in runner preparation out of `runtime/agent/runner.go` into the new internal execution runner while preserving existing profile-run error kinds and run semantics.
- [x] 1.3 Keep `acp-stdio` as a delegated internal execution path by having the internal execution runner call ACP-specific internals instead of branching through public runner helpers.

## 2. Public Runner Slim-Down

- [x] 2.1 Update `agent.NewRunner` to compose the internal execution runner from existing dependencies and remove public runner fields/helpers that only exist for execution-path branching.
- [x] 2.2 Simplify `agent.Runner.Run` into a thin delegation layer and keep `ModelsLocator`, `AutoMigrate`, `ReadSession`, and `ListSessions` behavior unchanged.

## 3. Coverage and Documentation

- [x] 3.1 Add or update tests so internal execution runner coverage owns direct/profile/ACP path behavior and public runner tests focus on wiring and thin orchestration.
- [x] 3.2 Update runtime architecture/docs to state that public runner APIs stay minimal while internal runtime code owns execution-path selection and mode-specific setup.

## 4. Final Verification

- [x] 4.1 Run `make affected-lint-test` from the repository root and fix any failures.
- [x] 4.2 Confirm the implementation still satisfies the `agent-profile-execution-settings` spec delta for internal execution-path ownership and unchanged run behavior.
