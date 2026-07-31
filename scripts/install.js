#!/usr/bin/env node
// Copyright (c) 2026 Lingying. SPDX-License-Identifier: MIT
// postinstall — downloads ly binary for the current platform.
//
//   npm install -g @lingying/cli    → downloads prebuilt binary from GitHub Releases
//   go install github.com/lingying/ly-cli@latest  → builds from source
//   git clone && make install       → builds locally
//
// If no prebuilt binary exists for this version, falls back with a helpful
// message pointing to the other install methods (exit code 0 — npm install
// should still succeed so go install / make install remain usable).
const fs = require("fs");
const path = require("path");
const { execSync, execFileSync } = require("child_process");

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
const GITHUB_URL = `https://github.com/lingying/ly-cli/releases/download/v${VERSION}/${asset}`;

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

// ── Download ──
console.log(`📦 Downloading ${NAME} ${VERSION} (${PLATFORM}-${ARCH})...`);

function download(url) {
  return new Promise((resolve, reject) => {
    const lib = url.startsWith("https") ? require("https") : require("http");
    lib.get(url, { headers: { "User-Agent": "ly-installer" } }, (res) => {
      if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
        return resolve(download(res.headers.location));
      }
      if (res.statusCode === 404) return reject(new Error("NO_RELEASE"));
      if (res.statusCode !== 200) return reject(new Error(`HTTP ${res.statusCode}`));
      const chunks = [];
      res.on("data", (c) => chunks.push(c));
      res.on("end", () => resolve(Buffer.concat(chunks)));
    }).on("error", reject);
  });
}

function extractTarGz(data, outDir) {
  // Use system tar (all macOS/Linux have it) with explicit args — no shell.
  const tmp = path.join(outDir, asset);
  fs.mkdirSync(outDir, { recursive: true });
  fs.writeFileSync(tmp, data);
  try {
    execFileSync("tar", ["-xzf", tmp, "-C", outDir], { stdio: "ignore" });
  } finally {
    fs.unlinkSync(tmp);
  }
}

function extractZip(data, outDir) {
  const tmp = path.join(outDir, asset);
  fs.mkdirSync(outDir, { recursive: true });
  fs.writeFileSync(tmp, data);
  try {
    execFileSync("powershell", [
      "-Command",
      `Expand-Archive -Force '${tmp}' '${outDir}'`,
    ], { stdio: "ignore" });
  } finally {
    fs.unlinkSync(tmp);
  }
}

async function main() {
  try {
    const data = await download(GITHUB_URL);
    if (isWindows) {
      extractZip(data, binDir);
    } else {
      extractTarGz(data, binDir);
    }

    // Binary might be nested; find and move it
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
      console.log(`   Install from source:`);
      console.log(`     go install github.com/lingying/ly-cli@latest`);
      console.log(`   or:`);
      console.log(`     git clone https://github.com/lingying/ly-cli && cd ly-cli && make install`);
    } else {
      console.error(`❌ ${err.message}`);
    }
    // Don't fail npm install — user can still use source-based install
    process.exit(0);
  }
}

main();
