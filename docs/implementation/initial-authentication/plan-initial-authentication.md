# Plan: Initial Authentication & Authorization

## 1. Introduction / Overview

This plan introduces authentication and authorization (AuthN/Z) to Sonalmod. The system currently identifies users via an explicit `userId` request parameter, which is insecure and impractical for multi-user scenarios. The goal is to add a proper identity layer with:

- **JWT-based access tokens** (short-lived, ~30 minutes) for API authentication
- **Opaque refresh tokens** (long-lived, ~30 days) for session persistence
- **File-system user store** (password-protected accounts, Argon2id hashing)
- **CLI user management** (`user add`, `user list`, `user change-password`)
- **Runtime CallerIdentity** — context-based user identity replacing the explicit `userId` parameter
- **Frontend auth** — login screen, auth store, route guards, and automatic token management

All users are equal (no roles). User management UI is deferred to a later phase.

---

## 2. Business Logic

### Authentication Flow
1. An administrator creates users via CLI (`sonalmod user add --username alice --password secret`).
2. A user navigates to the SPA. If unauthenticated, the login page is displayed.
3. The user submits username + password → `POST /api/v1/auth/login`.
4. The backend validates credentials and returns an access token (JWT) and a refresh token (opaque string).
5. The frontend stores the access token in memory and the refresh token in `localStorage`.
6. All subsequent API calls include `Authorization: Bearer <accessToken>`.
7. When the access token expires (401 response), the frontend automatically calls `POST /api/v1/auth/refresh` with the refresh token to obtain new tokens.
8. If the refresh also fails, the user is redirected to the login page.

### Token Lifecycle
- **Access token**: JWT signed with HS256 (HMAC-SHA256). Contains claims `sub` (user ID), `username`, `iat`, `exp`. Lifetime: 30 minutes.
- **Refresh token**: cryptographically random 32-byte string (base64url-encoded). Server stores its SHA-256 hash. Lifetime: 30 days. On use, the old token is deleted and a new one issued (rotation).

### CallerIdentity Transition
The runtime module is auth-agnostic. It defines a `CallerIdentity` interface (`UserID() string`) in its public HTTP API contract. The sonalmod backend's auth middleware validates the JWT, creates a `CallerIdentity` implementation, and places it in the request context. The runtime handler extracts it from context instead of reading `userId` from the request body or query.

### User Storage
Users are stored as JSON files under `<dataDir>/auth/users/<userId>.json`. Fields: `id`, `username`, `passwordHash`, `createdAt`, `updatedAt`. For login (lookup by username), the store scans the directory — acceptable for the expected low user count.

### JWT Signing Key
If `auth.jwtSigningKey` config is empty (default), a random 256-bit key is auto-generated and persisted at `<dataDir>/auth/jwt-signing-key`. On subsequent starts, the persisted key is read. An explicit config value takes precedence.

---

## 3. High-Level Architecture

```
┌─────────────┐   POST /api/v1/auth/login      ┌─────────────────┐
│             │ ─────────────────────────────► │   Auth Handler    │
│  sonal-ui   │   POST /api/v1/auth/refresh     │   (sonalmod)      │
│  (SPA)      │ ─────────────────────────────► │                   │
│             │                                 │  ┌─────────────┐ │
│  Auth Store │   Authorization: Bearer <JWT>   │  │ Auth Service│ │
│  Login Page │ ──────────┐                     │  │  - login    │ │
│  Route Guard│           │                     │  │  - refresh  │ │
│             │           ▼                     │  │  - me       │ │
│             │   ┌──────────────┐              │  └──────┬──────┘ │
│             │   │Auth Middleware│──CallerID──► │         │        │
│             │   └──────┬───────┘   context    │  ┌──────▼──────┐ │
│             │          │                      │  │ User Store  │ │
│             │          ▼                      │  │ (FS JSON)   │ │
│             │   ┌──────────────┐              │  ├─────────────┤ │
│             │   │Runtime HTTP  │              │  │ JWT Service │ │
│             │   │Handler       │              │  ├─────────────┤ │
│             │   │(CallerID     │              │  │ Refresh     │ │
│             │   │ from ctx)    │              │  │ Token Store │ │
│             │   └──────────────┘              │  │ (FS)        │ │
└─────────────┘                                 │  └─────────────┘ │
                                                └─────────────────┘
      CLI                                       
  sonalmod user add/list/change-password ──────► User Store
```

### Components Involved

| Component | Module | Role |
|:---|:---|:---|
| `CallerIdentity` interface | `runtime/httpapi` | Public contract: user identity from context |
| `AgentAPIServer` | `runtime/internal/agentapi` | Reads CallerIdentity from context |
| `internal/auth` package | `apps/sonalmod` | Password hashing, user store, JWT, refresh tokens, auth service |
| Auth middleware | `apps/sonalmod` | JWT validation → CallerIdentity in context |
| Auth API handlers | `apps/sonalmod` | Login, refresh, me endpoints |
| CLI `user` subcommands | `apps/sonalmod` | User management from terminal |
| Auth store + login | `apps/sonal-ui` | Svelte 5 reactive auth state, login page, token management |

---

## 4. Detailed Architecture

### 4.1 Runtime Module (`runtime/`)

#### 4.1.1 CallerIdentity (new file: `runtime/httpapi/identity.go`)

```go
type CallerIdentity interface {
    UserID() string
}

func ContextWithCallerIdentity(ctx context.Context, id CallerIdentity) context.Context
func CallerIdentityFromContext(ctx context.Context) CallerIdentity  // nil if absent
```

A private context key type ensures no collisions. The interface lives in the public `httpapi` package so embedders (sonalmod) can implement it and set it in context.

#### 4.1.2 AgentAPIServer Changes (`runtime/internal/agentapi/server.go`)

- `parseAgentRunRequest`: read `CallerIdentity` from `r.Context()`. If present, use its `UserID()`. If absent, fall back to `req.UserId` from the JSON body (transitional backward compatibility). If both are empty, return 400.
- `ReadSession`: same pattern — prefer CallerIdentity from context, fall back to `params.UserId` query.
- After the full transition (Task 5.1), the body/query fallback is removed and `userId` is dropped from the OpenAPI spec.

