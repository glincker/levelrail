## Security advisory announcement

<!-- Item file: feed-src/announcements/YYYY/<id>.json. Keep exploit details out of this PR and out of the item. -->

## What this tells instance owners

<!-- One or two plain sentences: what to do and by when. -->

## Checklist

- [ ] Tier chosen: `advisory` (one offline security key) or `critical` (both offline security keys). Not signed in CI.
- [ ] `applies_to` is a real semver range (not `*`) and the fixed release is the first version outside it
- [ ] `expires_at` set, at most 366 days after `issued_at`
- [ ] The fixed release is already published and installable from Settings > Updates
- [ ] No exploit details, proof of concept, affected endpoint or reproduction steps before the fixed release is available
- [ ] Links only to `levelrail.com` or `github.com/glincker/` (no query, no fragment)
- [ ] No HTML, images, shell commands or secrets in title, body or i18n
- [ ] `cta` is `updates` (an internal route, never a URL)
- [ ] `cd tools && go run ./announce validate ../feed-src` passes
- [ ] Reviewed by a second maintainer
- [ ] I know signing happens offline after merge (see the owner runbook), not in CI
