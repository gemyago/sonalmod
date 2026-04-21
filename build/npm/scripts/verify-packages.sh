#!/usr/bin/env bash
# build/npm/scripts/verify-packages.sh
#
# Validates npm release tarballs produced by the build pipeline.
# Verifies: tarball contents, expected packages present, package.json correctness.
#
# Convention follows: build/npm/scripts/resolve-npm-platform.sh
# (standalone, CLI flags, --self-test for local/CI testing)
#
# Usage:
#   verify-packages.sh --tarballs-dir dist/tarballs --version 1.2.3 \
#     --platforms linux/amd64,linux/arm64,darwin/arm64,windows/amd64
#   verify-packages.sh --self-test
#
# Checks performed:
#   1. Expected tarballs exist for all platforms + ui + app.
#   2. @sonalmod/ui tarball contains package/dist/index.html and package/package.json.
#   3. Platform tarballs contain package/bin/sonalmod[.exe] and package/package.json.
#   4. @sonalmod/app tarball contains package/bin/sonalmod.js and package/package.json.
#   5. package.json version field matches expected version in all packages.
# Member paths are matched after stripping a leading "./" from each tar entry (GNU tar / CI).
# Extraction uses the raw stored member name (see tar_cat_member) so `tar -xzOf` works on Linux.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
  echo "Usage: $0 --tarballs-dir <dir> --version <version> --platforms <goos/goarch,...>"
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

# Convert npm package name to expected tarball filename prefix.
# @sonalmod/app -> sonalmod-app, @sonalmod/app-linux-x64 -> sonalmod-app-linux-x64
tarball_prefix() {
  local pkg_name="$1"
  echo "${pkg_name#@}" | tr '/' '-'
}

# Member paths as printed by `tar -tzf` differ by platform/toolchain: some use a leading
# `./` (common with GNU tar / certain npm versions). Normalize so checks match everywhere.
tar_list_members_normalized() {
  tar -tzf "$1" 2>/dev/null | sed '/^$/d; s|^\./||'
}

# Raw member name as stored (e.g. ./package/package.json). `tar -xzOf tgz package/x` fails on
# GNU tar when the member is recorded as ./package/x — extraction must use the exact name.
tar_raw_member_for_normalized_path() {
  local tarball="$1"
  local normalized="$2"
  local line norm
  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    norm="${line#./}"
    if [[ "$norm" == "$normalized" ]]; then
      printf '%s\n' "$line"
      return 0
    fi
  done < <(tar -tzf "$tarball" 2>/dev/null)
  return 1
}

tar_cat_member() {
  local tarball="$1"
  local normalized_path="$2"
  local raw
  raw=$(tar_raw_member_for_normalized_path "$tarball" "$normalized_path") || return 1
  tar -xzOf "$tarball" "$raw" 2>/dev/null
}

verify_tarball_contains() {
  local tarball="$1"
  local expected_path="$2"
  local label="$3"
  local failures_ref="$4"

  if tar_list_members_normalized "$tarball" | grep -qxF "$expected_path"; then
    echo "  PASS: $label contains $expected_path"
  else
    echo "  FAIL: $label tarball $(basename "$tarball") missing required member: $expected_path"
    local parent="${expected_path%/*}"
    local under_parent
    under_parent=$(tar_list_members_normalized "$tarball" | grep -F "${parent}/" 2>/dev/null || true)
    if [[ -n "$under_parent" ]]; then
      echo "         Members under ${parent}/:"
      echo "$under_parent" | head -25 | sed 's/^/           /'
    else
      echo "         No entries under ${parent}/ after normalizing paths (no leading ./)."
      echo "         First members in archive (up to 25):"
      tar_list_members_normalized "$tarball" | head -25 | sed 's/^/           /'
    fi
    eval "(( ${failures_ref}++ ))" || true
  fi
}

verify_tarball_version() {
  local tarball="$1"
  local expected_version="$2"
  local label="$3"
  local failures_ref="$4"

  local actual_version
  actual_version=$(tar_cat_member "$tarball" "package/package.json" | grep '"version"' | head -1 | sed 's/.*"version":[[:space:]]*"\([^"]*\)".*/\1/')
  if [[ "$actual_version" == "$expected_version" ]]; then
    echo "  PASS: $label version is $expected_version"
  else
    echo "  FAIL: $label tarball $(basename "$tarball") inner package.json version mismatch"
    echo "         Expected version field: '$expected_version'"
    echo "         Read from package/package.json: '${actual_version:-<empty or unreadable>}'"
    eval "(( ${failures_ref}++ ))" || true
  fi
}

