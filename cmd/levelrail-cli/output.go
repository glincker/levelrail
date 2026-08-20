package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"
	"time"
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

// printConditionsHuman prints an app's or database's current reconcile
// conditions ("apps status"/"databases status" output): current status,
// not a history log, matching what GetConditions itself returns.
func printConditionsHuman(out io.Writer, conditions []conditionResource) {
	if len(conditions) == 0 {
		_, _ = fmt.Fprintln(out, "no conditions reported yet")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TYPE\tSTATUS\tREASON\tMESSAGE\tLAST TRANSITION")
	for _, c := range conditions {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", c.Type, c.Status, c.Reason, c.Message, c.LastTransitionTime.Format(time.RFC3339))
	}
	_ = tw.Flush()
}

// printAppNetworkHuman prints "apps network" output: the live traffic
// path, container's declared port plus whatever host port Docker
// currently has bound.
func printAppNetworkHuman(out io.Writer, n networkResource) {
	_, _ = fmt.Fprintf(out, "container port:  %d\n", n.ContainerPort)
	if n.Running && n.HostPort > 0 {
		_, _ = fmt.Fprintf(out, "host port:       %d\n", n.HostPort)
	} else {
		_, _ = fmt.Fprintln(out, "host port:       (not running)")
	}
	_, _ = fmt.Fprintf(out, "running:         %t\n", n.Running)
}

// printLogEntriesHuman prints "apps logs" output: one line per entry,
// oldest first (the order handleQueryLogs' underlying store returns
// them in), timestamp and stream prefixed so stdout/stderr lines are
// distinguishable without needing --json.
func printLogEntriesHuman(out io.Writer, entries []logEntryResource) {
	if len(entries) == 0 {
		_, _ = fmt.Fprintln(out, "no log entries in range")
		return
	}
	for _, e := range entries {
		_, _ = fmt.Fprintf(out, "%s %s %s\n", e.Timestamp.Format(time.RFC3339), e.Stream, e.Message)
	}
}

// printDatabaseHuman prints one database resource in a human-readable,
// non-JSON form ("databases get" output).
func printDatabaseHuman(out io.Writer, d databaseResource) {
	_, _ = fmt.Fprintf(out, "name:     %s\n", d.Name)
	_, _ = fmt.Fprintf(out, "engine:   %s\n", d.Engine)
	_, _ = fmt.Fprintf(out, "version:  %s\n", d.Version)
	if d.NodeID != "" {
		_, _ = fmt.Fprintf(out, "node:     %s\n", d.NodeID)
	}
}

// printDatabasesTable prints a compact, aligned table of databases
// ("databases list" output), the same shape printAppsTable uses for apps.
func printDatabasesTable(out io.Writer, dbs []databaseResource) {
	if len(dbs) == 0 {
		_, _ = fmt.Fprintln(out, "no databases")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tENGINE\tVERSION\tNODE")
	for _, d := range dbs {
		node := d.NodeID
		if node == "" {
			node = "(local)"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", d.Name, d.Engine, d.Version, node)
	}
	_ = tw.Flush()
}
