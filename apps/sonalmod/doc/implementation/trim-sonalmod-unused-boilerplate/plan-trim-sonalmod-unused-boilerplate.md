# Plan: Trim `apps/sonalmod` unused boilerplate (lean module)

## 1. Introduction / overview

**Goal:** Remove foundation-era scaffolding from `apps/sonalmod` so the module reflects the current product path: a runnable process with config, logging, OpenTelemetry, an HTTP server with health (and generated apigen surface), and runtime agent integration—without MCP, demo domain services (math, time, echo, users/pets), Petstore, SQLite for those features, or the sample `jobs` binary.

**Problem solved:** Unused code and dependencies inflate build surface, config noise, and cognitive load. The OpenSpec change [`trim-sonalmod-unused-boilerplate`](../../../../../openspec/changes/trim-sonalmod-unused-boilerplate/proposal.md) and capability spec [`sonalmod-lean-module`](../../../../../openspec/changes/trim-sonalmod-unused-boilerplate/specs/sonalmod-lean-module/spec.md) define normative post-cleanup expectations.

**Non-goals:** Redesigning the public HTTP API beyond deleting dead handlers; changing `runtime/` agent behavior except how sonalmod wires it; adding replacement demo features.

**Success criteria (from spec):** No `cmd/mcp` or `internal/api/mcp`; no petstore package; `go list ./...` under `apps/sonalmod` succeeds; `go run . start` still runs full DI and starts the HTTP server (and `start -e test --noop` still completes setup); domain error types used by HTTP middleware remain available so 400/404/409 mapping is preserved unless deliberately documented otherwise.

---

## 2. Business logic

There is no new product behavior beyond **subtracting** unused layers:

- **Remove** MCP servers (stdio + HTTP) and all code that exists only to expose math/time tools over MCP.
- **Remove** application services and commands/queries for echo, math, time-demo, users, and pets, plus metrics tied only to users.
- **Remove** SQLite persistence and repositories that existed only for users/pets; **remove** the Petstore HTTP client and related config.
- **Keep** structured domain errors (`NotFoundError`, `InvalidInputError`, `ConflictError`) and HTTP middleware behavior that maps them to status codes (same semantics as today).
- **Resolve** `NewRuntime` / `Runtime.HTTPHandler`: today DI constructs `Runtime` but the main HTTP router does not mount `HTTPHandler`; either mount the agent HTTP API on the main mux (preferred per design if agent HTTP is part of the minimal app) or stop constructing `Runtime` until needed—document the choice in the implementation summary.

---

## 3. High-level architecture

| Area | After cleanup |
|------|----------------|
| **Entry** | Root `main.go` + Cobra (`start` only); no `cmd/jobs` / `cmd/mcp`. |
| **Config** | Viper + embedded YAML: HTTP, OTEL, OpenAI, pprof, graceful shutdown—no `mcpServer`, `petstore`, or DB keys unless something still consumes them. |
| **DI** | `internal/wireup.Setup`: telemetry, config, `ident` (still used by HTTP correlation middleware), optional `apptime` if anything remains; slim or empty `app.Register`; slim `infrastructure.Register` (likely `httpclient` + `ClientFactory` for `NewRuntime` only). |
| **HTTP** | `internal/api/http`: server, middleware, health/v1 routes; **no** MCP tree. |
| **Runtime** | `internal/runtime.go`: agent runner + `httpapi` handler—**wired to HTTP** or not constructed, per decision. |

---

## 4. Detailed architecture

### 4.1 MCP removal

- Delete `apps/sonalmod/cmd/mcp/**` and `apps/sonalmod/internal/api/mcp/**`.
- Remove `mcpServer.*` from `internal/config/default.yaml` (and env-specific YAML), `config/provide.go`, and any tests that assert those keys.
- Run `go mod tidy` and drop `github.com/mark3labs/mcp-go` from `go.mod` if nothing else imports it.

### 4.2 Demo app layer removal

- Delete files under `internal/app/` that implement echo, math, time, users, pets (commands, queries, metrics, repositories interfaces, testing helpers, mocks), except **retain** `errors.go` (and `errors_test.go` if kept) or **move** error types to e.g. `internal/api/http/errors` and update imports.
- **Today** `internal/api/http/v1controllers/register.go` only registers `HealthController`; generated mocks under `v1controllers/mock_pets_commands_test.go` are for removed types—delete with the pet domain.
- Replace `app.Register` in `internal/app/register.go`: either remove the file’s providers entirely and delete `Register`, or leave a no-op `Register` only if dig wiring requires it—prefer removing unused registration.