#### 4.1.3 OpenAPI Spec Changes (`runtime/internal/agentapi/openapi.yaml`)

Deferred to Task 5.1 (after frontend stops sending `userId`):
- Remove `userId` from `AgentRunRequest` required properties.
- Remove `userId` query parameter from `GET /sessions/{sessionId}`.
- Regenerate `api.gen.go` and sonal-ui types.

### 4.2 Sonalmod Backend (`apps/sonalmod/`)

#### 4.2.1 Configuration

**`internal/config/default.yaml`** — new top-level `auth` section:
```yaml
auth:
  jwtSigningKey: ""        # empty → auto-generate and persist in dataDir
  accessTokenTTL: 30m
  refreshTokenTTL: 720h   # 30 days
```

**`internal/config/provide.go`** — new DI bindings:
```
config.auth.jwtSigningKey      → string
config.auth.accessTokenTTL     → time.Duration
config.auth.refreshTokenTTL    → time.Duration
```

#### 4.2.2 Password Hashing (new file: `internal/auth/password.go`)

Uses `golang.org/x/crypto/argon2` (Argon2id variant).

Interface (consumer-defined, co-located):
```go
type passwordHasher interface {
    Hash(password string) (string, error)
    Verify(password, hash string) (bool, error)
}
```

Implementation stores parameters (memory, time, threads, salt length, key length) in the hash string using the standard `$argon2id$v=19$m=...,t=...,p=...$<salt>$<hash>` format for self-describing verification.

Recommended parameters: memory 64 MB, time 1, parallelism 4, salt 16 bytes, key 32 bytes.

#### 4.2.3 User Store (new file: `internal/auth/user_store.go`)

File-system backed. Directory: `<dataDir>/auth/users/`.

```go
type User struct {
    ID           string    `json:"id"`
    Username     string    `json:"username"`
    PasswordHash string    `json:"passwordHash"`
    CreatedAt    time.Time `json:"createdAt"`
    UpdatedAt    time.Time `json:"updatedAt"`
}

type UserStoreDeps struct {
    DataDir string `name:"config.dataDir"`
    IDGen   ident.Generator
    Logger  *slog.Logger
}
```

Consumer-defined interface (used by auth service):
```go
type userStore interface {
    Create(ctx context.Context, params CreateUserParams) (*User, error)
    GetByUsername(ctx context.Context, username string) (*User, error)
    GetByID(ctx context.Context, id string) (*User, error)
    List(ctx context.Context) ([]User, error)
    UpdatePassword(ctx context.Context, id string, newHash string) error
}
```

Storage layout: `<dataDir>/auth/users/<userId>.json`. Lookup by username scans the directory. File operations use atomic write (write to temp file, then rename) for crash safety.

#### 4.2.4 JWT Service (new file: `internal/auth/jwt.go`)

Uses `github.com/golang-jwt/jwt/v5`.

```go
type JWTClaims struct {
    jwt.RegisteredClaims
    Username string `json:"username"`
}

type JWTServiceDeps struct {
    SigningKey      string        `name:"config.auth.jwtSigningKey"`
    AccessTokenTTL time.Duration `name:"config.auth.accessTokenTTL"`
    DataDir        string        `name:"config.dataDir"`
    Logger         *slog.Logger
}
```

Consumer-defined interface:
```go
type jwtService interface {
    GenerateAccessToken(userID, username string) (string, error)
    ValidateAccessToken(tokenStr string) (*JWTClaims, error)
}
```

**Key resolution**: If `SigningKey` is empty, read from `<DataDir>/auth/jwt-signing-key`. If that file doesn't exist, generate a random 256-bit key, write it to the file, and use it. The resolved key is cached in the struct for the lifetime of the process.

#### 4.2.5 Refresh Token Store (new file: `internal/auth/refresh_store.go`)

File-system backed. Directory: `<dataDir>/auth/refresh-tokens/`.

```go
type RefreshToken struct {
    UserID    string    `json:"userId"`
    TokenHash string    `json:"tokenHash"` // SHA-256 of the opaque token
    ExpiresAt time.Time `json:"expiresAt"`
    CreatedAt time.Time `json:"createdAt"`
}
```

Consumer-defined interface:
```go
type refreshTokenStore interface {
    Create(ctx context.Context, userID string, ttl time.Duration) (opaqueToken string, err error)
    Validate(ctx context.Context, opaqueToken string) (userID string, err error)
    Delete(ctx context.Context, opaqueToken string) error
    DeleteAllForUser(ctx context.Context, userID string) error
}
```

Token format: 32 random bytes, base64url-encoded. Storage file: `<dataDir>/auth/refresh-tokens/<sha256hex>.json`. `Validate` hashes the presented token, reads the file, checks expiry. `Delete` removes the file.

#### 4.2.6 Auth Service (new file: `internal/auth/service.go`)

Orchestrates login, refresh, and user info retrieval.

```go
type AuthServiceDeps struct {
    UserStore         userStore
    JWTService        jwtService
    RefreshTokenStore refreshTokenStore
    PasswordHasher    passwordHasher
    Logger            *slog.Logger
}

type LoginResult struct {
    AccessToken  string
    RefreshToken string
    User         UserInfo
}

type UserInfo struct {
    ID       string `json:"id"`
    Username string `json:"username"`
}

type RefreshResult struct {
    AccessToken  string
    RefreshToken string
    User         UserInfo
}
```

Consumer-defined interface:
```go
type authService interface {
    Login(ctx context.Context, username, password string) (*LoginResult, error)
    Refresh(ctx context.Context, refreshToken string) (*RefreshResult, error)
    CurrentUser(ctx context.Context, userID string) (*UserInfo, error)
}
```

**Login flow**: find user by username → verify password → generate access token → create refresh token → return all.

