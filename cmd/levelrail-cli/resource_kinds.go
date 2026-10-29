package main

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// resourceKind maps a typed kind word onto its v2 route segment.
type resourceKind struct {
	Path     string
	Singular string
}

var resourceKinds = map[string]resourceKind{
	"app": {"apps", "app"}, "apps": {"apps", "app"},
	"db": {"databases", "database"}, "database": {"databases", "database"}, "databases": {"databases", "database"},
	"node": {"nodes", "node"}, "nodes": {"nodes", "node"},
}

func resolveResourceKind(word string) (resourceKind, error) {
	if k, ok := resourceKinds[strings.ToLower(word)]; ok {
		return k, nil
	}
	known := make([]string, 0, len(resourceKinds))
	for w := range resourceKinds {
		known = append(known, w)
	}
	sort.Strings(known)
	return resourceKind{}, newValidationError("unknown resource kind %q, want one of: %s", word, strings.Join(known, ", "))
}

// expandShortFlags rewrites kubectl style -o and -l to the long flags the shared parser knows.
func expandShortFlags(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		switch {
		case a == "-o":
			out = append(out, "--output")
		case strings.HasPrefix(a, "-o="):
			out = append(out, "--output="+strings.TrimPrefix(a, "-o="))
		case a == "-n":
			out = append(out, "--namespace")
		case strings.HasPrefix(a, "-n="):
			out = append(out, "--namespace="+strings.TrimPrefix(a, "-n="))
		case a == "-l":
			out = append(out, "--selector")
		case strings.HasPrefix(a, "-l="):
			out = append(out, "--selector="+strings.TrimPrefix(a, "-l="))
		default:
			out = append(out, a)
		}
	}
	return out
}

// editorRunner is swapped in tests; the real one hands the terminal to $EDITOR.
var editorRunner = func(editor, path string) error {
	cmd := exec.Command("sh", "-c", editor+` "$1"`, "edit", path) //nolint:gosec // the operator chose this editor command
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor %q: %w", editor, err)
	}
	return nil
}

func pickEditor(lookupEnv func(string) (string, bool)) string {
	for _, k := range []string{"VISUAL", "EDITOR"} {
		if v, ok := lookupEnv(k); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return "vi"
}
