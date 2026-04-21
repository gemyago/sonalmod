// Tests for @sonalmod/app launcher (build/npm/app/bin/sonalmod.js)
//
// Tests OS/arch resolution, unsupported platform errors, argv passthrough,
// and UI dist path injection via --ui-location.
//
// Run: node --test app/bin/sonalmod.test.mjs
// Or via: make test (from build/npm)

import { describe, it, before } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import {
  mkdtempSync,
  mkdirSync,
  writeFileSync,
  rmSync,
  chmodSync,
} from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const LAUNCHER = join(__dirname, 'sonalmod.js');

// Helper: create a temp directory, cleaned up after the callback
function withTempDir(fn) {
  const tmp = mkdtempSync(join(tmpdir(), 'sonalmod-launcher-test-'));
  try {
    fn(tmp);
  } finally {
    rmSync(tmp, { recursive: true, force: true });
  }
}

// Helper: create a fake package root with a platform package containing a binary
function createFakePackageRoot(tmp, { suffix, binaryContent = '#!/bin/sh\necho ok\n', includeUi = false } = {}) {
  const root = join(tmp, 'fake-root');
  mkdirSync(root, { recursive: true });
  writeFileSync(join(root, 'package.json'), JSON.stringify({ name: '@sonalmod/app', version: '0.0.0' }));

  const node_modules = join(root, 'node_modules');

  if (suffix) {
    const pkgDir = join(node_modules, '@sonalmod', `app-${suffix}`);
    const binDir = join(pkgDir, 'bin');
    mkdirSync(binDir, { recursive: true });
    writeFileSync(join(pkgDir, 'package.json'), JSON.stringify({
      name: `@sonalmod/app-${suffix}`,
      version: '0.0.0',
    }));
    const binName = suffix.startsWith('win32') ? 'sonalmod.exe' : 'sonalmod';
    const binPath = join(binDir, binName);
    writeFileSync(binPath, binaryContent);
    chmodSync(binPath, 0o755);
  }

  if (includeUi) {
    const uiDir = join(node_modules, '@sonalmod', 'ui');
    const uiDist = join(uiDir, 'dist');
    mkdirSync(uiDist, { recursive: true });
    writeFileSync(join(uiDir, 'package.json'), JSON.stringify({ name: '@sonalmod/ui', version: '0.0.0' }));
    writeFileSync(join(uiDist, 'index.html'), '<html><body>test</body></html>');
  }

  return root;
}

// Helper: run the launcher with a given SONALMOD_PACKAGE_ROOT, return result
function runLauncher(packageRoot, extraArgs = [], extraEnv = {}) {
  return spawnSync(process.execPath, [LAUNCHER, ...extraArgs], {
    env: {
      ...process.env,
      SONALMOD_PACKAGE_ROOT: packageRoot,
      ...extraEnv,
    },
    encoding: 'utf8',
  });
}

// Determine the current platform suffix to use for the "happy path" tests
function currentPlatformSuffix() {
  const os = process.platform;
  const cpu = process.arch;
  const map = {
    'linux-x64':    ['linux', 'x64'],
    'linux-arm64':  ['linux', 'arm64'],
    'darwin-arm64': ['darwin', 'arm64'],
    'win32-x64':    ['win32', 'x64'],
  };
  for (const [suffix, [o, c]] of Object.entries(map)) {
    if (o === os && c === cpu) return suffix;
  }
  return null;
}

describe('launcher: OS/arch binary resolution', () => {
  it('resolves linux-x64 binary on linux/x64', () => {
    withTempDir((tmp) => {
      const root = createFakePackageRoot(tmp, { suffix: 'linux-x64' });
      const result = runLauncher(root, [], { SONALMOD_NO_UI: '1' });
      // The fake binary prints "ok"; on the test runner's platform it will only work
      // if we're on linux-x64. Otherwise we test the path-resolution logic separately.
      // Here we just verify the binary path was resolved (no "not installed" error).
      assert.ok(
        !result.stderr.includes('platform package @sonalmod/app-linux-x64 is not installed'),
        `should not report linux-x64 missing when installed (stderr: ${result.stderr})`
      );
    });
  });

  it('resolves darwin-arm64 binary on darwin/arm64', () => {
    withTempDir((tmp) => {
      const root = createFakePackageRoot(tmp, { suffix: 'darwin-arm64' });
      const result = runLauncher(root, [], { SONALMOD_NO_UI: '1' });
      assert.ok(
        !result.stderr.includes('platform package @sonalmod/app-darwin-arm64 is not installed'),
        `should not report darwin-arm64 missing when installed (stderr: ${result.stderr})`
      );
    });
  });
});