**Refresh flow**: validate refresh token → get user by ID → delete old refresh token → generate new access token + new refresh token → return all (refresh token rotation).

**Errors**: use sentinel errors (`ErrInvalidCredentials`, `ErrInvalidRefreshToken`, `ErrUserNotFound`) so HTTP handlers can map to appropriate status codes.

#### 4.2.7 Auth Middleware (new file: `internal/api/http/middleware/auth.go`)

```go
type AuthMiddleware func(http.Handler) http.Handler

type AuthMiddlewareDeps struct {
    dig.In
    JWTService jwtService  // consumer-defined interface
    Logger     *slog.Logger
}

func NewAuthMiddleware(deps AuthMiddlewareDeps) AuthMiddleware
```

Behavior:
1. Read `Authorization` header. If missing or malformed → 401.
2. Extract Bearer token. Validate via `jwtService.ValidateAccessToken`.
3. If invalid/expired → 401.
4. Create `CallerIdentity` implementation with `userID` from claims.
5. Set `httpapi.ContextWithCallerIdentity(ctx, identity)` on the request.
6. Call `next.ServeHTTP`.

The `CallerIdentity` implementation is a simple unexported struct:
```go
type jwtCallerIdentity struct{ userID string }
func (j *jwtCallerIdentity) UserID() string { return j.userID }
```

#### 4.2.8 Auth API Handlers (new file: `internal/api/http/v1controllers/auth.go`)

Manually registered (not via apigen) for full middleware control.

```go
type AuthControllerDeps struct {
    dig.In
    AuthService authService // consumer-defined interface
    Logger      *slog.Logger
}

type AuthController struct { ... }
func NewAuthController(deps AuthControllerDeps) *AuthController
func (c *AuthController) Login() http.Handler      // POST /api/v1/auth/login
func (c *AuthController) Refresh() http.Handler     // POST /api/v1/auth/refresh
func (c *AuthController) Me() http.Handler          // GET  /api/v1/auth/me
```

**Request/response shapes** (JSON, camelCase per module conventions):

`POST /api/v1/auth/login`
```json
// Request
{ "username": "string", "password": "string" }
// Response 200
{ "accessToken": "jwt...", "refreshToken": "opaque...", "user": { "id": "uuid", "username": "string" } }
// Response 401
{ "error": "invalid credentials" }
```

`POST /api/v1/auth/refresh`
```json
// Request
{ "refreshToken": "opaque..." }
// Response 200 — same shape as login
// Response 401
{ "error": "invalid or expired refresh token" }
```

`GET /api/v1/auth/me` (requires Authorization header)
```json
// Response 200
{ "id": "uuid", "username": "string" }
// Response 401
{ "error": "unauthorized" }
```

#### 4.2.9 Route Registration Changes (`internal/api/http/register.go`)

```go
type V1RoutesDeps struct {
    dig.In
    // ... existing fields ...
    AuthController *v1controllers.AuthController
    AuthMiddleware middleware.AuthMiddleware
}

func SetupV1Routes(deps V1RoutesDeps) {
    // Existing: health (public)
    rootHandler.RegisterHealthRoutes(deps.HealthController)

    // Auth routes (public — no auth middleware)
    deps.HTTPRouter.HandleRoute("POST", "/api/v1/auth/login", deps.AuthController.Login())
    deps.HTTPRouter.HandleRoute("POST", "/api/v1/auth/refresh", deps.AuthController.Refresh())

    // Auth routes (protected)
    deps.HTTPRouter.HandleRoute("GET", "/api/v1/auth/me",
        deps.AuthMiddleware(deps.AuthController.Me()))

    // Runtime (protected — auth middleware wraps the handler)
    deps.HTTPRouter.Handle(
        "/api/v1/runtime/",
        deps.AuthMiddleware(http.StripPrefix("/api/v1/runtime", deps.Runtime.HTTPHandler)),
    )
}
```

#### 4.2.10 DI Wiring

New registrations in `internal/auth/register.go`:
```go
func Register(container *dig.Container) error {
    return di.ProvideAll(container,
        NewArgon2idHasher,
        NewUserStore,
        NewJWTService,
        NewRefreshTokenStore,
        NewAuthService,
    )
}
```

In `internal/wireup.go`, add `auth.Register(container)` to the `Setup` function.

In `internal/api/http/v1controllers/register.go`, add `NewAuthController`.

In `internal/api/http/server/register.go` or in the `start` command's `PreRunE`, ensure `middleware.NewAuthMiddleware` is provided.

#### 4.2.11 CLI User Commands

New subcommand group under `sonalmod user`:
- `sonalmod user add --username <name> --password <pass>` — creates user, prints user ID
- `sonalmod user list` — prints table of users (id, username, createdAt)
- `sonalmod user change-password --username <name> --password <newpass>` — updates password hash

These commands use `internal/auth.UserStore` and `internal/auth.PasswordHasher` resolved from the DI container.

### 4.3 Sonal-UI (`apps/sonal-ui/`)

#### 4.3.1 Auth Store (new file: `src/lib/auth/auth-store.svelte.ts`)

Svelte 5 rune-based reactive state:
```typescript
class AuthStore {
    accessToken = $state<string | null>(null)
    refreshToken = $state<string | null>(null)
    user = $state<AuthUser | null>(null)
    isLoading = $state(true)
    isAuthenticated = $derived(this.user !== null && this.accessToken !== null)

    setAuth(data: { accessToken: string; refreshToken: string; user: AuthUser })
    clearAuth()
    tryRestoreSession(): Promise<void>  // reads refresh token from localStorage, attempts refresh
}

export const authStore = new AuthStore()
```

`AuthUser` type: `{ id: string, username: string }`.

#### 4.3.2 Auth API Client (new file: `src/lib/auth/auth-api.ts`)

```typescript
export async function loginApi(params: { baseUrl: string; username: string; password: string }): Promise<LoginResponse>
export async function refreshApi(params: { baseUrl: string; refreshToken: string }): Promise<RefreshResponse>
export async function meApi(params: { baseUrl: string; accessToken: string }): Promise<MeResponse>
```

