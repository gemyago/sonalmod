#!/usr/bin/env bash
# build/npm/scripts/stage-platform-package.sh
#
# Stages a per-platform npm package directory (@sonalmod/app-<os>-<arch>)
# for publishing. Copies the Go binary and writes a distribution package.json
# with os/cpu platform fields.
#
# Convention follows: build/npm/scripts/resolve-npm-platform.sh
# (standalone, CLI flags, --self-test for local/CI testing)
#
# Usage:
#   stage-platform-package.sh --goos linux --goarch amd64 --version 1.2.3 \
#     --go-dist dist/go --output dist/npm/app-linux-x64
#   stage-platform-package.sh --self-test
#
# Output layout after staging:
#   <output>/
#     package.json     <- @sonalmod/app-<suffix> manifest with os/cpu/version
#     bin/
#       sonalmod       <- copied Go binary (sonalmod.exe on windows)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
ROOT_PKG_JSON="${REPO_ROOT}/package.json"

# Prints a root package.json field as JSON (string/object/array) for embedding in package.json.
root_pkg_json_value() {
  local key="$1"
  node -e '
const fs = require("fs");
const p = JSON.parse(fs.readFileSync(process.argv[1], "utf8"));
const k = process.argv[2];
const v = p[k];
if (v === undefined || v === null) {
  console.error("Error: missing " + k + " in " + process.argv[1]);
  process.exit(1);
}
process.stdout.write(JSON.stringify(v));
' "$ROOT_PKG_JSON" "$key"
}

usage() {
  echo "Usage: $0 --goos <GOOS> --goarch <GOARCH> --version <version> --go-dist <go-dist-dir> --output <output-dir>"
  echo "       $0 --self-test"
}

npm_os() {
  case "$1" in
    linux)   echo "linux"  ;;
    darwin)  echo "darwin" ;;
    windows) echo "win32"  ;;
    *)       echo "Error: unsupported GOOS: $1" >&2; return 1 ;;
  esac
}

npm_cpu() {
  case "$1" in
    amd64) echo "x64"   ;;
    arm64) echo "arm64" ;;
    *)     echo "Error: unsupported GOARCH: $1" >&2; return 1 ;;
  esac
}

stage_platform_package() {
  local goos="$1"
  local goarch="$2"
  local version="$3"
  local go_dist="$4"
  local output="$5"

  local os_val
  os_val=$(npm_os "$goos") || exit 1
  local cpu_val
  cpu_val=$(npm_cpu "$goarch") || exit 1
  local suffix="${os_val}-${cpu_val}"
  local pkg_name="@sonalmod/app-${suffix}"

  # Locate source binary
  local bin_ext=""
  if [[ "$goos" == "windows" ]]; then
    bin_ext=".exe"
  fi
  local src_bin="${go_dist}/${goos}/${goarch}/sonalmod${bin_ext}"

  if [[ ! -f "$src_bin" ]]; then
    echo "Error: binary not found: $src_bin" >&2
    exit 1
  fi

  if [[ -z "$version" ]]; then
    echo "Error: --version is required" >&2
    exit 1
  fi

  local bin_dest="${output}/bin"
  mkdir -p "$bin_dest"

  cp "$src_bin" "${bin_dest}/sonalmod${bin_ext}"
  chmod +x "${bin_dest}/sonalmod${bin_ext}"

  if [[ ! -f "$ROOT_PKG_JSON" ]]; then
    echo "Error: root package.json not found: $ROOT_PKG_JSON" >&2
    exit 1
  fi
  local root_license root_author root_repository
  root_license=$(root_pkg_json_value license)
  root_author=$(root_pkg_json_value author)
  root_repository=$(root_pkg_json_value repository)

  cat > "$output/package.json" <<PKGJSON
{
  "name": "${pkg_name}",
  "version": "${version}",
  "description": "Sonalmod platform binary for ${os_val}/${cpu_val}",
  "license": ${root_license},
  "author": ${root_author},
  "repository": ${root_repository},
  "os": ["${os_val}"],
  "cpu": ["${cpu_val}"],
  "files": [
    "bin/sonalmod${bin_ext}",
    "package.json"
  ]
}
PKGJSON

  echo "[stage-platform-package] staged ${pkg_name}@${version} -> $output"
}

