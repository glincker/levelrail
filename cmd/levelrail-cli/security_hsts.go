package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runSecurityHSTS(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || (args[0] != "status" && args[0] != "enable" && args[0] != "disable") {
		_, _ = fmt.Fprint(stderr, securityUsage(prog))
		return exitUsage
	}
	verb := args[0]
	client, _, jsonOut, of, code, ok := securityCommand(prog, "security hsts "+verb, "print the result as JSON to stdout and nothing else", args[1:], stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	ctx := context.Background()
	var st apiclient.HSTSStatus
	var err error
	switch verb {
	case "status":
		st, err = client.GetHSTS(ctx)
	case "enable":
		st, err = client.SetHSTS(ctx, true)
	default:
		st, err = client.SetHSTS(ctx, false)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("security hsts %s: %w", verb, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, st, func() { printHSTS(stdout, st) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printHSTS(w io.Writer, st apiclient.HSTSStatus) {
	state := "off"
	if st.Enabled {
		state = "on"
	}
	_, _ = fmt.Fprintf(w, "HSTS is %s. Safe to enable: %t\n", state, st.Ready)
	for _, h := range st.Hosts {
		line := "trusted certificate"
		if !h.Verified {
			line = h.Error
		}
		_, _ = fmt.Fprintf(w, "  %s: %s\n", h.Host, line)
	}
	for _, msg := range st.Warnings {
		_, _ = fmt.Fprintf(w, "  note: %s\n", msg)
	}
}
