#!/usr/bin/env node
"use strict";

const { spawnSync } = require("node:child_process");

let launcher;
try {
  launcher = require.resolve("levelrail-cli/bin/levelrail-cli.js");
} catch {
  console.error("levelrail: the levelrail-cli package is missing. Reinstall, or use https://levelrail.com/install-cli.sh");
  process.exit(1);
}

const result = spawnSync(process.execPath, [launcher, ...process.argv.slice(2)], {
  stdio: "inherit",
  env: { ...process.env, LEVELRAIL_CLI_INSTALL_METHOD: "npm" },
});
if (result.error) {
  console.error(`levelrail: ${result.error.message}`);
  process.exit(1);
}
process.exit(result.status === null ? 1 : result.status);
