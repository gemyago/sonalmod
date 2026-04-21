# Distilled: `openapi-typescript` + `openapi-fetch` only

Stack assumption: **Vite** SPA (e.g. Svelte), **Vitest** for tests. No Hey API, Orval, or OpenAPI Generator—just types from the spec plus a small typed `fetch` wrapper.

---

## What each package does

| Package | When | Role |
| --- | --- | --- |
| **`openapi-typescript`** | Dev / CI | CLI: OpenAPI → **`paths`** (and optionally `components`) in a `.ts` / `.d.ts` file. **No runtime** in the output. |
| **`openapi-fetch`** | App runtime | **`createClient<paths>()`** — typed `GET`/`POST`/… that call `fetch` and return **`{ data, error, response }`**. |

You need **both**: types alone don’t send HTTP; `openapi-fetch` needs the generated **`paths`** type.

---

## Dependencies

- **Runtime:** `openapi-fetch`
- **Dev:** `openapi-typescript`, `typescript`
- **Node:** 20.x+ recommended for `openapi-typescript`

---

## Codegen (`openapi-typescript`)

**Single spec (local or URL):**

```bash
npx openapi-typescript ./openapi.yaml -o ./src/lib/api/v1.d.ts
# or
npx openapi-typescript https://example.com/openapi.yaml -o ./src/lib/api/v1.d.ts
```

**Multiple specs (v7):** use root **`redocly.yaml`** with **`apis`**, each with **`x-openapi-ts.output`** — glob-based multi-file flow is deprecated. Run `npx openapi-typescript` (often with no args when config drives outputs) or **`--redocly ./path/to/redocly.yaml`**.

**Private spec URL at codegen time:** Redocly **`resolve.http.headers`** (e.g. **`envVariable`**) — not `VITE_*` (that’s for the browser).

**CI drift check:** same inputs/outputs/flags as generate, plus **`--check`** so committed types match the spec.

**Optional:** **`--read-write-markers`** if you rely on **readOnly** / **writeOnly** and want stricter request/response typing for openapi-fetch.

**Scripts (typical):**

- `generate:api` — runs `openapi-typescript … -o …`
- `check:api` — same command + `--check`
- `test:ts` — `tsc --noEmit` (recommended alongside `--check`)

Docs: [Introduction](https://openapi-ts.dev/introduction), [CLI](https://openapi-ts.dev/cli).

---

## Runtime (`openapi-fetch`)

1. Import **`paths`** from the generated file.
2. Create one shared client (e.g. `src/lib/api/client.ts`):

   - **`createClient<paths>({ baseUrl, …fetchOptions })`**
   - **`baseUrl`**: often **`import.meta.env.VITE_API_BASE`** (string; include trailing slash if your paths are relative to host).
   - Pass **`headers`**, **`signal`**, custom **`fetch`**, etc. as needed (same as `fetch` options).
3. Call e.g. **`client.GET('/path', { params: { … } })`** — returns **`{ data, error, response }`**; **`response`** is the platform **`Response`** (use for status, headers, **`body`** streams).

**Auth / shared behavior:** **`client.use(middleware)`** with **`onRequest` / `onResponse` / `onError`** (see [Middleware & Auth](https://openapi-ts.dev/openapi-fetch/middleware-auth)).

Docs: [openapi-fetch](https://openapi-ts.dev/openapi-fetch/), [API](https://openapi-ts.dev/openapi-fetch/api).

---

## Vite: env and dev proxy

- Only **`VITE_*`** (default prefix) is exposed on **`import.meta.env`** in client code. Values are **bundled** — **never** put secrets there.
- **`server.proxy`** in `vite.config` is **dev-only**; use it to forward e.g. `/api` → backend and avoid CORS locally. Production needs its own gateway/reverse proxy.
- Augment **`ImportMetaEnv`** in `vite-env.d.ts` for your `VITE_API_BASE` (or similar).

Docs: [Env and mode](https://vite.dev/guide/env-and-mode), [server.proxy](https://vite.dev/config/server-options).

---

## Vitest: mocking HTTP

- **Preferred:** **MSW** — `setupServer` from **`msw/node`**, **`http`**, **`HttpResponse`**, with **`listen` / `resetHandlers` / `close`** in lifecycle hooks ([Vitest: Mocking requests](https://vitest.dev/guide/mocking/requests)).
- Match the **real URL** your client calls (**`baseUrl` + path**). Using an **absolute** `baseUrl` in tests keeps handlers straightforward.
- **`vi.stubEnv('VITE_API_BASE', …)`** when modules read env at import time; clean up with Vitest’s env unstub helpers.
- Alternatives: **`vi.stubGlobal('fetch', …)`** or **`createClient({ fetch: mockFetch })`** for narrow unit tests.

---

## SSE / streaming

Thin clients don’t replace **`EventSource`** or stream parsers. For streaming responses, use **`response`** from openapi-fetch and **`response.body`** / platform APIs, or bypass the client for those operations. See prior research on SSE if needed.

---

## Minimal end-to-end checklist

1. Add **`openapi-fetch`** + dev **`openapi-typescript`**.
2. **`generate:api`** and commit generated **`paths`** file(s).
3. Export **`createClient<paths>({ baseUrl: import.meta.env.VITE_API_BASE, … })`** from one module; use **`client.use`** for auth if needed.
4. Set **`VITE_API_BASE`** in `.env.*`; **no secrets** in `VITE_*`.
5. Optional: **`server.proxy`** for local dev.
6. Tests: MSW + **`vi.stubEnv`**; assert **`data` / `error`** per status.
7. CI: **`openapi-typescript … --check`** (same flags as generate) + **`tsc --noEmit`**; private spec fetch via Redocly + secrets.

---

## Source of this distill

Derived from [result.md](./result.md) and [task-T1-openapi-fetch-integration.md](./notes/task-T1-openapi-fetch-integration.md) in this folder (research dated 2026-03-30). Re-verify flags and URLs against [openapi-ts.dev](https://openapi-ts.dev/) before pinning versions long-term.
