package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"text/template"
)

func jpJSONString(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

type columnSpec struct {
	header string
	steps  []jpStep
}

// Spec is HEADER:.path,HEADER2:.path2 like kubectl custom-columns.
func parseColumnSpec(spec string) ([]columnSpec, error) {
	if strings.TrimSpace(spec) == "" {
		return nil, newValidationError("custom-columns needs HEADER:.path pairs")
	}
	var cols []columnSpec
	for _, part := range strings.Split(spec, ",") {
		header, path, ok := strings.Cut(part, ":")
		if !ok || strings.TrimSpace(header) == "" || strings.TrimSpace(path) == "" {
			return nil, newValidationError("custom-columns: %q is not HEADER:.path", part)
		}
		steps, err := jpParsePath(path)
		if err != nil {
			return nil, err
		}
		cols = append(cols, columnSpec{header: strings.TrimSpace(header), steps: steps})
	}
	return cols, nil
}

func writeCustomColumns(out io.Writer, spec string, v any) error {
	cols, err := parseColumnSpec(spec)
	if err != nil {
		return err
	}
	generic, err := toGenericJSON(v)
	if err != nil {
		return err
	}
	rows, ok := generic.([]any)
	if !ok {
		rows = []any{generic}
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	headers := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = c.header
	}
	_, _ = fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, row := range rows {
		cells := make([]string, len(cols))
		for i, c := range cols {
			got := jpWalk([]any{row}, c.steps)
			if len(got) == 0 {
				cells[i] = "<none>"
			} else {
				cells[i] = jpJoinResults(got)
			}
		}
		_, _ = fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	return tw.Flush()
}

// One compact JSON object per slice element; a non-list value is a single line.
func writeNDJSON(out io.Writer, v any) error {
	rows, ok := v.([]any)
	if !ok {
		rows = []any{v}
	}
	for _, row := range rows {
		if err := writeJSONLine(out, row); err != nil {
			return err
		}
	}
	return nil
}

// Runs the template once per row, one line each, over the generic JSON shape so .name matches the JSON keys.
func writeTemplate(out io.Writer, text string, v any) error {
	tpl, err := template.New("output").Option("missingkey=zero").Parse(text)
	if err != nil {
		return newValidationError("--template: %s", err)
	}
	rows, ok := v.([]any)
	if !ok {
		rows = []any{v}
	}
	for _, row := range rows {
		if err := tpl.Execute(out, row); err != nil {
			return newValidationError("--template: %s", err)
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}
	return nil
}
