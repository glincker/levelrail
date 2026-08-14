package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"
)

// Exit codes. Distinct on purpose (per the CLI's own design brief: "real,
// distinct exit codes... with the actual error message on stderr either
// way"), so a script or an AI agent driving this CLI can branch on
// *why* a call failed without parsing stderr text.
const (
	exitOK         = 0
	exitUsage      = 1 // unknown command, missing positional argument, bad flag syntax
	exitValidation = 2 // well-formed flags/file, but the request they describe is invalid
	exitNetwork    = 3 // could not reach the API at all (connection refused, DNS, timeout)
	exitAPIError   = 4 // the API was reached and returned a non-2xx response
)

// exitCodeForError classifies err into one of the exit codes above.
// errors.As unwraps through fmt.Errorf("...: %w", ...) wrapping, so a
// *apiError still classifies correctly however many layers of context
// client.go's own do() and its callers added on the way up.
func exitCodeForError(err error) int {
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		return exitAPIError
	}
	var valErr *validationError
	if errors.As(err, &valErr) {
		return exitValidation
	}
	return exitNetwork
}

// validationError marks a failure as "the request itself is invalid"
// (a bad flag combination, an app.yaml this CLI can't yet turn into a
// request), as opposed to a network or API failure: the same
// distinction flyctl/railway/vercel's own CLIs make between a usage
// mistake and something failing downstream.
type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }

func newValidationError(format string, args ...any) error {
	return &validationError{msg: fmt.Sprintf(format, args...)}
}

// writeJSONValue marshals v as indented JSON to out, the CLI's --json
// mode contract: this is the only thing written to stdout in that mode.
func writeJSONValue(out io.Writer, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode json output: %w", err)
	}
	_, err = fmt.Fprintln(out, string(data))
	return err
}

// writeJSONError writes {"error": "..."} to out: --json mode's error
// shape, deliberately the same {"error": "..."} field name
// internal/api/respond.go's own apiError already uses, so a caller
// parsing this CLI's JSON output and the control plane's own JSON error
// responses can use one code path for both.
func writeJSONError(out io.Writer, err error) error {
	return writeJSONValue(out, map[string]string{"error": err.Error()})
}

// printAppHuman prints one app resource in a human-readable, non-JSON
// form.
func printAppHuman(out io.Writer, a appResource) {
	_, _ = fmt.Fprintf(out, "name:     %s\n", a.Name)
	_, _ = fmt.Fprintf(out, "image:    %s\n", a.Image)
	_, _ = fmt.Fprintf(out, "port:     %d\n", a.Port)
	if len(a.Domains) > 0 {
		_, _ = fmt.Fprintf(out, "domains:  %v\n", a.Domains)
	}
	if a.NodeID != "" {
		_, _ = fmt.Fprintf(out, "node:     %s\n", a.NodeID)
	}
}

// printAppsTable prints a compact, aligned table of apps: list output's
// human-readable form.
func printAppsTable(out io.Writer, apps []appResource) {
	if len(apps) == 0 {
		_, _ = fmt.Fprintln(out, "no apps")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tIMAGE\tPORT\tNODE")
	for _, a := range apps {
		node := a.NodeID
		if node == "" {
			node = "(local)"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", a.Name, a.Image, a.Port, node)
	}
	_ = tw.Flush()
}
