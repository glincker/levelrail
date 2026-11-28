# Announcement feed test vectors

Shared golden vectors for the feed verifier. `tools/announce` (CI side) and the
instance side verifier must produce the same result for every case.

## Layout

```
vectors/roles.json            public keys and tier limits used by every case
vectors/<case>/feed.v1.json   the exact signed bytes
vectors/<case>/feed.v1.json.sig  JSON list of {"key_id","sig"}, base64 ed25519 over the feed bytes
vectors/<case>/context.json   the verifier inputs
vectors/<case>/expect.json    the expected outcome
```

`context.json`: `now` (RFC 3339), `last_sequence` (highest sequence already stored),
optional `max_bytes` (override of the 256 KiB size cap), optional `revoked` (key ids
revoked in instance state).

`expect.json`: `ok`, `error_code` (empty when ok), `accepted` (item ids kept, in feed
order), `dropped` (items removed with a `code`; the human `detail` is not part of the
contract).

## Rules the vectors pin down

1. Size cap first. Over the cap: `too_large`.
2. Every signature from a known, unrevoked key must verify over the exact bytes, else
   `bad_signature`. Unknown key ids are skipped (rotation). No valid trusted signature:
   `no_trusted_signature`.
3. `schema` must be 1 (`bad_schema`), `sequence` must be above `last_sequence`
   (`replay`), feed `expires_at` plus the clock skew (24h) must be after `now`
   (`feed_expired`).
4. Items are checked one by one and dropped, never failing the feed:
   `unknown_tier`, `unknown_kind`, `duplicate_id`, `tier_not_signed`, `expired`,
   `content` (any content rule: html, link allowlist, commands, secrets, caps).
5. Tier signing: `critical` needs two distinct signing keys whose roles list `critical`;
   every other tier needs one signing key whose roles list that tier.

## Cases

| Case | Outcome |
| --- | --- |
| `valid` | all three items accepted |
| `critical_one_signature` | critical item dropped `tier_not_signed` |
| `role_mismatch` | advisory signed only by the newsletter key, dropped |
| `revoked_key` | critical dropped because S2 is revoked |
| `tampered` | `bad_signature` |
| `wrong_key` | `no_trusted_signature` |
| `replay` | `replay` |
| `expired_feed` | `feed_expired` |
| `expired_item` | item dropped `expired` |
| `html` | item dropped `content` |
| `bad_link` | item dropped `content` |
| `oversize` | `too_large` (context sets `max_bytes` 512) |

## Keys

The keys are TEST keys derived from the public seed phrase
`levelrail-announce-test-vector-key/<id>` (SHA-256 of it is the ed25519 seed). No key
file is stored. They protect nothing and must never appear in a production roles list.

## Regenerating

`go test ./announce -run TestVectors -update` from the `tools` folder rewrites the
files from the table in `vectors_test.go`, then checks them. Review the diff: a change
in a signed byte is a change in the contract with the instances.
