# Contributing to Sonalmod

## Project Setup

Please have the following tools installed: 
* [direnv](https://github.com/direnv/direnv) 
* [nvm](https://github.com/nvm-sh/nvm) - to setup node
* Many modules are golang based, so have [gobrew](https://github.com/kevincobain2000/gobrew#install-or-update) installed.

## AI Frameworks

[OpenSpec](https://github.com/fission-ai/openspec) - good for structured flow. Use `openspec init`. Not committing to the repo for now.

[GSD](https://github.com/gsd-build/get-shit-done) - not sure if it will stick around, but for now it is what we use:
```sh
# Use your agent if needed
npx get-shit-done-cc@latest --local --codex --opencode
```

Also tried:
- BMAD - very complex, so we skipped it for now.

## Product Docs

Canonical domain vocabulary for planning, design, and copy: [docs/domain-terminology.md](./docs/domain-terminology.md).

## Typical Monorepo Tasks

If not using direnv and nvm, make sure to have go and node of a correct version as per [.nvmrc](.nvmrc) and [go.work](go.work) files.

Make sure to install deps from root of the monorepo:
```bash
# Install root deps first
npm i

# Then use nx to setup per module deps
npx nx run-many -t install-deps
```

This project uses Nx, some quick cheat-sheet:
```bash
# Run tests of specific module:
npx nx test sonalmod

# Run tests bypassing cache:
npx nx test sonalmod --skipNxCache

# Run all tests
npx nx run-many -t test

# Run all lint
npx nx run-many -t lint

# Run affected tasks (e.g lint):
npx nx affected --target=lint

# To see all available tasks for a specific module, use:
nx show project sonalmod --json
```

To run all affected lint and tests, use `make affected-lint-test`

## Go modules specific instructions

If you want more control over deps management in go modules.

Install/Update dependencies (run from go modules): 
```sh
# Install
go mod download
go get -u tool
go install tool

# Update:
go get -u ./... && go mod tidy
```

## Run locally

```bash
# Start backend in a separate terminal
# Install deps as per above instructions for go modules
cd apps/sonalmod
go run ./cmd/sonalmod start

# Start frontend in a separate terminal
cd apps/sonal-ui
npm i
npm run dev

# Start everything in one terminal
nx run-many -t dev
```

Frontend host/port: http://localhost:5173
Backend host/port: http://localhost:8080

### Combined local mode (backend serves built UI)

To run the backend serving the built UI locally:
```bash
# Build UI and start backend with UI location (from repo root)
make -C build/npm local-run
```

This builds the UI with Nx and starts the backend with `--ui-location apps/sonal-ui/dist`.

## Release Build

This is mostly to test the release build pipeline locally.

### Run the full release pipeline locally

```bash
# Full release build (stable version)
make -C build/npm release VERSION=1.2.3

# Pre-release build
make -C build/npm release VERSION=1.2.3-alpha.1

# Development build (default version 0.0.0-dev)
make -C build/npm release
```

This single command drives the full pipeline: UI build, cross-platform Go binary compilation, npm package staging, `npm pack`, and tarball verification. Tarballs are written to `build/npm/dist/tarballs/`.

### Individual build steps

```bash
make -C build/npm binaries              # cross-compile Go binaries only
make -C build/npm ui                    # build the UI only
make -C build/npm stage-ui              # stage @sonalmod/ui package
make -C build/npm stage-platform-packages  # stage per-platform binary packages
make -C build/npm stage-app             # stage @sonalmod/app launcher package
make -C build/npm pack                  # npm pack all staged packages -> tarballs
make -C build/npm verify                # verify tarballs (contents + smoke tests)
make -C build/npm test                  # run script self-tests and launcher tests
make -C build/npm clean                 # remove build/npm/dist/
```

### Script self-tests

Each build script supports `--self-test` for standalone validation:
```bash
build/npm/scripts/resolve-npm-platform.sh --self-test
build/npm/scripts/parse-semver-tag.sh --self-test
```

### CI/CD release workflows

Releases are triggered by creating a release in GitHub.

Pre-release tags (e.g. `v1.2.3-alpha.1`) are published to the `alpha` dist-tag and marked as GitHub pre-releases.