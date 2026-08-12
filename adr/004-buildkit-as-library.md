# ADR 004: BuildKit as a library, not `docker build`

Status: Accepted
Date: 2026-08-11

## Context

Build time is the part of a deploy the user is staring at a spinner for, and
it's also the part most self-hosted deploy platforms leave entirely to
whatever `docker build` or `docker compose build` gives them for free. Phase
0's competitor clones back this up indirectly: nowhere in
`docs-local/research/prior-art-*.md` does a studied competitor expose
build-cache controls as a first-class, programmatically-driven feature.
Coolify's rolling update starts from `start_by_compose_file()` running
`docker compose ... up --build -d` (`prior-art-coolify.md` Q1,
`ApplicationDeploymentJob.php:1971`), and Dokploy's build pipeline is
described as "a single large shell string executed remotely and piped to a
log file" (`prior-art-dokploy.md` section 5). Neither treats the build
system as something worth its own architecture; it's whatever the Docker
CLI happens to do.

## Decision

Use `moby/buildkit`'s Go client library, imported directly into the control
plane binary, to drive builds, not `docker build` shelled out as a
subprocess and not the plain Docker Engine API's legacy build endpoint.
This gets remote build cache, parallel stage execution, and cache mounts as
programmatically controlled features rather than CLI flags a user has to
know to pass. CLAUDE.md 4.4 states the cost of not doing this plainly:
remote cache is "the difference between a 6 minute Next.js build and a 40
second one." That number is the plan's own stated rationale, not yet an
independently measured Phase 0 finding, since detailed build-time
benchmarking wasn't part of the competitor research; it's recorded here as
the reasoning the decision was made on, not as a verified result.

Auto-detection for repos without a Dockerfile uses Railpack, not Nixpacks.
Dockerfile and Compose are first-class build input types alongside it.
Static sites are served directly by the embedded Caddy instance (4.5) with
no container at all, skipping the build-and-run-a-web-server-container step
entirely for the case that doesn't need one.

A Phase 0 spike proving out the BuildKit Go client standalone has since
landed in `internal/build/` per CLAUDE.md's Phase 0 work list ("Read the
BuildKit Go client examples and get a programmatic build working
standalone"). See the Verified section below.

## Rejected alternatives

- **Plain `docker build` (or `docker compose build`), shelled out or called
  via the Engine API's legacy build endpoint.** No remote cache, no
  programmatic control over cache-mount behavior, no parallel stage
  execution beyond whatever the local BuildKit daemon backing `docker build`
  happens to do on its own. This is genuinely the status quo across the
  competitor set studied: Coolify runs `docker compose ... up --build -d`
  directly, and Dokploy's build step is an opaque shell string rather than
  a library call it can tune cache behavior on. Both approaches mean cache
  is whatever's on the local daemon's disk, with no way to share it across
  build nodes (relevant later, since Phase 3 scopes dedicated build nodes
  specifically so builds don't compete with production workloads for CPU,
  a design that only pays off if build cache can be addressed remotely
  rather than living on whichever box happened to run the last build).
- **Nixpacks over Railpack.** CLAUDE.md 4.4 states Railpack is Nixpacks'
  successor and is "significantly lighter," and that's the basis this
  decision rests on. This is worth flagging explicitly as taken on the
  plan's own authority rather than on Phase 0 research: none of the five
  `prior-art-*.md` docs cover build auto-detection in any depth (Coolify's
  own auto-detection is Nixpacks-based per its stack, but the research
  didn't go deep on comparing it against Railpack specifically), so unlike
  the other rejected alternatives in this ADR set, there isn't an
  independent research finding to cite here, only the stated decision.
  Flagging this now so a future agent doesn't mistake the absence of a
  citation for an oversight.

## Consequences

- The control plane binary now embeds a BuildKit client, which pulls
  BuildKit's dependency graph into the single-binary build (ADR 001);
  BuildKit is a Go-native project, so this is a real library import, not a
  wrapper around a subprocess, consistent with the "no shelling out"
  principle already established for Docker access in ADR 002 and ADR 003.
- Remote build cache needs somewhere to live and be addressed from, which
  Phase 3's dedicated build nodes depend on: cache has to be a shared,
  network-addressable resource, not implicitly local-disk-only the way
  `docker build`'s default cache is. This has to be designed before Phase 3
  needs it, not discovered at that point.
- Because static sites skip containers entirely, the reconciler and the
  ingress layer (4.5) both need a code path for "this deploy target isn't a
  container Docker manages," which is a real branch in the deploy state
  machine, not a special case that can be papered over later. Get this
  named explicitly in the app spec's `build.type` enum (4.9 already lists
  `static` alongside `dockerfile`/`compose`/`railpack`) rather than left
  implicit.
- Build log streaming (Phase 1: "BuildKit integration: Dockerfile builds
  with remote cache, build log streaming over SSE") has to consume
  BuildKit's own progress/status API rather than parsing stdout text the
  way a shelled-out `docker build` would force. This is more work up front
  and a better result: structured build events instead of scraped log
  lines.
- The 6-minute-to-40-second number in CLAUDE.md 4.4 is currently a stated
  target, not a measured one. The Phase 0 spike deliberately does not touch
  `SolveOpt.CacheImports`/`CacheExports`, so that number is still unmeasured
  as of this ADR; it becomes a real, checkable claim once Phase 1 wires up
  remote cache.

## Verified

Phase 0 spike (`internal/build/`, full findings in
`docs-local/research/buildkit-spike.md`) proves the core claim of this ADR:
a real image builds from a real Dockerfile through BuildKit's Go client,
with zero `docker` CLI shelling anywhere in the path. Verified twice:
`internal/build/build_test.go`'s `TestClient_Build_Live` builds the fixture
and independently re-checks the result via `ImageInspectWithRaw` (not by
trusting BuildKit's own success response), and a separate manual run
confirmed the same image with the real `docker` CLI (`docker images`,
`docker run`, `docker rmi`).

Connection method: `github.com/docker/docker/client/buildkit`'s
`ClientOpts()`, dialing BuildKit's `/session` and `/grpc` endpoints through
the existing Docker Engine API client's `DialHijack`, the same mechanism
`docker buildx build` uses for the default `docker` driver. No separate
buildkitd, no extra socket, no new daemon dependency beyond what
`internal/docker` already requires.

What is still unverified, honestly: remote cache (`CacheImports`/
`CacheExports` untouched, so the 6-minute-to-40-second claim above stays
unmeasured), SSE log streaming to a frontend, app-spec-driven `Request`
construction, and behavior against a bare Linux dockerd rather than Docker
Desktop's `docker` driver on macOS. All four are named explicitly as Phase
1 scope in `docs-local/research/buildkit-spike.md`, not silently deferred.

One process finding worth recording here: the spike caught a real bug in
this repo's own `.gitignore` (a bare `build/` entry, meant for a future
`web/build/` frontend directory, unintentionally also matching
`internal/build/` since unanchored gitignore patterns match at any depth,
which would have silently dropped this entire package from `git add -A`).
Fixed by anchoring the rule to `/web/build/` before this work was
committed.
