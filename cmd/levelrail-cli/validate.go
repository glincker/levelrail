package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/iac"
)

func validateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s validate [FILE|DIR|-] [--var NAME=VALUE] [--var-file PATH] [--json]

Runs the parse, schema and semantic checks that apply runs, locally. No
login, no network. Exit 0 when valid, 1 when any problem is found.
Placeholders without a value are checked with a stand-in value.
`, prog)
}

type validateReport struct {
	Valid     bool        `json:"valid"`
	Resources int         `json:"resources"`
	Issues    []iac.Issue `json:"issues"`
}

// runValidate needs no API client: it shares the parser and builder the
// control plane uses for apply, so a pass here is a pass there.
func runValidate(prog string, args []string, stdout, stderr io.Writer, _ func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, validateUsage(prog)) }
	vars := stringMapFlag{}
	var varFiles stringListFlag
	jsonOut := fs.Bool("json", false, "print the result as JSON")
	fs.Var(vars, "var", "NAME=VALUE for a ${{ env.NAME }} placeholder")
	fs.Var(&varFiles, "var-file", "file of KEY=VALUE lines for placeholders")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 1 {
		_, _ = fmt.Fprint(stderr, validateUsage(prog))
		return exitUsage
	}
	target := "."
	if fs.NArg() == 1 {
		target = fs.Arg(0)
	}
	files, err := readIaCFiles([]string{target})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return iacExitError
	}
	fromFiles := map[string]string{}
	for _, p := range varFiles {
		m, err := readVarFile(p)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return iacExitError
		}
		for k, v := range m {
			fromFiles[k] = v
		}
	}
	values := map[string]string{}
	for _, name := range placeholderNames(files) {
		switch {
		case vars[name] != "":
			values[name] = vars[name]
		case fromFiles[name] != "":
			values[name] = fromFiles[name]
		default:
			values[name] = "placeholder"
		}
	}
	srcs := make([]iac.Source, 0, len(files))
	for _, f := range files {
		srcs = append(srcs, iac.Source{Name: f.Name, Data: []byte(f.Content)})
	}
	docs, issues := iac.ParseDocuments(srcs)
	var built []*iac.Resource
	if len(issues) == 0 {
		var more []iac.Issue
		built, more = iac.Build(docs, iac.Options{Vars: values})
		issues = append(issues, more...)
	}
	report := validateReport{Valid: len(issues) == 0, Resources: len(built), Issues: issues}
	if report.Issues == nil {
		report.Issues = []iac.Issue{}
	}
	if *jsonOut {
		if err := writeJSONValue(stdout, report); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return iacExitError
		}
	} else if report.Valid {
		_, _ = fmt.Fprintf(stdout, "valid: %d resource(s) in %d file(s)\n", report.Resources, len(files))
	} else {
		for _, i := range issues {
			_, _ = fmt.Fprintln(stderr, i.String())
		}
		_, _ = fmt.Fprintf(stderr, "invalid: %d problem(s)\n", len(issues))
	}
	if !report.Valid {
		return iacExitError
	}
	return exitOK
}
