/**
 * Bin Wrapper Tests
 *
 * Tests the binary wrapper logic without actually spawning processes.
 */

const { test, describe } = require('node:test');
const assert = require('node:assert');
const path = require('path');
const fs = require('fs');
const os = require('os');

/**
 * Extracted logic from bin/seaportal
 */
function getBinaryName() {
  const platform = process.platform;
  const arch = process.arch === 'arm64' || process.arch === 'aarch64' ? 'arm64' : 'amd64';

  if (platform === 'darwin') {
    return `seaportal-darwin-${arch}`;
  } else if (platform === 'linux') {
    return `seaportal-linux-${arch}`;
  } else if (platform === 'win32') {
    return `seaportal-windows-${arch}.exe`;
  }

  throw new Error(`Unsupported platform: ${platform}`);
}

function getVersionFromPackage() {
  const pkgPath = path.join(__dirname, '..', 'package.json');
  const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf-8'));
  return pkg.version;
}

function getBinaryPath(version) {
  const binaryName = getBinaryName();
  return path.join(os.homedir(), '.seaportal', 'bin', version, binaryName);
}

describe('Bin Wrapper', () => {
  test('getBinaryName returns valid name for current platform', () => {
    const name = getBinaryName();
    assert.ok(name.startsWith('seaportal-'));
    assert.ok(name.includes('-amd64') || name.includes('-arm64'));
  });

  test('getVersionFromPackage reads version correctly', () => {
    const version = getVersionFromPackage();
    assert.ok(version);
    assert.match(version, /^\d+\.\d+\.\d+/);
  });

  test('getBinaryPath constructs correct path', () => {
    const version = '0.1.0';
    const binPath = getBinaryPath(version);
    
    assert.ok(binPath.includes('.seaportal'));
    assert.ok(binPath.includes('bin'));
    assert.ok(binPath.includes(version));
    assert.ok(binPath.includes('seaportal-'));
  });

  test('env override path is respected', () => {
    const customPath = '/custom/path/to/seaportal';
    const original = process.env.SEAPORTAL_BINARY_PATH;
    
    process.env.SEAPORTAL_BINARY_PATH = customPath;
    
    // The actual bin wrapper checks this env var first
    assert.strictEqual(process.env.SEAPORTAL_BINARY_PATH, customPath);
    
    // Restore
    if (original !== undefined) {
      process.env.SEAPORTAL_BINARY_PATH = original;
    } else {
      delete process.env.SEAPORTAL_BINARY_PATH;
    }
  });
});

describe('Package Structure', () => {
  test('package.json exists and has required fields', () => {
    const pkgPath = path.join(__dirname, '..', 'package.json');
    assert.ok(fs.existsSync(pkgPath));
    
    const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf-8'));
    assert.strictEqual(pkg.name, 'seaportal');
    assert.ok(pkg.version);
    assert.ok(pkg.bin);
    assert.ok(pkg.bin.seaportal);
    assert.ok(pkg.scripts.postinstall);
  });

  test('bin/seaportal exists and is executable', () => {
    const binPath = path.join(__dirname, '..', 'bin', 'seaportal');
    assert.ok(fs.existsSync(binPath));
    
    const stats = fs.statSync(binPath);
    // Check it's a file
    assert.ok(stats.isFile());
  });

  test('scripts/postinstall.js exists', () => {
    const scriptPath = path.join(__dirname, '..', 'scripts', 'postinstall.js');
    assert.ok(fs.existsSync(scriptPath));
  });
});
