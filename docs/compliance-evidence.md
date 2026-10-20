---
title: "Compliance evidence"
description: Run periodic access reviews and export a signed evidence pack of audit, access, backup, approval, security setting and certificate records for an auditor.
"og:url": "https://docs.levelrail.com/compliance-evidence"
---

Auditors ask the same questions every cycle: who has access, was that reviewed, were backups taken and restore tested, who approved production deploys, and what were the security settings. This page covers the two features that answer them from records the control plane already keeps: **access reviews** and **evidence packs**.

## What this is, and what it is not

An evidence pack is a signed export of platform records for a date range. It is evidence an auditor can examine, for example for SOC 2 or ISO 27001 style requests about access control, change management and backup practice.

It is **not a certification**, an attestation, or proof that you meet any framework. It only covers what this control plane knows about. Controls that live elsewhere (laptops, HR onboarding, the cloud console) are outside it. Treat it as supporting material, not a verdict.

<InlineToc default-open />

## Access reviews

An access review is a campaign that snapshots every grant at the moment it starts:

| Row kind | What it lists |
| --- | --- |
| User account | Each user, their abilities, and when they last signed in. |
| Team membership | Each user in each team, with the team role. |
| API token | Each live token, its abilities, its owner or agent, and when it was last used. |
| Environment grant | Each environment a user is explicitly granted. |

Reviewers mark each row **keep** or **revoke**. Nothing changes while you decide. When every row has a decision, **Complete review** applies the revocations through the same operations the Users, Teams and Tokens screens use, and writes one audit row per revocation. Guards still hold: you cannot remove yourself, the last user or the last root account.

If a revocation fails, the successful ones stay applied, the failing rows show the reason, the campaign stays open, and you can retry or switch the row to keep. A campaign is only marked completed when every revocation applied.

A campaign has a due date. An open campaign past its due date appears in the attention center for root accounts. Set **repeat every N days** to have the next review opened automatically after the previous one is completed and N days have passed since it started.

```bash
levelrail-cli compliance access-review start --name "Q4 review" --due-days 14 --repeat-days 90
levelrail-cli compliance access-review show <campaign-id>
levelrail-cli compliance access-review keep <campaign-id> <item-id> <item-id>
levelrail-cli compliance access-review revoke <campaign-id> <item-id>
levelrail-cli compliance access-review complete <campaign-id>
```

Database roles created through database access are not part of a review yet. Review them from each database's access page.

## Evidence packs

Pick a date range (up to 366 days) in **Security center, Compliance**, or:

```bash
levelrail-cli compliance evidence create --from 2026-07-01 --to 2026-09-30 --out q3-evidence.zip
levelrail-cli compliance evidence verify <pack-id>
```

Generation runs as a background job with progress, so it never blocks a request, and large packs are written to disk and streamed on download. A pack is only listed as ready after the archive was fully written, hashed and moved into place. If generation fails or the control plane restarts mid-way, the pack is marked failed and no partial file is offered.

### Contents

| File | What it holds |
| --- | --- |
| `audit-log.ndjson` | Audit log rows in the range, one JSON object per line, with token-shaped values masked. |
| `access-reviews.json` | Campaigns created or completed in the range, with every row and decision. |
| `backups.json` | Backup runs in the range with status, size and restore test results. |
| `deploy-approvals.json` | Deploy approval requests, who requested and decided, and the outcome. |
| `security-settings.json` | Security policy, posture checks, session lifetime and audit retention at generation time. |
| `tls-certificates.json` | Uploaded and managed certificate names with dates. Certificate bodies and private keys are never included. |
| `manifest.json` | SHA-256 and size of every file, the range, the signing key id and a statement of what the pack is. |
| `manifest.sig` | Detached HMAC-SHA256 signature of `manifest.json`. |

Values that look like secrets are replaced with a placeholder before they reach the archive, using the same classifier as environment variable display.

### Verifying a pack

```bash
levelrail-cli compliance evidence verify <pack-id>
```

Verification recomputes every file hash against the manifest and checks the signature, and fails if a file was changed, added or removed, or if the manifest or signature was altered.

The signature key is derived from the control plane master key and never leaves it, so verification is done by the control plane that produced the pack. After a master key rotation, packs signed before the rotation can no longer be signature checked, though their file hashes still match the manifest. Keep the archive and its SHA-256 (shown in the pack list) with your audit records.

Everything here is root only. Without a master key configured, access reviews work but packs cannot be generated.
