# ADR 044: Assets, a thin manager over user-supplied S3-compatible buckets

Status: Accepted

Date: 2026-10-25

## Context

Apps need somewhere to put user uploads, images, downloads and build output
that should not live in a container volume. Operators already have buckets on
Cloudflare R2, AWS S3, Backblaze B2 or a self-hosted Garage or MinIO, and want
one place to connect them, upload, copy a URL and see what it costs.

Most of the plumbing exists:

- Storage destinations (`backup_targets`, `docs/object-storage.md`) hold
  envelope-encrypted credentials, provider presets (`internal/objectstore/presets.go`)
  and a connection probe with typed failure reasons.
- App buckets (`storage_buckets`, migration 1220) carve a key prefix out of a
  destination, with browse, upload, presigned GET and PUT
  (`objectstore.Client.PresignPut`), quota, expiry, and a rate limited public
  read route at `/api/v1/public/buckets/{id}/{key}`.
- `objectstore.Client.Usage` totals bytes and objects under a prefix, and
  `pricing.go` holds list prices per GB-month, overridable by
  `APP_STORAGE_PRICE_PER_GB_MONTH_<PRESET>`.
- The ingress has an in-memory response cache with purge and hit metrics
  (`docs/cdn-and-caching.md`).

What is missing is a product surface that treats a bucket as a place to serve
files from: public base URLs, a serving mode, a URL list, upload from a browser
without passing bytes through the control plane, scoped upload tokens, and
honest numbers on size, traffic and cost. A bucket-provisioning feature
(levelrail runs Garage on enrolled nodes and creates buckets and keys) is also
planned, and both should land on one resource rather than two.

## Decision

### 1. Levelrail never stores asset bytes

Asset bytes live only in the operator's bucket. The control plane stores
metadata and credentials, never a copy.

- **Browser-direct upload through presigned PUT.** The API returns a
  presigned PUT URL for one key. The signature binds `Content-Type` and
  `Content-Length`, so the browser cannot upload a different type or a
  larger body than it declared. The declared size is checked against the
  bucket quota and `APP_ASSETS_UPLOAD_MAX_BYTES` before signing. TTL is capped
  by `APP_ASSETS_PRESIGN_MAX_TTL`.
- **Small uploads through the API.** Bodies up to `APP_API_MAX_BODY_BYTES`
  (today 16 MiB) may go through `PUT .../objects`, for the CLI and for
  scripts that cannot do a two-step upload. Larger bodies get a `413` that
  names the presign route.
- **CORS is checked, not assumed.** Browser-direct upload needs a CORS rule
  on the provider bucket. The connection probe gains a CORS step; a missing
  rule is a typed fix that calls `PutBucketCors` with the dashboard origin when
  the key has permission, and a guided manual fix with the exact JSON when it
  does not.
- Single PUT only in v1 (provider limit, 5 GiB on S3 and R2). Presigned
  multipart is a later slice.

Alternatives rejected:

- **Upload through the control plane.** One code path and no CORS, but every
  byte crosses the control plane twice (in and out), large uploads hold a
  request and memory, and the control plane becomes the bandwidth bottleneck.
  Kept only for small bodies.
- **Presigned POST with a policy (`content-length-range`).** Enforces a size
  range server side, but not every provider implements it (R2 does not), and
  the signed `Content-Length` on PUT gives the same guarantee for a declared
  size.
- **A built-in object store.** Out of scope for this ADR. Managed Garage
  (section 6) is how levelrail runs storage, and it still sits behind the same
  S3 API.

Consequences: an asset upload cannot fail because the control plane is busy or
restarting once the URL is issued. Levelrail does not see the upload complete,
so the asset index is updated by a `HEAD` on the key after the client reports
completion, and by the listing (section 2).

### 2. Resource model

Three resources, all built on what exists:

