# Plan: Initial Svelte boilerplate (`apps/sonal-ui`)

## 1. Introduction / overview

**Problem:** `apps/sonal-ui` is reserved for the Sonalmod browser SPA but has no Vite/Svelte application yet (`AGENTS.md` still describes a future `package.json` / Vite project).

**Goal:** Add a **minimal, maintainable** Svelte 5 SPA boilerplate under `apps/sonal-ui`: standard Vite build (`dist/`), client-side routing, component tests, reproducible npm installs, and repo-level **lint/test** hooks so the root **Coding Task Completion Protocol** (`make lint`, `make test`) stays satisfied after implementation.

**Non-goals (this iteration):** Embedding static assets in Go, SSR, production deployment, Playwright E2E, root **npm workspaces** (optional later per [svelte-boilerplate-research.md](./svelte-boilerplate-research.md)).

---

## 2. Business logic

There is no product business logic yet. The boilerplate should only:

- Render a small set of **dummy routes** (e.g. home + one extra page) with **navigation** so routing is proven end-to-end.
- Expose **`VITE_*`** env usage via a trivial example (e.g. app title or placeholder API base URL) to establish the pattern for future API integration.
- Prove **tests** with at least one **meaningful** component-level test (e.g. rendered copy or navigation) using Vitest + Testing Library—not only empty scaffold files.

---

## 3. High-level architecture

| Layer | Choice |
| --- | --- |
| Build | **Vite** + **Svelte 5** + **TypeScript** (`npm create vite@latest` → `svelte-ts` or equivalent official template) |
| Routing | **`svelte-spa-router` v5** (Svelte 5 line; hash-based routing fits static `dist/` without server rewrites—see research) |
| Unit / component tests | **Vitest** (Vite-native) + **`@testing-library/svelte`** + **`svelteTesting()`** from `@testing-library/svelte/vite`, **`jsdom`** or **`happy-dom`** |
| Package root | **`apps/sonal-ui`** as its own npm package: committed **`package-lock.json`**, **`npm ci`**-friendly |
| Repo integration | **`Makefile`** in `apps/sonal-ui` (or documented npm scripts) invoked from **root** `Makefile` **`lint`** / **`test`** targets |

Alternative **SvelteKit** is documented in research as first-party routing; this plan intentionally chooses **plain Vite + Svelte** to minimize framework surface and match the “plan-aligned default” in the research doc for an isolated SPA shell.

---

## 4. Detailed architecture

### 4.1 Repository layout (after implementation)

- **`apps/sonal-ui/package.json`** — scripts: `dev`, `build`, `preview`, `test` (Vitest), `test:run` or CI mode (`vitest run`), `lint` (see below).
- **`apps/sonal-ui/package-lock.json`** — committed for reproducible installs.
- **`apps/sonal-ui/index.html`**, **`vite.config.ts`**, **`svelte.config.js`**, **`tsconfig*.json`** — standard Vite + Svelte TS setup.
- **`apps/sonal-ui/src/main.ts`** — boots the app.
- **`apps/sonal-ui/src/App.svelte`** — wraps **`svelte-spa-router`** (`Router` + route map or equivalent per library docs).
- **`apps/sonal-ui/src/pages/`** (or `routes/`) — **at least two** Svelte components for distinct URLs (e.g. `Home.svelte`, `About.svelte`).
- **`apps/sonal-ui/src/components/`** — shared UI (e.g. a simple nav bar linking routes).
- **`apps/sonal-ui/src/lib/`** — shared helpers / constants (optional for boilerplate).
- **`apps/sonal-ui/public/`** — static assets; **`dist/`** gitignored, produced by `vite build`.
- **`apps/sonal-ui/.env.example`** — documents safe **`VITE_*`** keys only (no secrets).
- **`apps/sonal-ui/vitest.config.ts`** (or `test` block in `vite.config.ts`) — `environment: 'jsdom'` (or `happy-dom`), `svelteTesting()` plugin wired.
- **`apps/sonal-ui/Makefile`** (recommended) — `lint` and `test` delegating to `npm run lint` / `npm run test:run` (or equivalent) so root `make` can call `$(MAKE) -C apps/sonal-ui`.

### 4.2 Linting

Official `create-vite` templates may not include ESLint. The implementation should add **whatever minimal lint stack** is needed so **`make lint` from `apps/sonal-ui` exits 0**—typically **`eslint`** + **`@typescript-eslint`** + **`eslint-plugin-svelte`** and/or **`svelte-check`** for type-aware Svelte validation. Exact tooling is left to the implementer; the acceptance criterion is a single **`npm run lint`** (or Makefile target) that CI and root Makefile can call.

### 4.3 Root `Makefile`

- Extend **`lint`** to run **`$(MAKE) -C apps/sonal-ui lint`** (after firecrawl, runtime, sonalmod—or order consistent with project preference).
- Extend **`test`** to run **`$(MAKE) -C apps/sonal-ui test`** and merge coverage only if the project adopts JS coverage merge later; **initially** it is acceptable for root `test` to run the UI tests without merging into the Go `go tool cover` HTML, as long as **failure fails the build**.

### 4.4 Documentation

- Update **`apps/sonal-ui/AGENTS.md`**: status (scaffold exists), how to **`npm ci`**, **`npm run dev`**, **`make lint` / `make test`** from module or repo root, Node/npm version expectations.
- Optional: short **`apps/sonal-ui/README.md`** for humans mirroring AGENTS runbook (keep duplication low).

---

## 5. Key architectural decisions

