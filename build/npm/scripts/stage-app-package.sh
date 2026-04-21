#!/usr/bin/env bash
# build/npm/scripts/stage-app-package.sh
#
# Stages the @sonalmod/app npm package directory for publishing.
# Copies the launcher script and writes a distribution package.json
# with optionalDependencies (platform packages) and dependencies (@sonalmod/ui).
#
# Convention follows: build/npm/scripts/resolve-npm-platform.sh
# (standalone, CLI flags, --self-test for local/CI testing)
#
# Usage:
#   stage-app-package.sh --version 1.2.3 --platforms linux/amd64,linux/arm64,darwin/arm64,windows/amd64 \
#     --output dist/npm/app
#   stage-app-package.sh --self-test
#
# Output layout after staging:
#   <output>/
#     package.json     <- @sonalmod/app manifest with bin, dependencies, optionalDependencies
#     bin/
#       sonalmod.js    <- launcher script (copied from build/npm/app/bin/sonalmod.js)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_NPM_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO_ROOT="$(cd "$BUILD_NPM_DIR/../.." && pwd)"
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
  echo "Usage: $0 --version <version> --platforms <goos/goarch,...> --output <output-dir>"
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

stage_app_package() {
  local version="$1"
  local platforms="$2"
  local output="$3"

  if [[ -z "$version" ]]; then
    echo "Error: --version is required" >&2
    exit 1
  fi

  if [[ -z "$platforms" ]]; then
    echo "Error: --platforms is required" >&2
    exit 1
  fi

  local launcher_src="${BUILD_NPM_DIR}/app/bin/sonalmod.js"
  if [[ ! -f "$launcher_src" ]]; then
    echo "Error: launcher not found: $launcher_src" >&2
    exit 1
  fi

  local bin_dest="${output}/bin"
  mkdir -p "$bin_dest"

  cp "$launcher_src" "${bin_dest}/sonalmod.js"

  # Build optionalDependencies JSON block from platform list
  local optional_deps=""
  IFS=',' read -ra platform_arr <<< "$platforms"
  for platform in "${platform_arr[@]}"; do
    local goos
    goos=$(echo "$platform" | cut -d/ -f1)
    local goarch
    goarch=$(echo "$platform" | cut -d/ -f2)
    local os_val
    os_val=$(npm_os "$goos") || exit 1
    local cpu_val
    cpu_val=$(npm_cpu "$goarch") || exit 1
    local suffix="${os_val}-${cpu_val}"
    if [[ -n "$optional_deps" ]]; then
      optional_deps="${optional_deps},"$'\n'
    fi
    optional_deps="${optional_deps}    \"@sonalmod/app-${suffix}\": \"${version}\""
  done

  if [[ ! -f "$ROOT_PKG_JSON" ]]; then
    echo "Error: root package.json not found: $ROOT_PKG_JSON" >&2
    exit 1
  fi
  local root_license root_author root_description root_repository
  root_license=$(root_pkg_json_value license)
  root_author=$(root_pkg_json_value author)
  root_description=$(root_pkg_json_value description)
  root_repository=$(root_pkg_json_value repository)

  cat > "$output/package.json" <<PKGJSON
{
  "name": "@sonalmod/app",
  "version": "${version}",
  "description": ${root_description},
  "license": ${root_license},
  "author": ${root_author},
  "repository": ${root_repository},
  "bin": {
    "sonalmod": "bin/sonalmod.js"
  },
  "files": [
    "bin/sonalmod.js",
    "package.json"
  ],
  "dependencies": {
    "@sonalmod/ui": "${version}"
  },
  "optionalDependencies": {
${optional_deps}
  },
  "engines": {
    "node": ">=18.0.0"
  }
}
PKGJSON

  echo "[stage-app-package] staged @sonalmod/app@${version} -> $output"
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
      echo "FAIL: $message (expected '$needle' not found)"
      ((failures++))
    fi
  }

  echo "Running stage-app-package self-tests..."

  local output_dir="$tmp_dir/app"
  local platforms="linux/amd64,linux/arm64,darwin/arm64,windows/amd64"

  stage_app_package "1.2.3" "$platforms" "$output_dir"

  assert_file_exists "$output_dir/bin/sonalmod.js" "launcher script copied"
  assert_file_exists "$output_dir/package.json"    "package.json written"

  local pkg_json
  pkg_json=$(cat "$output_dir/package.json")

  local pkg_name
  pkg_name=$(echo "$pkg_json" | grep '"name"' | head -1 | sed 's/.*"name":[[:space:]]*"\([^"]*\)".*/\1/')
  assert "$pkg_name" "@sonalmod/app" "package name"

  local pkg_version
  pkg_version=$(echo "$pkg_json" | grep '"version"' | head -1 | sed 's/.*"version":[[:space:]]*"\([^"]*\)".*/\1/')
  assert "$pkg_version" "1.2.3" "package version"

  assert_contains "$pkg_json" '"@sonalmod/app-linux-x64": "1.2.3"'    "optional dep linux-x64"
  assert_contains "$pkg_json" '"@sonalmod/app-linux-arm64": "1.2.3"'  "optional dep linux-arm64"
  assert_contains "$pkg_json" '"@sonalmod/app-darwin-arm64": "1.2.3"' "optional dep darwin-arm64"
  assert_contains "$pkg_json" '"@sonalmod/app-win32-x64": "1.2.3"'    "optional dep win32-x64"
  assert_contains "$pkg_json" '"@sonalmod/ui": "1.2.3"'               "dependency @sonalmod/ui"
  assert_contains "$pkg_json" '"sonalmod": "bin/sonalmod.js"'         "bin field"

  local exp_lic exp_auth exp_desc exp_repo
  exp_lic=$(root_pkg_json_value license)
  exp_auth=$(root_pkg_json_value author)
  exp_desc=$(root_pkg_json_value description)
  exp_repo=$(root_pkg_json_value repository)
  assert_contains "$pkg_json" "\"license\": ${exp_lic}" "package license matches root"
  assert_contains "$pkg_json" "\"author\": ${exp_auth}" "package author matches root"
  assert_contains "$pkg_json" "\"description\": ${exp_desc}" "package description matches root"
  assert_contains "$pkg_json" "\"repository\": ${exp_repo}" "package repository matches root"

  # --- Test 2: pre-release version ---
  local out2="$tmp_dir/app-prerelease"
  stage_app_package "2.0.0-alpha.1" "linux/amd64" "$out2"

  local v2
  v2=$(grep '"version"' "$out2/package.json" | head -1 | sed 's/.*"version":[[:space:]]*"\([^"]*\)".*/\1/')
  assert "$v2" "2.0.0-alpha.1" "pre-release version preserved"

  local dep_v2
  dep_v2=$(grep '"@sonalmod/ui"' "$out2/package.json" | sed 's/.*"@sonalmod\/ui":[[:space:]]*"\([^"]*\)".*/\1/')
  assert "$dep_v2" "2.0.0-alpha.1" "@sonalmod/ui dep version matches pre-release"

  if [[ $failures -eq 0 ]]; then
    echo "All tests passed!"
    return 0
  else
    echo "$failures test(s) failed."
    return 1
  fi
}

# === Parse args ===
VERSION=""
PLATFORMS=""
OUTPUT=""
SELF_TEST=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)   VERSION="$2";   shift 2 ;;
    --platforms) PLATFORMS="$2"; shift 2 ;;
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

if [[ -z "$VERSION" || -z "$PLATFORMS" || -z "$OUTPUT" ]]; then
  echo "Error: --version, --platforms, and --output are required" >&2
  usage
  exit 1
fi

stage_app_package "$VERSION" "$PLATFORMS" "$OUTPUT"
