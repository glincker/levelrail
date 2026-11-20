package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/GLINCKER/levelrail/internal/pages"
)

// runPagesImport converts existing hosting config files in a folder to
// _redirects and _headers. It only writes files when --write is given.
func runPagesImport(prog string, args []string, stdout, stderr io.Writer) int {
	dir := ""
	write := false
	for _, a := range args {
		switch {
		case a == "--write":
			write = true
		case a == "-h" || a == "--help":
			_, _ = fmt.Fprintf(stdout, "Usage: %s pages import <dir> [--write]\n\nReads redirect and header rules from rules files in dir, prints the converted _redirects and _headers and a report of unsupported rules. With --write the two files are saved in dir.\n", prog)
			return exitOK
		case dir == "" && len(a) > 0 && a[0] != '-':
			dir = a
		default:
			_, _ = fmt.Fprintf(stderr, "%s: unexpected argument %q\n", prog, a)
			return exitUsage
		}
	}
	if dir == "" {
		_, _ = fmt.Fprintf(stderr, "Usage: %s pages import <dir> [--write]\n", prog)
		return exitUsage
	}
	res, err := pages.ImportDir(dir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
		return exitUsage
	}
	if len(res.Sources) == 0 {
		_, _ = fmt.Fprintf(stdout, "No redirect or header rules found in %s\n", dir)
		return exitOK
	}
	redirects, headers := res.Render()
	for _, s := range res.Sources {
		_, _ = fmt.Fprintf(stdout, "read %s\n", s)
	}
	_, _ = fmt.Fprintf(stdout, "\n_redirects\n%s\n_headers\n%s", redirects, headers)
	if len(res.Unsupported) > 0 {
		_, _ = fmt.Fprintf(stdout, "\nUnsupported (%d), not converted:\n", len(res.Unsupported))
		for _, u := range res.Unsupported {
			_, _ = fmt.Fprintf(stdout, "  %s\n", u)
		}
	}
	if !write {
		_, _ = fmt.Fprintln(stdout, "\nDry run. Pass --write to save these files.")
		return exitOK
	}
	for name, body := range map[string]string{"_redirects": redirects, "_headers": headers} {
		if body == "" {
			continue
		}
		target := filepath.Join(dir, name)
		if old, err := os.ReadFile(target); err == nil { //nolint:gosec // path is the user-chosen dir
			// Keep what was there so a bad import never loses hand-written rules.
			if err := os.WriteFile(target+".bak", old, 0o644); err != nil { //nolint:gosec // site files are public
				_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
				return exitUsage
			}
			_, _ = fmt.Fprintf(stdout, "backed up existing %s to %s.bak\n", target, target)
		}
		if err := os.WriteFile(target, []byte(body), 0o644); err != nil { //nolint:gosec // site files are public
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
			return exitUsage
		}
		_, _ = fmt.Fprintf(stdout, "wrote %s\n", filepath.Join(dir, name))
	}
	return exitOK
}