run_tests() {
  local failures=0
  local tmp_dir
  tmp_dir=$(mktemp -d)
  # shellcheck disable=SC2064
  trap "rm -rf '$tmp_dir'" RETURN

  assert() {
    local actual="$1"
    local expected="$2"
    local message="$3"
    if [[ "$actual" == "$expected" ]]; then
      echo "PASS: $message"
    else
      echo "FAIL: $message (expected '$expected', got '$actual')"
      ((failures++))
    fi
  }

  assert_file_exists() {
    local filepath="$1"
    local message="$2"
    if [[ -f "$filepath" ]]; then
      echo "PASS: $message"
    else
      echo "FAIL: $message (file not found: $filepath)"
      ((failures++))
    fi
  }

  assert_contains() {
    local haystack="$1"
    local needle="$2"
    local message="$3"
    if [[ "$haystack" == *"$needle"* ]]; then
      echo "PASS: $message"
    else
      echo "FAIL: $message (expected '$needle' in '$haystack')"
      ((failures++))
    fi
  }

  echo "Running stage-platform-package self-tests..."

  # --- Setup: create a fake go-dist with a linux/amd64 binary ---
  local go_dist="$tmp_dir/go-dist"
  mkdir -p "$go_dist/linux/amd64"
  echo '#!/bin/sh' > "$go_dist/linux/amd64/sonalmod"
  echo 'echo sonalmod' >> "$go_dist/linux/amd64/sonalmod"
  chmod +x "$go_dist/linux/amd64/sonalmod"

  # --- Setup: create a fake windows/amd64 binary (.exe) ---
  mkdir -p "$go_dist/windows/amd64"
  echo '#!/bin/sh' > "$go_dist/windows/amd64/sonalmod.exe"
  chmod +x "$go_dist/windows/amd64/sonalmod.exe"

  # --- Test 1: linux/amd64 staging ---
  local out_linux="$tmp_dir/app-linux-x64"
  stage_platform_package "linux" "amd64" "1.2.3" "$go_dist" "$out_linux"

  assert_file_exists "$out_linux/bin/sonalmod"    "linux binary copied"
  assert_file_exists "$out_linux/package.json"    "linux package.json written"

  local pkg_name
  pkg_name=$(grep '"name"' "$out_linux/package.json" | sed 's/.*"name":[[:space:]]*"\([^"]*\)".*/\1/')
  assert "$pkg_name" "@sonalmod/app-linux-x64" "linux package name"

  local pkg_version
  pkg_version=$(grep '"version"' "$out_linux/package.json" | sed 's/.*"version":[[:space:]]*"\([^"]*\)".*/\1/')
  assert "$pkg_version" "1.2.3" "linux package version"

  local pkg_os
  pkg_os=$(grep '"os"' "$out_linux/package.json" | sed 's/.*\["linux"\].*/linux/')
  assert "$pkg_os" "linux" "linux package os field"

  local pkg_cpu
  pkg_cpu=$(grep '"cpu"' "$out_linux/package.json" | sed 's/.*\["x64"\].*/x64/')
  assert "$pkg_cpu" "x64" "linux package cpu field"

  local exp_lic exp_auth exp_repo linux_json
  exp_lic=$(root_pkg_json_value license)
  exp_auth=$(root_pkg_json_value author)
  exp_repo=$(root_pkg_json_value repository)
  linux_json=$(cat "$out_linux/package.json")
  assert_contains "$linux_json" "\"license\": ${exp_lic}" "linux package license matches root"
  assert_contains "$linux_json" "\"author\": ${exp_auth}" "linux package author matches root"
  assert_contains "$linux_json" "\"repository\": ${exp_repo}" "linux package repository matches root"

  # --- Test 2: windows/amd64 staging (.exe) ---
  local out_win="$tmp_dir/app-win32-x64"
  stage_platform_package "windows" "amd64" "2.0.0-beta.1" "$go_dist" "$out_win"

  assert_file_exists "$out_win/bin/sonalmod.exe" "windows binary copied with .exe"
  assert_file_exists "$out_win/package.json"     "windows package.json written"

  local win_name
  win_name=$(grep '"name"' "$out_win/package.json" | sed 's/.*"name":[[:space:]]*"\([^"]*\)".*/\1/')
  assert "$win_name" "@sonalmod/app-win32-x64" "windows package name"

  local win_os
  pkg_json=$(cat "$out_win/package.json")
  assert_contains "$pkg_json" '"win32"' "windows package.json has win32 os"

  # --- Test 3: error when binary not found ---
  local err_output
  set +e
  err_output=$(stage_platform_package "linux" "arm64" "1.0.0" "$go_dist" "$tmp_dir/err-out" 2>&1)
  local exit_code=$?
  set -e
  assert "$exit_code" "1" "exits 1 when binary not found"
  assert_contains "$err_output" "binary not found" "error message mentions 'binary not found'"

  if [[ $failures -eq 0 ]]; then
    echo "All tests passed!"
    return 0
  else
    echo "$failures test(s) failed."
    return 1
  fi
}

# === Parse args ===
GOOS_VAL=""
GOARCH_VAL=""
VERSION=""
GO_DIST=""
OUTPUT=""
SELF_TEST=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --goos)      GOOS_VAL="$2";  shift 2 ;;
    --goarch)    GOARCH_VAL="$2"; shift 2 ;;
    --version)   VERSION="$2";   shift 2 ;;
    --go-dist)   GO_DIST="$2";   shift 2 ;;
    --output)    OUTPUT="$2";    shift 2 ;;
    --self-test) SELF_TEST=true;  shift   ;;
    -h|--help)   usage; exit 0            ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

if $SELF_TEST; then
  run_tests
  exit $?
fi

if [[ -z "$GOOS_VAL" || -z "$GOARCH_VAL" || -z "$VERSION" || -z "$GO_DIST" || -z "$OUTPUT" ]]; then
  echo "Error: --goos, --goarch, --version, --go-dist, and --output are required" >&2
  usage
  exit 1
fi

stage_platform_package "$GOOS_VAL" "$GOARCH_VAL" "$VERSION" "$GO_DIST" "$OUTPUT"