find_tarball() {
  local tarballs_dir="$1"
  local prefix="$2"
  local version="${3:-}"
  # Prefer exact name (avoids ambiguous find order when multiple ${prefix}-*.tgz exist).
  if [[ -n "$version" ]]; then
    local exact="${tarballs_dir}/${prefix}-${version}.tgz"
    if [[ -f "$exact" ]]; then
      echo "$exact"
      return 0
    fi
  fi
  local found
  found=$(find "$tarballs_dir" -maxdepth 1 -name "${prefix}-*.tgz" 2>/dev/null | sort | head -1)
  echo "$found"
}

# Tarball inner package.json version (for disambiguating stale or oddly named packs).
tarball_pkg_version() {
  local tarball="$1"
  tar_cat_member "$tarball" "package/package.json" | grep '"version"' | head -1 | sed 's/.*"version":[[:space:]]*"\([^"]*\)".*/\1/'
}

# Launcher only: sonalmod-app-<version>.tgz — not sonalmod-app-<os>-<cpu>-<version>.tgz
resolve_sonalmod_app_launcher_tarball() {
  local tarballs_dir="$1"
  local version="$2"
  local exact="${tarballs_dir}/sonalmod-app-${version}.tgz"
  if [[ -f "$exact" ]]; then
    echo "$exact"
    return 0
  fi

  local candidates=()
  local f
  while IFS= read -r f; do
    [[ -n "$f" ]] && candidates+=("$f")
  done < <(find "$tarballs_dir" -maxdepth 1 -name 'sonalmod-app-*.tgz' 2>/dev/null | sort)

  local non_platform=()
  for f in "${candidates[@]}"; do
    local base
    base=$(basename "$f")
    if [[ "$base" =~ ^sonalmod-app-(linux|darwin|win32)-(x64|arm64)- ]]; then
      continue
    fi
    non_platform+=("$f")
  done

  local c match=""
  for c in "${non_platform[@]}"; do
    local pv
    pv=$(tarball_pkg_version "$c")
    if [[ "$pv" == "$version" ]]; then
      match="$c"
      break
    fi
  done
  if [[ -n "$match" ]]; then
    echo "$match"
    return 0
  fi
  if [[ ${#non_platform[@]} -eq 1 ]]; then
    echo "${non_platform[0]}"
    return 0
  fi
  echo ""
  return 1
}

verify_packages() {
  local tarballs_dir="$1"
  local version="$2"
  local platforms="$3"

  local failures=0

  echo "[verify] Starting package verification (version: $version)"

  # --- Verify @sonalmod/ui ---
  local ui_prefix
  ui_prefix=$(tarball_prefix "@sonalmod/ui")
  local ui_tarball
  ui_tarball=$(find_tarball "$tarballs_dir" "$ui_prefix" "$version")

  if [[ -z "$ui_tarball" ]]; then
    echo "FAIL: @sonalmod/ui tarball not found in $tarballs_dir"
    ((failures++))
  else
    echo "[verify] @sonalmod/ui -> $(basename "$ui_tarball")"
    verify_tarball_contains "$ui_tarball" "package/package.json"       "@sonalmod/ui" failures
    verify_tarball_contains "$ui_tarball" "package/dist/index.html"    "@sonalmod/ui" failures
    verify_tarball_version  "$ui_tarball" "$version"                   "@sonalmod/ui" failures
  fi

  # --- Verify @sonalmod/app (launcher; not per-platform sonalmod-app-<os>-<cpu>-…) ---
  local app_tarball_clean
  app_tarball_clean=$(resolve_sonalmod_app_launcher_tarball "$tarballs_dir" "$version") || true

  if [[ -z "$app_tarball_clean" ]]; then
    echo "FAIL: @sonalmod/app tarball not found in $tarballs_dir"
    ((failures++))
  else
    echo "[verify] @sonalmod/app -> $(basename "$app_tarball_clean")"
    verify_tarball_contains "$app_tarball_clean" "package/package.json"       "@sonalmod/app" failures
    verify_tarball_contains "$app_tarball_clean" "package/bin/sonalmod.js"    "@sonalmod/app" failures
    verify_tarball_version  "$app_tarball_clean" "$version"                   "@sonalmod/app" failures
  fi

  # --- Verify per-platform packages ---
  IFS=',' read -ra platform_arr <<< "$platforms"
  for platform in "${platform_arr[@]}"; do
    local goos
    goos=$(echo "$platform" | cut -d/ -f1)
    local goarch
    goarch=$(echo "$platform" | cut -d/ -f2)
    local os_val
    os_val=$(npm_os "$goos") || { ((failures++)); continue; }
    local cpu_val
    cpu_val=$(npm_cpu "$goarch") || { ((failures++)); continue; }
    local suffix="${os_val}-${cpu_val}"
    local pkg_name="@sonalmod/app-${suffix}"
    local platform_prefix
    platform_prefix=$(tarball_prefix "$pkg_name")

    local platform_tarball
    platform_tarball=$(find_tarball "$tarballs_dir" "$platform_prefix" "$version")

    if [[ -z "$platform_tarball" ]]; then
      echo "FAIL: $pkg_name tarball not found in $tarballs_dir"
      ((failures++))
      continue
    fi

    local bin_ext=""
    if [[ "$goos" == "windows" ]]; then
      bin_ext=".exe"
    fi

    echo "[verify] $pkg_name -> $(basename "$platform_tarball")"
    verify_tarball_contains "$platform_tarball" "package/package.json"            "$pkg_name" failures
    verify_tarball_contains "$platform_tarball" "package/bin/sonalmod${bin_ext}"  "$pkg_name" failures
    verify_tarball_version  "$platform_tarball" "$version"                        "$pkg_name" failures
  done

  if [[ $failures -eq 0 ]]; then
    echo "[verify] All checks passed."
    return 0
  else
    echo "[verify] $failures check(s) failed."
    return 1
  fi
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

  assert_contains() {
    local haystack="$1"
    local needle="$2"
    local message="$3"
    if [[ "$haystack" == *"$needle"* ]]; then
      echo "PASS: $message"
    else
      echo "FAIL: $message (expected '$needle' not found)"
      ((failures++))
    fi
  }

  echo "Running verify-packages self-tests..."

  # --- Test 1: tarball_prefix converts names correctly ---
  assert "$(tarball_prefix "@sonalmod/app")"            "sonalmod-app"            "app prefix"
  assert "$(tarball_prefix "@sonalmod/ui")"             "sonalmod-ui"             "ui prefix"
  assert "$(tarball_prefix "@sonalmod/app-linux-x64")"  "sonalmod-app-linux-x64"  "platform prefix"

  # --- Test 2: npm_os/npm_cpu mappings ---
  assert "$(npm_os linux)"   "linux"  "linux os"
  assert "$(npm_os darwin)"  "darwin" "darwin os"
  assert "$(npm_os windows)" "win32"  "windows os"
  assert "$(npm_cpu amd64)"  "x64"    "amd64 cpu"
  assert "$(npm_cpu arm64)"  "arm64"  "arm64 cpu"

  # --- Test 3: verify_packages fails when tarballs dir is empty ---
  local empty_dir="$tmp_dir/empty-tarballs"
  mkdir -p "$empty_dir"
  local err_output
  set +e
  err_output=$(verify_packages "$empty_dir" "1.0.0" "linux/amd64" 2>&1)
  local exit_code=$?
  set -e
  assert "$exit_code" "1" "exits 1 when tarballs missing"
  assert_contains "$err_output" "FAIL" "reports failures when tarballs missing"

  # --- Test 4: verify_packages passes with correctly structured tarballs ---
  local tarballs_dir="$tmp_dir/tarballs"
  mkdir -p "$tarballs_dir"

  # Helper: create a tarball where staged dir contents appear under "package/" inside the archive.
  # Uses cp to a "package" directory to avoid --transform (GNU-only). Works on macOS and Linux.
  make_tarball() {
    local stage_dir="$1"
    local out_tarball="$2"
    local parent_dir
    parent_dir=$(dirname "$stage_dir")
    local pkg_dir="$parent_dir/package"
    cp -r "$stage_dir" "$pkg_dir"
    (cd "$parent_dir" && tar -czf "$out_tarball" "package")
    rm -rf "$pkg_dir"
  }

  # Create a minimal valid tarball for @sonalmod/ui
  local ui_stage="$tmp_dir/ui-stage"
  mkdir -p "$ui_stage/dist"
  echo '<html></html>' > "$ui_stage/dist/index.html"
  echo '{"name":"@sonalmod/ui","version":"1.0.0"}' > "$ui_stage/package.json"
  make_tarball "$ui_stage" "$tarballs_dir/sonalmod-ui-1.0.0.tgz"

  # Create a minimal valid tarball for @sonalmod/app
  local app_stage="$tmp_dir/app-stage"
  mkdir -p "$app_stage/bin"
  echo 'console.log("launcher");' > "$app_stage/bin/sonalmod.js"
  echo '{"name":"@sonalmod/app","version":"1.0.0"}' > "$app_stage/package.json"
  make_tarball "$app_stage" "$tarballs_dir/sonalmod-app-1.0.0.tgz"

  # Create a minimal valid tarball for @sonalmod/app-linux-x64
  local plat_stage="$tmp_dir/plat-stage"
  mkdir -p "$plat_stage/bin"
  echo '#!/bin/sh' > "$plat_stage/bin/sonalmod"
  echo '{"name":"@sonalmod/app-linux-x64","version":"1.0.0"}' > "$plat_stage/package.json"
  make_tarball "$plat_stage" "$tarballs_dir/sonalmod-app-linux-x64-1.0.0.tgz"

  local verify_output
  set +e
  verify_output=$(verify_packages "$tarballs_dir" "1.0.0" "linux/amd64" 2>&1)
  local verify_exit=$?
  set -e

  if [[ $verify_exit -eq 0 ]]; then
    echo "PASS: verify_packages passes with valid tarballs"
  else
    echo "FAIL: verify_packages should pass with valid tarballs (output: $verify_output)"
    ((failures++))
  fi

  # --- Test 4b: GNU-style member paths ./package/... (tar -tzf lists leading ./) ---
  make_tarball_dot_prefix() {
    local stage_dir="$1"
    local out_tarball="$2"
    local parent_dir
    parent_dir=$(dirname "$stage_dir")
    local pkg_dir="$parent_dir/package"
    cp -r "$stage_dir" "$pkg_dir"
    (cd "$parent_dir" && tar -czf "$out_tarball" ./package)
    rm -rf "$pkg_dir"
  }
  local tarballs_dot="$tmp_dir/tarballs-dotprefix"
  mkdir -p "$tarballs_dot"
  local ui_dot="$tmp_dir/ui-dotprefix"
  mkdir -p "$ui_dot/dist"
  echo '<html></html>' > "$ui_dot/dist/index.html"
  echo '{"name":"@sonalmod/ui","version":"1.0.0"}' > "$ui_dot/package.json"
  make_tarball_dot_prefix "$ui_dot" "$tarballs_dot/sonalmod-ui-1.0.0.tgz"
  local app_dot="$tmp_dir/app-dotprefix"
  mkdir -p "$app_dot/bin"
  echo 'console.log("launcher");' > "$app_dot/bin/sonalmod.js"
  echo '{"name":"@sonalmod/app","version":"1.0.0"}' > "$app_dot/package.json"
  make_tarball_dot_prefix "$app_dot" "$tarballs_dot/sonalmod-app-1.0.0.tgz"
  local plat_dot="$tmp_dir/plat-dotprefix"
  mkdir -p "$plat_dot/bin"
  echo '#!/bin/sh' > "$plat_dot/bin/sonalmod"
  echo '{"name":"@sonalmod/app-linux-x64","version":"1.0.0"}' > "$plat_dot/package.json"
  make_tarball_dot_prefix "$plat_dot" "$tarballs_dot/sonalmod-app-linux-x64-1.0.0.tgz"

  local verify_dot_out
  set +e
  verify_dot_out=$(verify_packages "$tarballs_dot" "1.0.0" "linux/amd64" 2>&1)
  local verify_dot_exit=$?
  set -e
  if [[ $verify_dot_exit -eq 0 ]]; then
    echo "PASS: verify_packages accepts ./package-prefixed tar members"
  else
    echo "FAIL: verify_packages should accept ./package members (output: $verify_dot_out)"
    ((failures++))
  fi

  # --- Test 5: launcher tarball selection when version does not start with a digit ---
  local tarballs_beta="$tmp_dir/tarballs-beta"
  mkdir -p "$tarballs_beta"
  local ui_beta="$tmp_dir/ui-beta"
  mkdir -p "$ui_beta/dist"
  echo '<html></html>' > "$ui_beta/dist/index.html"
  echo '{"name":"@sonalmod/ui","version":"beta"}' > "$ui_beta/package.json"
  make_tarball "$ui_beta" "$tarballs_beta/sonalmod-ui-beta.tgz"

  local app_beta="$tmp_dir/app-beta"
  mkdir -p "$app_beta/bin"
  echo 'console.log("launcher");' > "$app_beta/bin/sonalmod.js"
  echo '{"name":"@sonalmod/app","version":"beta"}' > "$app_beta/package.json"
  make_tarball "$app_beta" "$tarballs_beta/sonalmod-app-beta.tgz"

  local plat_beta="$tmp_dir/plat-beta"
  mkdir -p "$plat_beta/bin"
  echo '#!/bin/sh' > "$plat_beta/bin/sonalmod"
  echo '{"name":"@sonalmod/app-linux-x64","version":"beta"}' > "$plat_beta/package.json"
  make_tarball "$plat_beta" "$tarballs_beta/sonalmod-app-linux-x64-beta.tgz"

  local verify_beta_out
  set +e
  verify_beta_out=$(verify_packages "$tarballs_beta" "beta" "linux/amd64" 2>&1)
  local verify_beta_exit=$?
  set -e
  if [[ $verify_beta_exit -eq 0 ]]; then
    echo "PASS: verify_packages finds launcher tarball when version is non-numeric prefix"
  else
    echo "FAIL: verify_packages should accept non-[0-9] version (output: $verify_beta_out)"
    ((failures++))
  fi

  # --- Test 6: multiple launcher-shaped tarballs — pick the one whose inner version matches ---
  local td_multi="$tmp_dir/two-launchers"
  mkdir -p "$td_multi"
  local s_old="$tmp_dir/inner1" s_new="$tmp_dir/inner2"
  mkdir -p "$s_old/bin" "$s_new/bin"
  echo 'console.log("a");' > "$s_old/bin/sonalmod.js"
  echo 'console.log("b");' > "$s_new/bin/sonalmod.js"
  echo '{"name":"@sonalmod/app","version":"1.0.0"}' > "$s_old/package.json"
  echo '{"name":"@sonalmod/app","version":"2.0.0"}' > "$s_new/package.json"
  make_tarball "$s_old" "$td_multi/sonalmod-app-1.0.0.tgz"
  make_tarball "$s_new" "$td_multi/sonalmod-app-mangled-name.tgz"
  local got_launcher
  got_launcher=$(resolve_sonalmod_app_launcher_tarball "$td_multi" "2.0.0") || true
  assert "$(basename "$got_launcher")" "sonalmod-app-mangled-name.tgz" "resolve launcher by inner package version when filename is not sonalmod-app-\${version}.tgz"

  if [[ $failures -eq 0 ]]; then
    echo "All tests passed!"
    return 0
  else
    echo "$failures test(s) failed."
    return 1
  fi
}

# === Parse args ===
TARBALLS_DIR=""
VERSION=""
PLATFORMS=""
SELF_TEST=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --tarballs-dir) TARBALLS_DIR="$2"; shift 2 ;;
    --version)      VERSION="$2";      shift 2 ;;
    --platforms)    PLATFORMS="$2";    shift 2 ;;
    --self-test)    SELF_TEST=true;    shift   ;;
    -h|--help)      usage; exit 0              ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

if $SELF_TEST; then
  run_tests
  exit $?
fi

if [[ -z "$TARBALLS_DIR" || -z "$VERSION" || -z "$PLATFORMS" ]]; then
  echo "Error: --tarballs-dir, --version, and --platforms are required" >&2
  usage
  exit 1
fi

verify_packages "$TARBALLS_DIR" "$VERSION" "$PLATFORMS"
