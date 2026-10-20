package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func settingsProfileUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings profile get [flags]                                       show the server profile and what each includes
  %[1]s settings profile set minimal|standard|suite [--dry-run] [--confirm] [--restart]

The profile decides which optional subsystems start. It is saved at once and
applies at the next control plane restart (--restart asks for one now). Moving
down never deletes rows; a shorter retention needs --confirm. Root only.
`, prog)
}

func runSettingsProfile(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsProfileUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsProfileUsage(prog))
		return exitOK
	case "get":
		return runListCommand(prog, args[1:], stdout, stderr, lookupEnv, listCommandParams[apiclient.ServerProfile]{
			cmdLabel:  "settings profile get",
			jsonUsage: "print the profile as JSON to stdout and nothing else",
			usageText: fmt.Sprintf("Usage:\n  %s settings profile get [flags]\n\nFlags:\n", prog),
			fetch: func(c *Client, ctx context.Context) (apiclient.ServerProfile, error) {
				return c.GetServerProfile(ctx)
			},
			errVerb: "get the server profile",
			print:   printServerProfile,
		})
	case "set":
		return runSettingsProfileSet(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown settings profile subcommand %q\n\n", prog, args[0])
	_, _ = fmt.Fprint(stderr, settingsProfileUsage(prog))
	return exitUsage
}

func printServerProfile(out io.Writer, p apiclient.ServerProfile) {
	_, _ = fmt.Fprintf(out, "profile:     %s (from %s)\n", p.Configured, p.Source)
	if p.RestartRequired {
		_, _ = fmt.Fprintf(out, "running:     %s, restart the control plane to apply %s\n", p.Active, p.Configured)
	}
	if p.Recommended != "" {
		_, _ = fmt.Fprintf(out, "recommended: %s (this machine has little memory)\n", p.Recommended)
	}
	for _, i := range p.Profiles {
		_, _ = fmt.Fprintf(out, "  %-9s mesh %-3s plugins %-3s experimental %-2d retention %dd\n", i.ID, yesNo(i.Mesh), yesNo(i.Plugins), i.Experimental, i.RetentionDays)
	}
}

func yesNo(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func runSettingsProfileSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings profile set", "print the profile state as JSON to stdout and nothing else", stderr)
	dryRun := fs.Bool("dry-run", false, "check the move and print what it would cost, saving nothing")
	confirm := fs.Bool("confirm", false, "accept a shorter metrics and log retention")
	restart := fs.Bool("restart", false, "restart the control plane afterwards to apply it")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, settingsProfileUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprint(stderr, settingsProfileUsage(prog))
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.SetServerProfile(context.Background(), apiclient.UpdateServerProfileRequest{Profile: fs.Arg(0), Confirm: *confirm, DryRun: *dryRun})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set server profile %q: %w", fs.Arg(0), err))
	}
	if *restart && !*dryRun && res.RestartRequired {
		if _, rerr := client.RestartControlPlane(context.Background()); rerr != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("saved %s, but restarting the control plane failed: %w", res.Configured, rerr))
		}
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		switch {
		case *dryRun:
			_, _ = fmt.Fprintf(stdout, "dry run: moving to %s is allowed\n", fs.Arg(0))
		case res.RestartRequired && *restart:
			_, _ = fmt.Fprintf(stdout, "profile %s saved, control plane restarting to apply it\n", res.Configured)
		case res.RestartRequired:
			_, _ = fmt.Fprintf(stdout, "profile %s saved, restart the control plane to apply it (or rerun with --restart)\n", res.Configured)
		default:
			_, _ = fmt.Fprintf(stdout, "profile is %s\n", res.Configured)
		}
	})
}
