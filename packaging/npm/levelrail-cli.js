#!/usr/bin/env node
"use strict";

const { spawnSync } = require("node:child_process");

const OS = { linux: "linux", darwin: "darwin", win32: "windows" };
const ARCH = { x64: "amd64", arm64: "arm64" };

function binaryPath() {
  if (process.env.LEVELRAIL_CLI_BINARY) return process.env.LEVELRAIL_CLI_BINARY;
  const os = OS[process.platform];
  const arch = ARCH[process.arch];
  if (!os || !arch) {
    throw new Error(`levelrail-cli has no build for ${process.platform}/${process.arch}`);
  }
  const pkg = `levelrail-cli-${os}-${arch}`;
  const file = process.platform === "win32" ? "levelrail-cli.exe" : "levelrail-cli";
  try {
    return require.resolve(`${pkg}/bin/${file}`);
  } catch {
    throw new Error(
      `the ${pkg} package is missing. Reinstall without --omit=optional, or use the install script: https://levelrail.com/install-cli.sh`,
    );
  }
}

let bin;
try {
  bin = binaryPath();
} catch (err) {
  console.error(`levelrail-cli: ${err.message}`);
  process.exit(1);
}

const result = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });
if (result.error) {
  console.error(`levelrail-cli: ${result.error.message}`);
  process.exit(1);
}
process.exit(result.status === null ? 1 : result.status);
