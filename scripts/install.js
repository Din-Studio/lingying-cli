#!/usr/bin/env node
// Copyright (c) 2026 Lingying. SPDX-License-Identifier: MIT
// postinstall — downloads ly binary for the current platform.
//
//   npm install -g @lingying/cli    → downloads prebuilt binary from GitHub Releases
//   go install github.com/Din-Studio/lingying-cli@latest  → builds from source
//   git clone && make install       → builds locally
//
// If no prebuilt binary exists for this version, falls back with a helpful
// message pointing to the other install methods (exit code 0 — npm install
// should still succeed so go install / make install remain usable).
const fs = require("fs");
const path = require("path");
const crypto = require("crypto");
const { execFileSync } = require("child_process");

const NAME = "ly";
const VERSION = require("../package.json").version;

const PLATFORM = { darwin: "darwin", linux: "linux", win32: "windows" }[process.platform];
const ARCH = { x64: "amd64", arm64: "arm64" }[process.arch];
if (!PLATFORM || !ARCH) {
  console.error(`❌ ly: unsupported platform ${process.platform}/${process.arch}`);
  process.exit(1);
}

const isWindows = process.platform === "win32";
const ext = isWindows ? ".zip" : ".tar.gz";
const asset = `${NAME}-${VERSION}-${PLATFORM}-${ARCH}${ext}`;
const RELEASE_BASE = `https://github.com/Din-Studio/lingying-cli/releases/download/v${VERSION}`;
const BINARY_URL = `${RELEASE_BASE}/${asset}`;
const CHECKSUM_URL = `${RELEASE_BASE}/checksums.txt`;

// Domain allowlist for redirects — only follow redirects to these hosts.
const ALLOWED_HOSTS = new Set([
  "github.com",
  "objects.githubusercontent.com",
  "github-releases.githubusercontent.com",
]);

const binDir = path.join(__dirname, "..", "bin");
const dest = path.join(binDir, NAME + (isWindows ? ".exe" : ""));

// ── Already installed? ──
if (fs.existsSync(dest)) {
  try {
    const out = execFileSync(dest, ["--version"], { encoding: "utf-8", timeout: 5000 }).trim();
    if (out.includes(VERSION)) {
      console.log(`✅ ${NAME} ${VERSION} already installed`);
      process.exit(0);
    }
    console.log(`📦 ly ${out} → upgrading to ${VERSION}`);
  } catch (_) { /* binary broken, re-download */ }
}

// ── Use local build if in repo root ──
const localBinary = path.join(__dirname, "..", "..", NAME);
if (fs.existsSync(localBinary)) {
  console.log(`📦 Using local build: ${localBinary}`);
  fs.mkdirSync(binDir, { recursive: true });
  fs.copyFileSync(localBinary, dest);
  fs.chmodSync(dest, 0o755);
  console.log(`✅ ${NAME} ${VERSION} ready`);
  process.exit(0);
}

// ── Download helpers ──

/**
 * Parse a URL, following redirects only to allowed hosts.
 * Returns { body: Buffer, finalUrl: string }.
 */
function fetchWithRedirectCheck(url, maxRedirects = 10) {
  return new Promise((resolve, reject) => {
    const lib = url.startsWith("https") ? require("https") : require("http");
    lib.get(url, { headers: { "User-Agent": "ly-installer" } }, (res) => {
      if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
        if (maxRedirects <= 0) return reject(new Error("Too many redirects"));
        const target = new URL(res.headers.location, url);
        if (!ALLOWED_HOSTS.has(target.hostname)) {
          return reject(
            new Error(`Redirect to untrusted host: ${target.hostname} (allowed: ${[...ALLOWED_HOSTS].join(", ")})`)
          );
        }
        return resolve(fetchWithRedirectCheck(target.href, maxRedirects - 1));
      }
      if (res.statusCode !== 200) {
        // 404 specifically means no prebuilt binary.
        if (res.statusCode === 404) return reject(new Error("NO_RELEASE"));
        return reject(new Error(`HTTP ${res.statusCode} from ${url}`));
      }
      const chunks = [];
      res.on("data", (c) => chunks.push(c));
      res.on("end", () => resolve(Buffer.concat(chunks)));
    }).on("error", reject);
  });
}

/**
 * Verify `data` Buffer matches the expected SHA-256 hex digest.
 */
function verifyChecksum(data, expectedHex) {
  const actual = crypto.createHash("sha256").update(data).digest("hex");
  if (actual !== expectedHex.toLowerCase()) {
    throw new Error(
      `Checksum mismatch!\n  expected: ${expectedHex.toLowerCase()}\n  actual:   ${actual}`
    );
  }
}

/**
 * Download checksums.txt, parse out the expected SHA-256 for `asset`.
 */
