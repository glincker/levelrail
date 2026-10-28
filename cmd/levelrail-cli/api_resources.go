package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
)

type apiResourceRow struct {
	Kind       string `json:"kind"`
	Version    int    `json:"version"`
	SpecFields int    `json:"specFields"`
	Required   string `json:"required"`
}

func (d *schemaDoc) apiResources() []apiResourceRow {
	rows := make([]apiResourceRow, 0)
	for _, kind := range d.kinds() {
		node := d.kindNode(kind)
		props, _ := d.properties(d.resolve(node))
		spec := props["spec"]
		specProps, _ := d.properties(spec)
		required := "no"
		if req, _ := node["required"].([]any); len(req) > 0 {
			required = "yes"
		}
		rows = append(rows, apiResourceRow{Kind: kind, Version: 1, SpecFields: len(specProps), Required: required})
	}
	return rows
}

func runAPIResources(prog string, args []string, stdout, stderr io.Writer, _ func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" api-resources", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "print the result as JSON to stdout and nothing else")
	outputFlag, queryFlag := bindOutputQueryFlags(fs)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s api-resources [flags]\n\nLists every resource kind a document can declare, from the embedded schema. Works offline.\n\nFlags:\n  --json            print the result as JSON, nothing else\n  --output string   output format\n  --query string    JMESPath filter\n", prog)
	}
	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	format, err := resolveOutputFormat(*jsonOut, *outputFlag)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, err)
		return exitValidation
	}
	doc, err := loadSchemaDoc()
	if err != nil {
		return reportError(stdout, stderr, *jsonOut, err)
	}
	rows := doc.apiResources()
	if err := renderResult(stdout, format, *queryFlag, rows, func() {
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "KIND\tVERSION\tSPEC FIELDS\tSPEC REQUIRED")
		for _, r := range rows {
			_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\t%s\n", r.Kind, r.Version, r.SpecFields, r.Required)
		}
		_ = tw.Flush()
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