| Decision | Rationale |
| --- | --- |
| Vite + Svelte (TS), not SvelteKit | Smaller conceptual surface; static `dist/`; add third-party router deliberately (per research). |
| `svelte-spa-router` 5.x | Research default for non-Kit Svelte 5 SPA; active maintenance; hash routing fits static hosting. |
| Vitest + Testing Library | Aligns with Vite and official Svelte/Testing Library docs; `sv add` is Kit-oriented; manual setup is fine. |
| Isolated `package-lock.json` in `apps/sonal-ui` | Matches research and AGENTS “own package.json”; root `package.json` stays minimal. |
| TDD for first meaningful UI behavior | Project protocol requires tests for logic; first user-visible behavior (nav or copy) drives a failing test then implementation. |

---

## 6. Uncertainties

- **Exact version pins** for Vite, Vitest, `svelte-spa-router`, and `@testing-library/svelte` should be resolved at **`npm install`** time against peer dependency warnings; research notes some pins were not pre-locked.
- **ESLint vs `svelte-check` only:** final lint recipe may vary; both should not duplicate work unnecessarily.
- **Root `test` coverage merge:** Go coverage merge may ignore the UI initially; confirm with maintainers if a separate CI job for `apps/sonal-ui` is preferred before merging JS coverage artifacts. - We ignore the coverage for now

---

## 7. Related files

**Existing (reference):**

- [apps/sonal-ui/AGENTS.md](../../../../AGENTS.md)
- [apps/sonal-ui/doc/implementation/svelte-boilerplate/svelte-boilerplate-research.md](./svelte-boilerplate-research.md)
- [Makefile](../../../../../../Makefile) (repository root)

**To create or replace (during implementation):**

- `apps/sonal-ui/package.json`, `package-lock.json`
- `apps/sonal-ui/vite.config.ts`, `svelte.config.js`, `tsconfig.json` (and extends if any)
- `apps/sonal-ui/index.html`, `src/main.ts`, `src/App.svelte`, `src/app.html` / CSS as needed
- `apps/sonal-ui/src/pages/*`, `src/components/*`
- `apps/sonal-ui/vitest.config.ts` or merged test config
- `apps/sonal-ui/src/**/*.test.ts` or colocated tests (project convention to be chosen once)
- `apps/sonal-ui/.gitignore` (node_modules, dist, coverage, env local files)
- `apps/sonal-ui/.env.example`
- `apps/sonal-ui/Makefile`
- Optional: `apps/sonal-ui/README.md`, `.nvmrc` or `engines` in `package.json`

**To update:**

- `apps/sonal-ui/AGENTS.md`
- Root `Makefile` (`lint`, `test` targets)

---

## 8. Task list

Implementation must follow **TDD** where behavior is introduced: write a **failing** test for the next observable behavior, implement until green, refactor. Each task should leave the **repo buildable**; after UI tasks land, **root** `make lint` and `make test` must succeed per **Coding Task Completion Protocol**.

**Task 1.1: Scaffold Vite + Svelte + TypeScript in `apps/sonal-ui`**

- From repo root or `apps/sonal-ui`, generate the official **Svelte + TS** Vite template into `apps/sonal-ui` without clobbering `AGENTS.md` and `doc/` (use empty folder strategy or merge carefully).
- Ensure `npm install` / `npm ci` works; `npm run dev` and `npm run build` succeed; `dist/` is gitignored.
- Commit `package-lock.json`.

**Task 1.2: Add client routing and dummy pages**

- Add **`svelte-spa-router`** v5 and wire **`App.svelte`** with **at least two routes** and a **nav** between them (e.g. Home / About).
- Use **hash mode** unless the team explicitly chooses history mode with documented server rewrite needs (research: hash fits static export).

**Task 1.3: Vitest + Testing Library (TDD)**

- Add Vitest, `@testing-library/svelte`, `svelteTesting()`, `jsdom` or `happy-dom`, and optional `@testing-library/jest-dom` if matchers are used.
- Configure `npm run test` (watch) and `npm run test:run` (CI).
- **TDD:** Write a failing test that asserts **user-visible behavior** (e.g. route title, link presence, or navigation outcome); implement until passing.
- Run: `npm run test:run` in `apps/sonal-ui` — all tests pass.

**Task 1.4: Env pattern and static assets**

- Add **`.env.example`** with **`VITE_APP_TITLE`** (or similar) and read it in the shell (e.g. layout or home page). No secrets in repo.
- Keep **`public/`** usable (e.g. favicon placeholder optional).

**Task 1.5: Lint script + `apps/sonal-ui` Makefile**

- Add **`npm run lint`** (and fix any issues) using eslint/svelte-check or equivalent so **`make lint`** from `apps/sonal-ui` passes.
- Add **`apps/sonal-ui/Makefile`** with `lint` and `test` targets calling npm scripts.

**Task 1.6: Root Makefile + `AGENTS.md`**

- Update **root** `Makefile` to invoke **`$(MAKE) -C apps/sonal-ui lint`** and **`$(MAKE) -C apps/sonal-ui test`** (order consistent with the rest of the repo).
- Update **`apps/sonal-ui/AGENTS.md`**: status, commands, Node/npm notes, link to this plan folder if useful.

**Task 1.7: Completion protocol**

- From repository root: **`make lint`** — no errors.
- From repository root: **`make test`** — all tests pass (including UI).
- Confirm **`apps/sonal-ui/AGENTS.md`** reflects new workflows.

**Task 1.8: Compress implementation summaries**

- Follow [.context/compress-implementation-summaries.md](/.context/compress-implementation-summaries.md) to compress per-task summaries in this plan’s directory into `implementation-summary.md` and remove intermediate `summary-task-*.md` files (run only after implementation summaries exist).

---

## Research reference

Detailed tooling comparisons, router choice, SvelteKit trade-offs, and citations: [svelte-boilerplate-research.md](./svelte-boilerplate-research.md).
