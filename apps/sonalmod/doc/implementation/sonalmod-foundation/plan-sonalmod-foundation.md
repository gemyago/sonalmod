# Plan: `apps/sonalmod` — minimal foundation (structure-aligned, not product-bound)

> **Historical note:** This document describes the **original** foundation copy (including `cmd/jobs`, `cmd/mcp`, petstore, demo DB, and related boilerplate). The `apps/sonalmod` module was later **trimmed** per [`../trim-sonalmod-unused-boilerplate/plan-trim-sonalmod-unused-boilerplate.md`](../trim-sonalmod-unused-boilerplate/plan-trim-sonalmod-unused-boilerplate.md). The current run, install, and config story is in [`../../../AGENTS.md`](../../../AGENTS.md).

## 1. Introduction / overview

**Goal:** Add a **first working Go module** under `apps/sonalmod` with a **small HTTP API placeholder** (enough to prove config, startup, and one or two routes). **Package layout and practices** should follow `tmp/golang-backend-boilerplate` where practical (e.g. `internal/config`, HTTP concerns under `internal/api`, logging/deps style per [docs/golang-coding-guide.md](../../../../../docs/golang-coding-guide.md)).

**Out of scope for this slice:** Treating this as the full “installable Sonalmod application” narrative (see repo AGENTS pointers elsewhere) and **custom** product behavior beyond what the boilerplate sample already implements. **In scope:** copying boilerplate **`internal/app`** (users/pets), DB/infrastructure (including **`internal/infrastructure/httpclient`** and **`internal/infrastructure/petstore`**), generated **`v1routes`**, and **`telemetry/`** as listed in §4.1 — the module should still **build, run, and serve** HTTP (including any sample routes from the copy).

**Problem solved:** `apps/sonalmod` today has no `go.mod` or runnable entry. This work lands a **buildable module** you can `go run .` / `go install` from the **module root**, with a deliberately **simple** API surface (e.g. health + echo-style JSON) as a stand-in until real features arrive.

**Non-goals:** UI (`apps/sonal-ui`), depending on `runtime/` unless explicitly added later, production deploy, and ** wholesale** duplication of the entire boilerplate tree.

**Terminology / entrypoint:** The **primary `package main`** remains a **`main.go` in the module root** (`apps/sonalmod/main.go`), next to `go.mod`. The boilerplate also supplies **`cmd/jobs`** and **`cmd/mcp`** (see §4.1); those are **additional binaries** under `apps/sonalmod/cmd/`, not a replacement for the root entry. Installation is conceptually `cd apps/sonalmod && go install` (or `go run .` for the root binary; build other commands from `cmd/...` as needed).

## 2. Business logic

- **Placeholder API:** At minimum, something operators can hit (e.g. **`GET /health`**) and one **echo-style** JSON endpoint (e.g. **`POST /echo`** or **`GET /echo`** with a trivial payload) demonstrating request → response. Exact paths and bodies are implementation details; behavior should be obvious and testable.

## 3. High-level architecture

| Piece | Role |
|--------|------|
| `main.go` (module root) | Parse flags or env, load config, construct logger, start HTTP server, graceful shutdown. Keep thin; delegate to `internal/`. |
| `internal/config/` | Viper + YAML (or equivalent), aligned with boilerplate **if copied/adapted** from `tmp/golang-backend-boilerplate/internal/config`. |
| `internal/api/http/` (or similar) | HTTP server setup, routing, handlers — align with copied boilerplate **`v1routes`** / server code where those are brought in (§4.1); add **placeholder** routes (e.g. health/echo) only if they do not duplicate generated routes. |
| Other `internal/` packages | **`internal/di`** — **full copy** of the boilerplate `internal/di` tree (entire package as-is, including tests; see §4.1). Wire the app in `main` via `dig`; avoid manual constructor plumbing at the root. Also copy **`internal/infrastructure`** (must include **`internal/infrastructure/httpclient`** in full, including tests and **`middleware/`** subpackage — **petstore** builds on it), **`internal/infrastructure/petstore`** (keep the sample Petstore client), **`internal/app`**, **`telemetry/`**, **`cmd/jobs`**, **`cmd/mcp`**, generated **`v1routes`**, per §4.1. |

**Principle:** Mirror **folder names and dependency direction** (config → app → api) like the boilerplate, and copy the boilerplate subtrees listed in §4.1 (including OTEL/telemetry, jobs/MCP, infrastructure, and app/routes) so the module stays aligned with that graph.

## 4. Detailed architecture (by area)

### 4.1 Selective copy from the boilerplate (no full-tree rsync)

**Do not** copy the entire `tmp/golang-backend-boilerplate` tree into `apps/sonalmod`. Instead:

