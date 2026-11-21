// Command gen-field-docs writes one docs/<resource>-fields.md per registered field
// descriptor resource so the docs table cannot drift from the code.
//
// Usage (from the repo root): go run ./scripts/gen-field-docs
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/GLINCKER/levelrail/internal/fielddesc"
)

func main() {
	for _, name := range fielddesc.Names() {
		r, _ := fielddesc.Get(name)
		if err := os.WriteFile(filepath.Join("docs", r.Docs.File), []byte(fielddesc.DocsPage(r)), 0o644); err != nil { //nolint:gosec // docs file
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
