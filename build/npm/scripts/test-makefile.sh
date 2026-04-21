#!/usr/bin/env bash
# build/npm/scripts/test-makefile.sh
#
# Runs `make clean verify` once (full pipeline), then asserts dist/ layout: Go binary matrix,
# staged UI, per-platform packages, @sonalmod/app, and tarballs.
# Also checks dist/VERSION: explicit VERSION=… writes that value; without it, ref-derived content.
#
# Usage:
#   test-makefile.sh
#   test-makefile.sh --help
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_NPM_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

usage() {
  echo "Usage: $0"
  echo "  Runs make dist/VERSION after clean, then make clean verify, then checks staged packages, dist/go/, and tarballs."
}

read_platforms() {
  grep -m 1 "^platforms[[:space:]]*=" "${BUILD_NPM_DIR}/build.cfg" \
    | cut -d'=' -f2- \
    | sed 's/^[[:space:]]*//;s/[[:space:]]*$//'
}

expected_binary() {
  local goos="$1"
  if [[ "$goos" == "windows" ]]; then
    echo "sonalmod.exe"
  else
    echo "sonalmod"
  fi
}

run_makefile_dist_version_explicit_override_check() {
  echo "Checking dist/VERSION when VERSION is passed explicitly..."
  local got

  make -C "${BUILD_NPM_DIR}" clean
  make -C "${BUILD_NPM_DIR}" dist/VERSION VERSION=9.8.7-explicit-test
  read -r got < "${BUILD_NPM_DIR}/dist/VERSION" || true
  if [[ "$got" != "9.8.7-explicit-test" ]]; then
    echo "FAIL: dist/VERSION with VERSION=9.8.7-explicit-test expected '9.8.7-explicit-test', got '${got}'" >&2
    return 1
  fi
  echo "  PASS: dist/VERSION honors explicit VERSION=…"

  make -C "${BUILD_NPM_DIR}" clean
  make -C "${BUILD_NPM_DIR}" dist/VERSION VERSION=v4.5.6-strip-v
  read -r got < "${BUILD_NPM_DIR}/dist/VERSION" || true
  if [[ "$got" != "4.5.6-strip-v" ]]; then
    echo "FAIL: dist/VERSION with VERSION=v4.5.6-strip-v expected '4.5.6-strip-v', got '${got}'" >&2
    return 1
  fi
  echo "  PASS: dist/VERSION strips leading v from explicit VERSION"
}

run_makefile_dist_version_file_check() {
  echo "Checking dist/VERSION (make clean, then make dist/VERSION without VERSION=)..."
  make -C "${BUILD_NPM_DIR}" clean
  make -C "${BUILD_NPM_DIR}" dist/VERSION
  [[ -f "${BUILD_NPM_DIR}/dist/VERSION" ]] || {
    echo "FAIL: missing ${BUILD_NPM_DIR}/dist/VERSION" >&2
    return 1
  }
  echo "  PASS: dist/VERSION exists (ref-derived when VERSION not passed)"
}