1. **Inventory** the boilerplate and list **exact directories or files** worth reusing (examples below — adjust after inspection).
2. For each chosen path, use **explicit shell copies**, e.g.:

```bash
# Examples only — confirm paths exist before running
BOILER="tmp/golang-backend-boilerplate"
APP="apps/sonalmod"

mkdir -p "${APP}/internal"
cp -R "${BOILER}/internal/config" "${APP}/internal/"
cp -R "${BOILER}/internal/di" "${APP}/internal/"
```

3. **Module path:** Set `go.mod` to something like `github.com/gemyago/sonalmod/apps/sonalmod` (see [README](../../../../../README.md)); then **only under copied files** (and new code), replace imports:

```bash
OLD='github.com/gemyago/golang-backend-boilerplate'
NEW='github.com/gemyago/sonalmod/apps/sonalmod'
find "${APP}/internal" "${APP}/cmd" -type f -name '*.go' -exec grep -l "$OLD" {} \; | while read -r f; do
  sed -i '' "s|${OLD}|${NEW}|g" "$f"
done
# Include any other copied roots (e.g. generated routes under internal/api) in the same way.
```

4. **New code** (`main.go`, handlers) should import `NEW` from the start; run import rewrites on **every** copied tree, including generated **`v1routes`** / client packages once they exist under `${APP}`.

5. **`go mod tidy`** in `apps/sonalmod` and fix compile errors by **`go get` / adjusting imports** — **do not** delete files under **`internal/di`**, **`internal/infrastructure`**, **`internal/app`**, **`telemetry/`**, **`cmd/jobs`**, **`cmd/mcp`**, or generated routes/clients listed in §4.1 to “shrink” the tree; resolve deps instead.

**Candidates to copy (evaluate optional rows; required rows are fixed in the next subsection):**

| Boilerplate path | Typical reuse |
|------------------|----------------|
| `internal/config/` | Env-specific YAML + Viper wiring (high value if you want parity) |
| `internal/di/` | **`dig` helpers — full tree** (all `.go` files, tests, subpackages if any); no selective deletion to shrink the package |
| `internal/system/lifecycle/` | Shutdown hooks — optional for graceful stop |
| `internal/api/http/server/` | **Large**; likely **do not** copy wholesale — instead read patterns and implement a slim `ListenAndServe` in this module |

**Copy for v1 (required):** `cmd/jobs`, `cmd/mcp`, `internal/infrastructure` (including **`httpclient`** and **`petstore`** subtrees in full), `internal/app` (users/pets), generated `v1routes`, and full `telemetry/` for OTEL parity with the boilerplate.

### 4.2 Monorepo `go.work`

Add `use ./apps/sonalmod` to the repo root `go.work` (project uses Go 1.26.x per root [AGENTS.md](../../../../../AGENTS.md)). Run `go work sync` from the repo root.

### 4.3 HTTP API (boilerplate routes + optional placeholders)

Wire HTTP using the **copied** boilerplate server / **`v1routes`** (and related generated code) from §4.1. Add **placeholder** endpoints (e.g. **`GET /health`**, echo-style JSON) only where they do not conflict with generated routes. Tests: table-driven or `httptest`; follow [docs/testing-best-practices.md](../../../../../docs/testing-best-practices.md) where applicable.

Regenerate OpenAPI-derived code **from the boilerplate’s spec/process** when you update APIs; keep **`oapi-codegen`** (or whatever the sample uses) in the module’s tooling story.

### 4.4 Root `Makefile` integration

Follow the same pattern as other modules in the root [Makefile](../../../../../Makefile): `lint` and `test` delegate with `$(MAKE) -C <module> …` (see `tools/firecrawl` and `runtime`). Add `apps/sonalmod` there so repo-wide `make lint` and `make test` include it—e.g. `$(MAKE) -C apps/sonalmod lint` once a module `Makefile` exists, or document `go test ./...` from that directory until then. Coding task completion protocol applies once code exists.

### 4.5 `apps/sonalmod/AGENTS.md`

Update with: how to run (`go run .` from `apps/sonalmod`), how to install, env/config knobs, lint/test commands, and that this module is **scaffolding** until product scope is attached. No requirement to reference “installable app” positioning unless the team adds it later.

## 5. Key architectural decisions

