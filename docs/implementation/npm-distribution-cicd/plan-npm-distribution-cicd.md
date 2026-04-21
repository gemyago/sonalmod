# Plan: npm Distribution and CI/CD Pipeline for Sonalmod

## 1. Introduction / Overview

Sonalmod already builds successfully with Nx, but end-user distribution is not implemented yet. The next step is to make Sonalmod installable and runnable from npm while keeping the current development flow intact.

This plan introduces a release-ready strategy where:
- Users install Sonalmod from npm and run a sonalmod command.
- The backend can serve pre-built UI from a configured read-only filesystem location.
- UI serving is optional: if UI location is not provided, backend runs in API-only mode.
- CI/CD is extended from test-only to build, package, and publish.

### Success criteria
- npm install -g @sonalmod/app installs a runnable sonalmod command.
- npx @sonalmod/app start starts the backend with UI served automatically.
- Backend API endpoints continue to work on /api/v1/... and /health.
- Backend serves UI only when ui location is provided and valid.
- Tag-based GitHub Actions publishes npm packages for stable and pre-release channels.
- The full release build pipeline can be run locally with a single make command.
- GitHub Actions workflows are thin orchestrators that call make targets, not inline shell logic.

### Non-goals
- Docker as primary distribution channel in this iteration.
- Replacing current hash routing behavior in UI.
- Refactoring unrelated runtime/business features.

---

## 2. Distribution Strategy Investigation (and Recommendation)

### 2.1 Binary/npm packaging options

Option A: Single npm package containing all platform binaries
- Pros: one package.
- Cons: large downloads, slower installs, unnecessary artifacts per user.

Option B: npm main package plus platform-specific optional packages (recommended)
- Pros: proven pattern, small installs per user, clear platform targeting.
- Cons: requires launcher logic and package fan-out.

Option C: npm package with postinstall binary download
- Pros: fewer npm packages.
- Cons: install-time network fragility, harder enterprise support, weaker reproducibility.

Recommendation for binaries:
1. Main package: @sonalmod/app
2. Platform packages:
   - @sonalmod/app-linux-x64
   - @sonalmod/app-linux-arm64
   - @sonalmod/app-darwin-x64
   - @sonalmod/app-darwin-arm64
   - @sonalmod/app-win32-x64

### 2.2 UI distribution options

Option U1: Embed UI into Go binary
- Pros: single binary artifact.
- Cons: couples backend build with UI build tightly, less flexible rollout.

Option U2: Publish separate @sonalmod/ui package (recommended)
- Pros: clean separation of concerns, main package stays lean (no static assets mixed with launcher scripts), UI versioned and replaceable independently, launcher resolves UI path via require.resolve at runtime.
- Cons: one additional package to publish per release.

Option U3: Ship UI assets inside @sonalmod/app package
- Pros: no Go embedding, optional runtime serving, no extra UI package.
- Cons: main npm package bloated with static assets, tighter coupling between launcher and UI build.

Recommendation for UI:
- Publish a separate @sonalmod/ui package containing the Vite dist output.
- @sonalmod/app declares @sonalmod/ui as a dependency (pinned to same version).
- Launcher resolves the UI dist path via require.resolve('@sonalmod/ui') at runtime and passes it to the backend as --ui-location.

---

## 3. Business Logic

1. Install and execute flow
- User installs @sonalmod/app.
- npm resolves platform package via optionalDependencies and @sonalmod/ui via dependencies.
- Launcher resolves binary path and UI dist path, then starts the backend with --ui-location injected.

2. Runtime HTTP behavior
- Existing API routes remain unchanged.
- New optional UI serving path is enabled only when UI location is configured.
- If UI location is missing or invalid, server continues in API-only mode.

3. UI packaging behavior
- UI dist output is published as @sonalmod/ui (separate package).
- @sonalmod/app depends on @sonalmod/ui at the same version.
- Launcher resolves @sonalmod/ui dist directory and passes it as --ui-location by default (can be overridden or disabled).

4. Release behavior
- Git tag drives package versioning for all sonalmod npm packages.
- Stable and pre-release tags are both supported with channel-aware npm dist-tags.

---

## 4. High-Level Architecture

