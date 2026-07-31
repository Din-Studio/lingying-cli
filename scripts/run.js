// Copyright (c) 2026 Lingying. SPDX-License-Identifier: MIT
// npm bin runner — delegates to the platform binary.
const { execFileSync } = require("child_process");
const path = require("path");
const os = require("os");

const NAME = "ly";
const isWindows = os.platform() === "win32";
const bin = path.join(__dirname, "..", "bin", NAME + (isWindows ? ".exe" : ""));

try {
  execFileSync(bin, process.argv.slice(2), { stdio: "inherit" });
} catch (e) {
  process.exit(e.status || 1);
}
