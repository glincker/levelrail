# ADR 056: A signed runtime and base image advisory feed

Status: Proposed

Date: 2026-10-29

## Context

The question: when a security fix lands for a language runtime or a base
image (a Node or PHP release, an OpenSSL fix, a distro package update),
can a Levelrail operator learn which of their deployed apps and databases
run the affected content, see it on the release that is live in each
environment, and get a guided path to rebuild and redeploy, without the
platform changing anything on its own?

Today the pieces are partly there. Verified on `release/v0.8.11` by
reading the code:

1. **Supply chain scanning exists and is opt in.**
   `internal/supplychain/config.go` reads `APP_SCAN_ENABLED` (a server
   kill switch, default true), `APP_SCANNER` (`trivy` or `grype`),
   `APP_SCANNER_IMAGE` (pinned defaults
   `docker.io/aquasec/trivy:0.65.0` and `docker.io/anchore/grype:v0.99.1`),
   `APP_SCAN_TIMEOUT` (5 minutes), `APP_SCAN_OVERRIDE_TTL` (1 hour),
   `APP_SBOM_KEEP_PER_APP` (20) and `APP_SCAN_SWEEP_INTERVAL` (1 hour).
   Per app settings live in `app_supply_chain_settings`
   (`internal/store/migrations/0158_supply_chain.sql`) with
   `scan_enabled` default 0 and `scan_gate` default `off`. The gate
   (`gate.go`, `Decide`) blocks only in the `block_on_critical` or
   `block_on_high` modes and accepts a reasoned, expiring override.
2. **SBOMs exist only for some deploys.** `internal/build/attest.go`
   turns on BuildKit SBOM and provenance attestations behind
   `APP_BUILD_ATTEST` (default off, `config.go` reads it through
   `build.EnvAttest`). `docs/supply-chain.md` ("What is not covered")
   states that only Dockerfile builds get an SBOM: Railpack builds, image
   deploys, static sites and remote node builds record none. Provenance is
   `mode=min`, recorded, not signed or verified. Scans read the SBOM, not
   the image. SBOM files live on disk under
   `DataDir/supplychain/<app>/<deploy>.sbom.json`; the
   `deploy_supply_chain` table keeps counts and top vulnerabilities per
   deploy attempt, keyed by `attempt_id`.
3. **Vulnerability data comes from the scanner, at scan time.** The
   scanner container has network access because it downloads its own
   database (`docs/supply-chain.md`, "What a scan costs"). Levelrail
   holds no vulnerability data of its own, and a scan only runs at deploy
   time or on `ScanNow` (`service.go:261`). Nothing rescans a live release
   when new advisories appear. So a release that was clean on Monday is
   still shown clean on Friday after a runtime CVE is published.
