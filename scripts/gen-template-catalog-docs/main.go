// Command gen-template-catalog-docs regenerates the per-category tables in
// docs/template-catalog.md from internal/catalog.Templates, the same data
// GET /api/v1/service-templates serves. It only rewrites the block between
// the BEGIN/END markers; everything else in the doc (intro prose, "not
// verified" notes, "see also") is left untouched.
//
// Usage (from the repo root): go run ./scripts/gen-template-catalog-docs
// Pass -check to exit non-zero instead of writing when the doc is stale.
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
	beginMarker = "<!-- BEGIN GENERATED CATALOG TABLE -->"
	endMarker   = "<!-- END GENERATED CATALOG TABLE -->"
)

// categoryOrder is the display order: alphabetical, not count-descending,
// so the order doesn't reshuffle every time a category gains an entry.
var categoryOrder = []string{
	"AI",
	"Analytics",
	"Applications",
	"Automation",
	"Communication",
	"Dashboard",
	"Database Tools",
	"Developer Tools",
	"Finance",
	"Infrastructure",
	"IoT",
	"Media",
	"Monitoring",
	"Productivity",
	"Security",
	"Starter Kits",
	"Storage",
}

func escapeCell(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

func anchor(category string) string {
	return strings.ToLower(strings.ReplaceAll(category, " ", "-"))
}

// generateBlock renders every category's table, grouped and sorted by
// Template.Name within each group. A category present in the data but
// missing from categoryOrder is appended under "Other" rather than
// silently dropped, so a new, unlisted category can't go uncounted.
func generateBlock(templates []catalog.Template) string {
	byCategory := map[string][]catalog.Template{}
	for _, t := range templates {
		byCategory[t.Category] = append(byCategory[t.Category], t)
	}
	order := append([]string{}, categoryOrder...)
	for cat := range byCategory {
		found := false
		for _, c := range order {
			if c == cat {
				found = true
				break
			}
		}
		if !found {
			order = append(order, cat)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Generated from `internal/catalog.Templates` (%d entries as of this build). ", len(templates))
	b.WriteString("Run `go run ./scripts/gen-template-catalog-docs` after changing a `templates_*.go` file to refresh this section.\n\n")

	b.WriteString("| Category | Templates |\n| --- | --- |\n")
	for _, cat := range order {
		rows := byCategory[cat]
		if len(rows) == 0 {
			continue
		}
		fmt.Fprintf(&b, "| [%s](#%s) | %d |\n", cat, anchor(cat), len(rows))
	}
	b.WriteString("\n")

	for _, cat := range order {
		rows := byCategory[cat]
		if len(rows) == 0 {
			continue
		}
		// Case-insensitive: several real names (vLLM, n8n, copyparty,
		// llama.cpp) start lowercase and shouldn't sort after every
		// capitalized one.
		sort.Slice(rows, func(i, j int) bool {
			return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
		})
		fmt.Fprintf(&b, "## %s\n\n", cat)
		b.WriteString("| Template | Slogan | ID | Notes |\n| --- | --- | --- | --- |\n")
		for _, t := range rows {
			name := escapeCell(t.Name)
			if t.DocumentationURL != "" {
				name = fmt.Sprintf("[%s](%s)", name, t.DocumentationURL)
			}
			notes := ""
			if t.RequiresGPU {
				notes = "Requires GPU"
			}
			fmt.Fprintf(&b, "| %s | %s | `%s` | %s |\n", name, escapeCell(t.Slogan), t.ID, notes)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func generate(doc string, templates []catalog.Template) (string, error) {
	start := strings.Index(doc, beginMarker)
	end := strings.Index(doc, endMarker)
	if start == -1 || end == -1 || end < start {
		return "", fmt.Errorf("markers %q / %q not found in doc", beginMarker, endMarker)
	}
	before := doc[:start+len(beginMarker)]
	after := doc[end:]
	return before + "\n\n" + generateBlock(templates) + "\n" + after, nil
}

func main() {
	check := flag.Bool("check", false, "fail if the doc is stale instead of writing it")
	docPath := flag.String("doc", "docs/template-catalog.md", "doc to update")
	flag.Parse()

	docBytes, err := os.ReadFile(*docPath) //nolint:gosec // developer tool
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	doc := string(docBytes)

	got, err := generate(doc, catalog.Templates)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if got == doc {
		fmt.Printf("%s up to date (%d templates)\n", *docPath, len(catalog.Templates))
		return
	}
	if *check {
		fmt.Fprintf(os.Stderr, "%s is stale: run go run ./scripts/gen-template-catalog-docs\n", *docPath)
		os.Exit(1)
	}
	if err := os.WriteFile(*docPath, []byte(got), 0o644); err != nil { //nolint:gosec // docs file
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%s updated (%d templates)\n", *docPath, len(catalog.Templates))
}