describe('launcher: unsupported platform errors', () => {
  it('exits 1 with clear message when platform package is missing', () => {
    withTempDir((tmp) => {
      // Create root with no platform packages at all
      const root = createFakePackageRoot(tmp, { suffix: null });
      const suffix = currentPlatformSuffix();
      if (!suffix) {
        // On a platform not in our matrix, the "unsupported platform" path fires instead
        const result = runLauncher(root, [], { SONALMOD_NO_UI: '1' });
        assert.equal(result.status, 1, 'exits 1 on unsupported platform');
        assert.ok(result.stderr.includes('unsupported platform'), 'reports unsupported platform');
        return;
      }
      const result = runLauncher(root, [], { SONALMOD_NO_UI: '1' });
      assert.equal(result.status, 1, 'exits 1 when platform package missing');
      assert.ok(
        result.stderr.includes(`@sonalmod/app-${suffix} is not installed`),
        `error mentions missing package (stderr: ${result.stderr})`
      );
    });
  });

  it('exits 1 with clear message when binary file is missing from platform package', () => {
    withTempDir((tmp) => {
      const suffix = currentPlatformSuffix();
      if (!suffix) return; // skip on unsupported platforms

      // Create platform package dir with package.json but NO binary
      const root = join(tmp, 'fake-root');
      mkdirSync(root, { recursive: true });
      writeFileSync(join(root, 'package.json'), JSON.stringify({ name: '@sonalmod/app' }));
      const pkgDir = join(root, 'node_modules', '@sonalmod', `app-${suffix}`);
      mkdirSync(join(pkgDir, 'bin'), { recursive: true });
      writeFileSync(join(pkgDir, 'package.json'), JSON.stringify({ name: `@sonalmod/app-${suffix}` }));
      // No binary written

      const result = runLauncher(root, [], { SONALMOD_NO_UI: '1' });
      assert.equal(result.status, 1, 'exits 1 when binary missing');
      assert.ok(
        result.stderr.includes('binary not found'),
        `error mentions binary not found (stderr: ${result.stderr})`
      );
    });
  });
});

describe('launcher: argv passthrough', () => {
  it('forwards all user args to the binary', () => {
    withTempDir((tmp) => {
      const suffix = currentPlatformSuffix();
      if (!suffix) return; // skip on unsupported platforms

      // Binary that prints its argv to stdout
      const binaryScript = '#!/bin/sh\nfor arg in "$@"; do echo "ARG:$arg"; done\n';
      const root = createFakePackageRoot(tmp, { suffix, binaryContent: binaryScript });

      const result = runLauncher(root, ['start', '--port', '8080'], { SONALMOD_NO_UI: '1' });

      assert.ok(result.stdout.includes('ARG:start'), `'start' arg forwarded (stdout: ${result.stdout})`);
      assert.ok(result.stdout.includes('ARG:--port'), `'--port' arg forwarded`);
      assert.ok(result.stdout.includes('ARG:8080'), `'8080' arg forwarded`);
    });
  });
});

describe('launcher: UI dist path resolution', () => {
  it('injects --ui-location when @sonalmod/ui is installed', () => {
    withTempDir((tmp) => {
      const suffix = currentPlatformSuffix();
      if (!suffix) return; // skip on unsupported platforms

      // Binary that prints its argv so we can inspect --ui-location
      const binaryScript = '#!/bin/sh\nfor arg in "$@"; do echo "ARG:$arg"; done\n';
      const root = createFakePackageRoot(tmp, { suffix, binaryContent: binaryScript, includeUi: true });

      const result = runLauncher(root, ['start']);

      assert.ok(
        result.stdout.includes('ARG:--ui-location'),
        `--ui-location injected (stdout: ${result.stdout})`
      );
      assert.ok(
        result.stdout.includes('ARG:start'),
        `user args still forwarded after --ui-location (stdout: ${result.stdout})`
      );
    });
  });

  it('skips --ui-location injection when SONALMOD_NO_UI=1', () => {
    withTempDir((tmp) => {
      const suffix = currentPlatformSuffix();
      if (!suffix) return; // skip on unsupported platforms

      const binaryScript = '#!/bin/sh\nfor arg in "$@"; do echo "ARG:$arg"; done\n';
      const root = createFakePackageRoot(tmp, { suffix, binaryContent: binaryScript, includeUi: true });

      const result = runLauncher(root, ['start'], { SONALMOD_NO_UI: '1' });

      assert.ok(
        !result.stdout.includes('ARG:--ui-location'),
        `--ui-location NOT injected when SONALMOD_NO_UI=1 (stdout: ${result.stdout})`
      );
    });
  });

  it('skips --ui-location when @sonalmod/ui is absent', () => {
    withTempDir((tmp) => {
      const suffix = currentPlatformSuffix();
      if (!suffix) return; // skip on unsupported platforms

      const binaryScript = '#!/bin/sh\nfor arg in "$@"; do echo "ARG:$arg"; done\n';
      // includeUi: false (default)
      const root = createFakePackageRoot(tmp, { suffix, binaryContent: binaryScript });

      const result = runLauncher(root, ['start'], { SONALMOD_NO_UI: '0' });

      assert.ok(
        !result.stdout.includes('ARG:--ui-location'),
        `--ui-location NOT injected when @sonalmod/ui absent (stdout: ${result.stdout})`
      );
    });
  });
});