| Component | Responsibility | Module |
|---|---|---|
| UI static serving layer | Serves files from configured read-only UI directory | apps/sonalmod |
| UI location config/flag | Enables or disables UI serving at runtime | apps/sonalmod |
| UI build output | Produces Vite dist assets for packaging | apps/sonal-ui |
| @sonalmod/ui package | Carries UI static assets, consumed by launcher | build folder |
| @sonalmod/app package | User-facing launcher, resolves binary and UI path | build folder |
| npm platform packages | Carry per-OS/arch Go binaries | build folder |
| Build orchestration | Builds UI + binaries and stages npm package contents | root + module Make/Nx |
| CI release workflows | Validate, package, publish, and smoke-check | .github/workflows |

---

## 5. Detailed Architecture

### 5.1 Backend: serve UI from explicit ui location

Add runtime-configurable UI serving in apps/sonalmod.

Core behavior:
- Add config key and CLI flag for UI location (recommended naming: httpServer.uiLocation and --ui-location).
- If ui location is set and readable:
  - GET / returns index.html.
  - Static asset paths are served from the same directory.
- If ui location is empty or invalid:
  - UI routes are not mounted.
  - Server logs a clear warning/info and continues API-only.

Implementation notes:
- Use read-only filesystem access via os.DirFS and standard net/http static serving.
- Avoid writable operations and avoid startup failure due to missing UI.

### 5.2 Preserve current development flow

Current dev flow remains unchanged:
- Backend: go run . start from apps/sonalmod.
- UI: npm run dev from apps/sonal-ui with existing Vite proxy.

Optional local combined behavior:
- Build UI and run backend with --ui-location pointing to apps/sonal-ui/dist.

No build-tag-specific asset provider is needed in this strategy.

### 5.3 npm package topology and launcher behavior

UI package (@sonalmod/ui):
- Contains only the Vite dist output (static assets).
- No scripts or binaries.
- Note: apps/sonal-ui/package.json is the private development manifest used by Vite to build the SPA; it is never published. build/npm/ui/package.json is a separate distribution manifest created as part of release staging. It carries the public package name (@sonalmod/ui), the version from the git tag, and a files field pointing to the staged dist output. The two package.json files serve different purposes and coexist without conflict.

Main package (@sonalmod/app):
- Contains launcher script and package metadata.
- Declares optionalDependencies on platform packages pinned to same version.
- Declares dependency on @sonalmod/ui pinned to same version.

Platform packages (@sonalmod/app-<os>-<arch>):
- Contain one binary (sonalmod or sonalmod.exe).
- Use os/cpu package fields for platform filtering.

Launcher behavior:
- Resolves platform binary path from the installed platform package.
- Resolves UI dist path via require.resolve('@sonalmod/ui') (e.g. points to node_modules/@sonalmod/ui/dist).
- Passes --ui-location <ui-dist-path> to the backend by default (can be overridden or disabled via env/flag).
- Forwards all user args and process stdio.

### 5.4 Build and packaging orchestration

#### Local-first design principle

All build and packaging logic lives in standalone scripts and Makefiles that run identically on a developer machine and in CI. GitHub Actions workflows are thin orchestrators: they set up toolchains, export environment variables, and invoke make targets. No build logic is inlined in YAML.

This enables developers to:
- Run the full release build locally: `make -C build/npm release VERSION=0.1.0-alpha.1`
- Test individual scripts: `build/npm/scripts/resolve-npm-platform.sh --self-test`
- Debug CI failures locally without needing a full GitHub Actions run.

#### Build configuration: build.cfg

Platform matrix and other build parameters live in `build/npm/build.cfg` (ini-style, similar to the pattern in golang-backend-boilerplate/build/build.cfg). The Makefile reads values from this file using a `read_config` function.

Example platforms entry: `linux/amd64,linux/arm64,darwin/arm64,windows/amd64`

See reference file: `doc/implementation/npm-distribution-cicd/ref-build-cfg.cfg`

#### Makefile: build/npm/Makefile

The `build/npm/Makefile` is the primary entry point for the release build pipeline. Key design patterns (following golang-backend-boilerplate/build/Makefile):