Uses plain `fetch` (no openapi-fetch dependency since auth API is sonalmod-owned, not generated from the runtime spec).

#### 4.3.3 Authenticated Fetch Wrapper (new file: `src/lib/auth/auth-fetch.ts`)

A wrapper around `fetch` (or a helper for openapi-fetch) that:
1. Adds `Authorization: Bearer <accessToken>` from the auth store.
2. On 401 response, attempts one token refresh.
3. If refresh succeeds, retries the original request with the new token.
4. If refresh fails, clears auth state (triggers redirect to login).

The existing agent API client (`src/lib/agentapi/client.ts`) will be updated to use this wrapper or to accept an `accessToken` parameter.

#### 4.3.4 Login Page (new file: `src/pages/Login.svelte`)

Simple form with username and password fields. On submit:
1. Call `loginApi`.
2. On success, store tokens in auth store, `push('/chat')`.
3. On failure, show error message.

Styling: consistent with existing pages, clean and minimal.

#### 4.3.5 Route Guarding

In `App.svelte`, update the route map to use `svelte-spa-router`'s `wrap` function:
```typescript
import { wrap } from 'svelte-spa-router/wrap'

const routes = {
    '/login': Login,
    '/chat/:sessionId?': wrap({
        component: Chat,
        conditions: [() => authStore.isAuthenticated],
    }),
    '/about': wrap({
        component: About,
        conditions: [() => authStore.isAuthenticated],
    }),
    // ... redirect for '/'
}
```

When `conditions` fail, `conditionsFailed` event fires → `replace('/login')`.

#### 4.3.6 App Bootstrap Changes (`App.svelte` or root layout)

On mount, before rendering routes:
1. Call `authStore.tryRestoreSession()` (reads refresh token from localStorage, attempts refresh).
2. While loading, show a brief loading indicator.
3. If session restored → user lands on their requested route.
4. If not → redirected to login.

#### 4.3.7 API Call Updates

- Remove `userId` from `AgentRunRequest` body in agent API calls.
- Remove `userId` query parameter from `readSession` calls.
- Remove `VITE_AGENT_USER_ID` env var from `.env.example`, `vite-env.d.ts`, and all usage in `Chat.svelte`.
- Add `Authorization: Bearer <accessToken>` header to all agent API requests.
- Update Vite proxy config to forward `/api/v1/` (or add `/api/v1/auth` alongside the existing runtime proxy).

#### 4.3.8 Wireframe Updates (`ui-wireframe.md`)

Add login route, auth states, and update Chat behavior to reflect token-based auth instead of hardcoded userId.

---

## 5. Key Architectural Decisions

| # | Decision | Rationale |
|:--|:---------|:----------|
| 1 | **JWT access tokens + opaque refresh tokens** (not sessions/cookies) | Stateless verification for API calls; scales to future S2S. Refresh tokens provide persistence without long-lived JWTs. User explicitly requested this approach. |
| 2 | **Argon2id** for password hashing (not bcrypt) | Modern gold standard; memory-hard (GPU-resistant). `golang.org/x/crypto/argon2` available. Per conceptual research recommendation. |
| 3 | **CallerIdentity via context** (not request body) | Decouples runtime from auth mechanism. Runtime stays agnostic. Standard Go pattern for request-scoped identity. |
| 4 | **File-system user/token storage** (not database) | Matches existing data directory pattern (`agent.WithFileSystemStorage`). Sufficient for expected low user count. No new infrastructure dependency. |
| 5 | **HS256 JWT signing** (not RS256) | Single backend, no key distribution needed. Simpler. Adequate for current single-binary deployment. |
| 6 | **Auto-generated signing key** with config override | Zero-config local dev. Production can pin a key via `APP_AUTH_JWTSIGNINGKEY`. |
| 7 | **Refresh token rotation** | Old token deleted on use, new one issued. Limits impact of token theft. |
| 8 | **Manual route registration** for auth endpoints (not apigen) | Auth routes need selective middleware (login/refresh are public, /me is protected). Manual gives full control. Simple enough to not warrant code generation. |
| 9 | **Token storage**: access token in memory, refresh token in `localStorage` | Access token short-lived, not persisted. Refresh token persists across reloads. Standard SPA approach when not using httpOnly cookies. |
| 10 | **Transitional backward compatibility** in runtime handler | CallerIdentity preferred, `userId` body/query as fallback. Allows incremental migration. Removed in final cleanup task. |

---

## 6. Uncertainties

1. **SSE mid-stream token expiry**: If an access token expires during an active SSE stream, the connection was already authenticated at establishment time. Current design does not terminate active streams on token expiry. This seems acceptable but may need revisiting.
2. **Concurrent file writes**: The file-system stores use atomic write (temp + rename) for individual records. Under very high concurrency this could still have edge cases. Acceptable for the expected user count.
3. **Refresh token cleanup**: Expired refresh tokens are not proactively cleaned up (only deleted on use or user deletion). A background cleanup task may be needed later.
4. **CSRF**: Since we use Bearer tokens (not cookies), CSRF is not a concern for API endpoints. The login form itself could theoretically be CSRF-targeted, but the impact is minimal (attacker would need valid credentials).
5. **Vite proxy scope**: Currently only `/api/v1/runtime` is proxied. Needs expansion to also proxy `/api/v1/auth/` for local development.

---

## 7. Related Files

### Runtime Module (`runtime/`)

| File | Status | Purpose |
|:-----|:-------|:--------|
| `runtime/httpapi/identity.go` | **New** | CallerIdentity interface + context helpers |
| `runtime/httpapi/identity_test.go` | **New** | Tests for context round-trip |
| `runtime/internal/agentapi/server.go` | **Modified** | Read CallerIdentity from context |
| `runtime/internal/agentapi/server_test.go` | **Modified** | Tests for CallerIdentity extraction |
| `runtime/internal/agentapi/openapi.yaml` | **Modified** (Task 5.1) | Remove userId from spec |
| `runtime/internal/agentapi/api.gen.go` | **Regenerated** (Task 5.1) | Regenerated from spec |
| `runtime/httpapi/handler.go` | Unchanged | No changes needed |

