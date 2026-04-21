# Reference: build/npm/Makefile
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
#   make test                                # run script self-tests
#   make clean                               # remove dist/
#
# Usage (CI - GitHub Actions just calls this):
#   make -C build/npm release VERSION=$(GIT_TAG_WITHOUT_V)
#
# All targets work locally without CI.
#
# Adapted from: golang-backend-boilerplate/build/Makefile

.PHONY: FORCE
.SECONDEXPANSION:

# === Config helpers ===
define read_config
$(shell grep -m 1 "^$(2)\s*=\s*" $(1) | cut -d'=' -f2 | sed 's/^[[:space:]]*//;s/[[:space:]]*$$//')
endef

comma := ,

# === Version ===
# Passed explicitly (e.g. VERSION=1.2.3) or derived from git tag.
# Strip leading 'v' if present. Falls back to 0.0.0-dev for local dev.
VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null | sed 's/^v//' || echo "0.0.0-dev")

# === Platform matrix (from build.cfg) ===
# Format: GOOS/GOARCH pairs, comma-separated
# Example: linux/amd64,linux/arm64,darwin/arm64,windows/amd64
platforms    := $(call read_config,build.cfg,platforms)
platform_list := $(subst $(comma), ,$(platforms))

# === Directory layout ===
REPO_ROOT  := $(shell git rev-parse --show-toplevel)
DIST_DIR   := $(CURDIR)/dist
GO_DIST    := $(DIST_DIR)/go       # dist/go/linux/amd64/sonalmod
NPM_STAGE  := $(DIST_DIR)/npm     # dist/npm/ui/, dist/npm/app/, dist/npm/app-linux-x64/
TARBALLS   := $(DIST_DIR)/tarballs # dist/tarballs/*.tgz

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
	@echo "[build] $(GOOS)/$(GOARCH)"
	@mkdir -p $(GO_DIST)/$(GOOS)/$(GOARCH)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -C $(REPO_ROOT)/apps/sonalmod \
		-tags=release \
		-ldflags="-s -w" \
		-o $(GO_DIST)/$(GOOS)/$(GOARCH)/sonalmod$(BINEXT) \
		.

.PHONY: binaries
binaries:
	@$(MAKE) -j4 $(go_dist_targets)

# ============================================================
# UI Build
# ============================================================

.PHONY: ui
ui:
	npx nx build sonal-ui

# ============================================================
# npm Package Staging
# ============================================================
# Sentinel .staged files track completion so Make can detect up-to-date state.

# Stage @sonalmod/ui
$(NPM_STAGE)/ui/.staged: ui
	@mkdir -p $(NPM_STAGE)/ui
	scripts/stage-npm-ui.sh \
		--src $(REPO_ROOT)/apps/sonal-ui/dist \
		--version "$(VERSION)" \
		--output $(NPM_STAGE)/ui
	@touch $@

.PHONY: stage-ui
stage-ui: $(NPM_STAGE)/ui/.staged

# Stage per-platform packages (@sonalmod/app-linux-x64 etc.)
# Uses scripts/resolve-npm-platform.sh to map GOOS/GOARCH -> npm suffix.
.PHONY: stage-platform-packages
stage-platform-packages: binaries
	@for platform in $(platform_list); do \
		goos=$$(echo $$platform | cut -d/ -f1); \
		goarch=$$(echo $$platform | cut -d/ -f2); \
		npm_suffix=$$(scripts/resolve-npm-platform.sh --goos $$goos --goarch $$goarch --format suffix); \
		echo "[stage] @sonalmod/app-$$npm_suffix"; \
		scripts/stage-platform-package.sh \
			--goos $$goos \
			--goarch $$goarch \
			--version "$(VERSION)" \
			--go-dist "$(GO_DIST)" \
			--output "$(NPM_STAGE)/app-$$npm_suffix"; \
	done

# Stage @sonalmod/app (launcher + package metadata with optionalDependencies and dependencies)
.PHONY: stage-app
stage-app: stage-ui stage-platform-packages
	scripts/stage-app-package.sh \
		--version "$(VERSION)" \
		--platforms "$(platforms)" \
		--output "$(NPM_STAGE)/app"

# ============================================================
# Pack (npm pack -> tarballs)
# ============================================================

.PHONY: pack
pack: stage-app
	@mkdir -p $(TARBALLS)
	@for pkg_dir in $(NPM_STAGE)/*/; do \
		echo "[pack] $$pkg_dir"; \
		npm pack "$$pkg_dir" --pack-destination $(TARBALLS); \
	done

# ============================================================
# Verification
# ============================================================

.PHONY: verify
verify: pack
	scripts/verify-packages.sh \
		--tarballs-dir $(TARBALLS) \
		--version "$(VERSION)" \
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
	go run -C $(REPO_ROOT)/apps/sonalmod . start \
		--ui-location $(REPO_ROOT)/apps/sonal-ui/dist

# ============================================================
# Script self-tests (local dev and CI)
# ============================================================

.PHONY: test
test:
	@echo "Running script self-tests..."
	scripts/resolve-npm-platform.sh --self-test
	scripts/parse-semver-tag.sh --self-test
	@echo "All script tests passed."

# ============================================================
# Clean
# ============================================================

.PHONY: clean
clean:
	rm -rf $(DIST_DIR)