- Reads `build.cfg` for platform matrix (GOOS/GOARCH format).
- Uses `dist/%: FORCE` pattern for per-platform binary builds (one Make target per platform).
- Builds platform binaries in parallel with `make -j4 $(go_dist_targets)`.
- Uses sentinel `.staged` files to track npm package staging completion.
- Scripts handle all transformation logic; Makefile handles dependency ordering.

Main targets:
- `make binaries` - cross-compile Go binaries for all platforms (parallel matrix build)
- `make ui` - build the UI with Nx
- `make stage-ui` - stage @sonalmod/ui package directory
- `make stage-platform-packages` - stage per-platform npm package directories
- `make stage-app` - stage @sonalmod/app package directory
- `make pack` - run npm pack on all staged directories, producing tarballs
- `make verify` - validate tarballs (contents, binary execution, launcher resolution)
- `make release` - full pipeline (entry point for CI): `make -C build/npm release VERSION=<tag>`
- `make test` - run all script self-tests (for local dev and CI)
- `make clean` - remove dist/

See reference file: `doc/implementation/npm-distribution-cicd/ref-build-makefile.mk`

#### Nx workspace integration: build/npm/project.json

`build/npm` is registered as an Nx project so that `npx nx test npm-build` and `make affected-lint-test` pick up script self-tests automatically.

The `test` target calls `make test` (which runs `--self-test` on each supporting script). Key config details:
- `dependsOn: []` overrides the workspace default (`install-deps`) since bash scripts have no Go/npm deps.
- `inputs` includes all scripts and the Makefile so the Nx cache is invalidated when any of them change.
- No `lint` target needed unless shellcheck is added later.

See reference file: `doc/implementation/npm-distribution-cicd/ref-build-project-json.json`

#### Build scripts: build/npm/scripts/

Each script is standalone (no sourcing required), accepts CLI flags, and implements a `--self-test` flag for unit testing. Convention follows golang-backend-boilerplate/build/scripts/resolve-docker-tags.sh.

Key scripts:
- `resolve-npm-platform.sh` - converts GOOS/GOARCH to npm os/cpu/suffix (e.g. linux/amd64 → linux-x64)
- `parse-semver-tag.sh` - parses git tag into VERSION, PRERELEASE_ID, NPM_TAG for dist-tag selection
- `stage-npm-ui.sh` - copies UI dist and writes @sonalmod/ui package.json with correct version
- `stage-platform-package.sh` - stages per-platform npm package with binary and package.json
- `stage-app-package.sh` - stages @sonalmod/app with launcher, optionalDependencies, and dependencies
- `verify-packages.sh` - validates tarball contents, binary runs, launcher resolution; supports `--self-test`

See reference files:
- `doc/implementation/npm-distribution-cicd/ref-resolve-npm-platform.sh`
- `doc/implementation/npm-distribution-cicd/ref-parse-semver-tag.sh`

#### Deterministic release staging steps

1. Build UI with `make ui` (calls `npx nx build sonal-ui`).
2. Build Go binaries for all platforms in parallel with `make binaries` (matrix via `dist/%: FORCE`).
3. Stage @sonalmod/ui package directory with dist assets and package.json.
4. Stage per-platform @sonalmod/app-<os>-<arch> package directories with binaries.
5. Stage @sonalmod/app package directory with launcher script and metadata.
6. Run `npm pack` on all staged directories to produce tarballs.
7. Verify tarballs: content check, binary smoke test, launcher resolution test.

All steps are driven by `make -C build/npm release VERSION=<semver>`.

### 5.5 CI/CD release pipeline design

Context:
- Trunk-based development: all pull requests target main.
- The existing build-flow.yml already handles PR validation (lint + tests) and is not changed by this plan.
- A release is cut by pushing a semver tag (e.g. v1.4.0 or v1.4.0-alpha.1) to a commit on main.
- The tag is the single trigger and source of truth for version; no separate "release branch" is needed.

Relationship between build and release:
- "Build" in this context means compiling and packaging all release artifacts (UI dist, cross-platform binaries, npm tarballs) from the tagged commit.
- "Release" means publishing those artifacts to npm and creating a GitHub Release entry.
- They are kept in separate workflows so artifacts can be inspected and verified before anything is published, and so the publish step can be re-run independently if needed (e.g. after a transient npm outage).

