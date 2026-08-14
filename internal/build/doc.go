// Package build drives container image builds through BuildKit's Go
// client (github.com/moby/buildkit/client), never by shelling out to
// `docker build` or the `docker` CLI, matching the same "no CLI
// shelling" rule the node communication layer follows. It accepts
// plain Dockerfiles via the dockerfile.v0 frontend, per the declarative
// app spec's build config.
//
// This package is the Phase 0 spike only: it proves the client works end
// to end against the BuildKit instance embedded in a local Docker Engine,
// and loads the result into the local image store. Remote build cache,
// SSE log streaming to the frontend, and app-spec-driven config are
// Phase 1 work and are not implemented here. See
// docs-local/research/buildkit-spike.md for what was learned building
// this and what Phase 1 needs to add.
package build
