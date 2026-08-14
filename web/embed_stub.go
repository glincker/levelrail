//go:build !embedweb

package web

import "embed"

// DistFS is intentionally empty (Go's embed.FS zero value, no //go:embed
// directive) in the default build. web/dist is a gitignored build
// artifact that doesn't exist until `npm run build` runs; a bare
// //go:embed dist here would make plain `go build ./...` fail on any
// checkout that hasn't built the frontend yet, breaking it repo-wide,
// including every quick sanity `go build`/`go vet`/`golangci-lint run`
// both this project's git hooks and CI already rely on. The real
// frontend is embedded only by an explicit `-tags embedweb` build, after
// `cd web && npm run build`; see embed_real.go and Handler's doc
// comment for what happens when it isn't.
var DistFS embed.FS
