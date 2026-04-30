<!-- AGENTS.md — README for machines. Nearest file in the tree wins (hierarchical precedence). Keep this concise, concrete, and executable. -->

## Overview

This project is early in development; breaking public API changes are not a concern.

## ⚠️ IMPORTANT: Read Module-Specific AGENTS.md Files First

**Before performing ANY action** (running tests, editing files, debugging, etc.) in a specific module, **ALWAYS read the relevant module-specific AGENTS.md file first**. These files contain critical context, conventions, and requirements specific to each module. For high-level tests (integration/e2e, agent harness), read [tests/AGENTS.md](./tests/AGENTS.md) first.

```
<project layout>/
├── AGENTS.md                     # root AGENTS.md
├── runtime/                      # core agent and related infrastructure (go)
│   └── AGENTS.md
├── apps/
│   ├── sonal-ui/                 # Svelte/Vite SPA (js)
│   │   └── AGENTS.md
│   └── sonalmod/                 # Bundled Sonalmod backend (go)
│       └── AGENTS.md
├── build/
│   ├── AGENTS.md
│   ├── make/                     # shared make fragments (repo-root pinned golangci-lint, etc.)
│   └── npm/                      # pipeline sources — details in build/AGENTS.md
├── tests/                        # high-level integration/e2e tests — [tests/AGENTS.md](./tests/AGENTS.md)
│   └── AGENTS.md
└── tools/
    ├── firecrawl/                 # web scraping tool (go)
    │   └── AGENTS.md
    ├── workspacefs/              # workspace filesystem tools (go)
    │   └── AGENTS.md
    └── skills/                  # skills toolset (go)
        └── AGENTS.md
```

## Security
- NEVER hardcode secrets. Use env vars/secret stores. Authenticate to GHCR before push/pull when required.
- Validate/sanitize all external inputs. Do not disable security linters without explicit justification.

## Dev Environment

Tools/Frameworks:
- Go 1.26.x
- Node.js 24.x
- Svelte 5

Go and Node.js are managed by direnv (in .envrc) and nvm respectively. All dependencies are project scoped (e.g no global node_modules e.t.c).

AI Frameworks:
- OpenSpec - note that it may be used but not currently committed to the repo. This was a conscious decision of the user.

## Nx (monorepo tasks)

This monorepo is managed by Nx. Most typical tasks are:
- Run tests of specific module: `npx nx test sonalmod` (cached)
- Run lint of specific module: `npx nx lint sonalmod` (cached)
- Run test of specific module without caching: `npx nx test sonalmod --skipNxCache`
- Run lint of all affected modules without caching: `npx nx run-many -t lint --skipNxCache`

To run all affected lint and tests, use `make affected-lint-test`

Any weird issues from golangci-linter (like invalid suppression directives or similar) maybe caused by caching issues. Try to clean the cache with `make clean-lint-cache` from repo root (this will only remove the cache) and run the linter again.

> ⚠️ If golangci-lint reports findings that seem unrelated to your changes (e.g. stale suppression directives in untouched files), clean the cache first: `make clean-lint-cache`, then re-run

## npm Release Build Pipeline

All release build logic lives in `build/npm/`. The pipeline is local-first: every step runs identically on developer machines and in CI.

Key commands (run from repo root or with `-C build/npm`):
- Full release build: `make -C build/npm release VERSION=1.2.3`
- Build script self-tests: `make -C build/npm test` (also run by `npx nx test npm-build`)
- Local combined mode (backend serves built UI): `make -C build/npm local-run`
- Clean artifacts: `make -C build/npm clean`

CI/CD release workflows (`.github/workflows/`):
- `release-prepare.yml` — manual (`workflow_dispatch`): input **version**; creates a **draft** GitHub Release (`gh`, generated notes).
- `release-publish.yml` — when a release is **published** (`released`) or manual **ref**; rebuilds tarballs, publishes to npm via **OIDC** (Trusted Publishing; no `NPM_TOKEN` in the repo), uploads assets. Does **not** re-run Nx lint/test (covered on `main`). Setup: [`.github/RELEASE-FLOW.md`](.github/RELEASE-FLOW.md).

Releasing: run **Release Prepare**, review the draft on GitHub, then **publish** the release. Pre-releases (e.g. `v1.2.3-alpha.1`) publish to the `alpha` npm dist-tag and GitHub marks pre-releases accordingly.

## Coding Guide
- Always read [golang-coding-guide.md](./docs/golang-coding-guide.md) if planning to write golang code.

## Product Docs

Canonical domain vocabulary for planning, design, and copy: [docs/domain-terminology.md](./docs/domain-terminology.md).

## Project Rules and Conventions

AI must always follow the rules and conventions defined in this section. This section defines a project specific rules and conventions. Module level rules and conventions must also be followed.

The rules are:
- Update project rules and conventions when user corrects the behavior of AI.
- Each rule must aim to be a simple and clear one line (50-80 characters)

Gopher skill must be used prior to **writing** any Go code, or **planning** go code changes.

## Task Completion Protocol

AI must always follow this protocol when completing tasks. The protocol varies by task type.

Keep in mind that modules may have different task completion protocols, if defined, it must be prioritized over the general protocol.
If code changed is module scoped, run lint and test for that module only.

### Coding Task Completion Protocol

Apply this when any code files were changed (Go, YAML, config files, etc.).

Note: Documentation files (e.g., .md) are not considered code files and should follow the Non-Coding Protocol.

**Always** perform these steps before reporting completion:
1. Run `make affected-lint-test`
   - If findings appear unrelated to your changes, run `make clean-lint-cache` and re-run before fixing
2. Verify AGENTS.md is updated if commands, workflows, or architecture changed

Report task completion status:
- Lint/test: ✓ no errors
- AGENTS.md: ✓ updated / no changes needed

**Note:** Failing tests or lint errors mean the task is NOT complete. All failures must be resolved before completion.

### Non-Coding Task Completion Protocol

For tasks not involving code changes (investigation, documentation review or updates, committing, etc.):
- Summarize findings or actions taken
- Confirm any deliverables were produced
- No lint/test protocol required