4. **Each deploy attempt records a content identity.**
   `internal/store/deploy_attempt.go:69` defines `ImageDigest` ("a
   registry digest for a pulled image, or the local image ID of a
   build") and `DigestReason`, with values in
   `internal/store/deploy_attempt_safety.go`: `Resolved`,
   `AlreadyPinned`, `LocalBuild`, `PullFailedUsingCached`,
   `LocalImage`. A `LocalBuild` identity is a local image ID, not a
   registry digest, so it can never match a digest published by anyone
   else.
5. **Releases and environment bindings are in shadow.** Migration
   `3054_release_shadow.sql` creates `releases` (with
   `source_deploy_attempt_id` UNIQUE, `image_ref`, `image_tag`, no digest
   column), `environment_bindings` (one row per app, `release_id`, the
   comment says "nothing reads it for decisions yet") and the append only
   `environment_binding_history`. Migration `3055_approval_release_id.sql`
   adds `release_id` to `deploy_approvals`. The digest of a bound release
   is reachable today only through
   `releases.source_deploy_attempt_id` to the deploy attempt's
   `ImageDigest`.
6. **Image auto update and rollback already reason about digests.**
   `internal/imageupdate/scheduler.go` checks opted in apps every
   `APP_IMAGE_UPDATE_INTERVAL` (default 1 hour) through a `Checker`
   (`CheckImageUpdate`). `internal/rollbackref/rollbackref.go`
   (`Resolve`) picks the reference a rollback redeploys and verifies it
   still matches the recorded content. Database upgrades record
   `FromImageDigest` (`internal/store/db_upgrade.go:63`). I did not find
   a running image digest for managed databases outside the upgrade
   record.
7. **Alerting has a rule kind registry and an inbox.**
   `internal/alerting/rules.go` declares `Kind` constants (threshold,
   crashloop, `cert_expiry`, `patch_status`, scheduled task and node
   kinds). The inbox (`inbox.go:36`) is built from `HistoryEntry` rows
   with read, resolved and archive state. `patch_status.go` is the
   closest existing shape: a platform wide condition turned into a rule.
8. **The project already signs what it ships.**
   `.github/workflows/release.yml` signs `checksums.txt` keyless with
   `cosign sign-blob --bundle` (around `:356`) and signs the control
   plane and agent images by digest (`:637`, `:749`), using GitHub OIDC
   (`id-token: write`). Levelrail's own advisories are published as
   `LRSA-<year>-<number>` entries in `docs/security-advisories.md`, linked
   from `.github/SECURITY.md`. That list covers Levelrail itself, not the
   runtimes inside user images.
9. **The MCP tool listing has a token budget.**
   `internal/mcptools/budget_test.go` holds `modeTokenBudgets` and
   `compactTokenBudgets` with a per mode env override. Any new tool must
   fit.

The gap is not scanning. It is that nobody tells a running install "the
runtime inside these digests is now known to be vulnerable", that the
scanner path covers only SBOM backed Dockerfile builds, and that there is
no per release view of exposure.

## Decision (recommended)

Ship an opt in, signed, curated advisory feed that Levelrail pulls from a
static URL, verifies offline against keys embedded in the binary, and
matches locally against the image digests and stored SBOMs of live
releases. Findings appear as records, an alert rule kind, inbox items, a
release badge, CLI commands and a read only MCP tool. Remediation is a
set of prefilled actions the operator runs. Nothing deploys, rebuilds or
upgrades because of the feed unless the operator enabled it for that app,
and even then the normal approval, freeze and gate paths apply.

### Feed scope

The feed is narrow on purpose. It lists runtime and base image
advisories with an operator meaning: "images built from the official Node
20 line before 20.x.y carry CVE-N; fixed in 20.x.y". It is not a copy of a
general vulnerability database; the scanners already have one. Each entry
says which runtime or base it affects, how to recognize it, which version
fixes it, and a short plain text summary.

### Format

One JSON document, `feed.json`, plus a detached signature file,
`feed.json.sig`.

```
{
  "schema": 1,
  "sequence": 412,
  "issued_at": "2026-10-29T08:00:00Z",
  "expires_at": "2026-11-12T08:00:00Z",
  "key_id": "lrfeed-2026a",
  "advisories": [
    {
      "id": "LRRA-2026-0031",
      "aliases": ["CVE-2026-12345"],
      "severity": "high",
      "summary": "plain text, at most 280 chars",
      "affects": [
        {"kind": "package", "purl_type": "npm|deb|apk|generic",
         "name": "openssl", "introduced": "3.0.0", "fixed": "3.0.15"},
        {"kind": "runtime", "runtime": "node", "line": "20",
         "fixed": "20.18.1"},
        {"kind": "digest", "digests": ["sha256:..."]}
      ],
      "references": ["https://..."],
      "published_at": "...",
      "withdrawn": false
    }
  ]
}
```

`LRRA` (runtime advisory) keeps these IDs apart from `LRSA`, which stays
about Levelrail itself. `sequence` only grows. `expires_at` bounds how long
a feed is trusted. `aliases` carry upstream CVE IDs so a finding can be
cross checked against a scanner result for the same package.

### Signing, keys and rotation

- **Signer:** a maintainer controlled ed25519 key, used by a dedicated
  workflow that publishes the feed. The public keys ship inside the
  Levelrail binary as a small keyring (key ID to public key, at most two
  active). Verification is `crypto/ed25519` from the Go standard library:
  no network, no new dependency, works the same on an air gapped host.
- **Transparency:** the publishing workflow also runs
  `cosign sign-blob --bundle` on `feed.json`, the same keyless pattern as
  `checksums.txt` in `release.yml`. That bundle is for public audit. The
  binary does not require it, because keyless verification needs a trusted
  root and identity checks that are awkward offline.
- **Rotation:** release N adds the next key to the keyring. The feed is
  signed with the old key until most installs are on N, then switches its
  `key_id`. Release N plus one drops the old key. A key ID the binary does
  not know is a hard verify failure with a clear message ("upgrade
  Levelrail to read this feed").
- **Compromise:** removing a key needs a Levelrail release. Until the
  operator upgrades, a stolen key can sign a feed that verifies. The
  design limits what such a feed can do (see Abuse and trust), and the
  cosign transparency log gives a way to notice signatures the maintainers
  did not make.
- **Replay and freeze:** a feed with a lower `sequence` than the last
  accepted one is rejected. A feed past `expires_at` is kept for display
  but marked stale, and the dashboard says so. This stops an attacker from
  serving an old feed that hides a newer advisory, at least until expiry.

### Hosting and fetch

- A static URL, read from `APP_ADVISORY_FEED_URL`. The default points at
  a static path the project controls (open question: the project site or
  a pages branch of this repository). `APP_ADVISORY_FEED_MIRROR_URL` is an
  optional second source tried when the first fails.
- A plain `GET` with `If-None-Match`, every `APP_ADVISORY_FEED_INTERVAL`
  (default 6 hours) with up to 10 percent jitter. Response size is capped
  at `APP_ADVISORY_FEED_MAX_BYTES` (default 2 MiB); the fetch timeout is
  `APP_ADVISORY_FEED_TIMEOUT` (default 30 seconds). The outbound client
  reuses `kit/netguard` style guards so a redirect cannot pull from a
  private address.
- **Fallback:** on any fetch or verify failure, keep the last good feed,
  record the error on the feed state row, and raise a low severity
  platform item after `APP_ADVISORY_FEED_STALE_AFTER` (default 72 hours)
  without a good feed.
- **Air gapped installs:** `APP_ADVISORY_FEED_URL=off` disables fetching.
  The operator downloads `feed.json` and `feed.json.sig` elsewhere and
  runs `levelrail-cli advisories import feed.json feed.json.sig`. The
  same verification, sequence and expiry rules apply. The dashboard shows
  the feed age, so an air gapped install sees how old its data is.

### Matching

Matching runs locally after each accepted feed and after each succeeded
deploy, against the live set: for each app, the release bound in
`environment_bindings`, falling back to the latest succeeded deploy
attempt while bindings are in shadow. Three match kinds, each with a
confidence the UI shows:

1. **Digest (high).** The bound release's deploy attempt `ImageDigest`
   equals a digest the advisory lists. Only for `DigestReason` `Resolved`
   or `AlreadyPinned`; a `LocalBuild` image ID never matches a published
   digest and is skipped, not reported clean.
2. **SBOM package (high or medium).** The stored SBOM for that attempt
   (`ParseSBOM` in `internal/supplychain/sbom.go`) has a package whose
   purl type and name match and whose version falls in
   `[introduced, fixed)`. Version comparison is per ecosystem (semver for
   npm, dpkg and apk ordering for distro packages). An ecosystem without a
   reliable comparator yields medium confidence, never a silent match.
3. **Runtime fingerprint (low, labelled "possible").** For deploys with
   no SBOM (image deploys, Railpack, remote builds): image config
   environment values the official images set (for example
   `NODE_VERSION`, `PHP_VERSION`), OCI labels and the base image tag in
   the stored image reference. This is a guess and the UI says so.

Each app also reports an explicit "not checked" state with the reason
(no SBOM, local build ID, no bound release), so an empty list is never
read as safe.

**False positives:** a distro may backport a fix without changing the
upstream version, so a package match can be wrong. Findings link the
advisory alias to the scanner result for the same attempt when one
exists, and the operator can suppress per app with a required reason and
an expiry no longer than `APP_ADVISORY_SUPPRESS_MAX_TTL` (default 90
days). Suppressions are written to the audit log and the app timeline,
expire on their own, and are cleared when the matched digest changes.
`ack` marks a finding seen without hiding it.

### Outputs

- **Finding records** in `advisory_findings` (below), one per app,
  advisory and digest.
- **Alert rule kind** `runtime_advisory` in `internal/alerting/rules.go`,
  with a minimum severity and confidence filter. A default rule is created
  disabled, so turning the feed on does not page anyone.
- **Inbox item** through the normal history entry path, grouped per
  advisory so one OpenSSL advisory across 30 apps is one item with a
  count.
- **Dashboard badge** on the release and on the app header: highest open
  severity, confidence, and feed age.
- **CLI:** `levelrail-cli advisories list [--app] [--severity]`,
  `advisories show <id>`, `advisories ack <id> --app <app>`,
  `advisories suppress <id> --app <app> --reason ... --until ...`,
  `advisories import`, `advisories status` (feed sequence, age, key ID,
  last error).
- **MCP:** one read only tool, `advisories_list`, returning IDs,
  severity, app counts and confidence, not summaries by default. It is
  left out of the compact listing and must pass `budget_test.go`.

### Remediation helpers

Each finding offers the actions that fit the app, prefilled, none run on
their own:

- **Rebuild with a fresh base:** a Dockerfile build with a forced base
  pull, so a patched tag is picked up. If the Dockerfile pins a digest,
  the helper says so and links to the line rather than editing it.
- **Move an image deploy to a newer tag:** reuse the
  `CheckImageUpdate` path from `internal/imageupdate`. The tag comes
  from the registry and the app's own configuration, never from the feed.
- **Bump a database minor version:** route to the existing database
  upgrade flow (`db_upgrade.go`: backup, verify after, revert on
  failure).
- **Canary the redeploy:** offer the existing canary release flow
  (`0378_canary_releases.sql`) when the app has one configured.
- **Opt in automation:** a per app setting `advisory_auto_remediate`
  (default off) lets only the "rebuild with a fresh base" or "re pull the
  same tag" action start on a high confidence finding. It goes through
  deploy approvals, freezes and the supply chain gate like any deploy.
  The server switch `APP_ADVISORY_AUTO_REMEDIATE` (default off) must also
  be on.

### Privacy

The only outbound traffic is the feed `GET`. No install ID, app names,
digests, package lists or counts are sent. The user agent carries no
install identifier. All matching happens on the control plane. The
operator can point the URL at their own mirror.

### Abuse and trust

- **A feed cannot deploy.** No feed field is a command, image reference
  or URL the platform acts on. Remediation actions build their target
  from the app's own configuration. Auto remediation needs two switches
  that are off by default, and only re pulls or rebuilds what the app
  already uses.
- **Advisory text is untrusted data.** Summaries are plain text with
  length caps, rendered escaped (no markdown, no HTML), and passed to the
  assistant and MCP clients as quoted data, never as instructions. This
  follows the lesson in LRSA-2026-0004 about rendered assistant output.
  References render as plain links with a host shown and are never
  fetched by the server.
- **Field validation:** version strings, purl types, digests and IDs are
  checked against strict patterns; an entry that fails is dropped and
  counted, the rest of the feed is used. Caps on advisory count and on
  digests per advisory bound match cost.
- **Worst case with a stolen key:** false alarms, or hiding an advisory
  until the next honest feed (sequence rules stop a rollback, not a
  forged higher sequence). No code execution, no deploys.

### Data model sketch

Migration number taken at slice time, after checking origin.

- `advisory_feed_state`: single row (`CHECK (id = 1)`): `sequence`,
  `key_id`, `issued_at`, `expires_at`, `fetched_at`, `etag`, `source`
  (`url` or `import`), `last_error`, `last_error_at`.
- `advisories`: `id` PK, `sequence`, `severity`, `summary`, `aliases`
  JSON, `affects` JSON, `references` JSON, `published_at`, `withdrawn`.
- `advisory_findings`: `id` PK, `advisory_id`, `subject_kind` (`app` or
  `database`), `subject_name`, `deploy_attempt_id`, `release_id` nullable,
  `image_digest`, `match_kind` (`digest`, `package`, `fingerprint`),
  `confidence`, `evidence` JSON (package name and version, or env key),
  `state` (`open`, `acked`, `suppressed`, `resolved`), `first_seen_at`,
  `last_seen_at`, `resolved_at`. Unique on (`advisory_id`,
  `subject_kind`, `subject_name`, `image_digest`).
- `advisory_suppressions`: `advisory_id`, `subject_name`, `image_digest`,
  `reason`, `expires_at`, `created_by`, `created_at`.

### Rollout flag

`APP_ADVISORY_FEED` (default `off`). Off means no fetch, no matching, no
UI beyond a settings card that explains the feature. Import works only
when the flag is on.

## Alternatives rejected

- **Rescan every live SBOM on a timer with the existing scanner.** Useful
  and cheap to add later, but it only covers SBOM backed Dockerfile
  builds, needs the scanner image and its database download on every
  host, and gives package level noise rather than "your Node runtime is
  behind". It complements the feed, it does not replace it.
- **Use a public vulnerability database directly in the control plane.**
  Large, changes hourly, needs its own parser and version logic per
  ecosystem, and does not express "this base image line is fixed in tag
  X". It also turns every install into a heavy outbound client.
- **Keyless cosign verification only.** Matches what releases already
  use, but offline verification of a keyless bundle needs a pinned trusted
  root and identity rules, and an air gapped host cannot refresh them.
  Kept as the transparency layer instead.
- **Push notifications to installs.** Needs inbound reachability or a
  registry of installs, which breaks the privacy rule.
- **Automatic redeploy by default.** A wrong or forged advisory would
  restart production. Rejected outright.

## Security

The feed is a new trust root, so it gets the smallest possible
authority: it can add, change or withdraw findings and nothing else. Keys
are held only in the publishing workflow's protected environment with
required reviewers. Verification failures are logged without the feed
body. Suppressions and acks are authorized like other app settings, per
app and per environment, and audited. The MCP tool is read only and goes
through the same per app read checks as other tools.

## Failure modes

| Failure | Behavior |
| --- | --- |
| Feed URL down | Keep last good feed, retry next interval, try mirror, stale item after `APP_ADVISORY_FEED_STALE_AFTER`. |
| Bad signature or unknown key | Reject, keep last good, record `last_error`, show it in `advisories status`. |
| Lower sequence | Reject as replay. |
| Expired feed | Keep findings, mark the feed stale in UI and CLI. |
| Oversized or malformed entry | Drop the entry, count it, use the rest. |
| SBOM file pruned by retention | Keep the existing finding, mark evidence as historical, no new package matches for that attempt. |
| No digest (`LocalBuild`, `LocalImage`) | Report "not checked by digest", still try SBOM and fingerprint. |
| Matching slow on a large install | Bounded worker, per feed time budget from `APP_ADVISORY_MATCH_TIMEOUT` (default 2 minutes), resume next cycle. |
| Remediation deploy fails | Normal deploy failure path, rollback via `rollbackref`; the finding stays open. |

## What cannot be verified locally

- The real hosting location, its uptime and its cache headers.
- Key custody in the publishing workflow and the review rule on it.
- The curation workload: how many hours a week it takes to keep the feed
  correct and timely. This is the biggest unknown.
- False positive rate on real images, especially distro backports.
- Whether official runtime images keep setting the env values the
  fingerprint relies on.
- Whether `releases.image_ref` is digest pinned for every deploy kind; I
  saw no digest column and rely on the deploy attempt join.
- How managed databases record their running image digest outside an
  upgrade. I did not find it.
- Status of upstream data sources (vendor and distro trackers) as inputs
  to curation: not assessed in this ADR.

## Testing plan

- Unit: feed parse and verify with a test key; unknown key, bad
  signature, lower sequence, expired, oversized, bad field patterns.
- Unit: version range comparison per ecosystem with backport style
  versions as known cases.
- Unit: matching over fixture deploy attempts covering every
  `DigestReason`, with and without an SBOM (reuse
  `internal/supplychain/testdata`).
- Unit: suppression expiry and reset on digest change.
- Store: migration up on a copy of a current schema.
- Alerting: `runtime_advisory` rule evaluates, groups per advisory, and
  the default rule is disabled.
- MCP: the new tool passes `budget_test.go`.
- One focused test per slice; no broad guard tests that tie unrelated
  features together.

## Phases

1. Feed format, verification, storage and import (first slice).
2. Fetch loop, digest and SBOM matching, findings, CLI list and show.
3. Alert kind, inbox grouping, ack and suppress.
4. Dashboard badge on release and app, feed status card.
5. MCP read tool.
6. Remediation helpers (prefilled actions only).
7. Runtime fingerprint matching.
8. Opt in auto remediation, only after phases 1 to 6 have run on real
   installs.

The publishing workflow and the first curated entries run in parallel
with phase 2, so phase 2 has real data.

## First thin slice (under 2 days)

New package `internal/advisoryfeed`: types, strict parse, ed25519 verify
against an embedded keyring, sequence and expiry checks, size caps. One
migration for `advisory_feed_state` and `advisories`. A store with upsert
and list. `levelrail-cli advisories import` and `advisories status`,
gated by `APP_ADVISORY_FEED`. A test key and fixture feeds under
`testdata`. No fetching, matching, UI or alerting yet.

## Slices and file ownership

| Slice | Files owned | Env |
| --- | --- | --- |
| 1 Verify and import | `internal/advisoryfeed/*` (new), one new migration, the CLI advisories command file (new) | `APP_ADVISORY_FEED`, `APP_ADVISORY_FEED_MAX_BYTES` |
| 2 Fetch and match | `internal/advisoryfeed/fetch.go`, `match.go` (new), findings store file (new), wiring in the server start file | `APP_ADVISORY_FEED_URL`, `APP_ADVISORY_FEED_MIRROR_URL`, `APP_ADVISORY_FEED_INTERVAL`, `APP_ADVISORY_FEED_TIMEOUT`, `APP_ADVISORY_FEED_STALE_AFTER`, `APP_ADVISORY_MATCH_TIMEOUT` |
| 2b Publish | a new workflow file, a signing tool under `tools/` | workflow secrets only |
| 3 Alerting | `internal/alerting/rules.go` (one constant), `internal/alerting/runtime_advisory.go` (new), defaults entry | `APP_ADVISORY_SUPPRESS_MAX_TTL` |
| 4 Dashboard | new web components for the badge and status card, the release and app header views | none |
| 5 MCP | one new file in `internal/mcptools`, toolset registration | existing budget env |
| 6 Remediation | API handlers for prefilled actions, web action menu | none |
| 7 Fingerprint | `internal/advisoryfeed/fingerprint.go` (new) | none |
| 8 Auto remediation | per app setting migration, deploy trigger hook | `APP_ADVISORY_AUTO_REMEDIATE` |

Slices 3 and 5 touch shared registries (`rules.go`, the MCP toolset
list) and run one after the other, not in parallel.

## Consequences

- Operators get a per release answer to "am I exposed" for runtime level
  issues, including deploys the scanner never sees.
- The project takes on an ongoing curation duty. A stale feed is worse
  than none if the UI hides its age, so age is always shown.
- A new signing key to guard and rotate, with rotation tied to releases.
- Cost on an install: one small fetch every 6 hours and a local match
  pass; no scanner container is needed for digest or fingerprint matches.
- Bindings leaving shadow (ADR 050) make matching more exact; until then
  the latest succeeded attempt is the fallback.

## Open questions

1. Who curates the feed, on what response target, and is that sustainable
   before the project has more maintainers?
2. Where is the feed hosted: the project site or a pages branch of this
   repository?
3. Should managed databases record their running digest so they can be
   matched by digest, and in which slice?
4. Should a timed rescan of live SBOMs with the existing scanner ship
   alongside, as a second source of findings?
5. Should the user agent include the Levelrail version (useful for
   compatibility, a small fingerprint), or nothing at all?
6. Is two active keys enough, or should the feed carry a signed key
   rotation statement so a rotation does not need a release?