### Sonalmod Backend (`apps/sonalmod/`)

| File | Status | Purpose |
|:-----|:-------|:--------|
| `internal/config/default.yaml` | **Modified** | Add `auth.*` config keys |
| `internal/config/provide.go` | **Modified** | Provide `config.auth.*` DI bindings |
| `internal/auth/password.go` | **New** | Argon2id password hashing |
| `internal/auth/password_test.go` | **New** | Password hashing tests |
| `internal/auth/user_store.go` | **New** | File-system user store |
| `internal/auth/user_store_test.go` | **New** | User store tests |
| `internal/auth/jwt.go` | **New** | JWT generation/validation + key management |
| `internal/auth/jwt_test.go` | **New** | JWT tests |
| `internal/auth/refresh_store.go` | **New** | Refresh token file storage |
| `internal/auth/refresh_store_test.go` | **New** | Refresh token tests |
| `internal/auth/service.go` | **New** | Auth service (login, refresh, me) |
| `internal/auth/service_test.go` | **New** | Auth service tests |
| `internal/auth/register.go` | **New** | DI registration for auth package |
| `internal/api/http/middleware/auth.go` | **New** | JWT auth middleware |
| `internal/api/http/middleware/auth_test.go` | **New** | Auth middleware tests |
| `internal/api/http/v1controllers/auth.go` | **New** | Auth HTTP handlers |
| `internal/api/http/v1controllers/auth_test.go` | **New** | Handler tests |
| `internal/api/http/v1controllers/register.go` | **Modified** | Register AuthController |
| `internal/api/http/register.go` | **Modified** | Mount auth routes + auth middleware on runtime |
| `internal/api/http/server/register.go` | **Modified** | Provide AuthMiddleware |
| `internal/wireup.go` | **Modified** | Add auth.Register |
| `cli.go` | **Modified** | Add `user` command group |
| `main.go` | **Modified** | Wire `user` subcommands |

### Sonal-UI (`apps/sonal-ui/`)

| File | Status | Purpose |
|:-----|:-------|:--------|
| `src/lib/auth/auth-store.svelte.ts` | **New** | Reactive auth state (Svelte 5 runes) |
| `src/lib/auth/auth-store.svelte.test.ts` | **New** | Auth store tests |
| `src/lib/auth/auth-api.ts` | **New** | Auth HTTP client (login, refresh, me) |
| `src/lib/auth/auth-api.test.ts` | **New** | Auth API client tests |
| `src/lib/auth/auth-fetch.ts` | **New** | Authenticated fetch wrapper with auto-refresh |
| `src/lib/auth/auth-fetch.test.ts` | **New** | Auth fetch tests |
| `src/pages/Login.svelte` | **New** | Login page |
| `src/pages/Login.test.ts` | **New** | Login page tests |
| `src/App.svelte` | **Modified** | Route guards, auth bootstrap |
| `src/pages/Chat.svelte` | **Modified** | Remove userId, use auth tokens |
| `src/lib/agentapi/client.ts` | **Modified** | Accept/add Authorization header |
| `src/vite-env.d.ts` | **Modified** | Remove VITE_AGENT_USER_ID |
| `.env.example` | **Modified** | Remove VITE_AGENT_USER_ID |
| `vite.config.ts` | **Modified** | Expand proxy to /api/v1/ |
| `ui-wireframe.md` | **Modified** | Add login, auth states |

---

## 8. Task List

> **TDD approach**: each task writes failing tests first, then implements to make them pass. Each task must leave the codebase in a buildable state and pass the module's lint + test protocol.

---

### Task 1.1: Define CallerIdentity in runtime public contract

**Module:** `runtime/`

- Create `runtime/httpapi/identity.go`:
  - `CallerIdentity` interface with `UserID() string`
  - Private context key type
  - `ContextWithCallerIdentity(ctx, CallerIdentity) context.Context`
  - `CallerIdentityFromContext(ctx) CallerIdentity` (returns nil if absent)
- Write tests in `runtime/httpapi/identity_test.go`:
  - Round-trip: set identity in context, read it back
  - Absent: read from empty context returns nil
- Run: `make lint && make test` from `runtime/`
  - Verify all pass
- Write summary to `doc/implementation/initial-authentication/summary-task-1.1.md`
- All checks from completion protocol must be passed

---

### Task 1.2: Update runtime handler to read CallerIdentity from context

**Module:** `runtime/`

- Modify `runtime/internal/agentapi/server.go`:
  - In `parseAgentRunRequest`: check `httpapi.CallerIdentityFromContext(r.Context())`; if non-nil, use its `UserID()`. Otherwise fall back to `req.UserId` from JSON body. If both empty → 400.
  - In `ReadSession`: same pattern — CallerIdentity from context preferred over `params.UserId` query.
- Import `runtime/httpapi` in `server.go` (this is internal importing its own public package, which is valid).
- Write failing tests in `server_test.go`:
  - When CallerIdentity is in context, userId from body is ignored
  - When CallerIdentity is absent, userId from body is used (backward compat)
  - When both CallerIdentity and body userId are absent → 400
  - ReadSession: CallerIdentity in context used instead of query userId
- Run affected tests: `go test -v ./internal/agentapi/ --run <test pattern>` from `runtime/`
  - Verify failure is expectation (not compilation errors)
- Implement the logic
- Run affected tests — verify all pass
- Run: `make lint && make test` from `runtime/`
- Write summary to `doc/implementation/initial-authentication/summary-task-1.2.md`
- All checks from completion protocol must be passed

---

### Task 2.1: Add auth configuration to sonalmod

**Module:** `apps/sonalmod/`

- Add to `internal/config/default.yaml`:
  ```yaml
  auth:
    jwtSigningKey: ""
    accessTokenTTL: 30m
    refreshTokenTTL: 720h
  ```
