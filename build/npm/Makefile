# build/npm/Makefile
#
# Orchestrates the full npm release build:
#   - Cross-platform Go binary matrix (driven by build.cfg)
#   - UI dist staging
#   - npm package staging (@sonalmod/ui, @sonalmod/app-<os>-<arch>, @sonalmod/app)
#   - npm pack + tarball verification
#
# Usage (local dev):
#   make release VERSION=1.2.3               # full release build (stable)
#   make release VERSION=1.2.3-alpha.1       # pre-release build
#   make binaries                            # just build Go binaries
#   make test                                # run all tests (script self-tests + launcher tests)
#   make clean                               # remove dist/
#
# Usage (CI - GitHub Actions just calls this):
#   make -C build/npm release VERSION=$(GIT_TAG_WITHOUT_V)
#
# Publish downloaded tarballs (CI: after artifact download; requires NPM_TAG):
#   make -C build/npm publish VERSION=1.2.3 NPM_TAG=latest
#   npm --provenance: default when CI is set; override NPM_PUBLISH_EXTRA (e.g. empty to disable).
#
# Unpublish a version from npm (reverse order of publish; reads dist/VERSION — set VERSION=…):
#   make -C build/npm unpublish VERSION=1.2.3
#   Override NPM_UNPUBLISH like NPM_PUBLISH; NPM_UNPUBLISH_NOOP=1 to log only.

.PHONY: FORCE
# Empty recipe so prerequisites of FORCE (e.g. go matrix, publish-tgz-*) always rebuild.
FORCE:

.SECONDEXPANSION:

# === Config helpers ===
define read_config
$(shell grep -m 1 "^$(2)\s*=\s*" $(1) | cut -d'=' -f2 | sed 's/^[[:space:]]*//;s/[[:space:]]*$$//')
endef

comma := ,

# Directory containing this Makefile — use for parse-time paths so `make -f build/npm/Makefile`
# from the repo root still reads build.cfg and resolves platform suffixes (same as `make -C build/npm`).
THIS_MK_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))

# === Version ===
# Optional VERSION=… (CLI or environment) seeds $(DIST_DIR)/VERSION when building it; otherwise
# the file is derived from .git-meta/ref (refs/tags/* → tag sans leading v; else dev_version from build.cfg).
# Packaging/publish recipes read semver with the shell at recipe time, e.g.
#   $$(cat $(DIST_DIR)/VERSION)
# (not a Make variable — avoids expanding before $(DIST_DIR)/VERSION exists). Prerequisite
# $(DIST_DIR)/VERSION where needed.
VERSION_EXPLICIT := $(filter command line environment,$(origin VERSION))

# === Platform matrix (from build.cfg) ===
# Format: GOOS/GOARCH pairs, comma-separated
# Example: linux/amd64,linux/arm64,darwin/arm64,windows/amd64
platforms    := $(call read_config,$(THIS_MK_DIR)build.cfg,platforms)
dev_version  := $(call read_config,$(THIS_MK_DIR)build.cfg,dev_version)
platform_list := $(subst $(comma), ,$(platforms))
# npm suffix per matrix row (linux-x64, …) — same paths as sonalmod-app-<suffix>-<VERSION>.tgz
npm_suffix_list := $(foreach p,$(platform_list),$(shell $(THIS_MK_DIR)scripts/resolve-npm-platform.sh --goos $(word 1,$(subst /, ,$(p))) --goarch $(word 2,$(subst /, ,$(p))) --format suffix))
PUBLISH_PLATFORM_TARGETS := $(addprefix publish-tgz-platform-,$(npm_suffix_list))

# === Directory layout ===
REPO_ROOT  := $(shell git rev-parse --show-toplevel)
DIST_DIR   := dist
# Go cross-compile output root: dist/go/<goos>/<goarch>/sonalmod[.exe]
GO_DIST := $(DIST_DIR)/go
# Staged npm packages: dist/npm/ui/, dist/npm/app/, dist/npm/app-<platform>/
NPM_STAGE := $(DIST_DIR)/npm
# npm pack output: dist/tarballs/*.tgz
TARBALLS := $(DIST_DIR)/tarballs

$(DIST_DIR):
	@mkdir -p $@

