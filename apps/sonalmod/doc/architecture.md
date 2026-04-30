# Sonalmod backend architecture (brief)

HTTP server and CLI entrypoint for Sonalmod under `apps/sonalmod`: a single **`sonalmod`** binary built from `cmd/sonalmod` (`go run ./cmd/sonalmod start` / `go install ./cmd/sonalmod`). The app wires configuration, logging, OpenTelemetry, health, and mounts the **runtime** agent HTTP API. The long-term shape is a deployable backend that can serve or embed **`apps/sonal-ui`** as one unit.

## Stack

| Area | Choice |
| --- | --- |
| Language | **Go** (see repo root / module **`go.mod`**, currently **1.26.x**) |
| CLI | **Cobra** — root command **`sonalmod`**, subcommands e.g. **`start`** (HTTP), **`cli`** |
| DI | **`go.uber.org/dig`** — wiring in `internal.Setup`, `internal/di`, `*Register` packages |
| HTTP | **`net/http`** + **`http.ServeMux`** — route patterns `METHOD path` on `internal/api/http/server.HTTPRouter` |
| Config | **Viper** — embedded YAML under `internal/config/`, env prefix **`APP_`** (see `internal/config/load.go`) |
| Logging | **`log/slog`** — root logger options in `internal/telemetry`, optional JSON / file / OTEL bridge |
| Observability | **OpenTelemetry** — traces/metrics/logs hooks in `internal/telemetry` (off by default in `default.yaml`) |
| Generated REST (app-owned) | **apigen** — `//go:generate` on `internal/api/http/register.go` reads **`internal/api/http/v1routes.yaml`** |
| Agent HTTP | **`runtime/httpapi`** — handler built in `internal/runtime.go`, mounted under **`/api/v1/runtime/`** |

## Root package (engine.go)

- **`Engine`** (`engine.go`, package **`sonalmod`**) holds the **`dig.Container`** and **`viper.Viper`** produced by **`internal.Setup`**. It is the embeddable entrypoint when you do not run **`cmd/sonalmod`** (tests, other binaries, library-style use).
- **`NewEngine`** applies optional **`EngineOpt`** (JSON logs, log file path, **`env`**, default log level), then calls **`internal.Setup`** to register the full graph (config, telemetry, runtime, auth, app, HTTP constructors). Failures surface as **`failed to setup engine`**.
- **`StartHTTPServer`** is blocking until shutdown: it **`Register`s** **`server`** and **`http`** on the container (route wiring), optionally sets **`httpServer.uiLocation`**, then invokes **`StartupGroupFactory`** to start **`HTTPServer`**. Intended to be called **once** per process; **`EngineStartServerOpt`** supports **`noop`** for tests and **`uiLocation`** for static UI.
- **Typed accessors** on **`Engine`** resolve concrete types via **`container.Invoke`**: **`GetToolsRegistry`**, **`GetAgentRunner`**. Use these when you need the same instances as the HTTP server without starting it.
- **CLI user subcommands** (`cmd/sonalmod`, e.g. **`user add` / `list` / `change-password`**) obtain **`auth.UserStore`** and **`auth.Argon2idHasher`** by calling **`NewEngine`** with the same injected **`dig.Container`** as the command (`internal.WithEngineContainer`), then resolving those types from that container—no extra **`Engine`** getters.
- **Rule of thumb:** keep **`engine.go`** free of business logic—only engine configuration, HTTP lifecycle, and narrow **`Get*`** exports.

## Layout (conceptual)

- **`main.go`** / **`cli.go`** — Cobra commands, process lifecycle, **`internal.Setup`** before subcommands run.
- **`internal/wireup.go`** — loads config, registers DI (ident, shutdown hooks, **`NewRuntime`**, config providers, telemetry, app, infrastructure).
- **`internal/runtime.go`** — constructs **`agent.Runner`** (LLM provider, **`workspacefs`** tools, filesystem storage under configurable data dir, and a required persisted agent profile service for runner-owned profile execution) and exposes **`httpapi`** as **`HTTPHandler`**.
- **`internal/api/http/`** — HTTP composition: **`server/`** (router, HTTPServer, middleware chain), **`v1routes/`** (generated routes + handlers, e.g. health), **`v1controllers/`**, **`middleware/`**.
- **`internal/config/`** — **`default.yaml`** plus per-env **`local.yaml`**, **`test.yaml`**, **`production.yaml`**; optional **`*-user.yaml`** overrides.
- **`internal/telemetry/`** — slog, OTEL resource, HTTP middleware, pprof helper.
- **`internal/infrastructure/`** — outbound HTTP client factory and middleware (correlation, logging).

