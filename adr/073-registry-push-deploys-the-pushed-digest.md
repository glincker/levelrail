# ADR 073: Registry push deploys the pushed digest

Status: Proposed

Date: 2026-11-05

## Context

`POST /api/v1/hooks/image-update/{name}/{token}` already lets a registry
tell the control plane that a tag may have moved. It only re-checks the
tag, runs on a debounce, and skips apps in a protected environment, so a
push to a gated app never reached a human. GHCR and Docker Hub both send
push events: GitHub as a `package` event (optionally signed with
`X-Hub-Signature-256`, digest included), Docker Hub as unsigned JSON with a
repository and tag but no digest.

## Decision

1. **One URL, three shapes.** The hashed per-app URL token stays the
   authentication. A delivery with `X-GitHub-Event` is a GHCR push, a body
   with `push_data` and `repository.repo_name` is a Docker Hub push, and
   anything else keeps the old "check the tag" behavior.
2. **GHCR signature.** Rotating the webhook also mints a signing secret,
   stored in the secrets store and shown once. When one exists, a GitHub
   delivery without a valid `X-Hub-Signature-256` over the raw body gets
   the same 404 as an unknown token. No secrets store means URL token only.
3. **Same repository, same tag.** The pushed repository must equal the
   app's current image repository (compared normalized), otherwise 404, so
   an unsigned Docker Hub delivery cannot point an app at another image.
   A push to a different tag is acknowledged and ignored.
4. **Pin to the digest.** The deploy image is `repo:tag@digest`. GHCR
   supplies the digest. Docker Hub does not, so the control plane resolves
   the tag with the app's registry credentials before deploying.
5. **Same gates as a manual deploy.** The push runs through the normal
   deploy path under a `registry_push` principal: freeze, policy, disk and
   node guards apply. In a protected environment it files a deploy
   approval for the digest (requester has no human behind it, so any
   approver may decide). A redelivery while that approval is pending does
   not file a second one.
6. **Asynchronous and audited.** The hook answers 202 at once. The outcome
   is stored as the app's last auto-update result and as an audit row
   `registry_push.deploy` whose detail names provider, repository, tag and
   digest. The URL token never appears in the row.
7. **Settings, no migration.** `deploys.registry_push_max_body_bytes` and
   `deploys.registry_push_timeout` are runtime settings. Debounce stays for
   the digest-less path only; digest-carrying pushes dedupe on the digest
   already deployed and on pending approvals. No schema change.

## Consequences

- A leaked URL can only redeploy the app's own repository and tag, and for
  GHCR also needs the signing secret.
- The secret has no delete path in the secrets store yet; rotating replaces
  it, and the URL token is what is revoked by rotation.
- A push that arrives during a running deploy is recorded as not deployed
  and is not retried; the next push or the interval check catches up.
