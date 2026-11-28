package main

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/glincker/levelrail/internal/fielddesc"
)

// runFields implements "fields [resource]": it lists the registered form field descriptors
// locally, so docs and tools can discover them without a server.
func runFields(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "fields", "print the descriptors as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, fieldsUsage(prog)) }
	// The resource name comes first, as in the other verbs; flags follow.
	var positional []string
	flagArgs := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positional, flagArgs = args[:1], args[1:]
	}
	_, _, _, jsonOut, of, exitCode, ok := parseAPIFlags(fs, flagArgs, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	_ = lookupEnv

	if len(positional) == 0 {
		names := fielddesc.Names()
		if err := renderResult(stdout, of.Format, of.Query, names, func() {
			for _, n := range names {
				_, _ = fmt.Fprintln(stdout, n)
			}
		}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitCodeForError(err)
		}
		return exitOK
	}
	res, found := fielddesc.Get(positional[0])
	if !found {
		return reportError(stdout, stderr, jsonOut, newValidationError("unknown resource %q, run \"%s fields\" to list them", positional[0], prog))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() { printFieldsTable(stdout, res) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printFieldsTable(out io.Writer, res fielddesc.Resource) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KEY\tTYPE\tREQUIRED\tSHOWN WHEN\tFLAG\tDESCRIPTION")
	for _, f := range res.Fields {
		flag, req, typ := "-", "no", string(f.Type)
		if f.Flag != "" {
			flag = "--" + f.Flag
		}
		if f.Required {
			req = "yes"
		}
		if f.Secret {
			typ += " (write only)"
		}
		when := "always"
		if len(f.ShowWhen) > 0 {
			parts := make([]string, 0, len(f.ShowWhen))
			for _, c := range f.ShowWhen {
				parts = append(parts, c.Field+"="+strings.Join(c.Equals, "|"))
			}
			when = strings.Join(parts, " ")
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", f.Key, typ, req, when, flag, f.Doc)
	}
	_ = tw.Flush()
}

func fieldsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s fields [flags]             list the resources that have field descriptors
  %[1]s fields RESOURCE [flags]    list a resource's form fields, types, CLI flags and visibility rules

The same registry drives the dashboard forms, the CLI flags and the API at
GET /api/v1/field-descriptors/RESOURCE. Use -o json for tooling.
`, prog)
}
