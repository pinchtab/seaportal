#!/usr/bin/env node

const https = require('https');
const fs = require('fs');
const path = require('path');
const os = require('os');
const crypto = require('crypto');

const GITHUB_REPO = 'pinchtab/seaportal';

function getVersion() {
  const pkgPath = path.join(__dirname, '..', 'package.json');
  const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf-8'));
  return pkg.version;
}

function detectPlatform() {
  const platform = process.platform;

  let arch;
  if (process.arch === 'x64') {
    arch = 'amd64';
  } else if (process.arch === 'arm64') {
    arch = 'arm64';
  } else {
    throw new Error(
      `Unsupported architecture: ${process.arch}. Only x64 (amd64) and arm64 are supported.`
    );
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

  return { os: detectedOS, arch };
}

function getBinaryName(platform) {
  const { os, arch } = platform;
  if (os === 'windows') {
    return `seaportal-${os}-${arch}.exe`;
  }
  return `seaportal-${os}-${arch}`;
}

function fetchUrl(url, maxRedirects = 5) {
  return new Promise((resolve, reject) => {
    const attemptFetch = (currentUrl, redirectsRemaining) => {
      https
        .get(currentUrl, (response) => {
          if ([301, 302, 307, 308].includes(response.statusCode)) {
            if (redirectsRemaining <= 0) {
              reject(new Error(`Too many redirects from ${currentUrl}`));
              return;
            }
            let redirectUrl = response.headers.location;
            if (!redirectUrl) {
              reject(new Error(`Redirect without location header`));
              return;
            }
            try {
              redirectUrl = new URL(redirectUrl, currentUrl).toString();
            } catch {
              reject(new Error(`Invalid redirect URL: ${redirectUrl}`));
              return;
            }
            response.resume();
            attemptFetch(redirectUrl, redirectsRemaining - 1);
            return;
          }

          if (response.statusCode === 404) {
            reject(new Error(`Not found: ${currentUrl}`));
            return;
          }

          if (response.statusCode !== 200) {
            reject(new Error(`HTTP ${response.statusCode}: ${currentUrl}`));
            return;
          }

          const chunks = [];
          response.on('data', (chunk) => chunks.push(chunk));
          response.on('end', () => resolve(Buffer.concat(chunks)));
          response.on('error', reject);
        })
        .on('error', reject);
    };
    attemptFetch(url, maxRedirects);
  });
}

async function downloadChecksums(version) {
  const url = `https://github.com/${GITHUB_REPO}/releases/download/v${version}/checksums.txt`;
  try {
    const data = await fetchUrl(url);
    const checksums = new Map();
    data
      .toString('utf-8')
      .trim()
      .split('\n')
      .forEach((line) => {
        const parts = line.split(/\s+/);
        if (parts.length >= 2) {
          checksums.set(parts[1].trim(), parts[0].trim());
        }
      });
    return checksums;
  } catch (err) {
    throw new Error(
      `Failed to download checksums: ${err.message}\nEnsure v${version} is released on GitHub.`
    );
  }
}

function verifySHA256(filePath, expectedHash) {
  const hash = crypto.createHash('sha256');
  const data = fs.readFileSync(filePath);
  hash.update(data);
  return hash.digest('hex').toLowerCase() === expectedHash.toLowerCase();
}

async function downloadBinary(platform, version) {
  const binaryName = getBinaryName(platform);
  const binDir = path.join(os.homedir(), '.seaportal', 'bin');
  const versionDir = path.join(binDir, version);
  const binaryPath = path.join(versionDir, binaryName);

  // Verify existing binary
  if (fs.existsSync(binaryPath)) {
    try {
      const checksums = await downloadChecksums(version);
      if (checksums.has(binaryName) && verifySHA256(binaryPath, checksums.get(binaryName))) {
        console.log(`✓ SeaPortal binary verified: ${binaryPath}`);
        return;
      }
      console.warn(`⚠ Existing binary failed checksum, re-downloading...`);
      fs.unlinkSync(binaryPath);
    } catch {
      console.warn(`⚠ Could not verify existing binary, re-downloading...`);
      try {
        fs.unlinkSync(binaryPath);
      } catch {}
    }
  }

  console.log(`Downloading SeaPortal ${version} for ${platform.os}-${platform.arch}...`);
  const checksums = await downloadChecksums(version);

  if (!checksums.has(binaryName)) {
    throw new Error(
      `Binary not found in checksums: ${binaryName}\nAvailable: ${Array.from(checksums.keys()).join(', ')}`
    );
  }

  const expectedHash = checksums.get(binaryName);
  const downloadUrl = `https://github.com/${GITHUB_REPO}/releases/download/v${version}/${binaryName}`;

  if (!fs.existsSync(versionDir)) {
    fs.mkdirSync(versionDir, { recursive: true });
  }

  const tempPath = `${binaryPath}.tmp`;

  return new Promise((resolve, reject) => {
    console.log(`Downloading from ${downloadUrl}...`);

    const file = fs.createWriteStream(tempPath);
    let redirectCount = 0;

    const performDownload = (url) => {
      https
        .get(url, (response) => {
          if ([301, 302, 307, 308].includes(response.statusCode)) {
            if (redirectCount >= 5) {
              fs.unlink(tempPath, () => {});
              reject(new Error(`Too many redirects`));
              return;
            }
            let redirectUrl = response.headers.location;
            try {
              redirectUrl = new URL(redirectUrl, url).toString();
            } catch {
              fs.unlink(tempPath, () => {});
              reject(new Error(`Invalid redirect URL`));
              return;
            }
            redirectCount++;
            response.resume();
            performDownload(redirectUrl);
            return;
          }

          if (response.statusCode !== 200) {
            fs.unlink(tempPath, () => {});
            reject(new Error(`HTTP ${response.statusCode}: ${url}`));
            return;
          }

          response.pipe(file);

          file.on('finish', () => {
            file.close();

            if (!verifySHA256(tempPath, expectedHash)) {
              fs.unlink(tempPath, () => {});
              reject(new Error(`Checksum verification failed. Please try again.`));
              return;
            }

            try {
              fs.renameSync(tempPath, binaryPath);
              fs.chmodSync(binaryPath, 0o755);
            } catch (err) {
              fs.unlink(tempPath, () => {});
              reject(new Error(`Failed to finalize binary: ${err.message}`));
              return;
            }

            console.log(`✓ Verified and installed: ${binaryPath}`);
            resolve();
          });

          file.on('error', (err) => {
            fs.unlink(tempPath, () => {});
            reject(err);
          });
        })
        .on('error', reject);
    };

    performDownload(downloadUrl);
  });
}

(async () => {
  try {
    const platform = detectPlatform();
    const version = getVersion();

    if (!process.env.SEAPORTAL_BINARY_PATH) {
      try {
        await downloadBinary(platform, version);
      } catch (err) {
        const errMsg = err instanceof Error ? err.message : String(err);
        if (errMsg.includes('404') || errMsg.includes('Not found')) {
          console.warn('\n⚠ SeaPortal binary not yet available (release in progress).');
          console.warn('  On first use, run: npm rebuild seaportal');
          process.exit(0);
        }
        throw err;
      }
    }

    console.log('✓ SeaPortal setup complete');
    process.exit(0);
  } catch (err) {
    console.error('\n✗ Failed to setup SeaPortal:');
    console.error(`  ${(err instanceof Error ? err.message : String(err)).split('\n').join('\n  ')}`);
    console.error('\nTroubleshooting:');
    console.error('  • Check your internet connection');
    console.error('  • Verify the release: https://github.com/pinchtab/seaportal/releases');
    console.error('  • Try again: npm rebuild seaportal');
    console.error('\nFor custom binaries:');
    console.error('  export SEAPORTAL_BINARY_PATH=/path/to/seaportal');
    process.exit(1);
  }
})();
