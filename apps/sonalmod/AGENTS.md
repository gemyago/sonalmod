<!-- AGENTS.md — README for machines. Nearest file in the tree wins (hierarchical precedence). -->

## Status

**Lean module.** This process exposes the HTTP server with config, logging, OpenTelemetry, health, generated API routes, and runtime agent HTTP (for example profile-based `POST /api/runtime/agent-runs` and session-scoped runs under `/api/runtime/sessions/...`). Foundation-era extras (separate `cmd/jobs` / `cmd/mcp` binaries, petstore client, SQLite demo persistence, MCP) are not part of the tree; see `doc/implementation/trim-sonalmod-unused-boilerplate/`.

## Layout (module root)

Notable layout parts of `apps/sonalmod`:

```
.
├── cmd/sonalmod/     # `package main`: Cobra CLI (`start`, user commands, …)
├── doc/              # Architecture (`architecture.md`) and implementation notes under `doc/implementation/`
├── internal/         # Config, DI, HTTP API, auth, app layer, infrastructure, telemetry, …
├── engine.go         # Root package: thin embed/test surface (`NewEngine`, `StartHTTPServer`, typed getters)
└── project.json      # Nx project
```

## Run

From the module root (`apps/sonalmod`):

- **HTTP server:** `go run ./cmd/sonalmod start` (Cobra root is `sonalmod`; `start` runs the HTTP server).
- **CLI help:** `go run ./cmd/sonalmod --help`, `go run ./cmd/sonalmod start --help`.

## Install

From `apps/sonalmod` (with `GOBIN` or `GOPATH/bin` on your `PATH`):

- **Main binary:** `go install ./cmd/sonalmod` → installs `sonalmod` (binary name follows the `cmd/sonalmod` directory).

## Env / config

- **Embedded YAML:** `internal/config/default.yaml` is merged first, then `internal/config/<env>.yaml` (e.g. `local.yaml`, `test.yaml`, `production.yaml`). Optional per-developer overrides: `internal/config/<env>-user.yaml` (gitignored patterns may apply—file may be absent).
- **Environment:** Viper uses prefix **`APP`** and `AutomaticEnv()`. `-` and `.` in config keys are replaced with `_` when binding env vars (see `internal/config/load.go`). Override embedded defaults by setting the corresponding `APP_…` variable for the path you need. Top-level keys in `internal/config/default.yaml` include `defaultLogLevel`, `pprofListener`, `gracefulShutdownTimeout`, `httpServer`, `openTelemetry`, `skills`, and `agentRuntime` (see that file for the full tree). Provider configuration (API keys, base URLs, models) is managed exclusively via the Providers CRUD API / `ProvidersConfigService` — there are no `openai.*` config keys.
- **Agent runtime persistence (`agentRuntime` key):** `agentRuntime.storage.type` selects where the embedded agent runtime stores state (sessions, providers config, agent profiles with execution settings, etc.) — `"file"` (default; file-specific options under `agentRuntime.storage.file`, e.g. `baseDir`) or `"database"` (GORM-backed; set DSN via `APP_AGENTRUNTIME_DATABASE_DSN` or `agentRuntime.database.dsn`). `agentRuntime.database.tablePrefix` sets the GORM table name prefix for database-backed runtime tables (embedded default `sonalmod_` in `default.yaml`; override via `APP_AGENTRUNTIME_DATABASE_TABLEPREFIX`). The runtime library does not apply a prefix when this value is empty. When `"database"` is used, `agentRuntime.database.autoMigrate` (bool, default `true`) controls whether `runner.AutoMigrate()` runs on startup; set `APP_AGENTRUNTIME_DATABASE_AUTOMIGRATE=false` to disable.
- **Deprecated config/env (breaking):** if you used the previous `ai` / `sessionStorage` names, migrate as follows:

  | Old YAML path | New YAML path |
  | --- | --- |
  | `ai.sessionStorage` | `agentRuntime.storage` |
  | `ai.sessionStorage.type` | `agentRuntime.storage.type` |
  | `ai.database` | `agentRuntime.database` |

  | Old env var | New env var |
  | --- | --- |
  | `APP_AI_SESSIONSTORAGE_TYPE` | `APP_AGENTRUNTIME_STORAGE_TYPE` |
  | `APP_AI_DATABASE_DSN` | `APP_AGENTRUNTIME_DATABASE_DSN` |
  | `APP_AI_DATABASE_AUTOMIGRATE` | `APP_AGENTRUNTIME_DATABASE_AUTOMIGRATE` |
- **Skills config:** `skills.enabled` (bool, default `false`), `skills.paths` (list of directories; defaults to `~/.config/agents/skills` and `.agents/skills`), `skills.maxSkillBytes` (int, default 65536), `skills.maxCatalogEntries` (int, default 500). When `skills.enabled` is `true`, the runtime discovers `SKILL.md` files in the configured paths, registers `skills_list` and `skills_read` tools, and injects a compact `<available_skills>` block into the agent instruction. Non-existent paths, malformed files, and duplicate skill names produce warning logs and are skipped; they do not fail startup.
- **CLI flags** (global): `--env` / `-e` selects which `<env>.yaml` layer loads (default env for loaders is `local` when not overridden). Logging: `--log-level` / `-l`, `--json-logs`, `--logs-file` (tests).

## Lint / test

- **This module:** `make lint`, `make test` from `apps/sonalmod` (uses repo-root pinned `golangci-lint` from `bin/` unless `CI=true`).
- **Whole repo:** from the repository root, `make lint` and `make test` include this module via `$(MAKE) -C apps/sonalmod …`.

## Module Rules and Conventions

This section defines module-specific rules and conventions. Project-level rules and conventions must also be followed.

Use gopher skill as your primary source of golang coding conventions and best practices.

The rules are:
- Update module rules and conventions when user corrects the behavior of AI.
- OpenAPI JSON uses camelCase for property names or any other identifiers or keys; regenerate after spec edits.

## Purpose (directional)

- **Architecture overview:** [doc/architecture.md](doc/architecture.md) (stack, layout, wiring, API boundaries).
- Consumer-facing entrypoint is `package main` under `cmd/sonalmod/` (standard Go `cmd/<binary>` layout); application code lives under `internal/`.
- May depend on `runtime/` and coordinate with `apps/sonal-ui` for delivery when that work lands.
- `engine.go` is a thin surface for embedding and tests: it wraps the DI container after `internal.Setup` and exposes a small API (`NewEngine`, `StartHTTPServer`, typed `Get*` resolvers). Details: [doc/architecture.md](doc/architecture.md#root-package-enginego).

## Task Completion Protocol

Repository level task completion protocol **MUST ALWAYS** be followed. If you didn't follow it, this means task is not complete.
