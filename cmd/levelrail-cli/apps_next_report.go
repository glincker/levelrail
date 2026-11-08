package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func appsNextAdvisorUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps next-advisor <app> [flags]

Checks the saved config of a Next.js app (cache, skew protection, build memory)
and lists what to fix. No build is needed.
%[2]s`, prog, commonFlagsHelp)
}

func appsDeploysBuildReportUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploys build-report <app> <deploy-id> [flags]

Shows the Next.js route table (static, ISR or dynamic, with sizes) and build
warnings parsed from one deploy attempt's build log.
%[2]s`, prog, commonFlagsHelp)
}

func runAppsNextAdvisor(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps next-advisor", "print the response as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsNextAdvisorUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps next-advisor", "an app name", 1)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.NextAdvisor(context.Background(), rest[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("next advisor for %s: %w", rest[0], apiclient.NotSupported("the Next.js advisor", err)))
	}
	if rerr := renderResult(stdout, of.Format, of.Query, res, func() {
		if !res.Detected {
			_, _ = fmt.Fprintln(stdout, "Next.js not detected for this app.")
			return
		}
		if len(res.Findings) == 0 {
			_, _ = fmt.Fprintln(stdout, "No findings.")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "SEVERITY\tID\tTITLE\tFIX")
		for _, f := range res.Findings {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", f.Severity, f.ID, f.Title, f.Fix)
		}
		_ = tw.Flush()
	}); rerr != nil {
		_, _ = fmt.Fprintln(stderr, rerr)
		return exitCodeForError(rerr)
	}
	return exitOK
}

func runAppsDeploysBuildReport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploys build-report", "print the response as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeploysBuildReportUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps deploys build-report", "an app name and a deploy id", 2)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.NextBuildReport(context.Background(), rest[0], rest[1])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("build report for %s %s: %w", rest[0], rest[1], apiclient.NotSupported("Next.js build reports", err)))
	}
	if rerr := renderResult(stdout, of.Format, of.Query, res, func() {
		if !res.Detected {
			_, _ = fmt.Fprintln(stdout, "No Next.js build output found in this deploy's log.")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ROUTE\tKIND\tSIZE_KB\tFIRST_LOAD_KB")
		for _, r := range res.Routes {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%.1f\t%.1f\n", r.Path, r.Kind, r.SizeKB, r.FirstLoadKB)
		}
		_ = tw.Flush()
		if res.SharedFirstLoad != nil {
			_, _ = fmt.Fprintf(stdout, "First load JS shared by all: %.1f kB\n", *res.SharedFirstLoad)
		}
		for _, w := range res.Warnings {
			_, _ = fmt.Fprintf(stdout, "warning: %s\n", w)
		}
	}); rerr != nil {
		_, _ = fmt.Fprintln(stderr, rerr)
		return exitCodeForError(rerr)
	}
	return exitOK
}
