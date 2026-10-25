package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/stacktemplates"
)

func stackTemplatesUsage(prog string) string {
	return fmt.Sprintf("Usage:\n  %s stack templates\n\nLists the bundled starter stacks. Copy one out with '%s stack init <template>'.\n", prog, prog)
}

func stackInitUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s stack init <template> [--dir DIR] [--set key=value]... [--force]

Copies a bundled starter stack into DIR (default ./<name>) with variables filled in.
Variables: name (stack, project and app name), domain, image. Run '%[1]s stack templates' to list them.
A non-empty DIR is refused unless --force is set. Nothing is sent to the control plane.
`, prog)
}

func stackTemplates(prog string, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		_, _ = fmt.Fprint(stderr, stackTemplatesUsage(prog))
		if args[0] == "-h" || args[0] == "--help" {
			return exitOK
		}
		return exitUsage
	}
	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TEMPLATE\tDESCRIPTION")
	for _, t := range stacktemplates.List() {
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", t.Name, t.Description)
	}
	_ = tw.Flush()
	return exitOK
}

func stackInit(prog string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(prog+" stack init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, stackInitUsage(prog)) }
	var sets stringListFlag
	dir := fs.String("dir", "", "target directory (default ./<name>)")
	force := fs.Bool("force", false, "write into a non-empty directory")
	fs.Var(&sets, "set", "variable as key=value (repeatable)")
	// Accept the template before or after the flags.
	var pos []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return exitOK
			}
			return exitUsage
		}
		rest := fs.Args()
		if len(rest) == 0 {
			break
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
	if len(pos) != 1 {
		_, _ = fmt.Fprint(stderr, stackInitUsage(prog))
		return exitUsage
	}
	tpl, ok := stacktemplates.Find(pos[0])
	if !ok {
		_, _ = fmt.Fprintf(stderr, "%s stack init: unknown template %q (run '%s stack templates')\n", prog, pos[0], prog)
		return exitUsage
	}
	vars := map[string]string{}
	for _, kv := range sets {
		k, v, found := strings.Cut(kv, "=")
		if !found || k == "" {
			_, _ = fmt.Fprintf(stderr, "%s stack init: --set wants key=value, got %q\n", prog, kv)
			return exitUsage
		}
		vars[k] = v
	}
	files, err := tpl.Render(vars)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s stack init: %v\n", prog, err)
		return exitUsage
	}
	target := *dir
	if target == "" {
		target = tpl.Name
		if n := vars["name"]; n != "" {
			target = n
		}
	}
	if entries, err := os.ReadDir(target); err == nil && len(entries) > 0 && !*force {
		_, _ = fmt.Fprintf(stderr, "%s stack init: %s is not empty (use --force to write into it)\n", prog, target)
		return exitUsage
	}
	if err := os.MkdirAll(target, 0o750); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s stack init: %v\n", prog, err)
		return exitValidation
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(target, n), files[n], 0o600); err != nil {
			_, _ = fmt.Fprintf(stderr, "%s stack init: %v\n", prog, err)
			return exitValidation
		}
	}
	stackName := vars["name"]
	if stackName == "" {
		stackName = tpl.Name
	}
	_, _ = fmt.Fprintf(stdout, "wrote %d files to %s\nnext: read %s/SECRETS.md, then '%s stack plan %s -f %s'\n", len(names), target, target, prog, stackName, target)
	return exitOK
}
