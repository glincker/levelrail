## Newsletter item

<!-- Item file: feed-src/announcements/YYYY/<id>.json. Newsletter items show in a quiet list, never as a banner. -->

## Checklist

- [ ] Tier is `newsletter`, with no `cta` and no security content
- [ ] `expires_at` set, at most 366 days after `issued_at`
- [ ] No paid or enterprise wording, and no pricing
- [ ] No statements of product limits
- [ ] No competitor names or comparison claims
- [ ] Title has no urgency words (now, urgent, today)
- [ ] Links only to owned channels: `levelrail.com` or `github.com/glincker/` (no query, no fragment)
- [ ] No email capture, tracking links or subscribe forms
- [ ] No HTML, images, shell commands or secrets
- [ ] `cd tools && go run ./announce validate ../feed-src` passes
- [ ] Reviewed by a second maintainer
