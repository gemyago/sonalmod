# Release workflow (GitHub Actions)

Releases use **two workflows**: prepare a **draft** GitHub Release (manual), then **build, publish to npm, and attach assets** when that release is **published**. Lint and tests are **not** repeated here — they already run on `main` before merge.

## Triggers

| Workflow | When it runs |
|----------|----------------|
| **Release Prepare** ([`release-prepare.yml`](workflows/release-prepare.yml)) | **Manual** (`workflow_dispatch`) with input **version** (semver without `v`, e.g. `1.2.3` or `1.2.3-alpha.1`). Creates a **draft** release for `v<version>` with generated notes (`gh`), targeting the default branch so the tag is created when you publish the draft. |
| **Release Publish** ([`release-publish.yml`](workflows/release-publish.yml)) | When a GitHub Release is **published** (`release`, type `released` — including draft → published), **or** **manual** dispatch with input **ref** (prefer tag `v1.2.3`; commit with a single `v*` tag also works). |

Concurrency is limited so overlapping runs for the same tag/ref do not stomp each other’s work.

## Stage 1 — Release Prepare

1. Validate the **version** input.
2. Run `gh release create v<VERSION> --draft --generate-notes --target <default branch> --title v<VERSION>` (requires `contents: write` via `GITHUB_TOKEN`).

Re-run only if you need a new draft; delete or adjust an existing draft in the GitHub UI if the name collides.

## Stage 2 — Release Publish

Runs when the release is **published** (or on manual **ref** for recovery/testing).

1. **Checkout** the release tag (automatic path) or the given **ref** (manual path), then resolve a **semver tag** `v*` for `parse-semver-tag.sh`.
2. Install Node (root **`npm ci` only) and Go; **no** `nx affected` lint or test. The release build runs **`make release`**, whose **`ui`** prerequisite runs **`npm ci` in `apps/sonal-ui`** before the Vite build (see `build/npm/Makefile`).
3. **Build**: `make -C build/npm release VERSION=…` (rebuilds tarballs under `build/npm/dist/tarballs/`).
4. **Semver / npm dist-tag**: `build/npm/scripts/parse-semver-tag.sh` selects e.g. `latest` vs `alpha`.
5. **Publish to npm**: `make -C build/npm publish` with `VERSION` and `NPM_TAG` using **OIDC only** ([Trusted Publishing](#npm-authentication-oidc)).
6. **GitHub Release assets**: `gh release upload <tag> …/*.tgz --clobber` (the release already exists; attach the same tarballs built in step 3).

## npm authentication (OIDC)

CI **does not** use a repository **`NPM_TOKEN`**. Publishing relies on npm **[Trusted Publishers](https://docs.npmjs.com/trusted-publishers)** and short-lived **OIDC** from GitHub.

**Requirements:**

1. **`release-publish.yml`** declares `permissions: id-token: write` and uses **`actions/setup-node`** with `registry-url: https://registry.npmjs.org` so the npm CLI can authenticate in GitHub Actions.
2. On **npmjs.com**, register a **Trusted Publisher** (GitHub Actions) for this **repository**, with workflow file name **`release-publish.yml`** (must match exactly). Allow publishing for every **`@sonalmod/...`** package this workflow publishes.
3. Use a **current Node.js / npm** on the runner (see npm docs for minimum versions supporting Trusted Publishing).

**Local / emergency publishes** from a developer machine still use your normal npm login or a **personal** token in `~/.npmrc` — that is separate from CI and is not stored in this repo.

**If publish fails:** confirm the trusted publisher entry matches repo + workflow filename, branch/ref rules (if any), and package list on npm; confirm the npm CLI on the runner supports OIDC. Provenance is handled automatically for trusted publishing; the Makefile’s `--provenance` in CI is optional/redundant.

## Manual publish (recovery / pipeline test)

- **Automatic path failed after npm publish?** Re-run **Release Publish** with **ref** = the release tag (e.g. `v1.2.3`). Ensure a GitHub Release exists for that tag so `gh release upload` can attach assets.
- **Testing without a release:** Manual dispatch may fail at **Upload assets** if there is no GitHub Release for the resolved tag; npm publish may still have completed in an earlier attempt.

## Where the real build logic lives

The Makefiles and scripts under [`build/npm/`](../build/npm/) define targets such as `release` and `publish`. Root [`AGENTS.md`](../AGENTS.md) summarizes local and CI commands for the npm release pipeline.
