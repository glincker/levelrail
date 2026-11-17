// Package registry embeds the template registry so the binary serves it.
// registry/ stays the single source of truth; nothing is copied.
package registry

import "embed"

// Templates holds every registry/templates/<id>/ folder under the "templates" root.
//
//go:embed templates
var Templates embed.FS
