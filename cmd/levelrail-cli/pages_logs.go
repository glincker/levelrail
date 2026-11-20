package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runPagesLogs prints a Pages deploy's build log, or follows it live.
func runPagesLogs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "pages logs", "print one JSON object per line instead of text", stderr)
	var follow bool
	fs.BoolVar(&follow, "follow", false, "stream the log live until Ctrl+C or the deploy finishes")
	fs.BoolVar(&follow, "f", false, "shorthand for --follow")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() < 1 || fs.NArg() > 2 {
		_, _ = fmt.Fprintf(stderr, "%s: pages logs requires a site name and optionally a deploy id\n", prog)
		return exitUsage
	}
	name := fs.Arg(0)
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deployID := fs.Arg(1)
	if deployID == "" {
		deploys, err := client.ListStaticSiteDeploys(ctx, name)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list deploys of %q: %w", name, err))
		}
		for _, d := range deploys {
			if deployID == "" || d.StartedAt > latestStart(deploys, deployID) {
				deployID = d.ID
			}
		}
		if deployID == "" {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("site %q has no deploys", name))
		}
	}

	if follow {
		err := client.StreamStaticSiteDeployLog(ctx, name, deployID, func(e logStreamEntry) error {
			if jsonOut {
				return writeJSONLine(stdout, e)
			}
			_, err := fmt.Fprintf(stdout, "%s %s\n", e.Stream, e.Line)
			return err
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("stream log of %s/%s: %w", name, deployID, err))
		}
		return exitOK
	}
	log, err := client.GetStaticSiteDeployLog(ctx, name, deployID, 0)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("log of %s/%s: %w", name, deployID, err))
	}
	step := ""
	for _, l := range log.Lines {
		if jsonOut {
			_ = writeJSONLine(stdout, l)
			continue
		}
		if l.Step != step {
			step = l.Step
			_, _ = fmt.Fprintf(stdout, "== %s\n", step)
		}
		_, _ = fmt.Fprintf(stdout, "%s %s\n", l.Stream, l.Line)
	}
	return exitOK
}

// latestStart returns the start time of the deploy with the given id.
func latestStart(deploys []apiclient.StaticSiteDeploy, id string) string {
	for _, d := range deploys {
		if d.ID == id {
			return d.StartedAt
		}
	}
	return ""
}