- Add to `internal/config/provide.go`:
  ```go
  provideConfigValue(cfg, "auth.jwtSigningKey").asString(),
  provideConfigValue(cfg, "auth.accessTokenTTL").asDuration(),
  provideConfigValue(cfg, "auth.refreshTokenTTL").asDuration(),
  ```
- Run: `make lint && make test` from `apps/sonalmod/`
  - Verify all pass (existing tests must not break)
- Write summary to `doc/implementation/initial-authentication/summary-task-2.1.md`
- All checks from completion protocol must be passed

---

### Task 2.2: Implement password hashing with Argon2id

**Module:** `apps/sonalmod/`

- Add `golang.org/x/crypto` dependency (if not already present): `go get golang.org/x/crypto`
- Create `internal/auth/password.go`:
  - `Argon2idHasher` struct with configurable parameters (memory, time, threads, saltLen, keyLen)
  - `NewArgon2idHasher() *Argon2idHasher` with recommended defaults
  - `Hash(password string) (string, error)` — generates random salt, hashes, returns encoded string
  - `Verify(password, encodedHash string) (bool, error)` — parses encoded hash, re-derives, compares
- Write failing tests in `internal/auth/password_test.go`:
  - Hash produces non-empty string
  - Verify returns true for correct password
  - Verify returns false for incorrect password
  - Different calls produce different hashes (salting)
  - Verify handles malformed hash strings gracefully (error, not panic)
- Run affected tests: `go test -v ./internal/auth/ --run TestArgon2idHasher`
  - Verify failure is expectation
- Implement logic
- Run affected tests — verify all pass
- Run: `make lint && make test` from `apps/sonalmod/`
- Write summary to `doc/implementation/initial-authentication/summary-task-2.2.md`
- All checks from completion protocol must be passed

---

### Task 2.3: Implement file-system user store

**Module:** `apps/sonalmod/`

- Create `internal/auth/user_store.go`:
  - `User` struct (ID, Username, PasswordHash, CreatedAt, UpdatedAt)
  - `CreateUserParams` struct (Username, PasswordHash)
  - `UserStore` struct with `UserStoreDeps` (DataDir, IDGen, Logger)
  - `NewUserStore(deps UserStoreDeps) *UserStore`
  - Methods: `Create`, `GetByUsername`, `GetByID`, `List`, `UpdatePassword`
  - Directory auto-creation on first write
  - Atomic writes (temp file + rename)
  - Sentinel errors: `ErrUserNotFound`, `ErrUsernameExists`
- Write failing tests in `internal/auth/user_store_test.go`:
  - Create user + get by ID returns same user
  - Create user + get by username returns same user
  - Create duplicate username returns ErrUsernameExists
  - Get non-existent user returns ErrUserNotFound
  - List returns all created users
  - UpdatePassword changes the stored hash
  - UpdatePassword on non-existent user returns ErrUserNotFound
  - Use `t.TempDir()` for each test's data directory
  - Use faker for variable test data
- Run affected tests: `go test -v ./internal/auth/ --run TestUserStore`
  - Verify failure is expectation
- Implement logic
- Run affected tests — verify all pass
- Run: `make lint && make test` from `apps/sonalmod/`
- Write summary to `doc/implementation/initial-authentication/summary-task-2.3.md`
- All checks from completion protocol must be passed

---

### Task 2.4: Implement JWT service and refresh token store

**Module:** `apps/sonalmod/`

- Add `github.com/golang-jwt/jwt/v5` dependency: `go get github.com/golang-jwt/jwt/v5`
- Create `internal/auth/jwt.go`:
  - `JWTClaims` struct embedding `jwt.RegisteredClaims` + `Username string`
  - `JWTService` struct with `JWTServiceDeps`
  - `NewJWTService(deps JWTServiceDeps) (*JWTService, error)` — resolves signing key (config or auto-generate/persist)
  - `GenerateAccessToken(userID, username string) (string, error)`
  - `ValidateAccessToken(tokenStr string) (*JWTClaims, error)`
- Create `internal/auth/refresh_store.go`:
  - `RefreshTokenStore` struct
  - `NewRefreshTokenStore(deps RefreshTokenStoreDeps) *RefreshTokenStore`
  - `Create(ctx, userID, ttl) (opaqueToken, error)` — generates random token, stores SHA-256 hash as filename
  - `Validate(ctx, opaqueToken) (userID, error)` — hashes token, reads file, checks expiry
  - `Delete(ctx, opaqueToken) error` — removes file
  - `DeleteAllForUser(ctx, userID) error` — scans and removes all tokens for user
  - Sentinel error: `ErrInvalidRefreshToken`
- Write failing tests in `internal/auth/jwt_test.go`:
  - Generate token + validate returns correct claims
  - Validate expired token returns error
  - Validate tampered token returns error
  - Validate token with wrong key returns error
  - Auto-generated key persists to file and is reused
- Write failing tests in `internal/auth/refresh_store_test.go`:
  - Create + validate returns correct userID
  - Validate non-existent token returns error
  - Validate expired token returns error
  - Delete removes token (subsequent validate fails)
  - DeleteAllForUser removes all tokens for that user
  - Use `t.TempDir()` and faker
- Run affected tests: `go test -v ./internal/auth/ --run "TestJWTService|TestRefreshTokenStore"`
  - Verify failure is expectation
- Implement logic
- Run affected tests — verify all pass
- Run: `make lint && make test` from `apps/sonalmod/`
- Write summary to `doc/implementation/initial-authentication/summary-task-2.4.md`
- All checks from completion protocol must be passed

---

### Task 2.5: Implement auth service

**Module:** `apps/sonalmod/`

