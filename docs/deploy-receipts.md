---
title: Deploy receipts
description: A downloadable record of one deploy, with the commit, image digest, who asked, who approved and which gates ran.
---

# Deploy receipts

Every deploy has a receipt: one document that answers what ran, who asked for it, who approved it and which checks it passed. It is built on request from data the control plane already stores, so there is nothing to enable and no extra storage.

A receipt is **unsigned**. It is a convenient summary, not an attestation, and it proves nothing to a third party on its own. Signing is planned separately.

## What it contains

| Field | Source |
| --- | --- |
| `commit` | SHA, branch, author and message from the trigger |
| `image` | tag, digest and `digest_source` (resolved, already pinned, local build and so on) |
| `requester`, `approval` | who asked, the approval record, who decided and when, any freeze override reason |
| `sbom` | package count, scan status and finding counts; `recorded: false` with a note `not recorded` when no SBOM exists (for example builds on remote nodes) |
| `gate_timeline` | readiness and post-cutover watch events in order |
| `verdict`, `rerun_of` | how the deploy ended and the attempt it retried |

It never contains env values, secret values, tokens or build arguments. Anything unknown is written as `not recorded` rather than guessed.

## Download

- Dashboard: the **Receipt** button on a deploy's page.
- CLI: `levelrail-cli apps deploys receipt NAME DEPLOY_ID --out receipt.json` (add `--format md` for a readable version).
- API: `GET /api/v1/apps/{name}/deployments/{id}/receipt` with optional `?format=md`. Needs read access to the app.

## Approvals

For a protected environment the receipt names both people: the requester and the approver. The approval is matched to the deploy by the image and the time it was decided, so a deploy that bypassed approval shows `no approval on record`.
