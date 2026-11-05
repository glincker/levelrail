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
4. **Pin to the digest the registry serves.** The deploy image is
   `repo:tag@digest`. The control plane always resolves the tag with the
   app's registry credentials and deploys that digest. A GHCR payload
   digest is only a claim: when it differs from the resolved one the push
   is recorded as ignored (audit detail keeps both) and nothing deploys, so
   a forged or replayed delivery cannot choose the image.
5. **Same gates as a manual deploy.** The push runs through the normal
   deploy path under a `registry_push` principal: freeze, policy, disk and
   node guards apply. In a protected environment it files a deploy
   approval for the digest. Its requester is the person who last rotated
   the hook (recorded at rotation), so that person cannot approve it; a
   hook rotated before this ADR has no recorded person and files no
   approval until it is rotated once. One push per app is processed at a
   time and a redelivery while an approval is pending files no second
   one; a failed pending-approval lookup refuses the push.
6. **Window and debounce.** The auto-update apply window holds a push
   outside it (recorded as waiting), except in a protected environment,
   where a person approves anyway. Unsigned deliveries are debounced like
   the plain hook, since each costs a registry lookup; a signed repeat of
   the digest just handled is debounced too. The debounce is the runtime
   setting `deploys.registry_push_debounce`.
7. **Asynchronous and audited.** The hook answers 202 at once. The outcome
   is stored as the app's last auto-update result and as an audit row
   `registry_push.deploy` whose detail names provider, repository, tag and
   digest. The URL token never appears in the row.
8. **Settings, no migration.** `deploys.registry_push_max_body_bytes` and
   `deploys.registry_push_timeout` are runtime settings. Debounce stays for
   the digest-less path only; digest-carrying pushes dedupe on the digest
   already deployed and on pending approvals. No schema change.

## Consequences

- A leaked URL can only deploy what the registry currently serves for the
  app's own repository and tag, because the digest is resolved, never
  taken from the payload. The GHCR signing secret gates payload
  deliveries only: a URL-only POST with no push body still runs the plain
  tag check, which also resolves registry truth.
- Docker Hub bodies are accepted only for docker.io repositories, so a
  headerless body cannot stand in for a signed GHCR delivery.
- The secrets store has no delete. Rotating replaces the secret and the
  URL token; disabling auto-update stops pushes; an old secret of a
  deleted app is replaced on the next rotation, which a recreated app
  needs anyway because its hook is gone.
- Rotation is serialized per app, records the rotator and stages the new
  signing secret (accepted beside the old one) before the new URL is saved,
  so a failure up to the save restores both and leaves the working hook intact. After the
  save the caller always gets the URL and secret; a failed promotion is
  finished by the next signed delivery. A rotation by a caller with no person behind it
  clears the recorded rotator, so no approval can be filed until a person
  rotates again.