| Resource | Storage | What it holds |
| --- | --- | --- |
| Bucket connection | existing `backup_targets` row | provider preset, endpoint, region, bucket, encrypted key pair. A new `garage` preset (path style, endpoint required, region default `garage`) joins the presets. |
| Asset bucket | existing `storage_buckets` row with `purpose = 'assets'`, plus one `asset_bucket_settings` sidecar row | prefix (may be empty, meaning the whole provider bucket), project id, visibility, serving mode, public base URL, CDN base URL, referrer allowlist, quota, expiry |
| Asset | `assets` index table | key, size, content type, ETag, uploader principal, created time, last seen time |

- **The provider listing is the source of truth.** The `assets` table is an
  index for uploads brokered by levelrail (who uploaded it, from which token)
  and for fast URL lists. Objects written straight to the bucket appear through
  listing and are indexed lazily; an index row whose key no longer exists is
  dropped on the next listing that passes it.
- **Public base URLs.** Each asset bucket has an optional `public_base_url`
  (the provider public endpoint or a custom domain, for example
  `https://assets.example.com`) and an optional `cdn_base_url`. The URL list
  builds `base + "/" + key` with the key path-escaped; it never guesses a URL
  the operator did not configure. Without a base URL the list offers presigned
  GET links only.
- **Projects and scoped upload tokens.** An asset bucket belongs to one
  project. Uploads use the existing API tokens (ADR 012) and IAM policies; no
  new token type. A new IAM resource kind `asset_bucket:<id>` joins the
  resource refs in `internal/api/resource_ref_authz.go`, and project scope
  (`project:<id>`) covers every asset bucket in it. Abilities map as:
  list and presign GET on a private bucket need `read:sensitive`, upload and
  presign PUT need `write`, delete needs `write`, connect, change visibility or
  serving mode need `write:sensitive`. An upload token is a `write` token with
  an Allow policy on one asset bucket. It is meant for the operator's own app
  backend, which asks levelrail for a presigned URL and hands only that URL to
  the browser. A levelrail token never goes to a browser.

Alternatives rejected:

- **A new parallel `asset_buckets` table with its own credentials.** Cleaner
  names, but a second credential store, a second browse and presign
  implementation, and two things to migrate when managed Garage lands.
- **Index every object in SQLite as the source of truth.** Drifts the moment
  an app writes with its own credentials, and grows without bound on large
  buckets. The index is a cache, not the truth.
- **A dedicated upload token type.** Duplicates ability checks and audit. The
  IAM evaluator already answers "may this token write to this resource".

### 3. Three serving modes

The serving mode is a per asset bucket setting. It decides which URL the
dashboard, CLI and API hand out, and whether levelrail is in the request path.

| Mode | URL handed out | Levelrail in the path | Analytics |
| --- | --- | --- | --- |
| Direct (default) | `public_base_url/key` | no | estimated only |
| Proxied | `https://<asset domain>/key` served by the ingress | yes | measured |
| Third-party CDN | `cdn_base_url/key` | depends on the CDN's origin | from the CDN, not levelrail |

- **Direct.** The visitor fetches from the provider's public endpoint or the
  operator's custom domain. Levelrail adds zero latency and carries zero bytes.
  A custom domain on R2 is already behind Cloudflare's edge; a plain S3
  regional endpoint is not behind any CDN, so visitors far from that region pay
  the distance. Direct works only when the operator has made the bucket (or the
  custom domain) publicly readable at the provider; levelrail cannot flip that
  switch on every provider through the S3 API.