# COMMIT_SHA is used since github may checkout on the temporary PR branch
# that will have a different commit SHA than the merge commit SHA
.git-meta/HEAD: FORCE
	@mkdir -p .git-meta
	@if [ -n "$$GIT_COMMIT_SHA" ]; then \
		echo "$$GIT_COMMIT_SHA" | cut -c1-7 > $@.tmp; \
	else \
		git rev-parse --short HEAD > $@.tmp; \
	fi
	@if cmp -s $@ $@.tmp; then \
		rm $@.tmp; \
	else \
		mv $@.tmp $@; \
	fi

# COMMIT_REF is used since github may checkout on the temporary PR branch
# that will always have HEAD as a ref name
.git-meta/ref: .git-meta/HEAD
	@if [ -n "$$GIT_REF" ]; then \
		echo "$$GIT_REF" > $@.tmp; \
	else \
		git rev-parse --abbrev-ref HEAD > $@.tmp; \
	fi
	@cmp -s $@.tmp $@ || mv $@.tmp $@
	@rm -f $@.tmp

# Written to dist/ for consumers. If VERSION is passed (CLI or env), use it (strip leading v).
# Otherwise derived from .git-meta/ref so CI can use GIT_REF=refs/tags/<tag> (GitHub sends full
# ref paths). Non-tag refs -> dev_version (build.cfg).
# FORCE: recipe must run when only VERSION=… changes (VERSION is not a prereq Make can compare).
$(DIST_DIR)/VERSION: .git-meta/ref $(DIST_DIR) FORCE
	@if [ -z "$(VERSION_EXPLICIT)" ]; then \
		ref=$$(cat .git-meta/ref); \
		case $$ref in \
		refs/tags/*) printf '%s\n' "$${ref#refs/tags/}" | sed 's/^v//' > $@.tmp;; \
		*) printf '%s\n' "$(dev_version)" > $@.tmp;; \
		esac; \
	else \
		printf '%s\n' "$(VERSION)" | sed 's/^v//' > $@.tmp; \
	fi
	@cmp -s $@.tmp $@ || mv $@.tmp $@
	@rm -f $@.tmp

# ============================================================
# Go Binary Matrix Build
# ============================================================
# Each GOOS/GOARCH pair becomes a Make target: dist/go/linux/amd64
# FORCE ensures always-rebuild (binaries never cached by Make).
# Build in parallel: make -j4 $(go_dist_targets)
# ============================================================

go_dist_targets := $(patsubst %,$(GO_DIST)/%,$(platform_list))

$(GO_DIST)/%: FORCE
	$(eval GOOS   := $(word 1,$(subst /, ,$*)))
	$(eval GOARCH := $(word 2,$(subst /, ,$*)))
	$(eval BINEXT := $(if $(filter windows,$(GOOS)),.exe,))
	@echo "[build] $(GOOS)/$(GOARCH) (with ext '$(BINEXT)') in $(REPO_ROOT)"
	@mkdir -p $(GO_DIST)/$(GOOS)/$(GOARCH)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -C $(REPO_ROOT)/apps/sonalmod \
		-tags=release \
		-ldflags="-s -w" \
		-o $(CURDIR)/$(GO_DIST)/$(GOOS)/$(GOARCH)/sonalmod$(BINEXT) \
		./cmd/sonalmod

.PHONY: binaries
binaries:
	@echo "[build binaries] $(go_dist_targets)"
	@$(MAKE) -j4 $(go_dist_targets)

# ============================================================
# UI Build
# ============================================================

.PHONY: ui
# npm ci for this OS's lockfile graph, then Vite. Avoids nx-only builds replaying cache while
# @rolldown/binding-* is missing or from another platform (breaks make stage-ui).
ui:
	cd $(REPO_ROOT)/apps/sonal-ui && npm ci && npm run build

# ============================================================
# npm Package Staging
# ============================================================
# Sentinel .staged files track completion so Make can detect up-to-date state.

# Stage @sonalmod/ui
$(NPM_STAGE)/ui/.staged: ui $(DIST_DIR)/VERSION
	@mkdir -p $(NPM_STAGE)/ui
	scripts/stage-npm-ui.sh \
		--src $(REPO_ROOT)/apps/sonal-ui/dist \
		--version "$$(cat $(DIST_DIR)/VERSION)" \
		--output $(NPM_STAGE)/ui
	@touch $@

.PHONY: stage-ui
stage-ui: $(NPM_STAGE)/ui/.staged

# Stage per-platform packages (@sonalmod/app-linux-x64 etc.)
# Uses scripts/resolve-npm-platform.sh to map GOOS/GOARCH -> npm suffix.
.PHONY: stage-platform-packages
stage-platform-packages: binaries $(DIST_DIR)/VERSION
	@for platform in $(platform_list); do \
		goos=$$(echo $$platform | cut -d/ -f1); \
		goarch=$$(echo $$platform | cut -d/ -f2); \
		npm_suffix=$$(scripts/resolve-npm-platform.sh --goos $$goos --goarch $$goarch --format suffix); \
		echo "[stage] @sonalmod/app-$$npm_suffix"; \
		scripts/stage-platform-package.sh \
			--goos $$goos \
			--goarch $$goarch \
			--version "$$(cat $(DIST_DIR)/VERSION)" \
			--go-dist "$(GO_DIST)" \
			--output "$(NPM_STAGE)/app-$$npm_suffix"; \
	done

# Stage @sonalmod/app (launcher + package metadata with optionalDependencies and dependencies)
.PHONY: stage-app
stage-app: stage-ui stage-platform-packages $(DIST_DIR)/VERSION
	scripts/stage-app-package.sh \
		--version "$$(cat $(DIST_DIR)/VERSION)" \
		--platforms "$(platforms)" \
		--output "$(NPM_STAGE)/app"

# ============================================================
# Pack (npm pack -> tarballs)
# ============================================================

.PHONY: pack
pack: stage-app $(DIST_DIR)/VERSION
	@mkdir -p $(TARBALLS)
	@printf '%s\n' "$$(cat $(DIST_DIR)/VERSION)" > $(TARBALLS)/VERSION
	@# Clear old *.tgz so verify cannot match a stale launcher; Make exports CLI vars — unset NPM_TAG so npm pack is isolated from publish-only tags.
	@find $(TARBALLS) -maxdepth 1 -name '*.tgz' -delete
	@for pkg_dir in $(NPM_STAGE)/*/; do \
		echo "[pack] $$pkg_dir"; \
		unset NPM_TAG; npm pack "$$pkg_dir" --pack-destination $(TARBALLS); \
	done

