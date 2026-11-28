---
title: "In-app announcements"
description: What the signed announcement feed is, what an instance fetches and reveals, how to turn it off or use your own copy, and how to verify the feed yourself.
"og:url": "https://docs.levelrail.com/in-app-announcements"
---

Levelrail can show short notices inside the dashboard: a security fix is available, a maintenance window, a new feature. The notices come from one public, signed file. This page says exactly what an instance fetches and what that reveals.

## What the feed is

- One JSON file, `feed.v1.json`, and a detached signature list next to it. Both are public.
- The files live on the `feed` branch of the `glincker/levelrail` repository on GitHub. The authoring source is the `feed-src` folder of the same repository, and every change goes through a reviewed pull request.
- Every item is plain text with a small markdown subset (bold, emphasis, inline code, lists, links). No HTML, images or commands. Links are limited to `levelrail.com` and `github.com/glincker/`.
- The only action an item can offer is a link to Settings > Updates inside your own instance. Upgrading still uses the normal, verified update path.

## What an instance fetches and what it reveals

Each check downloads the one public file. GitHub sees your instance's IP address, the time, and the standard TLS and `User-Agent` details of a Go HTTP client. The request carries no query string, no cookie, no instance id, no version number and no counts. Which notices apply to you is decided locally, on your instance, from the same bytes every instance receives.

Levelrail the project receives nothing from this request, so it cannot tell how many instances read the feed.

## Turn it off, or use a copy you host

- The feed is off until an admin answers a prompt in the dashboard, and an admin can change the choice at any time in Settings > Updates. The choices are security notices only, all notices, or off.
- Nothing is fetched while it is off.
- To avoid contacting GitHub, set the feed URL setting to a copy you host yourself. Signatures are still checked against the keys built into Levelrail, so a copy cannot change the content. Levelrail does not run a mirror for you.
- Air gapped instances can load the same two files from a local path instead of a URL.

## Tiers

| Tier | Who sees it | Signed by | Can be dismissed |
| --- | --- | --- | --- |
| critical | admins | both offline security keys | snoozed only, while it still applies |
| advisory | admins | one offline security key | yes |
| feature, maintenance, info | signed in users | CI key | yes |
| newsletter | signed in users, in a separate quiet list | CI key | yes, and the whole list can be hidden |

An instance drops an item whose tier was not signed by keys allowed to sign that tier. A leaked low tier key can only publish low tier text.

## Verify the feed yourself

Clone the repository and use the same tool that CI uses. The instance applies the same rules.

```bash
git clone --branch feed https://github.com/glincker/levelrail.git levelrail-feed
git clone https://github.com/glincker/levelrail.git levelrail-src
cd levelrail-src/tools
go run ./announce verify \
  -feed ../../levelrail-feed/dist/feed.v1.json \
  -roles ../../levelrail-feed/keys/roles.json
```

`verify` checks the size cap, every signature over the exact bytes, the sequence number, feed expiry, then each item: tier signing, expiry, content rules and the link allowlist. It prints the items it accepts and the ones it drops, and exits non zero on any problem. Pass `-last-sequence N` to test replay protection against a feed you saw earlier.

Compare the public keys in `keys/roles.json` with the ones announced on the project's release notes before you trust them for anything beyond a read.

## Related

- [Advisory feed](/advisory-feed) matches the images your apps run against advisories you supply.
- The [changelog](/changelog) lists every release.
