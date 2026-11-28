# Announcement feed source

This folder is the authoring side of the in-app announcement feed. A workflow builds
`dist/feed.v1.json` from it and publishes the signed result to the orphan `feed`
branch. Instances fetch only that built file, never this folder.

## Layout

```
announcements/YYYY/<id>.json    one item per file, file name equals id
schema/announcement.v1.schema.json   JSON Schema of one item, every field documented
keys/roles.json                 public keys and the tiers each key may sign (add when the keys exist)
keys/roles.example.json         empty template
```

`dist/` is produced by the publish workflow and is never edited by hand.

## Adding an item

1. Pick the PR template that matches the item (security, release, feature, newsletter,
   maintenance) from `.github/PULL_REQUEST_TEMPLATE/`.
2. Add `announcements/<year>/<id>.json`. The id is `YYYY-MM-DD-short-slug`.
3. Run `go run ./tools/announce validate feed-src` from the `tools` module folder
   (`cd tools && go run ./announce validate ../feed-src`). The validate workflow runs the
   same command on every pull request.
4. A second maintainer reviews. Nothing in this folder is signed until after merge.

## Tiers

| Tier | Signed by | Seen by | Notes |
| --- | --- | --- | --- |
| critical | both security keys, offline | root | cannot be dismissed while it applies |
| advisory | one security key, offline | root | needs `applies_to` |
| feature, maintenance, info | CI key | everyone signed in | |
| newsletter | CI key | everyone signed in | quiet list, no cta, extra wording rules |

Critical and advisory items are never signed in CI. The publish workflow builds an
unsigned draft and asks the maintainers to sign offline with `tools/announce sign`.

## Content rules

Plain text plus a small markdown subset (paragraphs, bold, emphasis, inline code, `-`
lists, links). No HTML, images, fenced code, shell commands or secrets. Links are https
only, host `levelrail.com`, or `github.com` under `/glincker/`, with no query or
fragment. Titles are at most 120 characters, bodies at most 2000. `applies_to` is a
semver range such as `>=1.5.0 <1.5.14` and is required for critical and advisory.
`expires_at` is after `issued_at` and at most 366 days later.

Newsletter items also avoid paid or enterprise wording, statements of product limits,
competitor claims and urgency words in the title.

The examples under `announcements/2026/` show the shape. Review them before the first
real publish.
