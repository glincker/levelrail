//go:build embedweb

package web

import "embed"

// DistFS holds the built frontend (web/dist, produced by `npm run
// build`), embedded only when this module is built with -tags embedweb.
// See embed_stub.go for why that's opt-in rather than the default.
//
//go:embed all:dist
var DistFS embed.FS
