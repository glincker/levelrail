# Deploy safety

Four guarantees sit between "a deploy was triggered" and "that exact
content is serving": digest-truthful deploys, a stale-deploy guard,
deploy freeze windows, and a short hold of the previous release after a
cutover.

## Digest-truthful deploys

A floating tag such as `nginx:latest`, `main` or `1.27` names different
content over time. If a platform compares tags or container names, a
redeploy of the same tag can report success while the old image keeps
running.

Every image deploy (dashboard, `apps deploy`, API, pipeline deploy step,
`build.type: image` in `app.yaml`) resolves the tag to its content
digest at deploy time and pins the desired image to it, for example
`nginx:latest@sha256:3f1c...`. For a multi-arch image the digest is the
manifest list, so each node still pulls its own platform. Because the
pinned reference is part of the container identity, new content always
gets a new container and goes through the normal blue-green, rolling or
recreate cutover.

Builds are identified by the local image ID the build produced. Rebuilding
the same commit with different content (a newer base image, different
build args) therefore replaces the container instead of silently keeping
the old one.

The application controller then checks that the running container's image
ID matches what the pinned reference resolves to on its node. If it does
not, the app is reported `Ready: False` with reason `ImageDigestMismatch`
and the previous release keeps serving. It never reports `Deployed` while
the running content differs.

### How the digest was obtained

Each deploy in the history carries a digest and a reason:

| Reason | Meaning |
| --- | --- |
| `Resolved` | The registry answered for the tag at deploy time. |
| `AlreadyPinned` | The reference already carried a digest (a rollback, or you deployed `image@sha256:...`). |
| `LocalBuild` | The image ID of the image this deploy built. |
| `PullFailedUsingCached` | The registry could not be reached, so the locally cached image for the tag was deployed. The dashboard marks this in amber. |
| `Unresolved` | Neither the registry nor the local cache knew the tag; the node resolves it when it creates the container. |

To refuse a cached fallback, deploy with pull:

```bash
levelrail apps deploy web --pull              # re-resolve the current tag
levelrail apps deploy web --image nginx:1.27 --pull
```

With `--pull` (API: `"pull": true`) a deploy fails with 502 when the
registry is unreachable instead of deploying the cached image.

`apps deploys list` shows the digest, the rollout state the controller
last observed (`serving` or `mismatch`) and any reason.

### Rollback

History rows store the pinned reference, so rolling back to a row deploys
exactly the content that row ran, even for a floating tag that has since
moved. Rows recorded before this feature existed hold only a tag; rolling
back to one re-resolves that tag.

## Stale-deploy guard

Webhooks can arrive out of order, and a slow build can finish after a
faster, newer one. Every automatic deploy (git push, release event,
pipeline deploy step, a released freeze hold) gets a per-app sequence
number when it is triggered, plus the commit ordering the git provider
sent (`before` and `after` SHAs and the head commit timestamp). When it is
about to write desired state it is compared, in the same transaction, with
the last deploy that actually applied:

- a deploy triggered earlier than one already applied is `superseded`;
- a push whose commit is the parent of the deployed commit, or whose
  commit is older by timestamp, is `superseded` with a `stale commit`
  reason;
- redelivery of the commit that is already deployed is allowed.

Manual deploys and explicit rollbacks are never rejected. They do advance
the sequence, so an older build still in flight cannot undo a rollback.
Superseded deploys show in the history with their reason.

## Freeze windows

A freeze window starts at every match of a 5-field cron expression,
evaluated in a timezone (default UTC), and lasts a duration. Windows can
be set per app and globally; both apply.

While a window is active:

- git push and release webhooks, and pipeline deploy steps, are **held**:
  recorded as `Held (frozen)` in the deploy history and replayed
  automatically once the window ends. If several deploys are held for one
  app, only the newest is replayed; the others are marked superseded.
- manual deploys from the dashboard, CLI or API return 423 unless they
  carry an override with a reason. The reason is recorded on the deploy.

```bash
levelrail apps freeze set web --cron "0 17 * * 5" --duration 64h \
  --timezone Europe/Berlin --reason "weekend freeze"
levelrail apps freeze show web
levelrail apps deploy web --image app:hotfix --override-freeze --override-reason "CVE fix"
levelrail apps freeze clear web
```

In the dashboard, app **Deploy settings** has a **Deploy freeze** card, and
the deploy form asks for an override reason while a freeze is active.
Global windows are managed through `PUT /api/v1/settings/deploy-freeze`
(root only). The MCP server exposes `get_deploy_freeze` read-only.

Held deploys are checked every `APP_DEPLOY_HELD_RELEASE_INTERVAL`
(default `30s`).

## Holding the previous release

After a successful blue-green or rolling cutover the previous release's
running container is kept for `APP_DEPLOY_PREVIOUS_RELEASE_HOLD` (default
`3m`, `0` disables), measured from the newest container's creation. During
that window a rollback to it is instant (no pull, no cold start), and
routes that still point at it keep working. Once the window passes, the
next reconcile removes it. The `recreate` strategy never holds anything,
since it stops the old release before starting the new one.

## Limits

- On remote nodes reached over the agent transport the running image ID is
  not yet reported, so the controller cannot detect a mismatch there; the
  digest pinning itself still applies.
- Digest resolution runs on the control plane's Docker daemon. A private
  registry needs a registry credential on the app.
- Apps created or edited through the plain app create/update and compose
  paths keep the image reference exactly as given until their next deploy.