- **Proxied.** An asset domain is routed by the ingress to a handler that
  fetches the object with the bucket credentials and streams it, with the
  ingress cache in front. This is the only mode that serves a private bucket on
  a public URL, keeps all assets on one domain the operator controls, enforces
  levelrail's referrer allowlist and rate limits, and measures requests.
  The cost, stated plainly:
  - Every cache miss adds a full round trip from the control plane to the
    provider on top of the visitor's trip to the control plane. A visitor far
    from the control plane is slower than in Direct mode, hit or miss.
  - The ingress cache is one in-memory cache on the control plane
    (`APP_INGRESS_CACHE_MAX_BYTES`, default 64 MiB, objects up to
    `APP_INGRESS_CACHE_MAX_OBJECT_BYTES`, default 2 MiB). Large images and
    downloads are never cached and always pay the miss path. The cache starts
    empty after a restart.
  - Every byte crosses the control plane's network, so its bandwidth and the
    provider's egress (on misses) both apply.
  - Proxied serving runs in the control plane's ingress, so a control plane
    outage is an asset outage in this mode.
  Proxied is the right choice for private or modest-traffic assets, not for a
  global high-traffic image CDN.
- **Third-party CDN.** The operator puts a CDN in front of either the provider
  endpoint (origin is the bucket, levelrail out of the path) or the proxied
  asset domain (origin is levelrail, which then sees only CDN misses). Levelrail
  records the CDN base URL and hands it out. Purging that CDN is a plugin
  (phase 3), not core.

Alternatives rejected:

- **Proxied by default.** Gives analytics everywhere, at the price of added
  latency for every visitor and making the control plane a bandwidth bottleneck
  and a single point of failure for static files. Wrong default for a feature
  whose point is not storing bytes.
- **Direct only.** No answer for private buckets on a public URL, no single
  domain, no measured analytics.
- **A per node edge cache.** That is the multi-region ingress work in
  `docs/cdn-and-caching.md` ("Not yet"), not an assets feature. Proxied mode
  inherits it when it exists.

### 4. Analytics: measured in Proxied mode only

- **Proxied:** requests, bytes served, hit rate, status classes and top keys,
  from the ingress request metrics and cache counters, with their existing
  retention and their existing restart caveat. No client addresses are stored.
- **Direct and Third-party CDN:** levelrail does not see requests and says so.
  The page shows storage size and object count from a bounded listing
  (`APP_ASSETS_USAGE_MAX_OBJECTS`, refreshed on `APP_ASSETS_USAGE_INTERVAL`,
  showing "at least N" when the bound is hit), and an **estimated** monthly
  cost. Provider access logs are not ingested in this ADR.
