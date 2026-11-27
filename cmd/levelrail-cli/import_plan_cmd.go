package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

var composeFileNames = []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"}

// runImportPlanCmd is "import plan <file|dir>": the same planner as the bare
// import form, but it refuses --deploy so nothing can be created.
func runImportPlanCmd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var rest []string
	path := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--deploy" || a == "--apply":
			_, _ = fmt.Fprintf(stderr, "%s import plan never creates anything; run %s import -f FILE --deploy after reviewing the plan\n", prog, prog)
			return 2
		case a == "-h" || a == "--help":
			_, _ = fmt.Fprint(stderr, importUsage(prog))
			return 0
		case strings.HasPrefix(a, "-"):
			rest = append(rest, a)
			if !strings.Contains(a, "=") && flagTakesValue(a) && i+1 < len(args) {
				i++
				rest = append(rest, args[i])
			}
		case path == "":
			path = a
		default:
			rest = append(rest, a)
		}
	}
	if path == "" {
		_, _ = fmt.Fprintf(stderr, "usage: %s import plan <compose.yaml|dir> [--json]\n", prog)
		return 2
	}
	file, err := resolveComposePath(path)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	return runImport(prog, append([]string{"-f", file}, rest...), stdout, stderr, lookupEnv)
}

func flagTakesValue(a string) bool {
	switch strings.TrimLeft(a, "-") {
	case "json", "j":
		return false
	}
	return true
}

func resolveComposePath(p string) (string, error) {
	if p == "-" {
		return p, nil
	}
	st, err := os.Stat(p)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", p, err)
	}
	if !st.IsDir() {
		return p, nil
	}
	for _, n := range composeFileNames {
		c := filepath.Join(p, n)
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("no compose file in %s (looked for %s)", p, strings.Join(composeFileNames, ", "))
}

// printImportReport lists every dropped or changed field, worst first, so the
// operator sees the cost of the import before anything is deployed.
func printImportReport(out io.Writer, report []apiclient.ImportFinding) {
	if len(report) == 0 {
		return
	}
	_, _ = fmt.Fprintf(out, "\nimport report (%d findings):\n", len(report))
	for _, f := range report {
		_, _ = fmt.Fprintf(out, "  [%s] %s: %s\n", f.Severity, f.Path, f.Reason)
		if f.Fix != "" {
			_, _ = fmt.Fprintf(out, "      fix: %s\n", f.Fix)
		}
	}
}

// printImportStack lists what a compose import would create, what it cannot
// carry over and which names clash. Secret values are never printed.
func printImportStack(out io.Writer, p apiclient.ImportPlan) {
	if len(p.Resources) > 0 {
		_, _ = fmt.Fprintln(out, "\nwill create:")
		for _, r := range p.Resources {
			_, _ = fmt.Fprintf(out, "  %-9s %s  %s\n", r.Kind, r.Name, r.Detail)
		}
	}
	if len(p.SecretRefs) > 0 {
		_, _ = fmt.Fprintf(out, "\nsecret refs (values are not shown): %s\n", strings.Join(p.SecretRefs, ", "))
	}
	if len(p.Unsupported) > 0 {
		_, _ = fmt.Fprintln(out, "\nnot carried over:")
		for _, u := range p.Unsupported {
			who := u.Feature
			if u.Service != "" {
				who = u.Service + " " + u.Feature
			}
			_, _ = fmt.Fprintf(out, "  - %s: %s. Instead: %s\n", who, u.Reason, u.Alternative)
		}
	}
	if len(p.Conflicts) > 0 {
		_, _ = fmt.Fprintln(out, "\nname conflicts (already exist):")
		for _, c := range p.Conflicts {
			_, _ = fmt.Fprintf(out, "  - %s %s\n", c.Kind, c.Name)
		}
	}
}
