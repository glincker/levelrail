// Package stacks embeds the bundled starter stack templates.
package stacks

import "embed"

// FS holds one directory per template.
//
//go:embed web-app-postgres worker-queue static-site-cdn agent-app preview-env
var FS embed.FS