| Decision | Choice |
|----------|--------|
| Boilerplate usage | **Selective** *which* subtrees you copy + new code; **not** blind `rsync` of the whole tree. **`internal/di`** and the paths in §4.1 **“Copy for v1 (required)”** are copied **in full** (entire trees / generated artifacts as in the sample), not trimmed to shrink compile surface. |
| Entry binary | **Root `main.go`** at module root (`apps/sonalmod/main.go`); **additional** binaries under `cmd/jobs`, `cmd/mcp` per boilerplate. |
| API | **Boilerplate** OpenAPI/codegen routes (`v1routes`, etc.) **plus** optional minimal placeholders (health/echo) where useful. |
| Product narrative | **Out of scope** for this foundation slice |
| Module path | e.g. `github.com/gemyago/sonalmod/apps/sonalmod` (confirm if policy differs) |
| Dependency injection | **`go.uber.org/dig`** — `main` builds a container and invokes providers; **`internal/di`** is a **full** copy of the boilerplate package (not a minimal subset). Same general pattern as `tools/firecrawl` if you need a second reference. |

## 6. Uncertainties

- **Which config files to keep:** Boilerplate YAML may reference keys for server port, env, logging — trim to what `main` actually reads.
- **Lint config:** Whether to add `apps/sonalmod/.golangci.yml` or inherit from repo — align with root `make lint` when integrated.

## 7. Related files (expected touch set)

**New:**

- `apps/sonalmod/go.mod`, `go.sum`
- `apps/sonalmod/main.go`
- `apps/sonalmod/internal/api/http/*.go` (or equivalent; may overlap generated routes from boilerplate)
- `apps/sonalmod/internal/di/**` (full copy of boilerplate `internal/di`, including tests)
- **Required copies (§4.1):** `cmd/jobs/**`, `cmd/mcp/**`, `internal/infrastructure/**` (explicitly **`internal/infrastructure/httpclient/**`** and **`internal/infrastructure/petstore/**`**), `internal/app/**`, generated **`v1routes`**, `telemetry/**` (and any sibling paths the boilerplate lists for those features)
- Tests under the same module

**Optional (selective copy + edit):**

- `apps/sonalmod/internal/config/**` (from boilerplate)

**Repo:**

- Root `go.work`
- Root `Makefile`
- `apps/sonalmod/AGENTS.md`
- Optional: `apps/sonalmod/Makefile` mirroring patterns from boilerplate **only** if needed for CI

## 8. Task list (TDD; self-contained steps)

Follow **TDD** where logic warrants it. After code changes, follow the **coding task completion protocol** in root [AGENTS.md](../../../../../AGENTS.md): `make lint` and `make test`.

**Task 1.1: Module skeleton and root `main`**

- Add `apps/sonalmod/go.mod` with chosen module path.
- Add **`main.go` at module root** that starts a minimal HTTP server (can be one route first).
- **TDD:** Add a test that hits the server via `httptest` or starts listener on `:0`.
- **Checkpoint:** `go run .` from `apps/sonalmod` works.

**Task 1.2: Selective boilerplate reuse**

- List concrete paths under `tmp/golang-backend-boilerplate` to copy (see §4.1 — include **all** paths under **“Copy for v1 (required)”** plus **`internal/di`** and optional rows in the table).
- Run **`cp -R`** into `apps/sonalmod`; **`sed`** (or equivalent) import rewrites on **all** copied `.go` trees (`internal/**`, `cmd/**`, generated packages); **`go mod tidy`** — resolve deps; **do not** delete boilerplate files in required packages to force a smaller build.
- Wire `main` with **`dig`** per the copied app: register providers (config, logger, HTTP server, telemetry, DB, etc.) and `Invoke` the listen/run path. Load config if `internal/config` was copied; otherwise follow whatever the copied `main`/bootstrap expects.
- **Checkpoint:** `go build ./...` from `apps/sonalmod` (including `cmd/jobs`, `cmd/mcp` if present).

**Task 1.3: HTTP API (generated routes + optional placeholders)**

- **TDD:** Tests for key routes — fail first if adding new behavior (use paths that do not collide with generated **`v1routes`** unless testing those handlers).
- Wire **`v1routes`** / server from the copy; add **health/echo** placeholders only if still useful.
- **Checkpoint:** `go test ./...` passes in module.

**Task 1.4: `go.work` and root Makefile**

- Add `use ./apps/sonalmod`; `go work sync`.
- Extend root `Makefile` for lint/test coverage of this module.
- **Checkpoint:** `make lint` and `make test` from repo root succeed.

**Task 1.5: Documentation**

- Update `apps/sonalmod/AGENTS.md` with run/install/test instructions (see §4.5).

**Task 1.6: Implementation summaries**

- Write per-task `summary-task-X.Y.md` under this folder as required by team practice.

**Task 1.7: Compress implementation summaries**

- Follow [.context/compress-implementation-summaries.md](../../../../../.context/compress-implementation-summaries.md).

---

*Plan created per [.context/create-plan.md](../../../../../.context/create-plan.md). Implementation is intentionally out of scope for this document.*
