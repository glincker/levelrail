// Package build drives container image builds through BuildKit's Go
// client (github.com/moby/buildkit/client), never by shelling out to
// `docker build` or the `docker` CLI, matching the same "no CLI
// shelling" rule the node communication layer follows. It accepts plain
// Dockerfiles via the dockerfile.v0 frontend, per the declarative app
// spec's build config, and Railpack-detected sources via their own solve
// path (railpack.go).
//
// A build runs against the BuildKit instance embedded in whichever
// Docker Engine is local to the process running it, and Router
// (router.go) decides which process that is: this one, or a node an
// operator marked build-capable, reached over the agent transport.
package build
