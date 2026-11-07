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

## Cutover wiring (S11)

- Deploy tail: `finishDeploy` reads the previous desired service before saving the
  new one and, once the new release is current, calls `NextGraceBook.Begin` with the
  previous `NEXT_DEPLOYMENT_ID` and image. Only Next.js apps whose deploy set an id,
  with the setting on and grace above 0, open a window. Previews never do.
- Durable: windows live in `next_asset_grace` (migration 3202). The shared book writes
  through and `Load` restores it at boot, so a control plane restart keeps the window.
- Hold: `application.WithNextGraceHold` extends the previous release hold to the end of
  the window. The same logic keeps only the newest previous release, and
  `deploy.next_asset_grace_max_held` (default 1, max 3) caps how many generations get a
  window. Rollback and the next deploy use the normal retire path, so a release that is
  no longer the newest previous one is removed as before.
- Ingress: `internal/ingress/next_skew.go` adds a route before the host route. It
  matches `/_next/static/*` and `/_next/data/*` carrying `?dpl=<old id>` or the
  `x-deployment-id` header and proxies to the held previous containers. Everything
  else goes to the current release. The held containers are never in the normal
  upstream pool. The route is rebuilt by the normal ingress pass, so it appears with
  the deploy and disappears at the next resync after the window ends.

## Cost

A held generation keeps its containers running for the whole window (default 30
minutes): one extra copy of the app's memory and CPU. Lower
`deploy.next_asset_grace_seconds` or set it to 0 to avoid that.

## Still needs a real host

- A real cutover on a Next.js app with `deploymentId` honored by the build: confirm an
  old tab's chunk request with `?dpl=<old>` is answered by the held container, a request
  without it by the new one, and that the held container goes away after the window.
- The held upstream is the best ranked other running container of the service. With
  multi node placement or a failover pool that ranking is unproven.
- A control plane restart in the middle of a window, with the held containers still up.