Workflow 1: Release Build (release-build.yml)
Trigger: Tag push matching v*, and workflow_dispatch.

Stages:
1. Setup (checkout at tag, Node, Go, dependencies).
2. Validation (lint/test from the tagged commit via `make affected-lint-test`).
3. Build and package: `make -C build/npm release VERSION=$(GIT_TAG_WITHOUT_V)`
   - This single command drives UI build, binary matrix, staging, packing, and verification.
   - No build logic is inlined in YAML.
4. Upload artifacts (tarballs from build/npm/dist/tarballs/).

Workflow 2: Release Publish (release-publish.yml)
Trigger: workflow_run on successful completion of release-build, and workflow_dispatch.

Stages:
1. Download artifacts produced by release-build.
2. Parse semantic version and npm dist-tag from the tag using `build/npm/scripts/parse-semver-tag.sh`.
3. Select npm dist-tag:
   - stable tag vX.Y.Z -> latest
   - vX.Y.Z-alpha.N -> alpha
   - vX.Y.Z-beta.N -> beta
   - vX.Y.Z-rc.N -> rc
   - other pre-release identifiers -> next
4. Authenticate npm (prefer trusted publishing + provenance).
5. Publish in order:
   - platform packages
   - @sonalmod/ui
   - @sonalmod/app
6. Create GitHub Release:
   - mark prerelease=true for pre-release tags
   - attach checksums and package artifacts

### 5.6 Versioning and pre-release strategy

- Version is derived from git tag without leading v.
- All sonalmod npm packages share the exact same version.
- Pre-releases are first-class:
  - examples: v1.4.0-alpha.1, v1.4.0-beta.2, v1.4.0-rc.1
  - publish to non-latest dist-tag according to identifier mapping above
  - GitHub release marked as prerelease
- Stable releases:
  - example: v1.4.0
  - publish to latest

---

## 6. Key Architectural Decisions

1. Use npm optionalDependencies + per-platform packages for binaries.
2. Publish @sonalmod/ui as a separate npm package; @sonalmod/app depends on it.
3. Do not embed UI in Go binary; serve from configured read-only UI location.
4. Launcher resolves UI dist path at runtime via require.resolve and injects --ui-location.
5. Keep default backend startup in API-only mode so current dev flow remains unchanged.
6. Handle stable and pre-release tags explicitly with channel-aware npm dist-tags.
7. Separate release-build and release-publish workflows, both tag-triggered, with publish depending on build success.

---

## 7. Uncertainties

- Final naming of config/flag: httpServer.uiLocation vs similar alternatives. - fine to stick to this
- Whether launcher should always inject UI location by default or require explicit opt-in. - always for now
- First supported platform matrix scope (whether to include windows-arm64, linux-musl, etc). - start with linux-x64, linux-arm64, darwin-arm64, win32-x64
- npm trusted publishing readiness in @sonalmod org. - to be investigated later

---

## 8. Related Files

### Existing files likely to be updated

- apps/sonalmod/main.go
- apps/sonalmod/internal/api/http/register.go
- apps/sonalmod/internal/api/http/register_test.go
- apps/sonalmod/internal/api/http/server/router_test.go
- apps/sonalmod/internal/config/default.yaml
- apps/sonalmod/internal/config/provide.go
- apps/sonalmod/Makefile
- apps/sonalmod/project.json
- apps/sonal-ui/project.json
- apps/sonal-ui/vite.config.ts
- Makefile
- .github/workflows/build-flow.yml
- .github/workflows/tests-run.yml
- README.md
- AGENTS.md (if workflow/commands/architecture references change)

### New files/directories likely to be created

- doc/implementation/npm-distribution-cicd/summary-task-*.md
- doc/implementation/npm-distribution-cicd/ref-build-project-json.json (reference example)
- doc/implementation/npm-distribution-cicd/ref-build-makefile.mk (reference example)
- doc/implementation/npm-distribution-cicd/ref-build-cfg.cfg (reference example)
- doc/implementation/npm-distribution-cicd/ref-resolve-npm-platform.sh (reference example)
- doc/implementation/npm-distribution-cicd/ref-parse-semver-tag.sh (reference example)
- .github/workflows/release-build.yml
- .github/workflows/release-publish.yml
- build/npm/Makefile
- build/npm/build.cfg
- build/npm/project.json
- build/npm/scripts/resolve-npm-platform.sh
- build/npm/scripts/parse-semver-tag.sh
- build/npm/scripts/stage-npm-ui.sh
- build/npm/scripts/stage-platform-package.sh
- build/npm/scripts/stage-app-package.sh
- build/npm/scripts/verify-packages.sh
- build/npm/app/package.json
- build/npm/app/bin/sonalmod.js
- build/npm/ui/package.json
- build/npm/ui/dist/*
- build/npm/app-*/package.json