### 4.3 Infrastructure: DB, petstore, httpclient

- Remove `internal/infrastructure/petstore/**`.
- Remove `newDBProvider`, `Database`, users/pets repositories, `users_testing.go`, `pets_testing.go`, and inline `CREATE TABLE` in `database.go` (schema in `internal/infrastructure/database.go` today).
- Remove `app.Queryer` and related interfaces from `app` if only DB used them.
- **Keep** `internal/infrastructure/httpclient/**` for `NewRuntime` (`ClientFactory` for OpenAI-compatible LLM HTTP).
- **Strip** `database.*` and `petstore.*` from config and `infrastructure.Register`.

### 4.4 Jobs binary

- Delete `apps/sonalmod/cmd/jobs/**` (echo job only).
- Update `Makefile` `dist/bin` target: it builds `./cmd/...`; if `cmd/` is empty or gone, adjust to build only what exists (e.g. root module only) so `make` does not fail.
- Update `AGENTS.md` install section (remove `go install ./cmd/jobs` and `./cmd/mcp`).

### 4.5 Wireup and runtime

- **`internal/wireup.go`:** After removals, drop `app.Register` if empty; trim `infrastructure.Register`; keep `NewRuntime` only if still used.
- **`internal/runtime.go`:** If mounting agent HTTP: inject `*Runtime` (or `http.Handler`) into HTTP registration (`internal/api/http/register.go` / `server` / `root_handler`) and register a path prefix consistent with `runtime/httpapi` + `agentapi` routes (inspect `runtime/internal/agentapi` for paths). If not mounting in this change, remove `NewRuntime` from `wireup` and document follow-up (aligns with OpenSpec design “open question”).
- **`ident`/`apptime`:** Keep `ident` for correlation middleware. `apptime` is used by users/pets repos and MCP tests—if no remaining production code needs `apptime`, remove `apptime` registration from `wireup` and delete `internal/system/apptime` only if no imports remain (verify with `go test ./...`).

### 4.6 Middleware

- `internal/api/http/middleware/error_handler.go` imports `app` for error types. After shrinking `app`, either keep minimal `package app` with errors only or move types and update `error_handler.go` + `error_handler_test.go` imports.

### 4.7 Tests and coverage

- Remove tests that only covered deleted packages; adjust `.testcoverage.yaml` thresholds if the profile changes materially.
- Follow TDD where logic changes: for new wiring (e.g. runtime mount), add a test that fails without the route/handler, then implement.

---

## 5. Key architectural decisions

1. **Domain errors stay available** to HTTP error middleware (same types or moved package)—no silent change to 404/400/409 behavior.
2. **Database removed** with users/pets; no migrations beyond deleting embedded schema DDL in code.
3. **Petstore removed** with pet flows.
4. **Jobs and MCP binaries removed** entirely.
5. **Runtime wiring:** Prefer mounting `Runtime.HTTPHandler` on the main server if the agent API is part of the minimal app; otherwise defer and remove `NewRuntime` from DI to avoid constructing unused agents (see §6).

---

## 6. Uncertainties

- **Agent HTTP on main listener:** OpenSpec design leaves whether to mount `Runtime.HTTPHandler` in this change vs a follow-up. The plan should record the chosen option in the task summary.
- **Exact URL prefix** for the agent API when mounted: determined by `runtime`’s `agentapi` mux; confirm during implementation (read generated routes or `server.go` in `runtime/internal/agentapi`).
- **Empty `internal/app`:** If only `errors.go` remains, consider renaming package or moving errors to avoid an “app” package that contains no application services—cosmetic but affects imports across middleware/tests.

---

## 7. Related files

**Remove or replace (non-exhaustive):**

- `apps/sonalmod/cmd/mcp/**`, `apps/sonalmod/cmd/jobs/**`
- `apps/sonalmod/internal/api/mcp/**`
- `apps/sonalmod/internal/infrastructure/petstore/**`
- `apps/sonalmod/internal/infrastructure/database.go` (or heavily trim), `users_repository.go`, `pets_repository.go`, `*_testing.go` for repos
- `apps/sonalmod/internal/app/*` except `errors.go` / `errors_test.go` (or new error package)
- `apps/sonalmod/internal/api/http/v1controllers/mock_pets_commands_test.go`

