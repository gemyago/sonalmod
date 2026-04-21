# Svelte SPA at `apps/<app-name>`: tooling, routing, isolation, tests, and starters

## Executive summary

For a **hello-world-plus** Svelte SPA next to a Go backend (embedding later, out of scope here), official documentation presents two main paths: **Vite + `create-vite` Svelte** (minimal static `dist/`, you add routing) and **SvelteKit** (`npx sv create`, Vite-powered, file-based routing and layouts). Svelte’s getting-started material recommends **SvelteKit** as the default application framework; **Vite + Svelte** is documented as an alternative that outputs static assets and usually needs a **separate routing library** for multi-view SPAs. SvelteKit with **`@sveltejs/adapter-static`** can prerender or use **`fallback`** for SPA-style shells; the docs **warn** that pure SPA / fallback patterns have **performance and SEO downsides** versus prerendering or SSR, and for a **separate Go (or similar) backend** they **most recommend** deploying the frontend with **adapter-node or serverless adapters**—with SPA-behind-backend noted as an option with worse characteristics. For a **non-Kit** SPA, task research recommends **`svelte-spa-router` 5.x** as the default router (explicit Svelte 5 line, active 2026 releases); **`svelte-routing`** is **not** recommended as default for Svelte 5 without extra vetting. **Standalone** `apps/<app-name>` with its own `package.json` and **committed `package-lock.json`** supports `npm install`, `npm ci`, and scripts from that directory; **npm workspaces** at the repo root are optional and documented as a **single root lockfile** pattern when one install is desired—**no** Turborepo/Nx required. **Vitest** plus **`@testing-library/svelte`** (with `svelteTesting()` and `jsdom` or `happy-dom`) matches Vite projects; **`npx sv add vitest`** scaffolds tests when using the Svelte CLI. **Official `create-vite` `svelte` / `svelte-ts`** templates include **no** router or tests by default—strong fit for an npm-isolated SPA shell to which you add router and Vitest deliberately; **`npx sv create`** yields SvelteKit (SPA mode is **extra configuration**, not the default scaffold).

## Key findings

