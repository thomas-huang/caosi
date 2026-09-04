#!/usr/bin/env node

const crypto = require("node:crypto");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

const packageRoot = path.join(__dirname, "..");
const pkg = JSON.parse(fs.readFileSync(path.join(packageRoot, "package.json"), "utf8"));
const vendorDir = path.join(packageRoot, "vendor");

const repoBaseUrl =
  process.env.CAOSI_NPM_BASE_URL || "https://github.com/thomas-huang/caosi/releases/download";
const version = String(pkg.version || "").trim();
const versionTag = version.startsWith("v") ? version : `v${version}`;

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
  throw new Error(`unsupported platform ${process.platform}/${process.arch}`);
}

function skipDownload() {
  if (process.env.CAOSI_SKIP_DOWNLOAD === "1") {
    return "CAOSI_SKIP_DOWNLOAD=1";
  }
  if (version === "" || version === "0.0.0-dev" || version === "dev") {
    return `package version ${version || "(empty)"} has no GitHub Release`;
  }
  return "";
}

async function download(url, destination) {
  const response = await fetch(url, {
    headers: {
      "user-agent": `caosi-npm/${version}`
    },
    redirect: "follow"
  });
  if (!response.ok) {
    throw new Error(`download failed for ${url}: HTTP ${response.status}`);
  }
  const buf = Buffer.from(await response.arrayBuffer());
  fs.writeFileSync(destination, buf);
}

function parseChecksums(text) {
  const map = new Map();
  for (const line of text.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    const match = trimmed.match(/^([a-f0-9]{64})\s+\*?(.+)$/i);
    if (!match) {
      throw new Error(`invalid checksums line: ${line}`);
    }
    map.set(match[2], match[1].toLowerCase());
  }
  return map;
}

function sha256(filePath) {
  const hash = crypto.createHash("sha256");
  hash.update(fs.readFileSync(filePath));
  return hash.digest("hex");
}

async function main() {
  const reason = skipDownload();
  if (reason) {
    console.log(`caosi: skipping binary download (${reason})`);
    return;
  }

  const assetName = resolveAssetName();
  const binaryName = process.platform === "win32" ? "caosi.exe" : "caosi";
  const targetPath = path.join(vendorDir, binaryName);
  const checksumsUrl = `${repoBaseUrl}/${versionTag}/checksums.txt`;

  fs.mkdirSync(vendorDir, { recursive: true });

  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), "caosi-npm-"));
  const checksumsPath = path.join(tempDir, "checksums.txt");
  const downloadPath = path.join(tempDir, assetName);

  try {
    console.log(`caosi: downloading ${assetName} for ${process.platform}/${process.arch}`);
    await download(checksumsUrl, checksumsPath);
    const checksums = parseChecksums(fs.readFileSync(checksumsPath, "utf8"));
    const expectedSha = checksums.get(assetName);
    if (!expectedSha) {
      throw new Error(`checksums.txt does not contain ${assetName}`);
    }

    await download(`${repoBaseUrl}/${versionTag}/${assetName}`, downloadPath);
    const actualSha = sha256(downloadPath);
    if (actualSha !== expectedSha) {
      throw new Error(`checksum mismatch for ${assetName}`);
    }

    fs.copyFileSync(downloadPath, targetPath);
    if (process.platform !== "win32") {
      fs.chmodSync(targetPath, 0o755);
    }
    console.log(`caosi: installed binary to ${path.relative(packageRoot, targetPath)}`);
  } finally {
    fs.rmSync(tempDir, { recursive: true, force: true });
  }
}

main().catch((err) => {
  console.error(`caosi: postinstall failed: ${err.message}`);
  process.exit(1);
});
