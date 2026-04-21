<!-- AGENTS.md — README for machines. Nearest file in the tree wins (hierarchical precedence). -->

## Status

**Active.** Vite + Svelte 5 TypeScript SPA under this folder (`package.json`, `package-lock.json`). Root `make lint` / `make test` include this module (same order as other submodules: after `apps/sonalmod`).

Always follow [DESIGN.md](./DESIGN.md) when making changes to the UI.

## Node / npm

- On **GitHub Actions**, the reusable Tests workflow runs `actions/setup-node` with the repo root **`.nvmrc`**, then **`npm ci`** in `apps/sonal-ui`, before root `make lint` / `make test`.
- Use a **current Node.js LTS** (or newer) and the **npm** that ships with it. CI and local dev should match closely to avoid lockfile drift.
- Install dependencies with **`npm ci`** from `apps/sonal-ui` (requires `package-lock.json`; use `npm install` only when changing deps).
- **Cross-platform lockfile:** some toolchains expose peers only on certain OSes (e.g. Linux-only optional WASM helpers). If `npm ci` on CI reports packages “Missing from lock file” after a macOS `npm install`, add those packages as **direct `devDependencies`** (or `overrides`) so every platform records the same graph—avoid relying on regenerating the lock only on Linux.
- **`npm run dev`** — Vite dev server.
- **`npm run build`** / **`npm run preview`** — production build and local preview of `dist/`.

## OpenAPI → TypeScript (`openapi-typescript`)

- **Spec (single source of truth):** `../../runtime/internal/agentapi/openapi.yaml` (from this module; repo path `runtime/internal/agentapi/openapi.yaml`). Generated `components.schemas` include session listing (`SessionMetadata`, `SessionListResponse`) and provider model `summarization` alongside existing provider and chat types.
- **Codegen (Makefile only; no npm scripts):**
  - **`make generate-api`** — runs `openapi-typescript` and writes **`src/lib/agentapi/agentapi.generated.ts`**.
  - **`make check-api`** — same invocation with **`--check`** (fails if the generated file is out of date vs the spec).
- Run **`make generate-api`** after changing the spec or when onboarding. **`make check-api`** runs automatically as part of **`make lint`** (and therefore root **`make lint`** / CI).

## Lint and test (module)

From **`apps/sonal-ui`**:

- **`make lint`** — runs `npm run lint` (ESLint + `svelte-check` / TS checks), then **`make check-api`** (OpenAPI generated file in sync with the spec).
- **`make test`** — runs `npm run test:run` (Vitest in CI mode with v8 coverage: text summary and `coverage/` HTML + lcov). Global minimum thresholds in **`vite.config.ts`** must never go down.

## Module Rules and Conventions

Project level rules and conventions must also be followed.

- Tests: Avoid checking typescript types - it's a compiler job.
- Tests: Avoid excessive tests, general rule - one code branch one test.
- Tests: Avoid test global variables or setup if possible. Prefer fixture functions per test. Strong justification (via comment) is required for global setup.
- Tests: Generated sample data (IDs, ISO timestamps, titles, tokens, URLs, payloads) MUST use `@faker-js/faker` (`import { faker } from '@faker-js/faker'`); avoid hardcoded placeholder strings or dates. Exception: literals that match production for assertions (accessible names, button text, routes, theme keys, domain enums).

## Svelte

You are able to use the Svelte MCP server, where you have access to comprehensive Svelte 5 and SvelteKit documentation. Here's how to use the available tools effectively:

### Available Svelte MCP Tools:

#### 1. list-sections

Use this FIRST to discover all available documentation sections. Returns a structured list with titles, use_cases, and paths.
When asked about Svelte or SvelteKit topics, ALWAYS use this tool at the start of the chat to find relevant sections.

#### 2. get-documentation

Retrieves full documentation content for specific sections. Accepts single or multiple sections.
After calling the list-sections tool, you MUST analyze the returned documentation sections (especially the use_cases field) and then use the get-documentation tool to fetch ALL documentation sections that are relevant for the user's task.

#### 3. svelte-autofixer

Analyzes Svelte code and returns issues and suggestions.
You MUST use this tool whenever writing Svelte code before sending it to the user. Keep calling it until no issues or suggestions are returned.

#### 4. playground-link

Generates a Svelte Playground link with the provided code.
After completing the code, ask the user if they want a playground link. Only call this tool after user confirmation and NEVER if code was written to files in their project.
I

## Project Documentation

- Architecture overview: [doc/architecture.md](./doc/architecture.md)
- **UI wireframe (structure, states, behavior):** [ui-wireframe.md](./ui-wireframe.md)
- **CSS design system (two layers, same framework):** [src/app.css](./src/app.css) — tokens, theme, global defaults, shell utilities (**layer 1**); [src/styles/design-system.css](./src/styles/design-system.css) — **`ds-*`** primitives (**layer 2**). Roles, tokens glossary, class index: [DESIGN.md](./DESIGN.md) §10.

**LLM / agent note:** Read `ui-wireframe.md` before changing or explaining UI so composition, routing, and screen states stay accurate. When you change screens, routing, or user-visible behavior, **update `ui-wireframe.md` in the same change** so it remains the source of truth for layout and behavior (not styling).

## Module Rules and Conventions

Project level rules and conventions must also be followed.

- Method with more than 3 arguments (context does not count) is a warning sign. Use params struct instead.

## Purpose

- Browser SPA for Sonalmod (client-side routing, static `dist/` build).

## Task Completion Protocol

Repository level task completion protocol **MUST ALWAYS** be followed. If you didn't follow it, this means task is not complete.
