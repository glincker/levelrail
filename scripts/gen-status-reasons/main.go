// Command gen-status-reasons writes docs/status-reasons.md from the reason
// catalog. Run from the repo root: go run ./scripts/gen-status-reasons
package main

import (
	"fmt"
	"os"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
)

func main() {
	if err := os.WriteFile("docs/status-reasons.md", []byte(deploysafety.RenderReasonsMarkdown()), 0o644); err != nil { //nolint:gosec // public docs file
		fmt.Fprintln(os.Stderr, "gen-status-reasons:", err)
		os.Exit(1)
	}
}
