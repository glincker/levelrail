## Release announcement

<!-- Item file: feed-src/announcements/YYYY/<id>.json -->

## Release and audience

- Release tag:
- Who should see it (`applies_to`, `channels`):

## Checklist

- [ ] Tier chosen: `info` or `feature` for a normal release, `advisory` only if it carries security fixes (then use the security template)
- [ ] `applies_to` range is correct for the audience (for example `<1.6.0` to point older instances at the new release)
- [ ] `expires_at` set, at most 366 days after `issued_at`
- [ ] The release is published and the link resolves
- [ ] Links only to `levelrail.com` or `github.com/glincker/` (no query, no fragment)
- [ ] No HTML, images, shell commands or secrets
- [ ] Nothing here describes an unreleased or unproven feature
- [ ] `cd tools && go run ./announce validate ../feed-src` passes
- [ ] Reviewed by a second maintainer