**Update:**

- `apps/sonalmod/go.mod`, `go.sum`
- `apps/sonalmod/internal/wireup.go`
- `apps/sonalmod/internal/app/register.go` (or delete)
- `apps/sonalmod/internal/infrastructure/register.go`
- `apps/sonalmod/internal/config/provide.go`, `internal/config/default.yaml`, `test.yaml`, `local.yaml`, `production.yaml` as applicable
- `apps/sonalmod/internal/api/http/middleware/error_handler.go`, `error_handler_test.go`
- `apps/sonalmod/internal/api/http/register.go`, `server/router.go` or `root_handler.go` if mounting runtime
- `apps/sonalmod/Makefile` (dist target)
- `apps/sonalmod/AGENTS.md`
- `apps/sonalmod/.testcoverage.yaml` (if needed)

**Reference (read-only for routing):**

- `runtime/httpapi/handler.go`, `runtime/internal/agentapi/*`

---

## 8. Task list

Implementation must follow **TDD** where behavior changes: write failing tests first for new wiring, then implement. After each major task, keep the module buildable. Module completion: from `apps/sonalmod`, `make lint` and `make test`; from repo root, `make lint` and `make test` per repo [AGENTS.md](../../../../../AGENTS.md). Update `apps/sonalmod/AGENTS.md` when commands, paths, or install instructions change.

**Task 1.1: Remove MCP command and package tree**

- Delete `cmd/mcp` and `internal/api/mcp`.
- Remove `mcpServer` keys from embedded config and `config/provide.go`; fix `config` tests.
- Remove MCP-only `go.mod` requires; run `go mod tidy`.
- Run `make lint` and `make test` in `apps/sonalmod` and fix compile/test failures.

**Task 1.2: Remove petstore and pet domain from infrastructure and app**

- Delete `internal/infrastructure/petstore` and pet-related app types/commands/queries; remove `PetstoreClient` wiring from `infrastructure.Register`.
- Remove petstore config keys and providers.
- Delete or rewrite tests that only covered petstore/pets; run affected tests.

**Task 1.3: Remove SQLite DB and user domain**

- Remove `newDBProvider`, `Database`, users/pets repositories, `Queryer`/`UsersRepository`/`PetsRepository` if unused; remove DDL from `database.go` and delete repo files.
- Remove `database.*` config; remove `apptime`/`ident` from wireup only if no longer referenced (verify ident stays for HTTP).
- Run `make test` `./internal/infrastructure/...` and fix.

**Task 1.4: Remove remaining demo services (math, time, echo) and jobs**

- Delete echo, math, time service files and tests; clear `app.Register`.
- Delete `cmd/jobs`; update `Makefile` dist build if needed.
- Run `go test ./...` under `apps/sonalmod`.

**Task 1.5: Preserve domain errors for HTTP middleware**

- Ensure `NotFoundError`, `InvalidInputError`, `ConflictError` and constructors remain in `app` or a dedicated package; update `middleware` imports.
- Run `go test ./internal/api/http/middleware/... -run TestNewAppErrorHandler`.

**Task 1.6: Resolve runtime wiring**

- Decide per §5: mount `Runtime.HTTPHandler` on the main `HTTPRouter` (add integration test: request to agent path returns non-5xx or expected stub) **or** remove `NewRuntime` from `wireup` and adjust tests.
- Run tests for `internal/runtime.go` and HTTP server packages.

**Task 1.7: Documentation and cleanup**

- Update `AGENTS.md` (run, install, config keys—no MCP/jobs/petstore/DB unless reintroduced).
- Search repo for `cmd/mcp`, `petstore`, `mcpServer` in docs; update if needed.
- Final `make lint` and `make test` from repo root and `apps/sonalmod`.

**Task 1.8: Compress implementation summaries**

- Follow [compress-implementation-summaries.md](../../../../../.context/compress-implementation-summaries.md) to compress per-task `summary-task-*.md` files into a single `implementation-summary.md` after implementation tasks complete (and remove per-task summaries). If sub-agent execution is unavailable, note the limitation and keep per-task files until compression can be run.

---

## References

- OpenSpec proposal: `openspec/changes/trim-sonalmod-unused-boilerplate/proposal.md`
- Design: `openspec/changes/trim-sonalmod-unused-boilerplate/design.md`
- Spec: `openspec/changes/trim-sonalmod-unused-boilerplate/specs/sonalmod-lean-module/spec.md`
