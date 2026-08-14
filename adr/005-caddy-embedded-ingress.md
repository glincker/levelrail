# ADR 005: Caddy embedded as a Go library for ingress

Status: Accepted

Date: 2026-08-11

## Context

Levelrail needs TLS-terminating ingress that maps the domains declared in
`app.yaml` to running containers, issues and renews ACME
certificates without operator intervention, and, once multi-node ships
(4.6, Phase 3), shares certificate state across every node running ingress
rather than pinning it to whichever node happened to issue it. The platform's
ingress design settles how: import `caddyserver/caddy/v2` into the control plane binary and
drive it in-process through Caddy's admin API, not as a sibling process with
its own config file and reload cycle.

A Phase 0 spike in `internal/ingress/` has since proven this out, following
the Phase 0 work item "read Caddy's library embedding docs and get a
programmatic reverse proxy with auto TLS working standalone." See the
Verified section below.

## Decision

Import `caddyserver/caddy/v2` directly into the control plane binary.
Configure it by pushing JSON config through Caddy's in-process admin API,
never by writing a Caddyfile to disk and asking a separate process to
reload it. ACME issuance, HTTP/3, and automatic renewal are Caddy's own
behavior, invoked as a library call, not a shelled-out certbot-equivalent.
Certificate and ACME account state is persisted in Levelrail's own SQLite
store (4.7) rather than left in Caddy's default on-disk storage, so that
once ingress runs on more than one node, cert state is shared rather than
each node independently negotiating and holding its own copy.

## Rejected alternatives

- **Traefik**: the platform's own stated reason for rejecting it is that
  running it means a separate container with its own config surface, which breaks the
  single-binary story that 4.1 commits to. That is a self-contained
  argument and does not need competitor evidence to stand, but the
  evidence below shows what a separate-process proxy costs in practice for
  two competitors that made this exact choice.

- **CapRover's approach: nginx as a fully separate process, config
  regenerated and reloaded as a distinct step**: `prior-art-caprover.md`
  traces the actual mechanism: `LoadBalancerManager.rePopulateNginxConfigFile()`
  writes a new config to a `.fut` file, renames the old `.conf` to `.bak`,
  renames `.fut` to `.conf`, runs `nginx -t` inside the nginx container to
  validate syntax, and only then sends `SIGHUP` to reload
  (`src/user/system/LoadBalancerManager.ts`, lines 168-239), with reloads
  serialized through an in-process queue so concurrent regenerations don't
  race each other. The research doc's own conclusion (section on the 4.5
  bet) is that CapRover's actual nginx footprint is small, a 30MB memory
  reservation, so the credible argument for embedding isn't idle RAM, it's
  eliminating "a second config surface, a second process to keep in sync,
  atomic file-swap-then-reload choreography, a validate-then-reload round
  trip through `docker exec nginx -t`." That is precisely the class of
  failure an in-process admin API removes structurally: there is no
  file-on-disk and process-that-reads-it pair to fall out of sync in the
  first place, because there is only one process and one config source of
  truth.

- **Proxy state living outside the main control loop, generally**: Coolify's
  own issue tracker shows what this costs even for a proxy that
  isn't hand-rolled. `prior-art-coolify.md` documents issue
  [#7193](https://github.com/coollabsio/coolify/issues/7193), "Coolify
  Traefik Proxy No Longer Working After Recent Update" (51 comments,
  closed): the proxy silently stops routing ("no available server") after
  an update or reboot, because cutover correctness depends entirely on
  Traefik's live label discovery re-establishing itself correctly, a
  process running independently of and unsupervised by Coolify's own
  control loop. CapRover's file-swap-then-reload dance and Coolify's
  Traefik-relies-on-its-own-discovery failure are different mechanisms
  with the same root shape: proxy state that lives and gets reconciled
  outside the control plane's own reconcile loop can drift or fail
  silently in a way the control plane has no way to detect or correct.
  Driving Caddy in-process through its admin API keeps ingress state
  inside the same process, and eventually the same reconcile discipline
  (4.2), as everything else.

## Consequences