## Configuration and env

- **Layers:** embedded **`default.yaml`**, then **`internal/config/<env>.yaml`** (from **`--env` / `-e`**, default **`local`**), then optional **`internal/config/<env>-user.yaml`** for local secrets.
- **Env:** keys map to **`APP_…`** (Viper `AutomaticEnv()`); nested keys use underscores (e.g. **`APP_OPENAI_APIKEY`** for OpenAI). See **`internal/config/default.yaml`** and **`internal/config/provide.go`** for injected **`name:"config.…"`** bindings.
- **HTTP defaults:** e.g. **`httpServer.port`** **4501**, **`writeTimeout`** aligned with long SSE/agent runs (see comments in **`default.yaml`**). No secrets in repo.

## Repository integration

- **Module:** `github.com/gemyago/sonalmod/apps/sonalmod` with **`replace github.com/gemyago/sonalmod/runtime => ../../runtime`** for local **`runtime/`** development.
- **`apps/sonalmod/Makefile`:** **`make lint`** (**`golangci-lint`**), **`make test`** (coverage profile + **`go-test-coverage`** vs **`.testcoverage.yaml`**).
- **Root `Makefile`** runs **`$(MAKE) -C apps/sonalmod lint|test`** before **`apps/sonal-ui`**; Go coverage from this module is merged into the root merged HTML report (see root Makefile **`tail`** of **`apps/sonalmod/.cover/profile.out`**).

## API integration

- **Sonalmod-owned OpenAPI:** **`internal/api/http/v1routes.yaml`** — health, **`/api/v1/auth/*`** (login, refresh, me), and related schemas; **camelCase** in JSON per module conventions. Codegen updates **`internal/api/http/v1routes/`** via apigen (**`go generate`** on **`register.go`**).
- **Runtime agent API:** authoritative OpenAPI for the agent HTTP surface lives under **`runtime/`** (e.g. **`runtime/internal/agentapi/openapi.yaml`**). This process **strips** the prefix and forwards **`/api/v1/runtime/*`** to the runtime **`httpapi`** handler (see **`internal/api/http/register.go`** **`SetupV1Routes`**). Standard run endpoints are profile-based (`profileName`), agent profiles carry mode-specific **`executionSettings`** (`regular` by default or `acp-stdio` for ACP subprocess execution), regular profile execution stays runner-owned, and the public runtime surface no longer exposes `/opencode-*` endpoints. The **`apps/sonal-ui`** client generates TypeScript types from that runtime spec.

## Decisions (why not X)

- **`go.uber.org/dig` instead of a larger framework:** explicit constructor graphs, small surface, fits a single-binary service.
- **`net/http` + `ServeMux` instead of Chi/Echo:** standard library, route patterns with method prefix; middleware wrapped per route in **`HTTPRouter`**.
- **apigen for `v1routes` only:** keeps generated HTTP glue for the thin app API separate from the heavier **`runtime/httpapi`** implementation.

## Technical notes (one-liners)

- **Data directory:** injected as **`config.dataDir`** (default base **`data/`** relative to CWD) for agent storage and workspace tools; align paths when running from another working directory.
- **LLM HTTP client timeout** in **`internal/runtime.go`** should stay consistent with **`httpServer.writeTimeout`** for streaming runs.
- **Health** and **auth** routes use the generated v1 stack (**`RegisterHealthRoutes`**, **`RegisterAuthRoutes`**); **`GET /api/v1/auth/me`** is wrapped with **`AuthMiddleware`** in controller. **Agent** traffic is a separate subtree under **`/api/v1/runtime/`**.

For the browser client stack and env (**`VITE_*`**), see **`apps/sonal-ui/doc/architecture.md`**.
