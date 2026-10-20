---
title: "Publishing plugins"
tag: "Experimental"
description: "Sign a plugin, make a marketplace index entry and sign an index, all offline."
"og:url": "https://docs.levelrail.com/plugin-publishing"
---

This page is for plugin publishers and for whoever maintains a marketplace index. Everything runs offline with `go run ./scripts/plugin-publish`; nothing is uploaded. Hosting the files and publishing the image are your steps.

There are two keys with two jobs. The publisher key signs each plugin manifest. The index root key signs the whole index, and an index also lists the publisher keys it vouches for. Keep both private keys off the servers that host the files.

## 1. Make the keys

```bash
go run ./scripts/plugin-publish keygen -out publisher.key
go run ./scripts/plugin-publish keygen -out root.key
```

Each prints the public key and its fingerprint. The file holds a base64 ed25519 seed with mode 0600 and is never overwritten.

## 2. Sign the manifest

Build and push your image, then pin it by digest in `plugin.yaml` (see [plugins](/plugins)). Sign the manifest together with that digest:

```bash
go run ./scripts/plugin-publish sign -manifest plugin.yaml -key publisher.key > plugin.sig
```

## 3. Make the index entry

Host `plugin.yaml` at an https URL, then:

```bash
go run ./scripts/plugin-publish entry -manifest plugin.yaml -signature plugin.sig -key publisher.key \
  -manifest-url https://plugins.example.com/example-notify/plugin.yaml \
  -name "Example notify" -category chat -icon slack -changelog "First release" > entry.json
```

The command refuses to continue if the signature does not verify under the key. Abilities, outbound hosts, version and digest are copied from the manifest, so the listing cannot disagree with it. `-icon` is a brand icon key, leave it empty for a monogram tile.

## 4. Sign the index

```bash
go run ./scripts/plugin-publish index-sign -entry entry.json \
  -publisher "<publisher public key base64>=Example Org" \
  -sequence 2 -key root.key -out index.json
```

`-sequence` must be higher than the last index you published: control planes reject an older sequence as a replay. Repeat `-entry` and `-publisher` for more plugins and publishers. Entries from several publishers go in one index, each needs its publisher key listed.

Host `index.json` at an https URL. Operators add it with the URL and your root public key:

```bash
levelrail-cli plugins marketplace sources add --url https://plugins.example.com/index.json --root-key <root public key base64>
```

For a default source on a whole install, set `APP_MARKETPLACE_URL` and `APP_MARKETPLACE_ROOT_KEY` on the control plane.

## What the control plane checks

Before it shows or installs anything it verifies the index signature, the size cap, the sequence, every entry's shape, that each entry's publisher key is one the index lists, the manifest signature under that key, and that the fetched manifest equals the listing. Tampering with an entry after signing, signing with another root key, replaying an older index, listing a root or write ability, or pointing a manifest at a different digest each fail a test in `internal/plugin/marketplace_test.go`.

A sample index and manifest signed with throwaway keys live in `internal/plugin/testdata/marketplace`. Regenerate them with `UPDATE_MARKETPLACE_SAMPLE=1 go test ./internal/plugin -run Sample`.

## What you must do yourself

- Generate and protect the root and publisher keys.
- Host the index and manifests over https and publish the image to a registry.
- Raise `-sequence` on every index you publish, and never reuse a root key you lost control of.
