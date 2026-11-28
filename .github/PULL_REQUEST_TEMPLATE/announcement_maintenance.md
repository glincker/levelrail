## Maintenance notice

<!-- Item file: feed-src/announcements/YYYY/<id>.json -->

## What is affected and when

<!-- Plain sentences: the service or behaviour, the time window in UTC, what an instance owner should expect. -->

## Checklist

- [ ] Tier is `maintenance`
- [ ] The window is stated in UTC and `expires_at` is shortly after it ends
- [ ] `applies_to` and `channels` limit the notice to instances that are affected
- [ ] No exploit details or security claims (use the security template for those)
- [ ] Links only to `levelrail.com` or `github.com/glincker/` (no query, no fragment)
- [ ] No HTML, images, shell commands or secrets
- [ ] `cd tools && go run ./announce validate ../feed-src` passes
- [ ] Reviewed by a second maintainer