# ============================================================
# Verification
# ============================================================

.PHONY: verify
verify: pack $(DIST_DIR)/VERSION
	scripts/verify-packages.sh \
		--tarballs-dir $(TARBALLS) \
		--version "$$(cat $(DIST_DIR)/VERSION)" \
		--platforms "$(platforms)"

# ============================================================
# Release (full pipeline entry point, called by CI)
# ============================================================

.PHONY: release
release: verify
	@echo "Release artifacts ready:"
	@ls -lh $(TARBALLS)

# ============================================================
# Local development helper
# ============================================================
# Build UI and run backend with UI served (local combined mode)

.PHONY: local-run
local-run: ui
	go run -C $(REPO_ROOT)/apps/sonalmod ./cmd/sonalmod start \
		--ui-location $(REPO_ROOT)/apps/sonal-ui/dist

# ============================================================
# Publish to npm (CI: tarballs from `make release` in the same job)
# ============================================================
# Order: platform packages -> @sonalmod/ui -> @sonalmod/app (dependencies / optionalDeps).
# Requires NPM_TAG (npm dist-tag).
NPM_PUBLISH_EXTRA ?= $(if $(CI),--provenance,)

NPM_PUBLISH ?= $(if $(NPM_PUBLISH_NOOP),@echo "[noop] npm publish", npm publish)

.PHONY: publish guard-npm-tag publish-platform-packages publish-ui publish-app

guard-npm-tag:
	$(if $(NPM_TAG),,$(error NPM_TAG is required for publish (e.g. NPM_TAG=latest)))

# $(eval GOOS   := $(word 1,$(subst /, ,$*)))
# $(eval GOARCH := $(word 2,$(subst /, ,$*)))
# $(eval BINEXT := $(if $(filter windows,$(GOOS)),.exe,))
# @echo "[build] $(GOOS)/$(GOARCH) (with ext '$(BINEXT)') in $(REPO_ROOT)"

# Pattern must not use prefix publish-platform-* (would match publish-platform-packages).
publish-tgz-platform-%: guard-npm-tag $(DIST_DIR)/VERSION
	@printf '[publish] %s/sonalmod-app-%s-%s.tgz -> dist-tag=%s\n' '$(TARBALLS)' '$*' "$$(cat $(DIST_DIR)/VERSION)" '$(NPM_TAG)'
	test -f "$(TARBALLS)/sonalmod-app-$*-$$(cat $(DIST_DIR)/VERSION).tgz"
	$(NPM_PUBLISH) "$(TARBALLS)/sonalmod-app-$*-$$(cat $(DIST_DIR)/VERSION).tgz" --tag "$(NPM_TAG)" $(NPM_PUBLISH_EXTRA) --access public

