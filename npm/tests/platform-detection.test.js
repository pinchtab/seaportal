/**
 * Platform Detection Tests
 *
 * Verifies that the platform detection logic correctly maps Node.js process.platform/process.arch
 * to the goreleaser binary filenames.
 *
 * Matrix:
 *   process.platform | process.arch | Expected Binary
 *   ───────────────────────────────────────────────────────
 *   darwin          | x64          | seaportal-darwin-amd64
 *   darwin          | arm64        | seaportal-darwin-arm64
 *   linux           | x64          | seaportal-linux-amd64
 *   linux           | arm64        | seaportal-linux-arm64
 *   win32           | x64          | seaportal-windows-amd64.exe
 *   win32           | arm64        | seaportal-windows-arm64.exe
 */

const { test, describe } = require('node:test');
const assert = require('node:assert');

/**
 * Extracted detectPlatform logic from postinstall.js
 */
function detectPlatform(platform, arch) {
  let mappedArch;
  if (arch === 'x64') {
    mappedArch = 'amd64';
  } else if (arch === 'arm64') {
    mappedArch = 'arm64';
  } else {
    throw new Error(`Unsupported architecture: ${arch}. Only x64 (amd64) and arm64 are supported.`);
  }

  const osMap = {
    darwin: 'darwin',
    linux: 'linux',
    win32: 'windows',
  };

  const detectedOS = osMap[platform];
  if (!detectedOS) {
    throw new Error(`Unsupported platform: ${platform}`);
  }

  return { os: detectedOS, arch: mappedArch };
}

function getBinaryName(platform) {
  const { os, arch } = platform;
  if (os === 'windows') {
    return `seaportal-${os}-${arch}.exe`;
  }
  return `seaportal-${os}-${arch}`;
}

describe('Platform Detection', () => {
  describe('detectPlatform', () => {
    test('darwin + x64 → darwin-amd64', () => {
      const platform = detectPlatform('darwin', 'x64');
      assert.strictEqual(platform.os, 'darwin');
      assert.strictEqual(platform.arch, 'amd64');
    });

    test('darwin + arm64 → darwin-arm64', () => {
      const platform = detectPlatform('darwin', 'arm64');
      assert.strictEqual(platform.os, 'darwin');
      assert.strictEqual(platform.arch, 'arm64');
    });

    test('linux + x64 → linux-amd64', () => {
      const platform = detectPlatform('linux', 'x64');
      assert.strictEqual(platform.os, 'linux');
      assert.strictEqual(platform.arch, 'amd64');
    });

    test('linux + arm64 → linux-arm64', () => {
      const platform = detectPlatform('linux', 'arm64');
      assert.strictEqual(platform.os, 'linux');
      assert.strictEqual(platform.arch, 'arm64');
    });

    test('win32 + x64 → windows-amd64', () => {
      const platform = detectPlatform('win32', 'x64');
      assert.strictEqual(platform.os, 'windows');
      assert.strictEqual(platform.arch, 'amd64');
    });

    test('win32 + arm64 → windows-arm64', () => {
      const platform = detectPlatform('win32', 'arm64');
      assert.strictEqual(platform.os, 'windows');
      assert.strictEqual(platform.arch, 'arm64');
    });

    test('unsupported platform → error', () => {
      assert.throws(() => detectPlatform('freebsd', 'x64'), /Unsupported platform: freebsd/);
    });

    test('unsupported arch → error', () => {
      assert.throws(() => detectPlatform('linux', 'ia32'), /Unsupported architecture: ia32/);
    });
  });

  describe('getBinaryName', () => {
    test('darwin-amd64 → seaportal-darwin-amd64', () => {
      const name = getBinaryName({ os: 'darwin', arch: 'amd64' });
      assert.strictEqual(name, 'seaportal-darwin-amd64');
    });

    test('darwin-arm64 → seaportal-darwin-arm64', () => {
      const name = getBinaryName({ os: 'darwin', arch: 'arm64' });
      assert.strictEqual(name, 'seaportal-darwin-arm64');
    });

    test('linux-amd64 → seaportal-linux-amd64', () => {
      const name = getBinaryName({ os: 'linux', arch: 'amd64' });
      assert.strictEqual(name, 'seaportal-linux-amd64');
    });

    test('linux-arm64 → seaportal-linux-arm64', () => {
      const name = getBinaryName({ os: 'linux', arch: 'arm64' });
      assert.strictEqual(name, 'seaportal-linux-arm64');
    });

    test('windows-amd64 → seaportal-windows-amd64.exe', () => {
      const name = getBinaryName({ os: 'windows', arch: 'amd64' });
      assert.strictEqual(name, 'seaportal-windows-amd64.exe');
    });

    test('windows-arm64 → seaportal-windows-arm64.exe', () => {
      const name = getBinaryName({ os: 'windows', arch: 'arm64' });
      assert.strictEqual(name, 'seaportal-windows-arm64.exe');
    });
  });

  describe('Full Matrix', () => {
    const matrix = [
      { platform: 'darwin', arch: 'x64', expected: 'seaportal-darwin-amd64' },
      { platform: 'darwin', arch: 'arm64', expected: 'seaportal-darwin-arm64' },
      { platform: 'linux', arch: 'x64', expected: 'seaportal-linux-amd64' },
      { platform: 'linux', arch: 'arm64', expected: 'seaportal-linux-arm64' },
      { platform: 'win32', arch: 'x64', expected: 'seaportal-windows-amd64.exe' },
      { platform: 'win32', arch: 'arm64', expected: 'seaportal-windows-arm64.exe' },
    ];

    matrix.forEach(({ platform, arch, expected }) => {
      test(`${platform}/${arch} → ${expected}`, () => {
        const p = detectPlatform(platform, arch);
        const binary = getBinaryName(p);
        assert.strictEqual(binary, expected);
      });
    });
  });
});