- Create `internal/auth/service.go`:
  - Define consumer interfaces for dependencies: `userStore`, `jwtService`, `refreshTokenStore`, `passwordHasher`
  - `AuthService` struct with `AuthServiceDeps`
  - `NewAuthService(deps AuthServiceDeps) *AuthService`
  - `Login(ctx, username, password) (*LoginResult, error)`:
    - Get user by username → verify password → generate tokens → return
    - Wrong credentials → `ErrInvalidCredentials`
  - `Refresh(ctx, refreshToken) (*RefreshResult, error)`:
    - Validate refresh token → get user → delete old token → generate new tokens → return
    - Invalid token → `ErrInvalidRefreshToken`
  - `CurrentUser(ctx, userID) (*UserInfo, error)`:
    - Get user by ID → return info (no password)
  - Sentinel errors: `ErrInvalidCredentials`, `ErrInvalidRefreshToken`
- Write failing tests in `internal/auth/service_test.go`:
  - Login happy path: valid credentials → returns tokens + user info
  - Login with wrong password → ErrInvalidCredentials
  - Login with non-existent username → ErrInvalidCredentials
  - Refresh happy path: valid token → returns new tokens + user info
  - Refresh with invalid token → ErrInvalidRefreshToken
  - CurrentUser happy path: returns user info
  - CurrentUser with non-existent ID → ErrUserNotFound
  - **Mock dependencies** using mockery-generated mocks or manual mocks for consumer interfaces
  - Use faker for test data
- Run affected tests: `go test -v ./internal/auth/ --run TestAuthService`
  - Verify failure is expectation
- Implement logic
- Run affected tests — verify all pass
- Run: `make lint && make test` from `apps/sonalmod/`
- Write summary to `doc/implementation/initial-authentication/summary-task-2.5.md`
- All checks from completion protocol must be passed

---

### Task 2.6: Implement auth middleware

**Module:** `apps/sonalmod/`

- Create `internal/api/http/middleware/auth.go`:
  - Define consumer interface for JWT validation: `jwtValidator` with `ValidateAccessToken(string) (*auth.JWTClaims, error)`
  - `AuthMiddlewareDeps` struct (jwtValidator, Logger)
  - `NewAuthMiddleware(deps AuthMiddlewareDeps) AuthMiddleware`
  - `AuthMiddleware` type: `func(http.Handler) http.Handler`
  - Logic:
    1. Read `Authorization` header
    2. Parse `Bearer <token>` format
    3. Validate token via jwtValidator
    4. Create CallerIdentity, set in context via `httpapi.ContextWithCallerIdentity`
    5. Call next handler
    6. On any failure → JSON 401 response
- Write failing tests in `internal/api/http/middleware/auth_test.go`:
  - Valid token → next handler called, CallerIdentity in context
  - Missing Authorization header → 401, next not called
  - Malformed Authorization header (not "Bearer ...") → 401
  - Invalid/expired token → 401
  - Use `httptest.NewRecorder` and `httptest.NewRequest`
  - Mock jwtValidator
  - Verify CallerIdentity.UserID() in the next handler
- Run affected tests: `go test -v ./internal/api/http/middleware/ --run TestAuthMiddleware`
  - Verify failure is expectation
- Implement logic
- Run affected tests — verify all pass
- Run: `make lint && make test` from `apps/sonalmod/`
- Write summary to `doc/implementation/initial-authentication/summary-task-2.6.md`
- All checks from completion protocol must be passed

---

### Task 2.7: Implement auth API handlers, DI wiring, and route registration

**Module:** `apps/sonalmod/`

- Create `internal/api/http/v1controllers/auth.go`:
  - Define consumer interface `authService` (Login, Refresh, CurrentUser)
  - `AuthController` struct + `NewAuthController`
  - `Login() http.Handler`: parse JSON body → call authService.Login → return JSON response or 401
  - `Refresh() http.Handler`: parse JSON body → call authService.Refresh → return JSON response or 401
  - `Me() http.Handler`: read CallerIdentity from context → call authService.CurrentUser → return JSON or 401
- Create `internal/auth/register.go`:
  - `Register(container) error` — provides all auth components (hasher, user store, JWT service, refresh token store, auth service)
- Modify `internal/api/http/v1controllers/register.go`:
  - Add `NewAuthController` to DI registration
- Modify `internal/api/http/server/register.go`:
  - Add `middleware.NewAuthMiddleware` to DI registration
- Modify `internal/wireup.go`:
  - Add `auth.Register(container)` call
- Modify `internal/api/http/register.go`:
  - Add `AuthController` and `AuthMiddleware` to `V1RoutesDeps`
  - Register auth routes (login, refresh public; me protected)
  - Wrap runtime handler with AuthMiddleware
- Write failing tests in `internal/api/http/v1controllers/auth_test.go`:
  - Login: valid credentials → 200 with tokens
  - Login: invalid credentials → 401
  - Login: missing body fields → 400
  - Refresh: valid token → 200 with new tokens
  - Refresh: invalid token → 401
  - Me: with CallerIdentity in context → 200 with user info
  - Me: without CallerIdentity → 401
  - Mock authService
  - Use `httptest`
- Run affected tests: `go test -v ./internal/api/http/v1controllers/ --run TestAuthController`
  - Verify failure is expectation
- Implement logic and wiring
- Run affected tests — verify all pass
- Run: `make lint && make test` from `apps/sonalmod/`
- Write summary to `doc/implementation/initial-authentication/summary-task-2.7.md`
- All checks from completion protocol must be passed

---

### Task 3.1: Implement CLI user management commands

**Module:** `apps/sonalmod/`

- Modify `main.go` / `cli.go`:
  - Add `newUserCmd(container)` returning `*cobra.Command` with Use: `"user"`
  - Add subcommands: `newUserAddCmd`, `newUserListCmd`, `newUserChangePasswordCmd`
  - Register `user` command in `setupCommands()`
- `sonalmod user add --username <name> --password <pass>`:
  - Hash password → create user in store → print user ID and success message
- `sonalmod user list`:
  - List all users → print table (ID, Username, CreatedAt) — no passwords
- `sonalmod user change-password --username <name> --password <newpass>`:
  - Find user by username → hash new password → update store → print success
- Note: CLI commands use DI to resolve UserStore and PasswordHasher. Similar pattern to existing `cli` command.
- Write failing tests (if feasible given the `// coverage-ignore` pattern for CLI; at minimum, test the underlying functions):
  - User add creates a user that can be retrieved
  - User list returns the created user
  - Change password updates the user's hash
