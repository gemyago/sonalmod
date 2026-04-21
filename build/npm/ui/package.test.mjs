// Tests for @sonalmod/ui package staging and tarball contents.
// Verifies that staging produces correct package structure and that
// the resulting tarball contains UI dist files.
//
// Run: node --test ui/package.test.mjs
// Or via: make test (from build/npm)

import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync, execSync } from 'node:child_process';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync, readFileSync, readdirSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const STAGE_SCRIPT = join(__dirname, '..', 'scripts', 'stage-npm-ui.sh');

// Helper: create a fake UI dist directory with typical Vite output structure
function createFakeUiDist(root) {
  const distDir = join(root, 'dist');
  mkdirSync(join(distDir, 'assets'), { recursive: true });
  writeFileSync(join(distDir, 'index.html'), '<html><body>fake ui</body></html>');
  writeFileSync(join(distDir, 'assets', 'main-abc123.js'), 'console.log("main");');
  writeFileSync(join(distDir, 'assets', 'main-def456.css'), 'body { margin: 0; }');
  return distDir;
}

// Helper: create temp dir and clean up after test
function withTempDir(fn) {
  const tmp = mkdtempSync(join(tmpdir(), 'sonalmod-ui-test-'));
  try {
    fn(tmp);
  } finally {
    rmSync(tmp, { recursive: true, force: true });
  }
}

describe('@sonalmod/ui: staging script output', () => {
  it('creates dist/ directory with all source files', () => {
    withTempDir((tmp) => {
      const srcDist = createFakeUiDist(tmp);
      const outputDir = join(tmp, 'staged-ui');

      execFileSync('bash', [STAGE_SCRIPT, '--src', srcDist, '--version', '1.0.0', '--output', outputDir]);

      const stagedFiles = readdirSync(join(outputDir, 'dist'), { recursive: true });
      const fileList = stagedFiles.map((f) => f.toString());
      assert.ok(fileList.some((f) => f.includes('index.html')), 'index.html present in dist/');
      assert.ok(fileList.some((f) => f.includes('main-abc123.js')), 'js asset present in dist/');
      assert.ok(fileList.some((f) => f.includes('main-def456.css')), 'css asset present in dist/');
    });
  });

  it('writes package.json with correct name and version', () => {
    withTempDir((tmp) => {
      const srcDist = createFakeUiDist(tmp);
      const outputDir = join(tmp, 'staged-ui');

      execFileSync('bash', [STAGE_SCRIPT, '--src', srcDist, '--version', '2.3.4-alpha.1', '--output', outputDir]);

      const pkgJson = JSON.parse(readFileSync(join(outputDir, 'package.json'), 'utf8'));
      assert.equal(pkgJson.name, '@sonalmod/ui', 'package name is @sonalmod/ui');
      assert.equal(pkgJson.version, '2.3.4-alpha.1', 'package version matches');
    });
  });
});

describe('@sonalmod/ui: tarball contents', () => {
  it('npm pack produces tarball containing dist/index.html', () => {
    withTempDir((tmp) => {
      const srcDist = createFakeUiDist(tmp);
      const outputDir = join(tmp, 'staged-ui');
      const tarballDir = join(tmp, 'tarballs');
      mkdirSync(tarballDir, { recursive: true });

      execFileSync('bash', [STAGE_SCRIPT, '--src', srcDist, '--version', '1.0.0', '--output', outputDir]);

      execSync(`npm pack "${outputDir}" --pack-destination "${tarballDir}"`, { stdio: 'pipe' });

      // List the tarball contents and verify dist/index.html is present
      const tarballs = readdirSync(tarballDir).filter((f) => f.endsWith('.tgz'));
      assert.equal(tarballs.length, 1, 'exactly one tarball produced');

      const tarball = join(tarballDir, tarballs[0]);
      const tarContents = execSync(`tar -tzf "${tarball}"`, { encoding: 'utf8' });
      assert.ok(tarContents.includes('package/dist/index.html'), 'tarball contains package/dist/index.html');
      assert.ok(tarContents.includes('package/package.json'), 'tarball contains package/package.json');
    });
  });
});
