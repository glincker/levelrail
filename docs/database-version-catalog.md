# Database version catalog

The version lists, release channels and end-of-life dates offered when you create
or upgrade a database come from a published, signed file. Nobody edits version
numbers by hand.

## What the control plane does

- Ships an embedded catalog, so an offline or air-gapped install always works.
- Revalidates the published catalog every 24 hours with `If-None-Match`. A `304`
  only updates the last-checked time.
- Verifies the ed25519 signature and the schema, then merges it over the embedded
  catalog. The published file may add versions and update end-of-life dates. It can
  never change `auto_max`, `image_revert`, upgrade `levels` or advisory rules, and a
  version is only offered when the engine's version validation accepts it.
- Keeps the last good copy on disk. It is served for up to 90 days while the network
  or upstream fails, after that the embedded catalog is used and status says so.
- Never blocks startup.

Until the owner sets a real public key (below) remote refresh stays disabled and
`databases catalog status` reports that, rather than trusting unsigned data.

## Settings

| Setting | Default | Meaning |
| --- | --- | --- |
| `APP_DB_CATALOG_REMOTE` | `on` | `off` disables every remote lookup (air-gapped installs). |
| `APP_DB_CATALOG_URL` | jsDelivr URL of the `data` branch | Published file. The signature is read from the same URL plus `.sig`. https only, no cross-host redirects, 1 MiB cap. |
| `APP_DB_CATALOG_SOURCE` | `published` | `direct` builds the catalog in the control plane from the registries and endoflife.date. |

`direct` mode queries Docker Hub's OCI API and endoflife.date from every install,
once per 30 days with jitter, backoff and 429 handling. Trade-offs: it needs no trust
in the published file and no signing key, but each install calls third parties
(they see its IP address), it can hit anonymous rate limits that a single publisher
never would, and the result is only as good as the tag regexes in the build table.
It applies the same checks as the publisher: non-empty, no engine shrinking by more
than a third, version validation.

## Freshness

Measured from `generated_at` (published) or `fetched_at` (live or direct):

| Age | `freshness` |
| --- | --- |
| under 7 days | `fresh` |
| 7 to 35 days | `ok` |
| 35 to 90 days | `aging` |
| over 90 days | `stale` |

## Live check

`POST /api/v1/database-versions/refresh?engine=<id>` (CLI: `databases versions
<engine> --refresh`) queries the registry's tag list and endoflife.date for one
engine, validates and caches the result with its `fetched_at`. Per-engine cooldown is
10 minutes (the cached result comes back with a "checked just now" message), the
upstream timeout is 10 seconds, and a 429 is returned with `Retry-After`.

## API and CLI

- `GET /api/v1/database-versions?engine=<id>` (read)
- `POST /api/v1/database-versions/refresh?engine=<id>` (write-sensitive)
- `GET /api/v1/database-catalog` status (read), `POST /api/v1/database-catalog/refresh` (write-sensitive)
- `PUT /api/v1/settings/database-catalog` with `{"remote_enabled": bool}` (root)
- `databases versions <engine> [--refresh]`, `databases catalog status|refresh|set --remote on|off`

## Published schema (version 1)

```json
{
  "schema": 1,
  "generated_at": "2026-10-01T04:17:00Z",
  "engines": {
    "postgres": {
      "source": {
        "eol_url": "https://endoflife.date/postgresql",
        "tags_url": "https://registry-1.docker.io/v2/library/postgres/tags/list",
        "release_notes_url": "https://www.postgresql.org/docs/release/"
      },
      "versions": [
        {"version": "17.7", "tag": "17.7", "channel": "stable", "released": "2026-08-10", "eol": "2029-11-08", "recommended": false}
      ],
      "lines": [{"line": "17", "eol": "2029-11-08", "lts": false}]
    }
  }
}
```

`channel` is one of `stable`, `lts`, `rc`, `beta`, `eol`. Exactly one version per
engine has `recommended: true`: the newest stable version of the newest supported
line. Dates are `YYYY-MM-DD`; `eol` is omitted when none is announced.

## Publishing

`scripts/refresh-db-catalog` reads the engine table in
`internal/dbcatalogbuild/engines.go` (docker repo, endoflife.date slug, tag regexes,
release-notes URL, allowlisted lines) and writes `databases.v1.json` plus a
detached `.sig`. The run fails on a schema mismatch, an engine with no versions, or an
engine shrinking by more than a third versus the previous file. Supporting a new
major means adding its line to the table.

The workflow `.github/workflows/db-catalog.yml` runs on the 1st of each month and on
manual dispatch, signs with the `DB_CATALOG_SIGNING_KEY` secret and force-pushes a
`data` branch that holds only `catalog/databases.v1.json` and its `.sig`. New patch
releases can therefore lag up to a month. Maintainers can dispatch the workflow early
for an urgent security release, and operators can use the live check.

### One-time owner steps

1. Generate the key pair locally: `go run ./scripts/refresh-db-catalog -gen-key`.
2. Store the private half as the repository secret `DB_CATALOG_SIGNING_KEY`. Never commit it.
3. Paste the public half into the `PublicKey` constant in `internal/dbupgrade/remote_key.go`.
4. Run the workflow once (`workflow_dispatch`) to create the `data` branch.