run_makefile_binary_check() {
  echo "Checking Go binary matrix (dist/go/ after make verify)..."
  local platforms_csv failures=0
  platforms_csv="$(read_platforms)"
  if [[ -z "$platforms_csv" ]]; then
    echo "FAIL: could not read platforms from build.cfg" >&2
    return 1
  fi

  local platform goos goarch bin path
  # shellcheck disable=SC2086
  for platform in ${platforms_csv//,/ }; do
    goos="${platform%%/*}"
    goarch="${platform##*/}"
    bin="$(expected_binary "$goos")"
    path="${BUILD_NPM_DIR}/dist/go/${goos}/${goarch}/${bin}"
    if [[ -f "$path" ]]; then
      echo "  PASS: ${platform} -> ${path}"
    else
      echo "  FAIL: missing ${platform} binary at ${path}" >&2
      failures=$((failures + 1))
    fi
  done

  if [[ "$failures" -eq 0 ]]; then
    echo "  PASS: makefile binary matrix check complete."
    return 0
  fi
  echo "test-makefile: ${failures} binary check(s) failed." >&2
  return 1
}

run_makefile_stage_ui_check() {
  echo "Checking staged @sonalmod/ui (dist/npm/ui after make verify)..."
  local out="${BUILD_NPM_DIR}/dist/npm/ui"
  [[ -f "${out}/.staged" ]] || {
    echo "FAIL: missing ${out}/.staged" >&2
    return 1
  }
  [[ -f "${out}/package.json" ]] || {
    echo "FAIL: missing staged package.json" >&2
    return 1
  }
  [[ -f "${out}/dist/index.html" ]] || {
    echo "FAIL: missing staged dist/index.html" >&2
    return 1
  }
  echo "  PASS: staged UI layout OK"
}

run_makefile_stage_platform_packages_check() {
  echo "Checking per-platform packages (dist/npm/app-* after make verify)..."
  local platforms_csv failures=0
  platforms_csv="$(read_platforms)"
  if [[ -z "$platforms_csv" ]]; then
    echo "FAIL: could not read platforms from build.cfg" >&2
    return 1
  fi

  local platform goos goarch npm_suffix out bin
  # shellcheck disable=SC2086
  for platform in ${platforms_csv//,/ }; do
    goos="${platform%%/*}"
    goarch="${platform##*/}"
    npm_suffix="$("${SCRIPT_DIR}/resolve-npm-platform.sh" --goos "$goos" --goarch "$goarch" --format suffix)"
    out="${BUILD_NPM_DIR}/dist/npm/app-${npm_suffix}"
    bin="$(expected_binary "$goos")"
    if [[ -f "${out}/package.json" ]]; then
      echo "  PASS: ${platform} -> ${out}/package.json"
    else
      echo "  FAIL: missing staged package.json for ${platform} at ${out}/package.json" >&2
      failures=$((failures + 1))
      continue
    fi
    if [[ -f "${out}/bin/${bin}" ]]; then
      echo "  PASS: ${platform} -> ${out}/bin/${bin}"
    else
      echo "  FAIL: missing binary for ${platform} at ${out}/bin/${bin}" >&2
      failures=$((failures + 1))
    fi
  done

  if [[ "$failures" -eq 0 ]]; then
    echo "  PASS: per-platform package layout OK"
    return 0
  fi
  echo "test-makefile: ${failures} stage-platform-packages check(s) failed." >&2
  return 1
}

run_makefile_stage_app_check() {
  echo "Checking @sonalmod/app staging (dist/npm/app after make verify)..."
  local out="${BUILD_NPM_DIR}/dist/npm/app"
  local failures=0

  [[ -f "${out}/package.json" ]] || {
    echo "FAIL: missing staged ${out}/package.json" >&2
    return 1
  }
  [[ -f "${out}/bin/sonalmod.js" ]] || {
    echo "FAIL: missing launcher ${out}/bin/sonalmod.js" >&2
    return 1
  }

  if ! grep -q '"name"[[:space:]]*:[[:space:]]*"@sonalmod/app"' "${out}/package.json"; then
    echo "FAIL: expected package name @sonalmod/app in ${out}/package.json" >&2
    return 1
  fi

  local pkg_version
  pkg_version=$(grep -m1 '"version"' "${out}/package.json" | sed 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
  if [[ -z "$pkg_version" ]]; then
    echo "FAIL: could not read version from staged package.json" >&2
    return 1
  fi

  if ! grep -q "\"@sonalmod/ui\":[[:space:]]*\"${pkg_version}\"" "${out}/package.json"; then
    echo "FAIL: expected @sonalmod/ui dependency at version ${pkg_version}" >&2
    return 1
  fi
  echo "  PASS: dependency @sonalmod/ui@${pkg_version}"

  local platforms_csv platform goos goarch npm_suffix
  platforms_csv="$(read_platforms)"
  if [[ -z "$platforms_csv" ]]; then
    echo "FAIL: could not read platforms from build.cfg" >&2
    return 1
  fi

  # shellcheck disable=SC2086
  for platform in ${platforms_csv//,/ }; do
    goos="${platform%%/*}"
    goarch="${platform##*/}"
    npm_suffix="$("${SCRIPT_DIR}/resolve-npm-platform.sh" --goos "$goos" --goarch "$goarch" --format suffix)"
    if grep -q "\"@sonalmod/app-${npm_suffix}\":[[:space:]]*\"${pkg_version}\"" "${out}/package.json"; then
      echo "  PASS: optionalDependency @sonalmod/app-${npm_suffix}@${pkg_version}"
    else
      echo "  FAIL: missing optionalDependency @sonalmod/app-${npm_suffix}@${pkg_version} for ${platform}" >&2
      failures=$((failures + 1))
    fi
  done

  if [[ "$failures" -ne 0 ]]; then
    echo "test-makefile: ${failures} stage-app check(s) failed." >&2
    return 1
  fi
  echo "  PASS: @sonalmod/app staging OK"
}

run_makefile_publish_dry_run_check() {
  echo "Checking publish target (dry-run)..."
  # Ensures Makefile publish graph and platform tarball stems resolve (no bash loops in CI).
  if ! make -C "${BUILD_NPM_DIR}" -n publish NPM_TAG=test VERSION=1.0.0 >/dev/null 2>&1; then
    echo "FAIL: make -n publish NPM_TAG=test VERSION=1.0.0 failed" >&2
    return 1
  fi
  echo "  PASS: publish target dry-run OK"
}

run_makefile_unpublish_dry_run_check() {
  echo "Checking unpublish target (dry-run)..."
  # Ensures Makefile unpublish graph and platform package names resolve (reverse of publish).
  if ! make -C "${BUILD_NPM_DIR}" -n unpublish VERSION=1.0.0 >/dev/null 2>&1; then
    echo "FAIL: make -n unpublish VERSION=1.0.0 failed" >&2
    return 1
  fi
  echo "  PASS: unpublish target dry-run OK"
}

run_makefile_pack_check() {
  echo "Checking npm pack output (dist/tarballs after make verify)..."
  local npm_stage="${BUILD_NPM_DIR}/dist/npm"
  local tarballs_dir="${BUILD_NPM_DIR}/dist/tarballs"
  [[ -d "$tarballs_dir" ]] || {
    echo "FAIL: missing pack output dir ${tarballs_dir}" >&2
    return 1
  }
  [[ -d "$npm_stage" ]] || {
    echo "FAIL: missing staged npm dir ${npm_stage}" >&2
    return 1
  }
  local staged_count tarball_count
  staged_count="$(find "$npm_stage" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d '[:space:]')"
  tarball_count="$(find "$tarballs_dir" -maxdepth 1 -name '*.tgz' -type f | wc -l | tr -d '[:space:]')"
  if [[ -z "$staged_count" || "$staged_count" -eq 0 ]]; then
    echo "FAIL: no staged packages under ${npm_stage}" >&2
    return 1
  fi
  if [[ "$tarball_count" != "$staged_count" ]]; then
    echo "FAIL: expected ${staged_count} tarball(s) (one per staged package under dist/npm/), found ${tarball_count} in ${tarballs_dir}" >&2
    return 1
  fi
  [[ -f "${tarballs_dir}/VERSION" ]] || {
    echo "FAIL: missing ${tarballs_dir}/VERSION" >&2
    return 1
  }
  [[ -f "${BUILD_NPM_DIR}/dist/VERSION" ]] || {
    echo "FAIL: missing ${BUILD_NPM_DIR}/dist/VERSION" >&2
    return 1
  }
  local expected_version actual_version
  expected_version=$(grep -m1 '"version"' "${npm_stage}/app/package.json" | sed 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
  read -r actual_version < "${tarballs_dir}/VERSION" || true
  if [[ -z "$expected_version" || "$actual_version" != "$expected_version" ]]; then
    echo "FAIL: ${tarballs_dir}/VERSION must match staged @sonalmod/app version (got '${actual_version}', expected '${expected_version}')" >&2
    return 1
  fi
  # dist/VERSION is the canonical semver; pack copies the same value into tarballs/VERSION.
  if [[ ! -s "${BUILD_NPM_DIR}/dist/VERSION" ]]; then
    echo "FAIL: dist/VERSION missing or empty" >&2
    return 1
  fi
  echo "  PASS: tarball count matches staged packages (${tarball_count} in ${tarballs_dir}); VERSION file OK"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

run_makefile_dist_version_explicit_override_check
run_makefile_dist_version_file_check

echo "Invoking: make -C ${BUILD_NPM_DIR} clean verify"
make -C "${BUILD_NPM_DIR}" clean verify

run_makefile_stage_ui_check
run_makefile_stage_platform_packages_check
run_makefile_binary_check
run_makefile_stage_app_check
run_makefile_pack_check
run_makefile_publish_dry_run_check
run_makefile_unpublish_dry_run_check
echo "test-makefile: all checks passed."
