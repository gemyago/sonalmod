# Implementation Summary: Initial Authentication & Authorization

**Plan:** [plan-initial-authentication.md](./plan-initial-authentication.md)

## Overview

Introduced JWT-based authentication and authorization to Sonalmod: a file-system user store with Argon2id password hashing, JWT access tokens, opaque refresh tokens, auth middleware, login/refresh/me API endpoints, CLI user management commands, and Svelte 5 frontend auth (login page, auth store, route guards, token-aware fetch). The runtime's `CallerIdentity` interface replaces the insecure `userId` request parameter end-to-end.

## Tasks

### Task 1.1: Define CallerIdentity in runtime public contract
Added `CallerIdentity` interface and context helpers in `runtime/httpapi` with private context key and round-trip tests.

### Task 1.2: Update runtime handler to read CallerIdentity from context
Identity primitives were extracted to a new `runtime/internal/callerid` package (avoiding an import cycle with `httpapi`); `httpapi` re-exports via aliases. Handlers prefer `callerid.FromContext` over body/query `userId`, with precedence, fallback, and missing-identity tests.

### Task 2.1: Add auth configuration to sonalmod
Added an `auth` section to `default.yaml` (JWT signing key placeholder, token TTLs) and three DI bindings in `config/provide.go`.

### Task 2.2: Implement password hashing with Argon2id
Added `Argon2idHasher` with standard `$argon2id$...` encoding and constant-time verification; promoted `golang.org/x/crypto` to a direct dependency. The `passwordHasher` interface was deferred to the auth service (Task 2.5) per consumer-defined interfaces convention.

### Task 2.3: Implement file-system user store
JSON file-system user store with atomic writes, sentinel errors, and comprehensive tests; `userStore` consumer interface deferred to `service.go` (Task 2.5).

### Task 2.4: Implement JWT service and refresh token store
HS256 JWT service with configurable/auto-persisted signing key and a file-based refresh token store (random tokens, SHA-256 hashes, atomic writes). Used `golang-jwt/jwt/v5` (promoted from indirect).

### Task 2.5: Implement auth service
`AuthService` with Login, Refresh, CurrentUser operations, consumer interfaces, sentinel errors, mockery config, and tests. New `.mockery.yaml` added to `apps/sonalmod`.

### Task 2.6: Implement auth middleware
HTTP auth middleware validating `Authorization: Bearer` JWTs, attaching `CallerIdentity` to context, responding JSON 401 on failure. `dig.In` wiring deferred to Task 2.7.

### Task 2.7: Implement auth API handlers, DI wiring, and route registration
Auth handlers (login, refresh, me), DI wiring via bridge factories (`*DIParams` with `dig.In`), `/api/v1/auth/*` routes, and `/api/v1/runtime/` behind auth middleware.

### Task 3.1: Implement CLI user management commands
`sonalmod user` Cobra group with `add`, `list`, and `change-password` subcommands wired to `UserStore` and `Argon2idHasher`, with tests for run paths and error cases.

### Task 4.1: sonal-ui auth foundation
Typed auth API helpers for `/api/v1/auth/*`, Svelte 5 rune-based `AuthStore` with refresh token in `localStorage`, accessible Login page, and `/login` route in `ui-wireframe.md`.

### Task 4.2: Route guarding and token management
`createAuthFetch` for Bearer injection with single 401 refresh retry; `/login` route, `/chat` and `/about` guarded with `svelte-spa-router`'s `wrap`, session restore on mount with loading state.

### Task 4.3: Update API calls, remove VITE_AGENT_USER_ID, update wireframe
Agent API calls now use Bearer token from `authStore`; `userId` removed from run body and session query. `readSession` temporarily used raw `fetch` to bypass generated type requiring `userId` (resolved in Task 5.1). Vite dev proxy broadened from `/api/v1/runtime` to `/api/v1`.

### Task 5.1: Remove userId from runtime API spec and finalize
OpenAPI spec and runtime handler no longer accept `userId`; handlers require `CallerIdentity` from context and return 401 when absent. `api.gen.go` was edited by hand (oapi-codegen v2.6.0 discriminator bug). `readSession` in `client.ts` switched back to typed `openapi-fetch` client.

## Deviations & notes

- **Import cycle workaround (1.2)**: `CallerIdentity` primitives extracted to `runtime/internal/callerid`; `httpapi` re-exports via aliases to keep public API stable.
- **Consumer interfaces deferred (2.2, 2.3)**: `passwordHasher` and `userStore` interfaces were not added until `service.go` (Task 2.5) to avoid unused-type lint — consistent with consumer-defined interface convention.
- **DI bridge factories (2.7)**: `ServiceDeps` structs used private interface fields incompatible with `dig.In`, so bridge `*DIParams` structs were used instead of modifying those structs.
- **`readSession` raw fetch workaround (4.3 → fixed 5.1)**: Generated types required `userId` query param; raw fetch used as a temporary bypass, resolved when spec was updated.
- **`oapi-codegen` v2.6.0 bug (5.1)**: `go generate` panics on discriminator handling; `api.gen.go` was hand-edited rather than regenerated.
- **`gocognit` refactor (2.4)**: `DeleteAllForUser` was split into helpers after hitting complexity limit of 20.
- **`revive` stutter lint (2.5)**: `ServiceDeps` used instead of `AuthServiceDeps`; `AuthService` type kept with `//nolint:revive`.

## Completion

- Lint: ✓
- Type check: ✓
- Tests: ✓
