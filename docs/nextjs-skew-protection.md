# Next.js skew protection, deploy side

Slices S9 and S10 of ADR 074. A browser tab on an old Next.js build asks for old
`/_next/static/<hash>` chunks and server action ids after a cutover. This
describes what the deploy side does and what the ingress slice (S11) must add.

## Done

- `deploy.next_deployment_id` (seed `APP_DEPLOY_NEXT_DEPLOYMENT_ID`, default on):
  for Next.js sources (`build.DetectNext`) the Railpack deploy path sets
  `NEXT_DEPLOYMENT_ID` to the deploy attempt id (commit sha when there is none) as
  a build env var and as a container env var. A `NEXT_DEPLOYMENT_ID` the app sets
  itself is never overridden. Dockerfile builds are not touched: the Dockerfile
  owns its build and the user can declare the variable themselves.
- `deploy.next_asset_grace_seconds` (seed `APP_DEPLOY_NEXT_ASSET_GRACE_SECONDS`,
  default 1800, 0 off): `NextGraceBook` in `internal/deploy/next_assets.go` records
  the superseded release (service, deployment id, image, until).

## Design decision

Old assets are served by the old containers, not copied out of the image. The
ingress must route only `/_next/static/` and `/_next/data/` requests that carry
the old id (`?dpl=<id>`, or the `x-deployment-id` header) to the held previous
release; everything else goes to the new release.

The previous release hold (`deploy.previous_release_hold`, default 3m, ADR open
question 4) is too short for a 30 minute grace. Answer: the hold must be
extended to the grace window for Next.js apps in the reconciler
(`internal/reconcile/application`, `WithPreviousReleaseHold`), which this slice
may not touch.

## Not finished (for the ingress slice)

1. TODO: call `NextGraceBook.Begin` at cutover with the previous desired
   service's `NEXT_DEPLOYMENT_ID` and image. The book is in memory; a restart
   forgets it. A durable record needs a migration, deliberately not added here.
2. TODO: the reconciler keeps previous containers for `max(hold, grace)` when a
   grace record is active.
3. TODO: `internal/ingress/next_skew.go` (S11) reads `NextGraceBook.Active` and
   adds the id based route to the held containers.
