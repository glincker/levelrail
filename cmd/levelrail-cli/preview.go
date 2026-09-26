package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/diskspace"
)

// runPreview dispatches "preview <verb>": the CLI counterpart of the
// /api/v1/apps/{name}/preview routes.
func runPreview(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, previewUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, previewUsage(prog))
		return exitOK
	case "status", "enable", "disable", "capture", "prune":
		return runPreviewVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown preview subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, previewUsage(prog))
		return exitUsage
	}
}

func previewUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s preview status <app> [flags]                            show the app's deploy preview settings, storage and latest result
  %[1]s preview enable <app> [--mode metadata|screenshot] [--path /] [--wait-ms N] [flags]
                                                                turn deploy previews on (default mode: screenshot)
  %[1]s preview disable <app> [flags]                           turn deploy previews off

Modes: metadata reads the page's title and og:image with one small request (no browser, costs
about nothing); screenshot runs a browser container for about 10 s per deploy and pulls a
~143 MB image once.
  %[1]s preview capture <app> [flags]                           recapture the current release now
  %[1]s preview prune <app> [--all] [flags]                     delete old previews now (--all deletes every preview of the app)

Previews need the server to run with APP_PREVIEW_ENABLED=true (the default). New apps use the
server's default mode, metadata unless APP_PREVIEW_DEFAULT_MODE says otherwise.
Run "%[1]s preview <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runPreviewVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "preview " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	var (
		path   = fs.String("path", "", "path to capture, an absolute path on the app (default /)")
		mode   = fs.String("mode", "", "preview mode for enable: metadata or screenshot (default screenshot)")
		waitMS = fs.Int("wait-ms", -1, "extra milliseconds to let the page settle before the screenshot")
		all    = fs.Bool("all", false, "delete every preview of the app, including the current release's")
	)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s %s <app> [flags]\n\nFlags:\n", prog, label)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, label, "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	ctx := context.Background()
	var (
		res apiclient.PreviewStatus
		err error
	)
	switch verb {
	case "status":
		res, err = client.GetPreview(ctx, name)
	case "enable", "disable":
		req := apiclient.PreviewSettingsRequest{}
		on := verb == "enable"
		req.Enabled = &on
		if on && *mode != "" {
			req.Mode = mode
		}
		if *path != "" {
			req.Path = path
		}
		if *waitMS >= 0 {
			req.WaitMS = waitMS
		}
		res, err = client.SetPreview(ctx, name, req)
	case "capture":
		res, err = client.CapturePreview(ctx, name)
	default:
		pruned, perr := client.PrunePreview(ctx, name, *all)
		if perr != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s for app %q: %w", label, name, perr))
		}
		return writeScheduledTaskResult(stdout, stderr, of, pruned, func() {
			_, _ = fmt.Fprintf(stdout, "removed %d previews, freed %s\n", pruned.Removed, diskspace.HumanBytes(pruned.FreedBytes))
		})
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s for app %q: %w", label, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printPreviewHuman(stdout, res) })
}

func printPreviewHuman(out io.Writer, s apiclient.PreviewStatus) {
	state := s.Mode
	if state == "" {
		state = "off"
		if s.Enabled {
			state = "on"
		}
	}
	_, _ = fmt.Fprintf(out, "app:        %s\n", s.App)
	_, _ = fmt.Fprintf(out, "previews:   %s (path %s, wait %d ms)\n", state, s.Path, s.WaitMS)
	if !s.ServerEnabled {
		_, _ = fmt.Fprintln(out, "server:     previews are switched off (APP_PREVIEW_ENABLED is not true)")
	}
	if s.Capturing {
		_, _ = fmt.Fprintln(out, "capture:    in progress")
	}
	_, _ = fmt.Fprintf(out, "storage:    %d previews, %s (all apps: %s of %d MB budget)\n", s.Storage.AppCount, diskspace.HumanBytes(s.Storage.AppBytes), diskspace.HumanBytes(s.Storage.TotalBytes), s.MaxTotalMB)
	_, _ = fmt.Fprintf(out, "retention:  last %d per app plus production, %d days\n", s.KeepPerApp, s.TTLDays)
	if l := s.Latest; l != nil {
		line := fmt.Sprintf("%s (%s)", l.Status, l.DeploymentID)
		if l.Source != "" && l.Status == "ok" {
			line = fmt.Sprintf("%s, source %s (%s)", l.Status, l.Source, l.DeploymentID)
		}
		if l.Reason != "" {
			line = fmt.Sprintf("%s: %s %s", line, l.Reason, l.Detail)
		}
		_, _ = fmt.Fprintf(out, "latest:     %s\n", line)
	}
}