- Ingress config changes happen as Go function calls against Caddy's admin
  API, not as file writes plus a signal plus a hope that the reload
  succeeded. There is no `nginx -t`-equivalent round trip to shell out to
  and no separate process whose crash silently stops routing without the
  control plane knowing.
- Cert renewal failures are an in-process error the control plane can
  observe and surface directly, not a cron job's exit code on a machine
  the control plane has to go check.
- The sharper differentiation point is against where this category is
  actually heading, not where it's been. Coolify's own in-progress v5
  rewrite is independently moving from Traefik to Caddy
  (`app/Actions/V5/Proxy/GenerateCaddyIngressConfiguration.php`, per
  `prior-art-coolify.md`), which validates Caddy as the right proxy choice
  in this category. But V5's Caddy is still a separately-orchestrated
  Docker Compose service the control plane generates a Caddyfile and
  compose YAML for (`GenerateCaddyIngressConfiguration::handle()`), not a
  library embedded in the control plane's own process. Levelrail's 4.5
  decision is a strictly tighter architecture than even Coolify's rewrite
  lands on: one process, one binary, no generated-config-plus-orchestrated-
  container indirection in between. That gap survives V5 shipping and is
  worth stating plainly in comparison materials once it does.
- This is a real dependency commitment: Levelrail is now coupled to
  Caddy's admin API stability and its Go module's compatibility with
  whatever Go toolchain version the control plane targets. Caddy upgrades
  become control-plane binary upgrades, not an independent container bump.
- Static sites being served directly by embedded Caddy with no container
  (per 4.4) only works because ingress already lives in-process; this
  decision is a prerequisite for that shortcut, not an independent one.

## Verified

Phase 0 spike (`internal/ingress/`, full findings in
`docs-local/research/caddy-spike.md`) proves the core claim: Caddy runs
in-process, configured entirely through `caddy.Load`, the literal function
backing the admin API's own `POST /load` handler, not a side door. Two real
integration tests (no mocks, run with `-race`): a plain HTTP reverse proxy
through a live Caddy instance, and an HTTPS reverse proxy secured by
Caddy's `internal` (self-signed, fully offline) TLS issuer, with Caddy's
own logs confirming a real certificate was obtained
(`"certificate obtained successfully","issuer":"local"`) before the
handshake succeeds. Both tests hit real ephemeral ports with a real
`net/http` client against a real `httptest` backend.

What is explicitly not proven, per the spike doc's own caution: real ACME
issuance against a public domain. The `internal` issuer path is verified;
Let's Encrypt/ZeroSSL issuance, DNS-01/HTTP-01 challenge mechanics, rate
limits, and renewal timing are unexercised. This must be spot-checked
against a real domain before Phase 1's exit criterion (thesvg.org serving
over HTTPS with a valid cert) can be claimed done, not assumed to follow
automatically from the internal-issuer result.

One finding changes how Phase 1's ingress controller has to be written,
not just implementation detail: `caddy.Load` replaces Caddy's entire
process-wide config on every call, there is no per-app incremental update.
The reconciler for ingress will need to hold or cheaply rebuild the full
desired `Config` from every known app and call `Apply` with the complete
thing on every change. This fits the reconciler's level-triggered philosophy
well (re-derive from current desired state every time, don't assume
incremental progress), but it is a real constraint, not a free choice, and
it settles the "how does the ingress controller diff desired vs observed"
question ADR 002 leaves open per-controller: for ingress specifically,
there is no meaningful partial diff, only whole-config replacement.

Also confirmed, and worth recording since it directly supports this ADR's
"Rejected alternatives" argument against CapRover's file-swap-then-reload
approach: Caddy's `pki` app attempts an unprompted `sudo` trust-store
install by default, and Caddy keeps its own on-disk autosaved config copy
regardless of the configured storage module, unless both are explicitly
disabled (`install_trust: false`, `admin.config.persist: false`). Left
alone, the second one would have quietly recreated exactly the
"second config surface" problem this ADR argues embedding avoids. Both are
disabled in the spike driver; Phase 1 needs to keep them disabled.
