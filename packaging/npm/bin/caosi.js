#!/usr/bin/env node

const fs = require("node:fs");
const path = require("node:path");
const { spawn } = require("node:child_process");

function resolveAssetName() {
  switch (process.platform) {
    case "darwin":
      if (process.arch === "arm64") return "caosi-darwin-arm64";
      if (process.arch === "x64") return "caosi-darwin-amd64";
      break;
    case "linux":
      if (process.arch === "arm64") return "caosi-linux-arm64";
      if (process.arch === "x64") return "caosi-linux-amd64";
      break;
    case "win32":
      if (process.arch === "x64") return "caosi-windows-amd64.exe";
      break;
    default:
      break;
  }
  return "";
}

const assetName = resolveAssetName();
const binaryPath = assetName
  ? path.join(__dirname, "..", "vendor", assetName)
  : "";

if (!assetName) {
  console.error(`caosi: unsupported platform ${process.platform}/${process.arch}`);
  process.exit(1);
}

if (!fs.existsSync(binaryPath)) {
  console.error(
    "caosi: bundled binary is missing. Reinstall with `npm install -g @thomas-huang/caosi`."
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
