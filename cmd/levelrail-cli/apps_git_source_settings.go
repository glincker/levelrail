package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsGitSourceSettings sets an app's push path filters and forge status
// reporting. Only the flags given change; --paths and --paths-ignore replace
// the whole list, and an empty value clears it.
func runAppsGitSourceSettings(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps git-source settings", "print the settings as JSON to stdout and nothing else", stderr)
	var paths, ignore csvListFlag
	var report bool
	fs.Var(&paths, "paths", "deploy only when a push changes a file matching these globs (comma separated, ** supported); empty clears the filter")
	fs.Var(&ignore, "paths-ignore", "skip the deploy when every changed file matches these globs (comma separated); empty clears the filter")
	fs.BoolVar(&report, "report-status", true, "post deploy and pipeline state back to the git forge (GitHub Deployments and commit statuses)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps git-source settings <name> [flags]\n\nSets which pushes deploy by the files they changed, and whether deploy and\npipeline state is reported back to the git forge. Only the flags you pass\nchange. The server kill switch APP_GIT_STATUS_ENABLED=false turns reporting\noff for every app.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps git-source settings", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	var req apiclient.SetGitDeploySettingsRequest
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "paths":
			v := []string(paths)
			req.DeployPaths = &v
		case "paths-ignore":
			v := []string(ignore)
			req.DeployPathsIgnore = &v
		case "report-status":
			req.ReportStatus = &report
		}
	})
	if req.DeployPaths == nil && req.DeployPathsIgnore == nil && req.ReportStatus == nil {
		_, _ = fmt.Fprintf(stderr, "%s: apps git-source settings needs --paths, --paths-ignore or --report-status\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	got, err := client.SetGitDeploySettings(context.Background(), name, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set git source settings for app %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, got, func() {
		_, _ = fmt.Fprintf(stdout, "deploy_paths:        %s\n", strings.Join(got.DeployPaths, ", "))
		_, _ = fmt.Fprintf(stdout, "deploy_paths_ignore: %s\n", strings.Join(got.DeployPathsIgnore, ", "))
		_, _ = fmt.Fprintf(stdout, "report_status:       %t\n", got.ReportStatus)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// csvListFlag is a string list flag that accepts comma separated values and
// may be repeated. An empty value yields an empty (non-nil) list.
type csvListFlag []string

func (l *csvListFlag) String() string { return strings.Join(*l, ",") }

func (l *csvListFlag) Set(v string) error {
	if *l == nil {
		*l = csvListFlag{}
	}
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			*l = append(*l, part)
		}
	}
	return nil
}
