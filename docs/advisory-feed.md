---
title: "Advisory feed"
description: Match the images your apps run against a local, optionally signed advisory feed.
---

Levelrail can compare the images your apps run against a feed of advisories you provide. It is off by default and does nothing until you point it at a feed. Levelrail never changes an app on its own: a match is a finding with a fix hint, and you redeploy.

This slice reads a feed from a file or URL you control. Hosting a public feed and its signing key are separate decisions and are not part of this build.

## Turn it on

| Variable | Meaning |
| --- | --- |
| `APP_ADVISORY_FEED` | Path or `http(s)` URL of the feed. Empty means off. |
| `APP_ADVISORY_FEED_PUBKEY` | Optional base64 ed25519 public key (32 bytes). When set, the feed needs a detached signature. |
| `APP_ADVISORY_FEED_REFRESH` | How long a URL feed is reused before it is fetched again. Default `6h`. A file is reloaded only when it changes. |
| `APP_ADVISORY_FEED_MAX_BYTES` | Largest accepted feed. Default 4 MiB. |
| `APP_ADVISORY_REFRESH_ACTION` | Set to `on` to allow the refresh action below. Default off. |

There is no polling loop. The feed is read when something asks for it (the dashboard, the CLI or the attention feed), and matches are recomputed only when the feed or the set of running images changes.

## Signatures

With a public key configured, the signature is read from the feed location plus `.sig` and is the base64 of an ed25519 signature over the exact feed bytes. A missing or wrong signature rejects the whole feed and shows `invalid`.

A feed fetched from an `http(s)` URL needs the key: without `APP_ADVISORY_FEED_PUBKEY` it is rejected and shown as `invalid`.

Without a key a local file feed is labelled `unsigned` in the dashboard, the CLI and the API. It is still used, because you chose the source, but it is never presented as verified.

## Format

```json
{
  "version": 1,
  "generated_at": "2026-01-01T00:00:00Z",
  "entries": [
    {
      "id": "TEST-0001",
      "severity": "high",
      "summary": "One line description",
      "fixed_in": "20.5.0",
      "reference": "https://example.invalid/TEST-0001",
      "affected": [
        {
          "repository": "docker.io/library/node",
          "tags": ["20.1.0"],
          "ranges": [{ "from": "20.0.0", "before": "20.5.0" }],
          "digests": ["sha256:..."]
        }
      ]
    }
  ]
}
```

- `version` must be `1`. `severity` is `critical`, `high`, `medium` or `low`. `id` is unique in the feed.
- Each `affected` item needs a `repository` and at least one of `digests`, `tags` or `ranges`, so an entry can never match a whole repository by accident.
- Repositories compare after removing `docker.io/` and `library/`, so `node` and `docker.io/library/node` are the same.
- A range covers `from` (inclusive, optional) up to `before` (exclusive). Tags are read by their leading dotted number, so `20.2.1-alpine` is `20.2.1`. Tags that are not versions only match through `tags`.
- A digest matches when the app's last successful deploy resolved to that registry digest. Local builds have no registry digest and match by tag or range only.

## Where it shows

- Attention feed: one item, "N running images match a known advisory", critical when any match is critical.
- Security center, Checklist tab: a compact card with the feed state, signature label and per-app matches.
- CLI: `levelrail-cli security advisories list` and `levelrail-cli security advisories match`.
- API: `GET /api/v1/security/advisories` (read ability). Callers only see apps they can read.

## Refresh an affected app

When an app matches an advisory and the registry holds a newer digest of the same tag, you can redeploy it on that digest:

```
levelrail-cli security advisories match
levelrail-cli security advisories refresh <app> [--confirm]
```

`POST /api/v1/apps/{name}/advisory-refresh` (deploy ability on the app) pins the newest registry digest of the app's tag and sends it through the normal deploy path. Freeze windows, deploy policy, protected-environment approvals (`--confirm` returns a pending approval, nothing deploys until it is approved), release bindings (`APP_RELEASE_BINDINGS`), image signature enforcement and the registry digest check all still apply, and the request is audited.

It is refused when `APP_ADVISORY_REFRESH_ACTION` is not `on`, when the feed signature is not `verified`, when the app matches no advisory, when the registry has no newer digest than the one running, or when the registry cannot be reached. A newer digest is not proof of a fix: check that the match disappears afterwards, and move to the `fixed_in` version when it does not.
