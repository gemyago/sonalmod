#!/usr/bin/env node
// build/npm/app/bin/sonalmod.js
//
// Launcher for @sonalmod/app.
// Resolves the platform-specific Go binary and starts it with the correct args.
//
// Platform resolution: looks up @sonalmod/app-<os>-<arch> from optionalDependencies.
// UI resolution: resolves @sonalmod/ui dist path via require and injects --ui-location.
//
// Environment overrides (for testing):
//   SONALMOD_PACKAGE_ROOT  - root from which to resolve platform/ui packages (default: __dirname/../..)
//   SONALMOD_NO_UI         - set to "1" to skip --ui-location injection

'use strict';

const path = require('path');
const { spawnSync } = require('child_process');
const Module = require('module');

// === Platform resolution ===

const PLATFORM_MAP = {
  'linux-x64':    { os: 'linux',  cpu: 'x64'   },
  'linux-arm64':  { os: 'linux',  cpu: 'arm64'  },
  'darwin-arm64': { os: 'darwin', cpu: 'arm64'  },
  'win32-x64':    { os: 'win32',  cpu: 'x64'    },
};

function getNpmSuffix() {
  const os = process.platform;
  const cpu = process.arch;
  for (const [suffix, info] of Object.entries(PLATFORM_MAP)) {
    if (info.os === os && info.cpu === cpu) {
      return suffix;
    }
  }
  return null;
}

// Create a require function rooted at packageRoot so we can resolve packages
// installed relative to the @sonalmod/app package (or an override for tests).
function makeRequireFrom(packageRoot) {
  // We need a fake file path inside packageRoot to anchor Module.createRequire.
  return Module.createRequire(path.join(packageRoot, 'package.json'));
}

function resolveBinary(requireFromRoot, suffix) {
  const pkgName = `@sonalmod/app-${suffix}`;
  let pkgJsonPath;
  try {
    pkgJsonPath = requireFromRoot.resolve(`${pkgName}/package.json`);
  } catch (_) {
    process.stderr.write(
      `Error: platform package ${pkgName} is not installed.\n` +
      `Make sure you have installed @sonalmod/app on a supported platform.\n`
    );
    process.exit(1);
  }

  const pkgDir = path.dirname(pkgJsonPath);
  const isWindows = suffix.startsWith('win32');
  const binName = isWindows ? 'sonalmod.exe' : 'sonalmod';
  const binPath = path.join(pkgDir, 'bin', binName);

  const fs = require('fs');
  if (!fs.existsSync(binPath)) {
    process.stderr.write(
      `Error: binary not found at ${binPath}\n` +
      `The platform package ${pkgName} may be incomplete.\n`
    );
    process.exit(1);
  }

  return binPath;
}

function resolveUiLocation(requireFromRoot) {
  if (process.env.SONALMOD_NO_UI === '1') {
    return null;
  }
  try {
    const uiPkgJsonPath = requireFromRoot.resolve('@sonalmod/ui/package.json');
    return path.join(path.dirname(uiPkgJsonPath), 'dist');
  } catch (_) {
    return null;
  }
}

// === Main ===

const packageRoot = process.env.SONALMOD_PACKAGE_ROOT
  ? path.resolve(process.env.SONALMOD_PACKAGE_ROOT)
  : path.resolve(__dirname, '..', '..');

const requireFromRoot = makeRequireFrom(packageRoot);

const suffix = getNpmSuffix();
if (!suffix) {
  process.stderr.write(
    `Error: unsupported platform: ${process.platform}/${process.arch}\n` +
    `Supported platforms: ${Object.keys(PLATFORM_MAP).join(', ')}\n`
  );
  process.exit(1);
}

const binaryPath = resolveBinary(requireFromRoot, suffix);
const uiLocation = resolveUiLocation(requireFromRoot);

const userArgs = process.argv.slice(2);
const spawnArgs = uiLocation
  ? ['--ui-location', uiLocation, ...userArgs]
  : userArgs;

const result = spawnSync(binaryPath, spawnArgs, { stdio: 'inherit' });

if (result.error) {
  process.stderr.write(`Error: failed to start sonalmod: ${result.error.message}\n`);
  process.exit(1);
}

process.exit(result.status ?? 1);
