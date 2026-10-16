---
title: "Secret scanning"
description: "Find credentials committed to a repo or stored as plain env variables before a deploy builds, with a per-app off, warn or block policy and an allow-list."
---

Before a deploy builds, the control plane can check the build context for committed credentials and the environment values being injected for credentials stored as plain (non-secret) variables. It is on in `warn` mode by default and never stores or shows the value it found.

| Part | What it does | Default |
| --- | --- | --- |
| Build context scan | Reads the checked-out repository, inside the build directory only, and matches each line against the built-in rules. | On, `warn`. |
| Environment scan | Checks the plain env values of the deploy. Values marked secret, vault-backed or referenced from another resource are not scanned, they are not committed anywhere. | On, `warn`. |
| Policy | What a finding does to a deploy: `off`, `warn` or `block`. Set per app. | `warn`. |
| Allow-list | Fingerprints you have reviewed and accepted, each with who added it and why. | Empty. |

## What a finding contains

A finding holds the rule id, the file path and line (or the variable name), and a 16 character fingerprint. The fingerprint is a hash of the rule and the matched value, so it stays the same when the file moves and lets you allow-list one value without the value ever being stored. The value itself is never written to the database, the deploy log or the API.

## Rules

| Rule id | Matches |
| --- | --- |
| `aws-access-key-id` | AWS access key ids |
| `aws-secret-access-key` | A 40 character AWS secret next to an `aws` and `secret` name |
| `gcp-api-key` | Google API keys |
| `azure-storage-key` | Azure storage `AccountKey=` values |
| `private-key` | PEM private key headers (RSA, EC, DSA, OpenSSH, PGP) |
| `github-token`, `github-fine-grained-token` | GitHub personal, OAuth, app and fine-grained tokens |
| `gitlab-token` | GitLab personal access tokens |
| `slack-token`, `slack-webhook` | Slack tokens and incoming webhook URLs |
| `stripe-secret-key` | Stripe live secret and restricted keys |
| `sendgrid-key` | SendGrid API keys |
| `npm-token` | npm access tokens |
| `jwt` | JSON web tokens |
| `url-credentials` | A password inside a URL such as a database connection string. Placeholders like `${DB_PASSWORD}` are ignored. |
| `high-entropy-secret` | A long, high-entropy value assigned to a variable whose name says it is a credential (`SESSION_SECRET`, `API_TOKEN`). Browser-public prefixes such as `NEXT_PUBLIC_` are skipped. |

These are pattern checks, not proof. A finding means "look at this", and a clean scan does not mean the repository has no secrets.

## Policy

| Policy | A finding | A scan that cannot finish |
| --- | --- | --- |
| `off` | Nothing runs. | Nothing runs. |
| `warn` | The deploy goes live and is marked with the finding count and locations. | The deploy goes live and the failure is recorded. |
| `block` | The deploy stops before the build and the previous release keeps serving. The reason is shown in deploy history. | The deploy is blocked. A block policy never passes silently on a scanner error. |

A scan that stops at a size cap with no findings is recorded as partial and the deploy goes live with a note. Raise the caps below if that matters to you.

Set the policy on the app's deploy settings page, or with the CLI:

```bash
levelrail secrets scan web --policy block
```

## Allow-list

Allow a finding you have reviewed from the deploy that reported it, or by fingerprint:

```bash
levelrail secrets scan web --allow 0123456789abcdef --reason "public test key, rotated"
levelrail secrets scan web --policy-show
levelrail secrets scan web --unallow 0123456789abcdef
```

A reason is required. The entry records who added it and when, and the API call lands in the audit log.

## Scan from the CLI

```bash
levelrail secrets scan --path .          # local directory, no control plane needed
levelrail secrets scan web               # the app's plain env values now
levelrail secrets scan web --deploy ID   # what a deploy's scan recorded
levelrail apps deploys gate web          # includes the secret scan result
```

A scan with active findings exits 1, so it can gate a CI job.

## Limits

The scan only reads files under the build context, never follows symlinks and skips `.git`, `node_modules`, `vendor`, build output, lock files, images and binary files. It runs on the deploy job, not the request that triggered it, and stops at these caps:

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_SECRET_SCAN_MAX_FILES` | `5000` | Files read per scan. |
| `APP_SECRET_SCAN_MAX_FILE_BYTES` | `1048576` | Larger files are skipped. |
| `APP_SECRET_SCAN_MAX_TOTAL_BYTES` | `67108864` | Bytes read per scan. |
| `APP_SECRET_SCAN_MAX_FINDINGS` | `200` | Findings kept per scan. |
| `APP_SECRET_SCAN_TIMEOUT` | `30s` | Wall time before the scan is treated as unable to finish. |

## API

| Route | Purpose |
| --- | --- |
| `GET /api/v1/apps/{name}/secret-scan` | Policy, allow-list and limits. |
| `PUT /api/v1/apps/{name}/secret-scan` | Set the policy (`off`, `warn`, `block`). |
| `POST /api/v1/apps/{name}/secret-scan/run` | Scan the app's plain env values now. |
| `POST /api/v1/apps/{name}/secret-scan/allow` | Allow-list a fingerprint with a reason. |
| `DELETE /api/v1/apps/{name}/secret-scan/allow/{fingerprint}` | Remove an allow-list entry. |
| `GET /api/v1/apps/{name}/deployments/{id}/secret-scan` | The scan recorded for one deploy. |

## What it does not do

It does not scan git history, only the checked-out tree. It does not scan image layers or values resolved at runtime from a vault. It does not rotate anything it finds, see [secret rotation](/secret-rotation) for that.
