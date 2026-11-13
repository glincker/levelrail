---
title: "Proof automation"
description: "scripts/proof posts one sticky comment with script-generated evidence on a PR or ticket."
---

`scripts/proof` posts one sticky comment on a PR or ticket with evidence that
a script produced. It is a generated record, not a write-up.

## What it proves

- **Capabilities**: each line of `scripts/proof/capabilities.txt` is checked
  against the repo (CLI verb, API route, runtime setting, dashboard page, doc)
  and shown as PASS or MISSING with the exact check.
- **Metrics**: binary sizes, web chunk, CLI verbs, API paths, settings,
  migrations, test files, ADRs and docs pages, each with its source command.
- **Transcripts**: read-only CLI commands from `transcripts.txt`, shown as
  text with the exact command, trimmed to 20 lines.
- **Screenshots**: only pages mapped to files changed by the PR, at most
  `LEVELRAIL_PROOF_MAX_SHOTS` (default 4), 1280x800 WebP under 60 KB. A shot
  without a seed step is labelled "needs real data".

## The rule

Proof text is generated and never hand written. Every number or image in the
comment comes from a command run on the recorded commit. If something cannot
be measured the output says "not built" or exits with a reason, it is never
filled in by hand.

## Run it locally

    scripts/proof/run.sh --no-shots --dry-run            # print the comment
    scripts/proof/run.sh --pr 123                         # post or update
    scripts/proof/run.sh --issue 45 --pr 123 --strict     # fail on MISSING

Other flags: `--build` (opt in, builds both binaries), `--dry-run`.
`LEVELRAIL_PROOF_BASE` is the diff base (default `origin/main`),
`LEVELRAIL_PROOF_RSS=1` adds one idle RSS sample. Nothing installs a hook for
you; call `run.sh` from your own hook if you want it.

## Light by design

- Heavy steps run under `nice -n 15` and a hard `timeout` (shots 90 s,
  transcripts 30 s, whole run 5 min).
- No local builds unless `--build`; the workflow builds, local runs reuse
  existing binaries and `web/dist`.
- One headless browser, sequential, no video or tracing, no retina scale.
- Shots are skipped (tables still post) when load is above the core count
  (`LEVELRAIL_PROOF_MAX_LOAD`), free disk is under `LEVELRAIL_PROOF_MIN_DISK_GB`
  (10) or free memory is under `LEVELRAIL_PROOF_MIN_MEM_MB` (1024).
- The instance is a throwaway on a random high port with a temp data dir,
  removed by an exit trap. No Docker, never your real instance.

## Add a capability or a page

- Capability: add `label | kind | check` to `capabilities.txt`. Kinds are
  `cli-verb`, `route`, `setting`, `page` (path under `web/src/routes`) and `doc`.
- Page or card: add `path-glob | route | seed | label` to `affected.map`. A
  seed is a script in `scripts/proof/seed/`, or `none`.
- CLI change: add `path-glob | levelrail-cli ... --help` to `transcripts.txt`.

Images live on the `proof-artifacts` branch under `pr-<N>/<sha7>/`, newest
five sets kept. The `proof` workflow runs on the `proof` label or by dispatch.
