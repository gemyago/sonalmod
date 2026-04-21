# Sonal UI architecture (brief)

Browser SPA for Sonalmod under `apps/sonal-ui`: static **`dist/`** from Vite, no SSR in this iteration. Final goal is to have the UI embeddable in a backend app and deployed as a single unit.

## Stack

| Area | Choice |
| --- | --- |
| UI | **Svelte 5** + **TypeScript** |
| Build | **Vite** (`vite build` → `dist/`) |
| Routing | **`svelte-spa-router` v5** — **hash** URLs (`#/`, `#/about`) so a plain static host works without server rewrites |
| Unit / component tests | **Vitest** + **`@testing-library/svelte`** + **`svelteTesting()`** (Vite plugin), **jsdom**, optional **jest-dom** matchers |
| Lint | **ESLint** (TS + Svelte) + **`svelte-check`** / **tsc** via `npm run check` |

## Layout (conceptual)

- **`src/main.ts`** — bootstraps the app.
- **`src/App.svelte`** — shell + **`Router`** and route map.
- **`src/pages/`** — route-level views (e.g. Home, About).
- **`src/components/`** — shared UI (e.g. nav with `use:link`).
- **`src/lib/`** — shared helpers / constants (grow as needed).
- **`public/`** — static assets copied into `dist/` as-is.

## Configuration and env

- **Client env:** only **`VITE_*`** variables (see **`.env.example`**). Example: **`VITE_APP_TITLE`** for shell copy; **`VITE_AGENT_API_BASE_URL`** — full origin or a same-origin path such as **`/api/v1/runtime/`** when using the Vite dev **`server.proxy`** (see **`vite.config.ts`**); no secrets in repo.
- **Vitest:** committed **`.env.test`** where needed for deterministic test env.

## Repository integration

- **`apps/sonal-ui`** is its own npm package: **`package-lock.json`**, **`npm ci`** for installs.
- **`apps/sonal-ui/Makefile`:** `lint` → `npm run lint`, `test` → `npm run test:run`.
- **Root `Makefile`** runs **`$(MAKE) -C apps/sonal-ui lint`** and **`test`**. JS coverage is not merged into Go’s merged HTML report; UI failures still fail the root **`make test`**.

## API Integration

The Svelte UI consumes various backend APIs (including long running SSE streams). The runtime provides OpenAPI spec, see stream events are typed separately in the spec. API types are generated with `openapi-typescript`

## Decisions (why not X)

- **Plain Vite + Svelte, not SvelteKit:** smaller surface; deliberate third-party router; static export only.
- **Hash routing:** fits static hosting without rewrite rules.
- **Isolated lockfile in `apps/sonal-ui`:** matches monorepo layout; root stays Go-centric.

## Technical notes (Vite + Svelte template)

One-line reminders (80–100 characters); see upstream Vite/Svelte docs for detail.

- SvelteKit vs Vite: we use a minimal Vite SPA + hash router; static `dist/` only, no SSR in this app.
- Triple-slash `global.d.ts` avoids listing `compilerOptions.types`, hiding other workspace types.
- `allowJs` stays on: `.svelte` scripts can still be JS even if plain `.js` files were forbidden.
- HMR skips in-component state by default; use stores for values you need across hot reloads.
- Folders echo SvelteKit-like layout so a future SvelteKit migration stays less painful.
