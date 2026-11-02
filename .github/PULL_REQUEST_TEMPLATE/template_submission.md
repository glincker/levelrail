## Template

- Template id:
- Upstream project (`source_repo`):
- Image and exact tag:
- Listing: community / vendor (vendor org, if vendor):
- Maintainer contact (GitHub handle):
- Does the image publish linux/arm64? yes / no / unsure

## Contributor checklist

- [ ] Only `internal/catalog/manifests/<id>/template.yaml` and `compose.yaml` are added
- [ ] `scripts/template-lint.sh <id>` passes locally
- [ ] Exact version tag, image from the upstream namespace, a Docker Official Image or an allowed rebuilder
- [ ] Healthcheck present, named volumes only, no privileged, host network, added capabilities, devices or sockets
- [ ] License is stated with its class; no secrets or telemetry opt-out values in the files
- [ ] Listing is free and the same rules apply to everyone (see docs/add-a-template.md)

## Maintainer checklist

- [ ] `license_url` matches the stated class
- [ ] Description is honest and specific, first-run steps make sense
- [ ] Any exception (short tag, non-upstream image, start-time download) has a recorded reason
- [ ] Vendor proof checked (public org member or DNS TXT record), if listed by a vendor
