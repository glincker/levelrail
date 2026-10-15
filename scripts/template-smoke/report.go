package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

// writeReports writes the JSON and markdown reports where asked, and the
// markdown to stdout when no file was asked for.
func writeReports(results []result, o options, stdout io.Writer) error {
	if o.jsonOut != "" {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return fmt.Errorf("encode json: %w", err)
		}
		if err := os.WriteFile(o.jsonOut, append(data, '\n'), 0o600); err != nil {
			return fmt.Errorf("write json: %w", err)
		}
	}
	md := markdown(results)
	if o.mdOut != "" {
		if err := os.WriteFile(o.mdOut, []byte(md), 0o600); err != nil {
			return fmt.Errorf("write markdown: %w", err)
		}
	}
	if o.jsonOut == "" && o.mdOut == "" {
		_, _ = io.WriteString(stdout, md)
	}
	return nil
}

func markdown(results []result) string {
	counts := map[string]int{}
	for _, r := range results {
		counts[r.Status]++
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Template smoke report\n\n%d pass, %d fail, %d skip of %d templates.\n\n",
		counts[statusPass], counts[statusFail], counts[statusSkip], len(results))
	b.WriteString("| Template | Category | Result | Detail | Seconds |\n|---|---|---|---|---|\n")
	for _, r := range results {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %.0f |\n", r.ID, r.Category, r.Status, cell(r.Summary), r.Seconds)
	}
	for _, r := range results {
		if r.Status != statusFail {
			continue
		}
		fmt.Fprintf(&b, "\n## %s\n\n%s\n", r.ID, r.Summary)
		for _, s := range r.Services {
			if len(s.Logs) == 0 {
				continue
			}
			fmt.Fprintf(&b, "\n%s (%s, exit %d) last lines:\n\n```\n%s\n```\n", s.Name, s.State, s.ExitCode, strings.Join(s.Logs, "\n"))
		}
	}
	return b.String()
}

func cell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "|", "\\|"), "\n", " ")
}

var imageLine = regexp.MustCompile(`(?m)^\s*image:\s*["']?([^"'\s#]+)`)

// printPlan lists each template's images and their local size, so a run
// can be scoped to what is already pulled.
func printPlan(ctx context.Context, templates []catalog.Template, out io.Writer) {
	for _, t := range templates {
		var parts []string
		local := true
		for _, m := range imageLine.FindAllStringSubmatch(t.Compose, -1) {
			if size, ok := localImageSize(ctx, m[1]); ok {
				parts = append(parts, fmt.Sprintf("%s(%.2fGB)", m[1], float64(size)/bytesPerGB))
				continue
			}
			local = false
			parts = append(parts, m[1]+"(missing)")
		}
		_, _ = fmt.Fprintf(out, "%s\t%t\t%s\n", t.ID, local, strings.Join(parts, " "))
	}
}