---

## 9. Task List

All tasks follow TDD and must leave the codebase in a buildable state. Module-specific completion protocol must be followed in each task.

**Task 1.1: Define UI-location serving contract in backend (tests first)**
- Add failing tests in apps/sonalmod/internal/api/http/register_test.go for:
  - API routes remain intact (/api/v1/runtime/... and /health).
  - GET / serves UI only when ui location is configured and valid.
  - Static assets are served from configured UI directory.
  - Empty/missing ui location keeps API-only behavior.
- Run affected tests: go test -v ./internal/api/http/... --run TestSetupV1Routes
  - Verify failure is expectation-based (no unresolved stubs/compile errors).
- Implement minimal route wiring to satisfy tests.
- Re-run affected tests and then module checks:
  - make lint and make test from apps/sonalmod.
- Write summary to doc/implementation/npm-distribution-cicd/summary-task-1.1.md.
- All checks from completion protocol must be passed.

**Task 1.2: Add config and CLI support for ui location**
- Add failing tests for config/flag plumbing:
  - ui location config is injectable through DI.
  - --ui-location correctly overrides configuration.
- Run affected tests and verify expected failure.
- Implement config and flag wiring in main/config packages.
- Re-run tests and module checks:
  - make lint and make test from apps/sonalmod.
- Write summary to doc/implementation/npm-distribution-cicd/summary-task-1.2.md.
- All checks from completion protocol must be passed.

**Task 1.3: Verify dev flow remains unchanged**
- Add failing regression tests/checks proving default startup remains API-only and does not require UI build artifacts.
- Run affected tests and verify expected failure first.
- Implement any required adjustments so go run . start works exactly as today without ui location.
- Re-run tests and module checks:
  - make lint and make test from apps/sonalmod.
- Write summary to doc/implementation/npm-distribution-cicd/summary-task-1.3.md.
- All checks from completion protocol must be passed.

**Task 2.1: Create npm main + platform package structure (tests first)**
- Add failing launcher tests for:
  - OS/arch package resolution.
  - unsupported platform errors.
  - argv/stdin/stdout passthrough.
  - UI dist path resolution via require.resolve('@sonalmod/ui').
- Implement distribution package manifests and launcher for @sonalmod/app.
- Add platform package templates for initial platform matrix (@sonalmod/app-<os>-<arch>).
- Run package tests and smoke checks:
  - launcher executes staged binary.
  - package metadata (bin, os, cpu, optionalDependencies, dependencies) is correct.
- Run repo checks for affected scope: make affected-lint-test.
- Write summary to doc/implementation/npm-distribution-cicd/summary-task-2.1.md.
- All checks from completion protocol must be passed.

**Task 2.2: Create @sonalmod/ui package and wire launcher**
- Add failing packaging checks for:
  - UI dist files are present in @sonalmod/ui package tarball.
  - launcher resolves @sonalmod/ui dist path and passes it as --ui-location to backend.
- Implement @sonalmod/ui package manifest and release staging of apps/sonal-ui/dist into distribution/npm/ui/dist.
- Implement launcher UI path resolution (require.resolve-based, with override/disable support).
- Run affected checks:
  - make lint and make test from apps/sonal-ui.
  - package verification scripts.
- Write summary to doc/implementation/npm-distribution-cicd/summary-task-2.2.md.
- All checks from completion protocol must be passed.

**Task 2.3: Build release Makefile, build config, and scripts for npm artifacts**
- Create build/npm/build.cfg with platform matrix (linux/amd64,linux/arm64,darwin/arm64,windows/amd64).
- Create build/npm/project.json to register build/npm as an Nx project (name: npm-build):
  - test target calls `make test`, overrides dependsOn to [] (no go/npm deps), inputs: scripts/**/*.sh and Makefile.
  - Use doc/implementation/npm-distribution-cicd/ref-build-project-json.json as reference.