1. **Svelte positions SvelteKit (`sv create`) as the default app path; Vite + Svelte via `npm create vite@latest` is the documented minimal alternative with `dist/` output and a pointer to routing packages and SvelteKit SPA docs** ([Getting started — Svelte](https://svelte.dev/docs/svelte/getting-started), [Getting Started — Vite](https://vitejs.dev/guide/)).
2. **SvelteKit SPA mode uses `adapter-static` with `fallback` (e.g. entry HTML for non-prerendered routes), `ssr: false` patterns, and explicit warnings on performance, SEO, and reliance on JS** ([Single-page apps — SvelteKit](https://kit.svelte.dev/docs/single-page-apps), [adapter-static — SvelteKit](https://kit.svelte.dev/docs/adapter-static)).
3. **For an external Go (etc.) backend, SvelteKit “Separate backend” recommends deploying the frontend separately with adapter-node or serverless adapters; SPA served by the backend is an alternative with worse SEO/performance** ([Project types — SvelteKit](https://svelte.dev/docs/kit/project-types)).
4. **Default client router for a plain Vite+Svelte 5 SPA (non-Kit): `svelte-spa-router` 5.x**—npm and README target Svelte 5, March 2026 releases; hash-first fits static `dist/` without rewrites; alternatives include `svelte5-router` forks and `@roxi/routify` (heavier) ([svelte-spa-router README](https://github.com/ItalyPaleAle/svelte-spa-router/blob/main/README.md), npm registry; task T2).
5. **`svelte-routing` latest aligns with Svelte 4 in devDependencies and has open Svelte 5 support discussion; not the default for a Svelte 5 starter** (npm manifest, GitHub issues — task T2).
6. **Isolation: treat `apps/<app-name>` as the npm root—`package-lock.json` committed, `npm ci` for CI; without root `workspaces`, the app is fully independent** ([package-lock.json — npm](https://docs.npmjs.com/cli/v10/configuring-npm/package-lock-json), [npm ci — npm](https://docs.npmjs.com/cli/v10/commands/npm-ci)).
7. **Optional npm workspaces: root `package.json` `workspaces` globs, single root lockfile, `npm run test --workspace=` or `cd` into app** ([workspaces — npm](https://docs.npmjs.com/cli/v10/using-npm/workspaces))—**inference:** matches plan’s “minimal footprint” when the repo opts into one root install.
8. **Vitest reuses Vite config; pair with `@testing-library/svelte`, `svelteTesting()` plugin, `test.environment` jsdom/happy-dom, optional `@testing-library/jest-dom`; Svelte docs cover runes testing patterns (`$effect.root`, etc.)** ([Vitest — Getting Started](https://vitest.dev/guide/), [Setup — Svelte Testing Library](https://testing-library.com/docs/svelte-testing-library/setup), [Testing — Svelte](https://svelte.dev/docs/svelte/testing)).
9. **Concrete tree: `index.html`, `vite.config`, `src/main`, `App.svelte`, `src/pages/`, `src/components/`, `src/lib/`, `public/`, `dist/` on build; scripts `dev`, `build`, `preview`, `test` (`vitest`), optional `test:run`** ([create-vite template-svelte](https://github.com/vitejs/vite/tree/main/packages/create-vite/template-svelte), [Vite env and mode](https://vitejs.dev/guide/env-and-mode), task T5).
10. **`create-vite` svelte/svelte-ts: no router or tests in default `package.json`; `sv create` + Kit: file-based routes, Vitest/Playwright via `sv add`; community lists (e.g. Awesome Vite) need per-repo verification** ([Vite guide](https://vite.dev/guide/), [sv create — Svelte](https://svelte.dev/docs/cli/sv-create), [Awesome Vite](https://github.com/vitejs/awesome-vite) — task T6).

## Detailed sections (mirroring plan questions)

### Vite + vanilla Svelte vs SvelteKit SPA (2025–2026)

Official docs distinguish **minimal Vite + Svelte** (`npm create vite@latest` with `svelte` / `svelte-ts`, `vite-plugin-svelte`, static build typically under **`dist/`**) from **SvelteKit** as the **recommended** application framework (`npx sv create`), which is Vite-based but adds file-based routing, layouts, and the Kit adapter model ([Getting started — Svelte](https://svelte.dev/docs/svelte/getting-started), [Getting Started — Vite](https://vitejs.dev/guide/)). For multi-view SPAs without Kit, getting-started notes you **usually need a routing library** and links SPA terminology and SvelteKit-based SPA building ([Getting started — Svelte](https://svelte.dev/docs/svelte/getting-started)).

**SvelteKit + `adapter-static`** can prerender to static files; the **`fallback`** option serves SPA-style entry HTML for routes not prerendered. The adapter-static doc warns **`fallback` has large negative performance and SEO impacts** and is only recommended in certain circumstances, pointing to the SPA doc ([adapter-static — SvelteKit](https://kit.svelte.dev/docs/adapter-static)). The SPA doc describes client-rendered shells, **performance** (multiple round trips), **SEO**, and **no content if JS fails**, and recommends **prerendering as much as possible**; if all pages can be static, prefer **SSG** ([Single-page apps — SvelteKit](https://kit.svelte.dev/docs/single-page-apps)).

**Trade-off summary (from notes):** Vite+Svelte minimizes framework surface and outputs a static front; you **own routing and assembly**. SvelteKit gives **first-party routing, layouts, and `sv add` tooling** but more concepts; **pure SPA fallback** is **discouraged** except when trade-offs are accepted. For **dummy pages + nav + tests**, T1 recommends **aligning with team defaults: SvelteKit (`sv create`)** for built-in routing and documented Vitest add-ons, with **`adapter-static`** and **prerendering** where practical for static export; choose **bare Vite + create-vite** for **maximum minimalism** if you accept a third-party router ([task T1](notes/task-T1-vite-vs-sveltekit-spa.md)).

**Go-adjacent context:** Project-types doc states that with backends in **Go, Java, PHP, Ruby, Rust, or C#**, the **most recommended** approach is deploying SvelteKit **separately** with **adapter-node or serverless**; some teams use an **SPA served by the backend** with the caveat of **worse SEO and performance** ([Project types — SvelteKit](https://svelte.dev/docs/kit/project-types)). Embedding static files in Go is **not** detailed in these docs (task T1 limitation).

### Client-side routing for a non-SvelteKit SPA

Canonical **community** options surveyed in T2: **`svelte-spa-router`** (ItalyPaleAle)—**5.x for Svelte 5**, hash-oriented, active releases March 2026; **`svelte-routing`**—Svelte 4 line in npm devDeps, stale commits, Svelte 5 not settled; **`svelte5-router` (jpcutshall)** and **`@mateothegreat/svelte5-router`**—Svelte 5 peers, smaller or slower-moving communities; **`@roxi/routify`**—Svelte 5 allowed at peer level, filesystem-oriented, heavier toolchain ([task T2](notes/task-T2-spa-routing-options.md), [svelte.dev/packages](https://svelte.dev/packages)). **SvelteKit-only** is not a “plain Vite + router” stack; it is the first-party routing option when you adopt Kit ([Single-page apps — SvelteKit](https://kit.svelte.dev/docs/single-page-apps)).

**Default for a Vite-only starter:** **`svelte-spa-router`** at **5.x** (task T2). **When to deviate:** history/pretty URLs and declarative `<Route>`/`<Link>`—evaluate **svelte5-router** variants; filesystem routes—**Routify**; all-in-one framework—**SvelteKit** if the non-Kit constraint is dropped.

### Placement in the repo: npm-only, isolated `apps/<app-name>`

A folder with **`package.json`** is a normal npm package root when that directory is the working directory; **`npm install`** without arguments installs into that package’s `node_modules` ([npm-install — npm](https://docs.npmjs.com/cli/v10/commands/npm-install)). **Commit `package-lock.json`** next to the app’s `package.json` for reproducible installs; **`npm ci`** requires the lockfile and performs clean CI installs ([package-lock.json — npm](https://docs.npmjs.com/cli/v10/configuring-npm/package-lock-json), [npm-ci — npm](https://docs.npmjs.com/cli/v10/commands/npm-ci)).

**npm workspaces (minimal):** root `package.json` lists **`workspaces`** (e.g. `"./apps/*"`); one **`npm install` at the root** produces a **single root `package-lock.json`** and symlinks workspace packages; run per-app scripts via **`cd apps/<app>`** or **`npm run test --workspace=`** ([package.json — workspaces](https://docs.npmjs.com/cli/v10/configuring-npm/package-json#workspaces), [workspaces — npm](https://docs.npmjs.com/cli/v10/using-npm/workspaces)). **No** Turborepo/Nx is required for this pattern. Root README can list `apps/` and point to **`apps/<app-name>/README.md`** without requiring a root install ([task T3](notes/task-T3-npm-isolated-apps-folder.md)).

### Test stack: Vitest, Testing Library, optional E2E

**Vitest** is the Vite-native runner; it extends Vite config with a **`test`** block and documents **Vite ≥ 6** and **Node ≥ 20** in current getting-started material ([Vitest — Getting Started](https://vitest.dev/guide/)). **`@testing-library/svelte`** provides `render`, queries, `fireEvent`, etc.; setup docs recommend **`svelteTesting()`** from `@testing-library/svelte/vite`, **`@sveltejs/vite-plugin-svelte`**, **`vitest`**, and **`jsdom`** or **`happy-dom`**, plus optional **`@testing-library/jest-dom`** ([Setup — Svelte Testing Library](https://testing-library.com/docs/svelte-testing-library/setup)). **Svelte CLI:** **`npx sv add vitest`** installs packages and demo tests with unit/component options ([Svelte CLI — vitest](https://svelte.dev/docs/cli/vitest)). **Mocking:** Vitest **`vi.mock`**, **`vi.fn`**, **`vi.spyOn`**, env stubs ([Vitest — Mocking](https://vitest.dev/guide/mocking)). **E2E:** **Playwright** is documented as an optional full-browser test runner ([Playwright — Intro](https://playwright.dev/docs/intro)); orthogonal to component tests.

### Minimal tree, scripts, and `VITE_*`

**Baseline** from **create-vite** `template-svelte`: `index.html`, `vite.config`, `svelte.config`, `public/`, `src/main`, `src/App.svelte`, `src/assets/`, `src/lib/` ([create-vite template-svelte](https://github.com/vitejs/vite/tree/main/packages/create-vite/template-svelte)). **Extended SPA layout (task T5):** `src/pages/` (or `views/`) for route targets, `src/components/` for shared UI, optional `tests/` or colocated specs; **`dist/`** default build output, gitignored.

| Script | Role |
| --- | --- |
| `dev` | `vite` — development mode |
| `build` | `vite build` — production bundle to `dist/` |
| `preview` | `vite preview` — local preview of build |
| `test` | `vitest` — often `vitest run` in CI |

**Env:** Only **`VITE_*`** names are exposed to client code via **`import.meta.env`**; use **`.env`**, **`.env.local`**, **`.env.[mode]`** with mode overrides; `vite build` defaults to **production** mode ([Env and mode — Vite](https://vitejs.dev/guide/env-and-mode)). Examples: **`VITE_APP_TITLE`**, **`VITE_API_BASE_URL`** for non-secret config (task T5).

### Official templates and maintained starters

**`create-vite` `svelte` / `svelte-ts`:** Official, **no** router or test script in default `package.json`; **no** workspace tooling—fits **`apps/<name>`** with local `npm install` ([Vite guide](https://vite.dev/guide/), upstream template `package.json` — [task T6](notes/task-T6-starters-templates.md)). **`npx sv create`:** SvelteKit templates **`minimal`**, **`demo`**, **`library`**; **file-based routing** is the Kit model; **Vitest/Playwright** via **`sv add`**; default scaffold is **not** pure SPA—**SPA requires follow-up** (`adapter-static`, `fallback`, `ssr` options per docs) ([Creating a project — SvelteKit](https://svelte.dev/docs/kit/creating-a-project), [Single-page apps — SvelteKit](https://svelte.dev/docs/kit/single-page-apps), [sv add — Svelte](https://svelte.dev/docs/cli/sv-add)). **Community:** [Awesome Vite](https://github.com/vitejs/awesome-vite) lists Svelte-related entries; task T6 found **stale, empty, pnpm-pinned, or domain-specific** examples—**verify** before adopting.

**Plan-aligned default:** **`create-vite` + manual router + Vitest** for maximum isolation and no Kit concepts; **`sv create` + Kit** when you want first-party routing and **`sv add`** integrations, accepting SPA configuration as a second step ([task T6](notes/task-T6-starters-templates.md)).

---

## Source list

- https://svelte.dev/docs/svelte/getting-started — Svelte getting started; `sv create` vs `create-vite` alternatives, `dist/`, routing note.
- https://svelte.dev/docs/kit/project-types — SvelteKit project types; separate Go/etc. backend, adapter-node/serverless vs SPA.
- https://kit.svelte.dev/docs/single-page-apps — SPA mode, `ssr`, `adapter-static`, `fallback`, perf/SEO.
- https://kit.svelte.dev/docs/adapter-static — Prerender, `fallback`, warnings.
- https://vitejs.dev/guide/ — `npm create vite@latest`, Svelte templates, Node version, monorepo note.
- https://vite.dev/guide/ — Vite getting started (T6); templates and community link.
- https://svelte.dev/docs/kit/creating-a-project — SvelteKit project creation.
- https://svelte.dev/docs/cli/sv-create — `sv create` options and templates.
- https://svelte.dev/docs/cli/sv-add — Official add-ons (Vitest, Playwright).
- https://svelte.dev/docs/cli/vitest — Svelte CLI Vitest integration.
- https://svelte.dev/docs/cli/overview — Svelte CLI overview.
- https://svelte.dev/docs/svelte/testing — Svelte testing; runes, `$effect.root`.
- https://svelte.dev/packages — Curated packages (routing pointers).
- https://vitest.dev/guide/ — Vitest getting started, Vite integration, versions.
- https://vitest.dev/config/ — Vitest configuration.
- https://vitest.dev/guide/environment — jsdom, happy-dom, per-file env.
- https://vitest.dev/guide/mocking — `vi.mock`, spies, env.
- https://testing-library.com/docs/svelte-testing-library/intro — Testing Library for Svelte.
- https://testing-library.com/docs/svelte-testing-library/api — API (`render`, `fireEvent`, etc.).
- https://testing-library.com/docs/svelte-testing-library/setup — Vitest + `svelteTesting()` setup.
- https://playwright.dev/docs/intro — Playwright E2E.
- https://vitejs.dev/guide/env-and-mode — `VITE_*`, modes, `.env` loading.
- https://vitejs.dev/guide/cli — `vite`, `vite build`, `vite preview`.
- https://github.com/vitejs/vite/tree/main/packages/create-vite/template-svelte — Official Svelte template layout and `package.json`.
- https://github.com/vitejs/awesome-vite — Community templates list (README).
- https://github.com/ItalyPaleAle/svelte-spa-router — svelte-spa-router README (Svelte 5).
- https://registry.npmjs.org/svelte-spa-router/latest — npm manifest (task T2).
- https://docs.npmjs.com/cli/v10/using-npm/workspaces — npm workspaces.
- https://docs.npmjs.com/cli/v10/configuring-npm/package-json#workspaces — `workspaces` field.
- https://docs.npmjs.com/cli/v10/configuring-npm/package-lock-json — Lockfile semantics.
- https://docs.npmjs.com/cli/v10/commands/npm-install — Install behavior, lock precedence.
- https://docs.npmjs.com/cli/v10/commands/npm-ci — CI installs.

## Gaps and limitations

- **Firecrawl** was unreliable in discovery (empty searches/maps); evidence leans on **direct doc/registry/GitHub** access per task notes—**not** a gap in claims, but a **process** limitation for automated search.
- **Go static serving** (`embed`, `FileServer`, `200.html` fallback) is **not** specified in Svelte/Vite docs; only generic static/Apache examples in Kit SPA docs (T1).
- **Exact version pins** for `@sveltejs/kit`, `adapter-static`, Vite, Vitest, and `@testing-library/svelte` were **not** locked from npm in all tasks—implementers should verify peers at install time (T1, T4).
- **No hands-on integration tests** ran in-repo for each router against a live Vite+Svelte 5 app (T2); compatibility rests on **peer deps**, READMEs, and release timing.
- **Community templates** churn: Awesome Vite entries may be **empty, renamed, or outdated** (T6); exhaustive Turborepo/Nx enumerations were **out of scope**.
- **Svelte testing doc** fetch had mixed HTML in one segment (T4); narrative follows Testing Library + Vitest + Svelte testing overview.
- **npm v10** docs used consistently in T3; **v11** exists—teams should align CI npm major (T3).

## Confidence

- **High:** Official Svelte/Vite/SvelteKit positions on scaffolding, SPA trade-offs, adapter-static/fallback; npm lockfile/workspace mechanics; Vitest + Testing Library setup patterns from official sites; create-vite template contents; `svelte-spa-router` 5.x Svelte 5 targeting and 2026 release activity.
- **Medium:** Default **starter** choice between **Kit-first (T1)** vs **create-vite-first (T6)**—both are evidence-backed; the plan’s **isolation / no orchestration** preference slightly favors **create-vite + add-ons**, while **team onboarding** docs favor **Kit**—success depends on whether “plain Vite SPA” is a hard constraint.
- **Low:** Long-term maintenance of **community** routers other than svelte-spa-router **without** project-specific smoke tests; **exact** peer resolution edge cases for `svelteTesting()` in complex Vite graphs (docs link to a known issue).

## Suggested follow-ups

- Run **`npm create vite@latest`** and wire **`svelte-spa-router` 5**, **Vitest**, and **`@testing-library/svelte`** in a scratch `apps/<name>` to validate versions and one CI job (`npm ci` → `vitest run` → `vite build`).
- If choosing **SvelteKit**, spell out **minimal `adapter-static` + prerender** config for dummy routes and **only** add `fallback` / `ssr: false` where the SPA doc allows.
- Document **Go** static hosting and **history vs hash** routing for the eventual embed (out of current research scope).
- Re-verify **Awesome Vite** Svelte entries (or npm **keyword** search) before adopting any community starter.
- Pin **Node/npm** versions in CI to match Vite/Vitest documented floors.
