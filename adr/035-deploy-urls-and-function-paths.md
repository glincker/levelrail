# ADR 035: Immutable deploy URLs and function path routes

Status: Accepted

Date: 2026-10-18

## Context

Two ingress gaps came out of the platform comparison:

1. Vercel and Netlify give every deployment its own URL, so a past deploy can
   be opened and compared without rolling back. Here a past deploy only exists
   as an image pin (ADR 028 pins, `internal/deploywatch`) and a row in the
   deploy history.
2. A function should be reachable at `<domain>/<path>`, not only on its own
   host. Path forwarders already exist per domain (`internal/trafficpolicy`
   forwarders, compiled in `internal/ingress/policy_forward.go`), including
   prefix match, strip prefix and an app target. They fail for a function
   that is asleep, because the target dial only exists for a running app.

## Decision

Deploy URLs. A reconciler-adjacent syncer (`internal/deployurls`) keeps one
ordinary desired service per retained deploy, named `<app>-d<shortid>`,
running that attempt's pinned image and restored config, with one domain
`<app>-<shortid>.<apps base domain>` (or the sslip fallback host). Only the
newest `APP_DEPLOY_URL_KEEP` (default 3, 0 disables) succeeded attempts that
still have a rollback pin or a registry digest qualify, so image GC and
rollback pins stay the single source of retention. Each clone gets a sleep
setting with held requests, so an idle old deploy stops and wakes on the first
visit. Clones carry no volumes, host ports or hooks.

Function paths. No new route type. The route editor writes a forwarder rule
named `function:<name>` on the owning domain through the existing policy
store. The compiler learns to answer a forwarder that targets a sleeping app
with the wake handler instead of "not running".

Data plane. Both features produce ordinary services, domains and forwarder
rows. Caddy keeps serving the last applied config when the control plane is
down; nothing here needs a live call per request except the wake hook, which
already exists.

## Rejected alternatives

- Ingress routes to old containers kept by the application controller: needs
  the controller to hold N extra containers per app and a new drain model.
- A new `path_routes` table and a second route type in `BuildRoutesConfig`:
  duplicates forwarders, which already have validation, preview and audit.
- Digest-based URLs with no per-deploy service: nothing would answer them.
- Always-on old containers: idle CPU and memory cost for every past deploy.

## Consequences

- A deploy URL runs the old image with the same env, so side effects (jobs,
  webhooks) of an old deploy can fire while it is awake. Docs say so.
- Deploy URL services appear in the app list under their own names and are
  deleted when they fall out of the keep window.
