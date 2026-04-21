# Implementation Summary: Trim `apps/sonalmod` unused boilerplate (lean module)

**Plan:** [plan-trim-sonalmod-unused-boilerplate.md](./plan-trim-sonalmod-unused-boilerplate.md)

## Overview

Foundation-era scaffolding was removed from `apps/sonalmod`: MCP and petstore/demo layers, SQLite and user/pets domain, echo/math/time jobs and binaries, and obsolete docs. Domain HTTP errors stayed in `package app`; the agent runtime handler was mounted on the main HTTP router. Documentation and shared examples were aligned with the lean module, and per-task notes were consolidated here.

## Tasks

### Task 1.1: Remove MCP command and package tree

The MCP server (`cmd/mcp`, `internal/api/mcp`), MCP config and providers, and the `mcp-go` dependency were removed. `go mod tidy` required an explicit `require`/`replace` for the in-repo `runtime` package; `gomoddirectives` was excluded for `apps/sonalmod/go.mod` so the local `replace` passes lint.

### Task 1.2: Remove petstore and pet domain from infrastructure and app

Petstore infrastructure and the pet application layer (client, repos, commands, queries, mocks, OpenAPI assets) were removed, along with `user_pets` SQLite init, pet-related config and DI wiring, a stale CASCADE comment, and a dead golangci exclusion; `go mod tidy` was run in `apps/sonalmod`.

### Task 1.3: Remove SQLite DB and user domain

The SQLite stack and user application layer were removed (including DI, config, tests, and mocks), `wireup` no longer uses `apptime`, and `go mod tidy` dropped SQLite/otelsql-related direct requires.

### Task 1.4: Remove remaining demo services (math, time, echo) and jobs

Demo app services and their tests were removed, `app.Register` was reduced to only `di.ProvideAll`, the `cmd/jobs` echo binary was deleted, and the Makefile was updated so `dist/bin` builds the root module to `dist/bin/sonalmod` and `coverpkg` no longer includes `./cmd/...`.

### Task 1.5: Preserve domain errors for HTTP middleware

`NotFoundError`, `InvalidInputError`, and `ConflictError` remain in `internal/app/errors.go` (with tests); HTTP error middleware still maps them to 404, 400, and 409 via `errors.As`. No file moves or import rewrites were required.

### Task 1.6: Resolve runtime wiring

The main `HTTPRouter` mounts `Runtime.HTTPHandler` on the same agent-run paths used when `BaseURL` is empty, with `V1RoutesDeps` exposing `*internal.Runtime` and `register_test.go` integration tests confirming requests hit the mounted handler.

### Task 1.7: Documentation and cleanup

`apps/sonalmod/AGENTS.md` was rewritten for the lean module (install, config, routes, trim doc pointer); a historical note was added to the foundation plan so obsolete `cmd/jobs` / `cmd/mcp` / petstore content is not misleading; the shared `.context/create-http-clients.md` example logger group was renamed from `petstore-client` to `example-http-client`.

## Deviations & notes

- **Task 1.1 — `go mod` / lint:** Tidying needed `runtime` listed with `replace` to `../../runtime`; `gomoddirectives` forbids local `replace` by default, so a targeted exclusion for `apps/sonalmod/go.mod` was added with a short comment.
- **Task 1.2 — DB split:** `database.go` still existed for users until Task 1.3; `user_pets` table and init were removed so nothing referenced deleted pet DDL. A dead path exclusion was dropped from `.golangci.yml`.
- **Task 1.7:** `.context/create-http-clients.md` lives outside `apps/sonalmod` but was updated for consistent naming after petstore removal.

## Completion

- Lint: ✓
- Type check: ✓ (Go build / `go test` coverage)
- Tests: ✓
