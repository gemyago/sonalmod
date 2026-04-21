#!/usr/bin/env bash
# build/npm/scripts/stage-npm-ui.sh
#
# Stages @sonalmod/ui npm package directory for publishing.
# Copies the Vite dist output and writes a distribution package.json.
#
# Convention follows: build/npm/scripts/resolve-npm-platform.sh
# (standalone, CLI flags, --self-test for local/CI testing)
#
# Usage:
#   stage-npm-ui.sh --src <ui-dist-dir> --version <version> --output <output-dir>
#   stage-npm-ui.sh --self-test
#
# Input:
#   --src     path to Vite dist output directory (e.g. apps/sonal-ui/dist)
#   --version semver version string (e.g. 1.2.3 or 1.2.3-alpha.1)
#   --output  path to staged npm package directory (e.g. dist/npm/ui)
#
# Output layout after staging:
#   <output>/
#     package.json     <- @sonalmod/ui distribution manifest with correct version
#     dist/            <- copy of <src> contents
#       index.html
#       assets/
#       ...

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
  echo "Usage: $0 --src <ui-dist-dir> --version <version> --output <output-dir>"
  echo "       $0 --self-test"
}

stage_ui() {
  local src="$1"
  local version="$2"
  local output="$3"

  if [[ ! -d "$src" ]]; then
    echo "Error: UI dist directory not found: $src" >&2
    exit 1
  fi

  if [[ -z "$version" ]]; then
    echo "Error: --version is required" >&2
    exit 1
  fi

  local dist_dest="$output/dist"
  mkdir -p "$dist_dest"

  # Copy all dist assets (preserving directory structure)
  cp -r "$src/." "$dist_dest/"

  if [[ ! -f "$ROOT_PKG_JSON" ]]; then
    echo "Error: root package.json not found: $ROOT_PKG_JSON" >&2
    exit 1
  fi
  local root_license root_author root_repository
  root_license=$(root_pkg_json_value license)
  root_author=$(root_pkg_json_value author)
  root_repository=$(root_pkg_json_value repository)

  # Write distribution package.json
  cat > "$output/package.json" <<PKGJSON
{
  "name": "@sonalmod/ui",
  "version": "${version}",
  "description": "Sonalmod UI static assets",
  "license": ${root_license},
  "author": ${root_author},
  "repository": ${root_repository},
  "files": [
    "dist/**/*",
    "package.json"
  ]
}
PKGJSON

  echo "[stage-npm-ui] staged @sonalmod/ui@${version} -> $output"
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

  assert_dir_exists() {
    local dirpath="$1"
    local message="$2"
    if [[ -d "$dirpath" ]]; then
      echo "PASS: $message"
    else
      echo "FAIL: $message (directory not found: $dirpath)"
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

  echo "Running stage-npm-ui self-tests..."

  # --- Setup: create a fake UI dist directory ---
  local fake_dist="$tmp_dir/fake-dist"
  mkdir -p "$fake_dist/assets"
  echo '<html><body>test ui</body></html>' > "$fake_dist/index.html"
  echo 'console.log("main");' > "$fake_dist/assets/main-abc123.js"
  echo 'body { margin: 0; }' > "$fake_dist/assets/main-def456.css"

  local output_dir="$tmp_dir/output"

  # --- Test 1: basic staging creates output structure ---
  stage_ui "$fake_dist" "1.2.3" "$output_dir"

  assert_dir_exists "$output_dir/dist" "dist directory created"
  assert_file_exists "$output_dir/dist/index.html" "index.html copied"
  assert_dir_exists "$output_dir/dist/assets" "assets directory copied"
  assert_file_exists "$output_dir/dist/assets/main-abc123.js" "js asset copied"
  assert_file_exists "$output_dir/dist/assets/main-def456.css" "css asset copied"
  assert_file_exists "$output_dir/package.json" "package.json written"

  # --- Test 2: package.json contains correct name ---
  local pkg_name
  pkg_name=$(grep '"name"' "$output_dir/package.json" | sed 's/.*"name":[[:space:]]*"\([^"]*\)".*/\1/')
  assert "$pkg_name" "@sonalmod/ui" "package.json name is @sonalmod/ui"

  # --- Test 3: package.json contains correct version ---
  local pkg_version
  pkg_version=$(grep '"version"' "$output_dir/package.json" | sed 's/.*"version":[[:space:]]*"\([^"]*\)".*/\1/')
  assert "$pkg_version" "1.2.3" "package.json version is 1.2.3"

  local exp_lic exp_auth exp_repo ui_json
  exp_lic=$(root_pkg_json_value license)
  exp_auth=$(root_pkg_json_value author)
  exp_repo=$(root_pkg_json_value repository)
  ui_json=$(cat "$output_dir/package.json")
  assert_contains "$ui_json" "\"license\": ${exp_lic}" "package.json license matches root"
  assert_contains "$ui_json" "\"author\": ${exp_auth}" "package.json author matches root"
  assert_contains "$ui_json" "\"repository\": ${exp_repo}" "package.json repository matches root"

  # --- Test 4: pre-release version is preserved correctly ---
  local prerelease_output="$tmp_dir/output-prerelease"
  stage_ui "$fake_dist" "2.0.0-alpha.1" "$prerelease_output"
  local prerelease_version
  prerelease_version=$(grep '"version"' "$prerelease_output/package.json" | sed 's/.*"version":[[:space:]]*"\([^"]*\)".*/\1/')
  assert "$prerelease_version" "2.0.0-alpha.1" "package.json pre-release version preserved"

  # --- Test 5: error when src does not exist ---
  local err_output
  set +e
  err_output=$(stage_ui "$tmp_dir/nonexistent-dist" "1.0.0" "$tmp_dir/err-out" 2>&1)
  local exit_code=$?
  set -e
  assert "$exit_code" "1" "exits with error when src not found"
  if [[ "$err_output" == *"not found"* ]]; then
    echo "PASS: error message mentions 'not found'"
  else
    echo "FAIL: expected 'not found' in error: $err_output"
    ((failures++))
  fi

  if [[ $failures -eq 0 ]]; then
    echo "All tests passed!"
    return 0
  else
    echo "$failures test(s) failed."
    return 1
  fi
}

# === Parse args ===
SRC=""
VERSION=""
OUTPUT=""
SELF_TEST=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --src)       SRC="$2";     shift 2 ;;
    --version)   VERSION="$2"; shift 2 ;;
    --output)    OUTPUT="$2";  shift 2 ;;
    --self-test) SELF_TEST=true; shift ;;
    -h|--help)   usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

if $SELF_TEST; then
  run_tests
  exit $?
fi

if [[ -z "$SRC" || -z "$VERSION" || -z "$OUTPUT" ]]; then
  echo "Error: --src, --version, and --output are required" >&2
  usage
  exit 1
fi

stage_ui "$SRC" "$VERSION" "$OUTPUT"