publish-platform-packages: $(PUBLISH_PLATFORM_TARGETS)
	@echo "publish-platform-packages: $(PUBLISH_PLATFORM_TARGETS)"

publish-ui: guard-npm-tag publish-platform-packages
	@printf '[publish] %s/sonalmod-ui-%s.tgz -> dist-tag=%s\n' '$(TARBALLS)' "$$(cat $(DIST_DIR)/VERSION)" '$(NPM_TAG)'
	test -f "$(TARBALLS)/sonalmod-ui-$$(cat $(DIST_DIR)/VERSION).tgz"
	$(NPM_PUBLISH) "$(TARBALLS)/sonalmod-ui-$$(cat $(DIST_DIR)/VERSION).tgz" --tag "$(NPM_TAG)" $(NPM_PUBLISH_EXTRA) --access public

publish-app: guard-npm-tag publish-ui
	@printf '[publish] %s/sonalmod-app-%s.tgz -> dist-tag=%s\n' '$(TARBALLS)' "$$(cat $(DIST_DIR)/VERSION)" '$(NPM_TAG)'
	test -f "$(TARBALLS)/sonalmod-app-$$(cat $(DIST_DIR)/VERSION).tgz"
	$(NPM_PUBLISH) "$(TARBALLS)/sonalmod-app-$$(cat $(DIST_DIR)/VERSION).tgz" --tag "$(NPM_TAG)" $(NPM_PUBLISH_EXTRA) --access public

publish: publish-app

# ============================================================
# Unpublish from npm (reverse of publish: app -> ui -> platform packages)
# ============================================================
# Removes the semver in $(DIST_DIR)/VERSION from the registry (same scope/@ names as publish).

NPM_UNPUBLISH ?= $(if $(NPM_UNPUBLISH_NOOP),@echo "[noop] npm unpublish", npm unpublish)

UNPUBLISH_PLATFORM_TARGETS := $(addprefix unpublish-tgz-platform-,$(npm_suffix_list))

.PHONY: unpublish unpublish-platform-packages unpublish-ui unpublish-app

# Pattern must not use prefix unpublish-platform-* (would match unpublish-platform-packages).
unpublish-tgz-platform-%: unpublish-ui $(DIST_DIR)/VERSION
	@printf '[unpublish] @sonalmod/app-%s@%s\n' '$*' "$$(cat $(DIST_DIR)/VERSION)"
	$(NPM_UNPUBLISH) "@sonalmod/app-$*@$$(cat $(DIST_DIR)/VERSION)"

unpublish-platform-packages: $(UNPUBLISH_PLATFORM_TARGETS)
	@echo "unpublish-platform-packages: $(UNPUBLISH_PLATFORM_TARGETS)"

unpublish-ui: unpublish-app $(DIST_DIR)/VERSION
	@printf '[unpublish] @sonalmod/ui@%s\n' "$$(cat $(DIST_DIR)/VERSION)"
	$(NPM_UNPUBLISH) "@sonalmod/ui@$$(cat $(DIST_DIR)/VERSION)"

unpublish-app: $(DIST_DIR)/VERSION
	@printf '[unpublish] @sonalmod/app@%s\n' "$$(cat $(DIST_DIR)/VERSION)"
	$(NPM_UNPUBLISH) "@sonalmod/app@$$(cat $(DIST_DIR)/VERSION)"

unpublish: unpublish-platform-packages

# ============================================================
# Script self-tests and launcher tests (local dev and CI)
# ============================================================

.PHONY: test
test:
	@echo "Running script self-tests..."
	scripts/test-makefile.sh
	scripts/resolve-npm-platform.sh --self-test
	scripts/parse-semver-tag.sh --self-test
	scripts/stage-npm-ui.sh --self-test
	scripts/stage-platform-package.sh --self-test
	scripts/stage-app-package.sh --self-test
	scripts/verify-packages.sh --self-test
	@echo "Running launcher tests..."
	node --test app/bin/sonalmod.test.mjs
	@echo "Running packaging tests..."
	node --test ui/package.test.mjs
	@echo "All tests passed."

# ============================================================
# Clean
# ============================================================

.PHONY: clean
clean:
	rm -rf $(DIST_DIR)
