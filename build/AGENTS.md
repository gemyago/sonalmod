<!-- AGENTS.md — README for machines. Nearest file in the tree wins (hierarchical precedence). -->

## Overview

**Active.** This folder holds shared build infrastructure. The main release pipeline lives under **`npm/`** and shared make fragments live under **`make/`**; CI runs the same targets as developers.

## Layout

### `build/make/`

| Path | Role |
|------|------|
| `golangci-lint.mk` | Shared repo-root pinned `golangci-lint` install/rule fragment reused by Go-module Makefiles; exports per-module cache paths under `.cache/golangci-lint/` |

### `build/npm/`

| Path | Role |
|------|------|
| `Makefile` | Release flow: cross-compile Go matrix, stage UI/app npm packages, pack, verify |
| `build.cfg` | Platform matrix, npm scope, dev fallback semver (non-tag `VERSION`) |
| `scripts/` | Pipeline scripts; support **`--self-test`** where documented |
| `app/`, `ui/` | `@sonalmod/app` launcher and `@sonalmod/ui` package templates |
| `project.json` | Nx project **`npm-build`** (test target → `make test`; **`npm-build:test` depends on `sonal-ui:test`** so `make verify` never runs concurrently with UI Vitest / `apps/sonal-ui` npm install) |

Most scripts support `--self-test` for self-validation. Any updates to the script must be also update it's self-test section accordingly.

Some `Makefile` targets are tested vi [test-makefile.sh](./scripts/test-makefile.sh) script.

## Commands

From repo root (or `make -C build/npm …`):

- **`make release VERSION=1.2.3`** — full release build (use semver; pre-releases e.g. `1.2.3-alpha.1`)
- **`make binaries`** — Go cross-compiles only (matrix from `build.cfg`)
- **`make test`** — script self-tests + launcher/package checks (same as **`npx nx test npm-build`**)
- **`make local-run`** — backend serves built UI for local smoke
- **`make clean`** — remove `dist/`
- **`make publish VERSION=… NPM_TAG=…`** — publish tarballs under `dist/tarballs/` (CI after artifact download; requires `NPM_TAG` for the npm dist-tag)
- **`make unpublish VERSION=…`** — `npm unpublish` that semver for `@sonalmod/app`, `@sonalmod/ui`, and each `@sonalmod/app-<platform>` (reverse of publish order)

Release CI: `.github/workflows/release-prepare.yml` (draft release), `release-publish.yml` (publish + assets; root `AGENTS.md` summarizes npm dist-tags).

## Debugging Makefiles

```bash
# The -rR allows to filter-out built-in noise

# Allows to see how make is expanding the target and its prerequisites.
make -rR -d <target>

# Allows to see the Makefile variables and their values.
make -rR -p <target>
```

## Module rules

Project-level rules in root `AGENTS.md` apply. For this module:

- Prefer editing **`build/npm/Makefile`** and **`build/npm/scripts/`** over one-off CI-only logic; keep behavior identical locally and in CI.
- After changing Makefile, `build.cfg`, or pipeline scripts, run **`npx nx test npm-build`** (or **`make -C build/npm test`**) before reporting done.
- For Makefile targets prefer Makefile philosophy: targets are files but not just commands.

## Task Completion Protocol

Repository level task completion protocol **MUST ALWAYS** be followed. If you didn't follow it, this means task is not complete.
