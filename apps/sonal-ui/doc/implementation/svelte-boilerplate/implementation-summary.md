# Implementation Summary: Initial Svelte boilerplate (`apps/sonal-ui`)

**Plan:** [plan-svelte-boilerplate.md](./plan-svelte-boilerplate.md)

## Overview

The Sonal UI module now has a minimal Vite + Svelte 5 + TypeScript SPA with hash routing (`svelte-spa-router`), Vitest and Testing Library tests, env-driven title via `VITE_APP_TITLE`, ESLint + svelte-check linting, and a local Makefile. Root `Makefile` delegates lint and test to `apps/sonal-ui`, and `AGENTS.md` documents workflows. Work followed TDD where behavior was introduced and kept the repo buildable under the coding task completion protocol.

## Tasks

### Task 1.1: Scaffold Vite + Svelte + TypeScript

The official Vite + Svelte + TypeScript template was applied under `apps/sonal-ui` with `package-lock.json` committed and `dist`/editor ignores in place. Files were merged from a temporary subfolder so `AGENTS.md` and `doc/` were not overwritten.

### Task 1.2: Client routing and dummy pages

`svelte-spa-router` v5 wires `/` and `/about` to Home and About with hash URLs; `Nav.svelte` uses `use:link` for in-app navigation without manual `#` prefixes.

### Task 1.3: Vitest + Testing Library (TDD)

Vitest, jsdom, `svelteTesting()`, jest-dom setup, and `test` / `test:run` scripts were added. `App.test.ts` asserts home/about copy and navigation using `findByRole` (async-friendly for router resolution) and `userEvent` + `waitFor`.

### Task 1.4: Env pattern and static assets

`.env.example` and committed `.env.test` support deterministic Vitest; `VITE_APP_TITLE` is wired on Home with typings and tests; `index.html` title updated; `public/` assets remain usable.

### Task 1.5: Lint script + `apps/sonal-ui` Makefile

ESLint flat config (TypeScript + Svelte, Vitest globals), `npm run lint` runs ESLint then svelte-check/tsc, and a Makefile exposes `lint` and `test`. Removed `vitest/config` triple-slash from `vite.config.ts` for `@typescript-eslint/triple-slash-reference`.

### Task 1.6: Root Makefile + AGENTS.md

Root `Makefile` invokes `apps/sonal-ui` lint/test after `apps/sonalmod`. `AGENTS.md` covers status, `npm ci` / dev, module vs root `make`, Node/npm expectations, and the plan link.

### Task 1.7: Completion protocol

Root `make lint` and `make test` were verified; `AGENTS.md` ties UI work to the root completion protocol; per-task summaries through 1.6 were tracked in git.

## Deviations & notes

- **1.1:** Scaffold merged from a temp folder instead of in-place `create-vite` to avoid clobbering `AGENTS.md` and `doc/`.
- **1.3:** `findByRole` instead of `getByRole` for the first assertion to wait on async route resolution (test-only).
- **1.4:** `apps/sonalmod/internal/api/http/middleware/recover_test.go` adjusted so random HTTP status never picks 204 when encoding JSON (fixes intermittent `-shuffle=on` failure); needed for root `make test`, not UI-only.

## Completion

- Lint: ✓
- Type check: ✓
- Tests: ✓
