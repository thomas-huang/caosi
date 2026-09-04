#!/usr/bin/env node

const fs = require("node:fs");
const path = require("node:path");
const { spawn } = require("node:child_process");

const binaryName = process.platform === "win32" ? "caosi.exe" : "caosi";
const binaryPath = path.join(__dirname, "..", "vendor", binaryName);

if (!fs.existsSync(binaryPath)) {
  console.error(
    "caosi: bundled binary is missing. Reinstall with `npm install -g @thomas-huang/caosi` after a GitHub Release exists, or run `npm rebuild @thomas-huang/caosi`."
  );
  process.exit(1);
}

const child = spawn(binaryPath, process.argv.slice(2), {
  stdio: "inherit"
});

child.on("exit", (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }
  process.exit(code ?? 1);
});

child.on("error", (err) => {
  console.error(`caosi: failed to start bundled binary: ${err.message}`);
  process.exit(1);
});