async function fetchExpectedChecksum() {
  const body = await fetchWithRedirectCheck(CHECKSUM_URL);
  const text = body.toString("utf-8");
  // goreleaser format: <sha256>  <filename>
  // Lines look like:  b5bb9d8014a0f9b1d61e21e796d78dccdf1352f23cd32812f4850b878ae4944c  ly-0.1.0-darwin-amd64.tar.gz
  for (const line of text.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    const parts = trimmed.split(/\s+/);
    if (parts.length < 2) continue;
    const [hash, ...filenameParts] = parts;
    const filename = filenameParts.join(" "); // handle filenames with spaces defensively
    if (filename === asset) {
      if (!/^[0-9a-fA-F]{64}$/.test(hash)) {
        throw new Error(`Invalid checksum format in checksums.txt: "${hash}"`);
      }
      return hash;
    }
  }
  throw new Error(`Asset "${asset}" not found in checksums.txt`);
}

// ── Archive extraction ──

function extractTarGz(data, outDir) {
  const tmp = path.join(outDir, asset);
  fs.mkdirSync(outDir, { recursive: true });
  fs.writeFileSync(tmp, data);
  try {
    // Pre-check: list archive contents and verify no path traversal.
    const listing = execFileSync("tar", ["-tzf", tmp], { encoding: "utf-8" });
    for (const entry of listing.split("\n")) {
      const trimmed = entry.trim();
      if (!trimmed) continue;
      if (trimmed.startsWith("/") || trimmed.includes("..")) {
        throw new Error(`Path traversal detected in archive: "${trimmed}"`);
      }
    }
    execFileSync("tar", ["-xzf", tmp, "-C", outDir], { stdio: "ignore" });
  } finally {
    fs.unlinkSync(tmp);
  }
}

function extractZip(data, outDir) {
  // Use Node.js built-in instead of shelling out to PowerShell.
  // Node ≥16 doesn't have a built-in unzip — delegate to unzip on unix,
  // or use Expand-Archive on Windows with proper quoting.
  const tmp = path.join(outDir, asset);
  fs.mkdirSync(outDir, { recursive: true });
  fs.writeFileSync(tmp, data);
  try {
    // Pre-check: list archive contents and verify no path traversal.
    const listing = execFileSync("powershell", [
      "-NoProfile", "-Command",
      `[System.IO.Compression.ZipFile]::OpenRead('${tmp.replace(/'/g, "''")}').Entries | ForEach-Object { $_.FullName }`,
    ], { encoding: "utf-8" });
    for (const entry of listing.split(/\r?\n/)) {
      const trimmed = entry.trim();
      if (!trimmed) continue;
      if (trimmed.startsWith("/") || trimmed.startsWith("\\") || trimmed.includes("..")) {
        throw new Error(`Path traversal detected in archive: "${trimmed}"`);
      }
    }
    execFileSync("powershell", [
      "-NoProfile", "-Command",
      `Expand-Archive -Force -LiteralPath '${tmp.replace(/'/g, "''")}' -DestinationPath '${outDir.replace(/'/g, "''")}'`,
    ], { stdio: "ignore" });
  } finally {
    fs.unlinkSync(tmp);
  }
}

// ── Main ──

async function main() {
  try {
    // 1. Fetch checksums.txt and resolve the expected hash.
    console.log(`📦 Verifying checksums for ${NAME} ${VERSION}...`);
    const expectedHash = await fetchExpectedChecksum();

    // 2. Download the binary archive.
    console.log(`📦 Downloading ${NAME} ${VERSION} (${PLATFORM}-${ARCH})...`);
    const data = await fetchWithRedirectCheck(BINARY_URL);

    // 3. Verify SHA-256 checksum BEFORE extraction.
    verifyChecksum(data, expectedHash);
    console.log("✅ Checksum verified");

    // 4. Extract.
    if (isWindows) {
      extractZip(data, binDir);
    } else {
      extractTarGz(data, binDir);
    }

    // Binary might be nested; find and move it.
    if (!fs.existsSync(dest)) {
      const walk = (dir) => {
        for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
          const full = path.join(dir, entry.name);
          if (entry.isDirectory()) { walk(full); continue; }
          if (entry.name === NAME || entry.name === NAME + ".exe") {
            fs.renameSync(full, dest);
            return true;
          }
        }
        return false;
      };
      walk(binDir);
    }

    if (!fs.existsSync(dest)) {
      throw new Error("binary not found in archive");
    }

    fs.chmodSync(dest, 0o755);
    console.log(`✅ ${NAME} ${VERSION} installed`);
  } catch (err) {
    if (err.message === "NO_RELEASE") {
      console.log(`⚠  No prebuilt binary for v${VERSION} yet.`);
      console.log("   Install from source:");
      console.log("     go install github.com/Din-Studio/lingying-cli@latest");
      console.log("   or:");
      console.log("     git clone https://github.com/Din-Studio/lingying-cli && cd ly-cli && make install");
    } else {
      console.error(`❌ ${err.message}`);
    }
    // Don't fail npm install — user can still use source-based install.
    process.exit(0);
  }
}

main();
