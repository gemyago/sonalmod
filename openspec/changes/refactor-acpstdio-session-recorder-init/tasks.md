## 1. ACP runner API refactor

- [x] 1.1 Identify ACP profile runner entry points that currently use positional arguments and introduce request params struct types for those paths.
- [x] 1.2 Update ACP profile runner constructors/functions to accept the new params structs and adapt all internal call sites to compile.

## 2. Move session recorder ownership

- [x] 2.1 Move `acpstdio.SessionRecorder` initialization from `agent.Runner` wiring into the ACP profile runner constructor path.
- [x] 2.2 Simplify `runtime/agent/runner.go` ACP setup so runner only delegates to ACP internals without manual recorder construction.
- [x] 2.3 Keep ACP profile execution behavior unchanged (command launch, model override ignore semantics, and stream contract).

## 3. Validation and regression coverage

- [x] 3.1 Update or add ACP-focused unit tests to validate params-struct construction and internal session-recorder initialization ownership.
- [x] 3.2 Run `make affected-lint-test` from repository root and resolve all issues.
- [x] 3.3 Confirm change artifacts and runtime behavior still satisfy `agent-profile-execution-settings` spec delta.
