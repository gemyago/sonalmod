# Sonal UI

Web front end for Sonalmod: a **Svelte 5** + **TypeScript** single-page app built with **Vite**. You edit sources here; `vite build` produces a static **`dist/`** you can host anywhere.

## Prerequisites

- **Node.js** — use the version in the repo root **`.nvmrc`** (CI and local dev should match).
- **npm** — the one that ships with that Node is fine.

## Get running

From this folder (`apps/sonal-ui`):

```bash
npm ci
npm run dev
```

Open the URL Vite prints (usually `http://localhost:5173`). The app uses **hash** routes (for example `#/`, `#/about`) so it works on a static host without server rewrites.

Copy **`.env.example`** to **`.env`** if you need local overrides (only variables prefixed with **`VITE_`** are exposed to the client).

## Useful commands

| Command | What it does |
| --- | --- |
| `npm run dev` | Dev server with hot reload |
| `npm run build` | Production build → `dist/` |
| `npm run preview` | Serve `dist/` locally to verify the build |
| `npm run test` | Vitest in watch mode (while you work) |
| `make test` | One-shot tests + coverage (same as CI) |
| `make lint` | ESLint + `svelte-check` / TypeScript |

From the **repository root**, `make lint` and `make test` also run this app’s checks in order with the rest of the monorepo.

## Learn more

- **Stack, folders, env, and routing:** [doc/architecture.md](./doc/architecture.md)
- **Conventions, CI details, and how agents should work here:** [AGENTS.md](./AGENTS.md)

**IDE:** [VS Code](https://code.visualstudio.com/) with the [Svelte extension](https://marketplace.visualstudio.com/items?itemName=svelte.svelte-vscode) matches what this repo recommends.
