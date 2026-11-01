# ADR 067: External Account Binding and a CA picker

Status: Proposed

Date: 2026-11-01

Secret-sensitive: yes. Slices 2 and 3 handle an HMAC key and need an Opus
review before merge.

## Context

Let's Encrypt is the only CA an operator can use without hand work. When its
rate limits hit (a busy shared parent domain, many reinstalls while testing),
issuance stalls and Caddy serves its internal fallback certificate until the
limit clears. ZeroSSL and Google Trust Services also issue free certificates
over ACME, but both expect External Account Binding (EAB): a key id plus an
HMAC key from the CA account, sent when the ACME account is created.

What exists today, checked in code:

- `internal/ingress/config.go` mirrors Caddy's ACME issuer as its own
  `ACMEIssuer` type with three fields: module, email and `ca` (directory URL),
  plus `challenges`. The doc comment above it says EAB and the account key are
  deliberately not modelled yet.
- `AutomationPolicy.Issuers` is already a list, and `acmeWithFallbackTLSApp`
  puts the ACME issuer before `NewInternalIssuer()`, so an ordered issuer
  list is a supported shape today.
- `cmd/levelrail/boot_misc.go` (`acmeDefaultDirectory`) picks Let's Encrypt
  staging when `APP_ACME_STAGING` is true, else `APP_ACME_DIRECTORY_URL`, else
  empty (Let's Encrypt production).
- `internal/api/ingress_https.go` stores one `ACMEDirectoryURL` and email in
  ingress settings, detects staging by URL, and on a CA change calls
  `ingress.PurgeCertsFromOtherIssuers` so certificates from the old CA are
  replaced.
- `internal/secrets` provides envelope encryption; `internal/secretmask`
  masks registered values in every log line.

So any RFC 8555 CA without EAB works by URL today. CAs that need EAB do not.

## Decision

Add EAB and a small, ordered CA list. Global only in the first version.

1. **CA presets.** A picker with four entries: Let's Encrypt (default), ZeroSSL,
   Google Trust Services, and Custom (any directory URL). Each preset carries
   its production directory, its staging directory where the CA publishes one,
   and whether EAB is required. Preset URLs live in one table in
   `internal/ingress`, overridable per entry by env, so a CA moving its
   endpoint is a config change.
2. **Ordered fallback list.** Settings hold an ordered list of up to
   `APP_ACME_MAX_ISSUERS` CAs. The TLS policy emits one `ACMEIssuer` per entry,
   in order, followed by the existing internal fallback. Caddy tries issuers
   in order on each issuance and renewal. Recommended default for operators who
   opt in: Let's Encrypt first, one EAB CA second.
3. **EAB fields.** `ACMEIssuer` gains an `external_account` object with
   `key_id` and `mac_key`, matching Caddy's ACME issuer JSON. Both are required
   when the preset says EAB is required, refused when it is not.
4. **Global, not per host.** One list for the whole control plane. A per-host
   pin ("this domain only from CA X") is deferred; it multiplies policies and
   interacts with wildcard and DNS-01 policies in the same file. Revisit when a
   user needs a CA-specific domain.
5. **Staging toggle.** One toggle applies to every entry that has a staging
   directory. An entry without a published staging directory is skipped while
   staging is on, and the settings view says so. `APP_ACME_STAGING` keeps its
   meaning.
6. **Env compatibility.** `APP_ACME_DIRECTORY_URL` still works and becomes the
   first list entry when set and the stored list is empty. EAB from env reads
   `APP_ACME_EAB_KEY_ID` and `APP_ACME_EAB_MAC_KEY_FILE` (a file path, so the
   key is not in the process environment) and applies to that env entry only.
7. **Cert purge on change.** `PurgeCertsFromOtherIssuers` today keeps only
   certificates from the one configured CA. It becomes list-aware: certificates
   from any CA still in the list are kept; only CAs removed from the list are
   purged.
8. **Renewal.** No custom renewal logic. Renewal follows the issuer order, so
   a certificate first issued by the second CA may renew from the first once
   its limit clears. That is expected and the certificate detail shows the
   issuing CA (slice 2 adds it if the stored record lacks it). The renewal watchdog
   (`internal/certflow`) stays as is.
9. **CAA.** The domain doctor checks CAA records against every CA in the list,
   not only Let's Encrypt, and names which entry a CAA record would block.

Surfaces: API, CLI (`settings ingress set` gains `--ca`, `--ca-order`,
`--eab-key-id`, `--eab-mac-key-file`), MCP and a dashboard card.

## Alternatives rejected

- **Upload a certificate from the second CA by hand.** Works today
  (`internal/ingress/certimport.go`) and is the documented fallback, but
  renewal becomes the operator's job, which is the failure this project tries
  to remove.
- **Caddy's dedicated ZeroSSL issuer module** (fetches EAB with an API key).
  Covers one CA only and adds a second issuer shape to secure. Plain ACME plus
  EAB covers ZeroSSL and Google Trust Services with one path.
- **Per-host CA choice in the first version.** More policies, more test
  surface, no user asking yet.
- **Automatic CA switch on a rate-limit error.** The ordered issuer list
  already does this inside the ACME engine; a second switch on top would race
  it.

## Security

- The MAC key is a credential for the operator's CA account. It is stored
  through `internal/secrets`, added to the `secretmask` registry on load,
  never returned by API, CLI or MCP (a configured flag and the key id only),
  absent from exports, backups of settings definitions, and audit values
  (audit records the change, not the value).
- The key must reach the embedded ACME engine in the issuer JSON. Any code path
  that serializes the TLS app for display, diagnostics or a support bundle must
  redact `external_account.mac_key`. Slice 3 owns a test that renders every
  such path and asserts the key is absent.
- Key id and MAC key are validated for shape (non-empty, no newlines, base64url
  for the MAC key) before storage.
- Directory URLs for Custom go through `netguard.ValidateURL`; private
  directories need the existing private-network opt-in, for operators running
  their own CA.

## Failure modes

- Wrong EAB values: account creation fails on that CA, the next issuer is
  tried, and the error appears on the certificate detail with the CA named.
- Every ACME CA fails: internal fallback certificate, as today, plus the
  existing certificate alerts.
- CA rotates its directory URL: env override per preset, then a release that
  updates the table.
- Operator removes a CA from the list: certificates from it are purged and
  reissued from the remaining list; the UI warns before saving.

## What cannot be verified locally

- Real issuance from ZeroSSL and Google Trust Services with EAB, including
  which of them publish a staging directory and whether EAB values are
  reusable across accounts.
- Real fallback when Let's Encrypt returns a rate-limit error.
- Renewal moving back to the first CA.

Each needs a real host, a test domain, and the owner's CA accounts.

## Testing plan

- Unit: issuer list rendering (order, internal fallback last, staging skip
  rules), EAB required and refused per preset, env compatibility, list-aware
  purge, CAA check per CA.
- Secret handling: API reads, CLI output, MCP output, logs, exports and every
  config display path never contain the MAC key (captured buffers).
- Integration with a local ACME test server that supports EAB (if the existing
  test setup has one; otherwise a fake directory) for account creation with
  binding.
- Real host: issue from each CA with EAB, then force Let's Encrypt to fail
  (staging off, an invalid first directory) and confirm the second CA issues.
  Record in `docs/acme-verification-runbook.md`.

## Slices

| # | Slice | Files it owns | Effort | Model | Real host | Secret-sensitive |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | Preset table, `external_account` field, ordered issuer rendering | `internal/ingress/config.go`, new `internal/ingress/ca_presets.go` | 1 day | Opus (ingress config is shared and order-sensitive) | No | Yes (field shape) |
| 2 | Settings storage of the list, EAB through `internal/secrets`, masked reads, audit, env compatibility | `internal/api/ingress_https.go`, settings store, `cmd/levelrail/boot_misc.go`, migration | 1 day | Opus | No | Yes |
| 3 | Redaction test across all config display paths, list-aware purge | `internal/ingress` purge file, new redaction test | 0.5 day | Opus | No | Yes |
| 4 | CLI flags and MCP fields | `cmd/levelrail-cli/settings_ingress.go`, `internal/mcptools` ingress tool | 0.5 day | Sonnet | No | Yes (input only) |
| 5 | Dashboard CA card and locale keys | `web/src` ingress settings card, `web/src/locales/en/*.json` | 0.5 day | Sonnet | No | No |
| 6 | CAA check per CA in the domain doctor | `cmd/levelrail-cli/domains_doctor.go`, doctor API file | 0.5 day | Sonnet | No | No |
| 7 | Docs and real-host runbook entry | `docs/domains-and-ingress.md`, `docs/environment-variables.md`, `docs/acme-verification-runbook.md` | 0.5 day plus run | Sonnet | Yes | No |

Slices 1, 2 and 3 run in sequence (shared ingress files). 4, 5 and 6 can run in
parallel after 2.

New env: `APP_ACME_MAX_ISSUERS`, `APP_ACME_EAB_KEY_ID`,
`APP_ACME_EAB_MAC_KEY_FILE`, one directory override per preset
(`APP_ACME_CA_<PRESET>_DIRECTORY`, `APP_ACME_CA_<PRESET>_STAGING_DIRECTORY`).

## Consequences

- A second free CA without manual certificate uploads.
- One more stored credential type and a redaction duty on every config display
  path.
- Certificates on one host may come from different CAs over time.
- No `go.mod` change: the embedded engine already accepts the field.

## Open questions

- Per-host CA pin: build when a user asks, or never?
- Should the list default to two entries when the operator enables HTTPS, or
  stay Let's Encrypt only until they add a second CA? Proposed: Let's Encrypt
  only, second CA is opt-in.
- Does the existing test setup include an ACME server with EAB support, or
  does slice 1 need a fake?