- **Cost estimate.** Storage cost is size times `objectstore.PricePerGBMonth`.
  A new egress table beside it holds list egress prices per preset, overridable
  by `APP_STORAGE_EGRESS_PRICE_PER_GB_<PRESET>`. Starting defaults, to be
  checked against provider price pages when the slice lands: S3 internet egress
  0.09 USD per GB, R2 0 USD, B2 free up to three times the stored size and then
  0.01 USD per GB, Garage and MinIO 0. In Proxied mode the egress volume is
  measured; in Direct mode it is a number the operator enters ("if this bucket
  served N GB a month"). The comparison card shows the same volume priced on S3
  and on R2 side by side. Every cost figure carries the word "estimated", the
  date of the price table and the env override that changes it. Request
  (operation) charges and free tiers other than B2's are not modeled, and the
  card says so.

Alternative rejected: **show Direct mode traffic from provider APIs.** Each
provider exposes usage differently (some only through a separate analytics API
and token), and a half-supported number is worse than an honest blank. A
provider analytics plugin can add it later.

### 5. Safety

- **Default private.** A new asset bucket is private: no public base URL is
  handed out and Proxied serving requires a presigned or authenticated request.
- **Public toggle.** Turning a bucket public asks the operator to type its name,
  needs `write:sensitive` and step-up like other sensitive changes, and writes an
  audit log entry (who, when, bucket, old and new visibility, serving mode). The
  same applies to changing the serving mode or the base URLs.
- **Exposure probe.** In Direct mode, levelrail cannot make a bucket public or
  private at every provider, so it checks: an anonymous `HEAD` of a probe key
  through the public base URL. A bucket marked private in levelrail that answers
  the anonymous probe raises a warning and a typed fix ("make it public here, or
  turn off public access at the provider"). A bucket marked public that refuses
  it gets a guided fix.
- **Hotlink and referrer allowlist** (Proxied mode): a per bucket list of
  allowed `Referer` hosts; empty allows all. Requests with no `Referer` are
  allowed by default (direct visits, many apps) and can be refused per bucket.
  In Direct mode the allowlist is advisory only and the page says it is enforced
  at the provider or the CDN, not by levelrail.
- **Rate limits and caps from env, never hardcoded:**
  `APP_ASSETS_PUBLIC_RATE_PER_MIN` (per client IP on the proxied route),
  `APP_ASSETS_MAX_OBJECT_BYTES` (largest object served proxied),
  `APP_ASSETS_PRESIGN_RATE_PER_MIN` (presign issuing per token),
  `APP_ASSETS_UPLOAD_MAX_BYTES` and `APP_ASSETS_PRESIGN_MAX_TTL`. Defaults
  follow the existing public bucket route (`APP_PUBLIC_BUCKET_*`) so an install
  sees the same behavior. Active content types (HTML, SVG, XML, JavaScript) are
  served as downloads on the proxied route, as the public bucket route does now.
- **Credentials.** They stay in the destination's envelope-encrypted secrets
  (wrapped by the control plane master key), are never logged, never in
  audit details, never returned by the API, never exposed through MCP tools.
  Outbound calls keep the netguard SSRF check.
- **Prefix scoping is still not a boundary** (see the warning in
  `docs/object-storage.md`). An asset bucket with a prefix shares the provider
  key with everything else on that destination. The connect flow recommends a
  dedicated provider bucket and key for assets, and managed Garage (section 6)
  removes the limitation with per bucket keys.

### 6. One resource with managed Garage

Bucket provisioning will create a Garage bucket and key on levelrail-run nodes.
It lands on the same rows:

- A provisioned bucket is a `backup_targets` row with preset `garage` and a
  `managed` flag; its key is created through Garage's admin API and stored like
  any other credential. Asset buckets, app buckets, backups and log archive on
  top of it do not change.
- With a managed backend, levelrail can mint a per asset bucket key instead of
  sharing the destination key, which turns the prefix into a real boundary.
- Visibility and website serving become something levelrail can set itself on
  managed buckets, so the exposure probe becomes a reconcile check: desired
  visibility in the row, observed visibility from the probe, converge.
- Deleting a connection never deletes provider data; deleting a managed bucket
  is a separate, confirmed action owned by the provisioning feature.

Alternative rejected: **separate "managed storage" and "connected storage"
resources.** Every consumer would need two code paths and the UI two pages for
the same S3 API.

### 7. Phases

**v0.8.6, v1 (connect, browse, upload, URLs)**

- Connect a bucket (existing destination flow plus the `garage` preset and the
  CORS probe step).
- Asset buckets: create over a destination, browse (virtualized), presigned PUT
  from the browser with signed type and length, small upload through the API,
  delete, URL list with copy (base URL or presigned GET), private by default,
  public toggle with confirmation and audit, exposure probe.
- Direct serving mode only. Storage size and object count from a bounded
  listing, storage cost estimate from `pricing.go`.
- CLI: `levelrail-cli assets buckets (list|create|show|delete|set)`,
  `assets ls`, `assets upload <bucket> <file> [--key]` (presign for large files
  automatically), `assets url <bucket> <key>`, `assets rm`.
- API under `/api/v1/assets/...`, generated API reference and OpenAPI updated.
- Dashboard: an **Assets** page in the Storage area and an asset bucket detail
  page. MCP: read tools only (list buckets, list objects, get URL).

**v0.8.7**

- Proxied serving mode on an asset domain through the ingress cache, referrer
  allowlist, proxied rate limits and size cap.
- Measured analytics in Proxied mode, egress price table, S3 versus R2
  comparison card.
- Scoped upload tokens: `asset_bucket:<id>` IAM resource kind, project scope,
  token creation shortcut on the bucket page, presign rate limit per token.
- Third-party CDN base URL.
- Stacks: an `AssetBucket` kind in the IaC schema (ADR 042), referencing a
  destination by id or name, never inline credentials.

**Later**

- Cloudflare cache purge as a plugin under ADR 034, triggered from asset
  delete and replace.
- Image resize and format conversion (on the proxied route, cached), with its
  own ADR for CPU limits and abuse.
- Presigned multipart upload, provider analytics plugins, access log ingestion.

## Slices and file ownership

Shared files are owned by one slice each and edited in this order, so parallel
agents never touch the same file. Slices after the first only add their own
files plus the lines the owner slice left for them.

| Slice | Owns (new files) | Shared files it may touch |
| --- | --- | --- |
| A. Store | `internal/store/asset_buckets.go`, `internal/store/assets.go`, tests | one migration in `internal/store/migrations` (next free version, checked with `scripts/check-migration-versions.sh` on fresh `origin`; `purpose` column, `asset_bucket_settings`, `assets`) |
| B. objectstore | `internal/objectstore/presign_upload.go` (signed type and length), `internal/objectstore/cors.go`, `internal/objectstore/egress_pricing.go` (v0.8.7) | `presets.go` (`garage` preset), `probe.go` (CORS step) |
| C. API | `internal/api/assets*.go`, `internal/api/routes_assets.go`, tests | `internal/api/routes.go` (one `registerAssetRoutes` line), `internal/api/resource_ref_authz.go` (v0.8.7, `asset_bucket` kind) |
| D. CLI | `cmd/levelrail-cli/assets*.go`, tests | `cmd/levelrail-cli/main.go` (one `case "assets"` line and the usage text) |
| E. Dashboard | `web/src/routes/assets/*`, `web/src/components/assets/*`, `web/src/locales/en/assets.json` | `web/src/i18n/index.ts` (`assets` namespace), the sidebar nav file; `routeTree.gen.ts` is regenerated with `npx vite build`, never hand edited, and regenerated again on a merge conflict |
| F. Docs | `docs/assets.md` | `docs/docs.json` nav, `docs/public/llms.txt`; generated API reference, OpenAPI and MCP tool surface by their generators |

Order: A, then B and C, then D and E in parallel (they share no files), then F.
Each slice is one commit with its tests. The proxied mode in v0.8.7 owns
`internal/ingress` changes and lands as its own slice after an ingress review.

## Consequences

- Operators keep their data, their provider bill and their provider's
  durability; levelrail stays small and holds only metadata and keys.
- Direct mode, the default, has no levelrail failure mode on the read path and
  no measured traffic. The UI must label every Direct mode number as storage
  or estimate, never as traffic.
- Proxied mode makes the control plane a data path for files, with the latency,
  bandwidth and availability costs listed in section 3. It is opt in per bucket.
- Browser-direct upload depends on provider CORS, which adds a probe step and a
  typed fix to maintain.
- Reusing `backup_targets` and `storage_buckets` means asset buckets inherit
  their quota, expiry, presign and safety behavior, and any change to those
  tables now affects three features (backups, app buckets, assets).
- Price tables go stale. They live in one file, are overridable by env, and
  every figure is labeled estimated with the table's date.

## Open questions

1. Should the existing presign route require `write` for PUT (today any
   presign needs `read:sensitive`), and should that change also apply to app
   buckets in v0.8.6?
2. Asset domain for Proxied mode: a per bucket custom domain through the
   existing domain resources, or one instance-wide asset host with the bucket
   in the path, or both.
3. Should a Direct mode bucket with a failing exposure probe block the public
   toggle, or only warn?
4. Default for `Referer`-less requests in Proxied mode when an allowlist is set
   (allow or refuse).
5. Whether app buckets and asset buckets merge into one dashboard page in a
   later release, since they are now the same row with a different purpose.
