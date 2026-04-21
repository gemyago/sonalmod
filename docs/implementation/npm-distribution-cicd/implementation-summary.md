# Implementation Summary: npm Distribution and CI/CD Pipeline for Sonalmod

**Plan:** [plan-npm-distribution-cicd.md](./plan-npm-distribution-cicd.md)

## Overview

The backend gained optional static UI serving from a configurable directory (`httpServer.uiLocation` / `--ui-location`), with API routes unchanged and API-only behavior when no valid UI path is set. npm distribution adds `@sonalmod/app` with platform optional packages, `@sonalmod/ui` for Vite dist assets, a Node launcher, and `build/npm` Make-driven release staging, pack, and verify. CI adds tag-driven `release-build` (artifacts only) and `release-publish` (npm + GitHub Release assets). README and AGENTS document install, channels, and maintainer workflows.

## Tasks

### Task 1.1: Define UI-location serving contract in backend

Optional `UILocation` on `V1RoutesDeps` drives mounting static UI at `/` when the path is valid; otherwise the server stays API-only with clear logging. `RootLogger` was added to `V1RoutesDeps` so routing can log without a global logger (satisfies `sloglint`).

### Task 1.2: Add config and CLI support for ui location

`httpServer.uiLocation` in defaults and DI, `--ui-location` on `start` bound to viper, and tests in `main_test.go`. `newRootCmd` returns `(*cobra.Command, *viper.Viper)` so flags share the viper instance used by config load.

### Task 1.3: Verify dev flow remains unchanged

Regression tests lock empty `uiLocation` defaults and `start` without `--ui-location`; no further production changes were required.

### Task 2.1: Create npm main + platform package structure (tests first)

`@sonalmod/app` CJS launcher resolves platform binary and optional `@sonalmod/ui`, with TDD in Node (`sonalmod.test.mjs` ESM vs CJS launcher). `SONALMOD_PACKAGE_ROOT` supports isolated tests; early `resolve-npm-platform.sh` / `parse-semver-tag.sh` support `make test`.

### Task 2.2: Create @sonalmod/ui package and wire launcher

`stage-npm-ui.sh` stages `@sonalmod/ui` from a Vite dist with `--self-test` and packaging tests; `trap … RETURN` fixes temp cleanup in functions.

### Task 2.3: Build release Makefile, build config, and scripts for npm artifacts

Full staging/verify scripts (`stage-platform-package`, `stage-app-package`, `verify-packages.sh`), parallel binary matrix in Makefile, `make release` / `make test`. `verify-packages.sh` avoids GNU-only `tar --transform` for macOS; launcher is staged from a single canonical `build/npm/app/bin/sonalmod.js`.

### Task 3.1: Add CI release-build workflow (build + verify, no publish)

`release-build.yml` on `v*` tags and `workflow_dispatch`: lint/test, `make -C build/npm release`, upload tarballs. `workflow_dispatch` not exercised on real GitHub beyond YAML validity.

### Task 3.2: Add CI release-publish workflow with stable + pre-release handling

`release-publish.yml` publishes in platform → ui → app order and attaches assets to GitHub Releases with prerelease flags. Tag resolution via `workflow_run` may need `refs/tags/` stripping; full live dry-run depends on org credentials and real tag pushes.

### Task 3.3: Update user and developer documentation

README and AGENTS updated for npm install/npx, UI overrides, dist-tags, troubleshooting, local combined mode, and `build/npm` / release notes for agents.

## Deviations & notes

- **1.1:** `RootLogger` on `V1RoutesDeps` for structured logging in routing without globals.
- **1.2:** `newRootCmd` returns viper for shared flag/config binding; config-layer unit tests rely on integration-style tests elsewhere.
- **2.1:** ESM test file vs CJS launcher; `SONALMOD_PACKAGE_ROOT` for tests; semver scripts landed in 2.1 to satisfy `make test`.
- **2.2:** Placeholder `build/npm/ui/package.json` overwritten at staging; trap fix for shell cleanup.
- **2.3:** Launcher files completed here; portable tarball verification without GNU tar transforms; single canonical launcher path for staging.
- **3.1 / 3.2:** GitHub Actions end-to-end behavior validated primarily via YAML/scripts locally, not full cloud dry-runs.
- **Early finalize note:** Root `.gitignore` may ignore `build/npm/app/bin/`; if launcher files are missing on fresh clones, add a negated gitignore rule or relocate (see Task 2.1 finalize report).

## Completion

- Lint: ✓
- Type check: ✓ (Go toolchain; no separate project-wide typecheck beyond Go)
- Tests: ✓
