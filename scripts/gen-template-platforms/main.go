// Command gen-template-platforms rewrites the arm64 section of
// docs/templates-status.md from internal/catalog/platforms.json (merged into
// catalog.Templates). Pass -check to fail instead of writing when stale.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

const (
	beginMarker = "{/* BEGIN GENERATED ARM64 TABLE */}"
	endMarker   = "{/* END GENERATED ARM64 TABLE */}"
	anchorHead  = "## Upgrade paths, top 20"
	docPath     = "docs/templates-status.md"
)

func status(t catalog.Template) string {
	if len(t.Platforms) == 0 {
		return "not checked"
	}
	for _, p := range t.Platforms {
		if p == "linux/arm64" {
			return "yes"
		}
	}
	return "amd64 only"
}

func block(templates []catalog.Template, scannedAt string) string {
	ts := append([]catalog.Template(nil), templates...)
	sort.Slice(ts, func(i, j int) bool { return ts[i].ID < ts[j].ID })
	counts := map[string]int{}
	var rows strings.Builder
	for _, t := range ts {
		s := status(t)
		counts[s]++
		plats := "-"
		if len(t.Platforms) > 0 {
			plats = strings.Join(t.Platforms, ", ")
		}
		fmt.Fprintf(&rows, "| `%s` | %s | %s |\n", t.ID, s, plats)
	}
	var b strings.Builder
	b.WriteString(beginMarker + "\n")
	b.WriteString("## Arm64 support\n\n")
	b.WriteString("Generated from the registry scan in `internal/catalog/platforms.json` (`scripts/scan-catalog-platforms`), not from the smoke run. A template is `yes` only when every image it uses lists `linux/arm64` in its manifest. `not checked` means unknown, not unsupported: the scan could not read a manifest or the image tag is set at install time.\n\n")
	if scannedAt == "" {
		scannedAt = "never"
	}
	fmt.Fprintf(&b, "Last scan: %s. **%d yes, %d amd64 only, %d not checked** of %d templates.\n\n", scannedAt, counts["yes"], counts["amd64 only"], counts["not checked"], len(ts))
	b.WriteString("| Template | arm64 | Verified platforms |\n| --- | --- | --- |\n")
	b.WriteString(rows.String())
	b.WriteString(endMarker + "\n")
	return b.String()
}

func generate(doc, blk string) (string, error) {
	start := strings.Index(doc, beginMarker)
	end := strings.Index(doc, endMarker)
	if start >= 0 && end > start {
		return doc[:start] + blk + doc[end+len(endMarker)+1:], nil
	}
	i := strings.Index(doc, anchorHead)
	if i < 0 {
		return "", fmt.Errorf("neither markers nor %q found", anchorHead)
	}
	return doc[:i] + blk + "\n" + doc[i:], nil
}

func main() {
	check := flag.Bool("check", false, "fail if the doc is stale")
	flag.Parse()
	body, err := os.ReadFile(docPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out, err := generate(string(body), block(catalog.Templates, catalog.PlatformsData().ScannedAt))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if out == string(body) {
		return
	}
	if *check {
		fmt.Fprintln(os.Stderr, docPath+" is stale; run go run ./scripts/gen-template-platforms")
		os.Exit(1)
	}
	if err := os.WriteFile(docPath, []byte(out), 0o600); err != nil { //nolint:gosec // fixed repo doc path
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
