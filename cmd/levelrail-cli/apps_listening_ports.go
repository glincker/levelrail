package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
)

func appsListeningPortsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps listening-ports <app> [flags]

Shows whether the running container listens on the configured port. The
check reads the container's socket table, so it needs exec enabled for the
app; otherwise the reason is shown and the verdict is unknown.
%[2]s`, prog, commonFlagsHelp)
}

func runAppsListeningPorts(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps listening-ports", "print the response as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsListeningPortsUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps listening-ports", "an app name", 1)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.AppListeningPorts(context.Background(), rest[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("listening ports for %s: %w", rest[0], err))
	}
	if rerr := renderResult(stdout, of.Format, of.Query, res, func() {
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "CONFIGURED\tVERDICT\tLISTENING\tNOTE")
		ports := make([]string, 0, len(res.Listening))
		for _, p := range res.Listening {
			ports = append(ports, strconv.Itoa(p))
		}
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", res.Port, res.Verdict, strings.Join(ports, ","), res.Reason)
		_ = tw.Flush()
	}); rerr != nil {
		_, _ = fmt.Fprintln(stderr, rerr)
		return exitCodeForError(rerr)
	}
	return exitOK
}