- Create build/npm/Makefile following the local-first design:
  - Read platforms from build.cfg using read_config function.
  - Per-platform binary targets using dist/%: FORCE pattern (parallel with make -j4).
  - Sentinel .staged files to track npm package staging completion.
  - Targets: binaries, ui, stage-ui, stage-platform-packages, stage-app, pack, verify, release, test, clean.
  - Entry point: make -C build/npm release VERSION=<semver> (used by both local dev and CI).
  - Use doc/implementation/npm-distribution-cicd/ref-build-makefile.mk as reference.
- Create standalone scripts under build/npm/scripts/:
  - resolve-npm-platform.sh (GOOS/GOARCH → npm suffix/os/cpu, with --self-test).
  - parse-semver-tag.sh (git tag → VERSION, PRERELEASE_ID, NPM_TAG, with --self-test).
  - stage-npm-ui.sh (copies dist, writes package.json with version).
  - stage-platform-package.sh (copies binary, writes package.json with os/cpu/version).
  - stage-app-package.sh (writes launcher, package.json with optionalDependencies and dependencies).
  - verify-packages.sh (validates tarballs, binary execution, launcher resolution; with --self-test).
  - Use doc/implementation/npm-distribution-cicd/ref-resolve-npm-platform.sh and ref-parse-semver-tag.sh as references.
- Run make -C build/npm test (script self-tests) and make -C build/npm release VERSION=0.0.0-dev locally.
- Run repo-level checks: make affected-lint-test.
- Write summary to doc/implementation/npm-distribution-cicd/summary-task-2.3.md.
- All checks from completion protocol must be passed.

**Task 3.1: Add CI release-build workflow (build + verify, no publish)**
- Add failing CI validation path by running `make -C build/npm release VERSION=0.0.0-dev` locally first.
- Create .github/workflows/release-build.yml:
  - setup toolchains (Node, Go) and dependencies
  - run lint/test via `make affected-lint-test`
  - derive VERSION from git tag (strip leading v), export as env var
  - build and verify: `make -C build/npm release VERSION=$VERSION`
    - no build logic inlined in YAML; all logic is in the Makefile and scripts
  - upload tarballs from build/npm/dist/tarballs/ as workflow artifacts
- Ensure PR workflow does not publish anything.
- Verify workflow passes in PR context (via workflow_dispatch dry-run).
- Write summary to doc/implementation/npm-distribution-cicd/summary-task-3.1.md.
- All checks from completion protocol must be passed.

**Task 3.2: Add CI release-publish workflow with stable + pre-release handling**
- Add failing publish dry-run path first:
  - run `build/npm/scripts/parse-semver-tag.sh --tag v1.2.3-alpha.1` locally to validate version parsing and dist-tag selection.
- Create .github/workflows/release-publish.yml with:
  - tag trigger v*
  - version parsing via `build/npm/scripts/parse-semver-tag.sh` (no inline YAML parsing)
  - dist-tag selection (latest, alpha, beta, rc, next) from script output
  - trusted npm publishing/provenance
  - publish order: platform packages -> @sonalmod/ui -> @sonalmod/app
  - GitHub prerelease flag for pre-release tags
- Validate with dry-run and then controlled real publish.
- Write summary to doc/implementation/npm-distribution-cicd/summary-task-3.2.md.
- All checks from completion protocol must be passed.

**Task 3.3: Update user and developer documentation**
- Add failing documentation checklist/validation for new install and release commands.
- Update docs with:
  - npm install and npx usage
  - optional ui location behavior
  - stable vs pre-release tagging and channel rules
  - troubleshooting notes
- Update AGENTS.md if workflow/command architecture references changed.
- Run make affected-lint-test for any code/config touched with docs.
- Write summary to doc/implementation/npm-distribution-cicd/summary-task-3.3.md.
- All checks from completion protocol must be passed.

**Task 3.4: Compress implementation summaries**
- Follow [compress-implementation-summaries.md](/.context/compress-implementation-summaries.md) to compress the implementation summaries.