- Run: `make lint && make test` from `apps/sonalmod/`
- Write summary to `doc/implementation/initial-authentication/summary-task-3.1.md`
- All checks from completion protocol must be passed

---

### Task 4.1: Implement sonal-ui auth foundation (store + API client + login page)

**Module:** `apps/sonal-ui/`

- Create `src/lib/auth/auth-api.ts`:
  - `loginApi`, `refreshApi`, `meApi` functions using `fetch`
  - Types: `LoginRequest`, `LoginResponse`, `RefreshRequest`, `RefreshResponse`, `MeResponse`, `AuthUser`
- Create `src/lib/auth/auth-store.svelte.ts`:
  - `AuthStore` class with Svelte 5 runes ($state, $derived)
  - `setAuth`, `clearAuth`, `tryRestoreSession` methods
  - Refresh token persisted in `localStorage`
  - Singleton `authStore` export
- Create `src/pages/Login.svelte`:
  - Form: username input, password input, submit button
  - On submit: call `loginApi` → on success `authStore.setAuth` + `push('/chat')` → on failure show error
  - Accessible: labeled inputs, error `role="alert"`
- Write tests:
  - `src/lib/auth/auth-api.test.ts`: mock fetch, test request shapes and response parsing
  - `src/lib/auth/auth-store.svelte.test.ts`: test state transitions, localStorage interaction
  - `src/pages/Login.test.ts`: render login form, submit, mock API, verify redirect or error
  - Use `@faker-js/faker` for test data, `msw` for API mocking where appropriate
- Run: `make lint && make test` from `apps/sonal-ui/`
- Write summary to `doc/implementation/initial-authentication/summary-task-4.1.md`
- All checks from completion protocol must be passed

---

### Task 4.2: Implement route guarding and token management

**Module:** `apps/sonal-ui/`

- Create `src/lib/auth/auth-fetch.ts`:
  - `createAuthFetch(authStore)` → returns a fetch wrapper that:
    - Adds `Authorization: Bearer <accessToken>` header
    - On 401: attempts `refreshApi` → retries original request
    - On refresh failure: `authStore.clearAuth()`
- Modify `src/App.svelte`:
  - Add `/login` route pointing to `Login`
  - Wrap protected routes (`/chat`, `/about`) with `svelte-spa-router`'s `wrap` + auth condition
  - Handle `conditionsFailed` event → `replace('/login')`
  - On mount: call `authStore.tryRestoreSession()`, show loading state while pending
- Write tests:
  - `src/lib/auth/auth-fetch.test.ts`: test header injection, 401 retry, refresh failure
  - Update `src/App.svelte` tests (if they exist) for route guarding behavior
- Run: `make lint && make test` from `apps/sonal-ui/`
- Write summary to `doc/implementation/initial-authentication/summary-task-4.2.md`
- All checks from completion protocol must be passed

---

### Task 4.3: Update API calls, remove VITE_AGENT_USER_ID, update wireframe

**Module:** `apps/sonal-ui/`

- Modify `src/lib/agentapi/client.ts`:
  - Add optional `accessToken` parameter to all API functions (or accept a custom fetch/headers)
  - When provided, add `Authorization: Bearer <accessToken>` header
  - Remove `userId` from `AgentRunRequest` body in `startAgentRun` and `continueAgentRun`
  - Remove `userId` query param from `readSession`
- Modify `src/pages/Chat.svelte`:
  - Remove `agentUserId` const and all `VITE_AGENT_USER_ID` references
  - Import `authStore` and pass `accessToken` to API calls
  - Use the auth-fetch wrapper or pass token directly
- Modify `src/vite-env.d.ts`: remove `VITE_AGENT_USER_ID`
- Modify `.env.example`: remove `VITE_AGENT_USER_ID` line
- Modify `.env.test`: update if `VITE_AGENT_USER_ID` is referenced
- Modify `vite.config.ts`:
  - Add `/api/v1/auth` to the dev proxy (or change proxy prefix to `/api/v1/` to cover both runtime and auth)
- Update `ui-wireframe.md`:
  - Add login route and auth states
  - Update Chat behavior to reflect token-based auth
  - Remove references to VITE_AGENT_USER_ID
- Update existing tests in `Chat.test.ts` and API client tests to reflect the removal of userId
- Run: `make lint && make test` from `apps/sonal-ui/`
- Write summary to `doc/implementation/initial-authentication/summary-task-4.3.md`
- All checks from completion protocol must be passed

---

### Task 5.1: Remove userId from runtime API spec and finalize

**Module:** `runtime/` then `apps/sonal-ui/`

- Modify `runtime/internal/agentapi/openapi.yaml`:
  - Remove `userId` property from `AgentRunRequest` schema
  - Remove `userId` from `required` array in `AgentRunRequest`
  - Remove `userId` query parameter from `GET /sessions/{sessionId}`
- Regenerate: `go generate ./internal/agentapi` from `runtime/`
- Modify `runtime/internal/agentapi/server.go`:
  - Remove fallback to `req.UserId` from body — now CallerIdentity from context is required
  - Remove fallback to `params.UserId` from query in ReadSession
  - If CallerIdentity is absent → 401 (not 400, since it implies missing auth)
- Update tests in `runtime/internal/agentapi/server_test.go`:
  - Remove tests for body/query fallback
  - Add test: no CallerIdentity in context → 401
- Run: `make lint && make test` from `runtime/`
- Regenerate sonal-ui types: `make generate-api` from `apps/sonal-ui/`
- Update sonal-ui API client types if generated types changed
- Run: `make lint && make test` from `apps/sonal-ui/`
- Write summary to `doc/implementation/initial-authentication/summary-task-5.1.md`
- All checks from completion protocol must be passed

---

### Task 5.2: Compress implementation summaries

- Follow [compress-implementation-summaries.md](/.context/compress-implementation-summaries.md) to compress the implementation summaries.
