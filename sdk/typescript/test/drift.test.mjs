import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { dirname, join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "../../..");

// Fails when docs/public/openapi.json changed without regenerating sdk/typescript.
test("generated SDK matches docs/public/openapi.json", () => {
  const r = spawnSync(process.execPath, [join(root, "scripts/gen-ts-sdk.mjs"), "--check"], { encoding: "utf8" });
  assert.equal(r.status, 0, r.stderr || r.stdout);
});
