package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runTemplatesCompat lists the local record of templates that could not install on this host.
func runTemplatesCompat(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "templates compat", "print the result as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s templates compat [flags]\n\nLists templates whose install failed on this host, the gap class (capability,\nDocker socket, architecture, port, kernel module, device) and what unlocks it.\nA row clears when the template installs. The record is local and never uploaded.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	rows, err := client.ListTemplateCompat(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list template compat: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, rows, func() { printTemplateCompatTable(stdout, rows) })
}

func printTemplateCompatTable(out io.Writer, rows []apiclient.TemplateCompatItem) {
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(out, "no template compat gaps recorded on this host")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TEMPLATE\tARCH\tGAP\tFAILURES\tFIX")
	for _, r := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n", r.TemplateID, r.NodeArch, r.Class, r.Failures, r.Fix)
	}
	_ = tw.Flush()
}
