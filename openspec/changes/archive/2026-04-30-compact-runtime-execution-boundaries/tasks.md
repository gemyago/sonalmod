## 1. Move standard dispatch into agent-run internals

- [x] 1.1 Add or migrate unit tests in the standard internal agent-run test area to cover direct runs, regular-profile runs, ACP-profile delegation, and dispatch error classification before moving logic out of `profileexec.go`.
- [x] 1.2 Move profile lookup, effective-model resolution, profile instruction handling, and ACP-mode branching into `runtime/internal/agentrun.go`, then update `runtime/agent/runner.go` to delegate through the compacted internal agent-run path.
- [x] 1.3 Remove `runtime/internal/profileexec.go` and any remaining references once the agent-run path owns all standard dispatch behavior.

## 2. Consolidate ACP stdio internals

- [x] 2.1 Add or migrate ACP-focused tests so request mapping, executor launch, result translation, and session recording are covered through `runtime/internal/acpstdio` before moving generic ACP code out of `runtime/internal/codinglane`.
- [x] 2.2 Move generic ACP request mapper, executor request/result types, subprocess launch logic, and related helpers from `runtime/internal/codinglane` into `runtime/internal/acpstdio` while preserving current `acp-stdio` run semantics.
- [x] 2.3 Remove `runtime/internal/codinglane` remnants and update imports, comments, and names so generic ACP stdio code lives behind one ACP-focused internal boundary.

## 3. Tighten the public error contract

- [x] 3.1 Add or update `runtime/internal/agentapi` handler tests to assert stable `400`, `404`, and `500` problem details for profile-selection and dispatch failures without leaked wrapped internal error text.
- [x] 3.2 Keep or replace the internal execution error classifier as needed, and update `runtime/internal/agentapi/server.go` plus related runtime code so error kinds still drive status mapping while public problem details stay sanitized and stable.

## 4. Clean references and verify

- [x] 4.1 Update runtime docs, comments, and test naming so they describe `agentrun` as the built-in dispatch owner and `acpstdio` as the consolidated ACP boundary.
- [x] 4.2 Run `make affected-lint-test` from the repository root and resolve any failures introduced by the refactor.
- [x] 4.3 Confirm the final implementation satisfies the `agent-profile-execution-settings` delta: built-in dispatch lives in the standard agent-run path, ACP stdio internals are consolidated, and public profile-selection error responses stay stable.
