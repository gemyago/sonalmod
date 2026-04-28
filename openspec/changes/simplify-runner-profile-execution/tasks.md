## Important Apply Phase Constraint

Applying phase must follow [orchestrate-tasks](../../orchestrate-tasks.md) instruction.

---

## 1. Required Runner Dependencies

- [x] 1.1 Update `agent.NewRunner` so `RunnerArgs.AgentProfilesService` is required, add constructor validation tests, and update all runtime/app/test `NewRunner` call sites with a valid profiles-service stub or real service.
- [x] 1.2 Update `runtime/AGENTS.md` public contract text so `RunnerArgs.AgentProfilesService` is documented as required instead of optional.

## 2. Neutral Profile Run Errors

- [x] 2.1 Move generic profile run error kind/wrapper types out of `runtime/internal/profileexec` into a neutral internal runtime location usable by both `agent.Runner` and `internal/agentapi`.
- [x] 2.2 Update `AgentAPIServer.writeAgentRunError` and related tests to map the neutral profile run error type to the existing HTTP statuses without depending on `profileexec`.

## 3. Runner-Owned Regular Profile Execution

- [ ] 3.1 Extract a runner-owned built-in execution helper used by the no-profile path, preserving current direct model run behavior and tests.
- [ ] 3.2 Implement runner-owned profile loading and regular-profile execution in `Runner.Run`, including missing-profile, lookup-failure, default-model, request-model-override, profile-name, and profile-instructions tests.
- [ ] 3.3 Remove the `runnerProfileRegularRunner` adapter and any regular-profile tests that only verify the old dispatcher-to-runner callback layer.

## 4. ACP-Specific Profile Execution

- [ ] 4.1 Move ACP stdio result mapping and ACP session recording code/tests out of `profileexec` into an ACP-specific internal location, keeping existing ACP behavior green.
- [ ] 4.2 Add an ACP-specific profile run helper that accepts the resolved profile plus run request data, executes ACP stdio, records session events, and returns the standard `RunResult`.
- [ ] 4.3 Delegate `acp-stdio` profiles from `Runner.Run` to the ACP-specific helper, with runner-level tests for ACP success, ACP executor error stream behavior, and ignored request-level model.

## 5. Remove Generic Profile Dispatcher

- [x] 5.1 Delete `profileexec.Dispatcher`, `RegularRunRequest`, dispatcher-only tests, and the `Runner.profileRuns` field after runner-owned dispatch and ACP delegation are in place.
- [x] 5.2 Verify no remaining generic package, type, or test names imply a profile execution wrapper layer; surviving code must be runner-owned or ACP-specific.

## 6. Final Verification

- [ ] 6.1 Update runtime or bundled-backend architecture docs if they still describe optional runner profiles service or generic profile dispatch.
- [ ] 6.2 Run `make affected-lint-test` from the repository root and fix all lint, test, generation, or API drift failures.
- [ ] 6.3 Confirm the final implementation satisfies the spec delta: runner construction requires profiles service, regular profile runs are runner-owned, and ACP-specific execution is the only delegated profile mode.
