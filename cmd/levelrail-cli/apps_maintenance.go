package main

import (
	"fmt"
	"io"
	"time"
)

// runAppsMaintenance dispatches "apps maintenance show|set|clear <app>". It
// reuses the app-wide handlers of "domains maintenance --app", so the server
// stays the one place that enforces bounds, ability and all-or-none.
func runAppsMaintenance(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsMaintenanceUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsMaintenanceUsage(prog))
		return exitOK
	case "show", "set", "clear":
		return runAppsMaintenanceVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps maintenance subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsMaintenanceUsage(prog))
		return exitUsage
	}
}

func appsMaintenanceUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps maintenance show <app> [flags]    show maintenance state of every domain of the app
  %[1]s apps maintenance set <app> [flags]     enable maintenance mode on every domain, all or none
  %[1]s apps maintenance clear <app> [flags]   disable maintenance mode on every domain

Visitors get a 503 page with Retry-After while the container keeps running.
"set" needs deploy access; "show" needs read access. Pass --for, --until or
--max-duration so it cannot be left on by accident. For one domain, use
"%[1]s domains maintenance".

Run "%[1]s apps maintenance <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsMaintenanceVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	cmd := "apps maintenance " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, cmd, domainsMaintenanceJSONUsage, stderr)
	var opts maintenanceSetOptions
	if verb == "set" {
		opts = maintenanceSetFlags(fs)
	}
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s %s <app> [flags]\n\nFlags:\n", prog, cmd)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, cmd, "an app name", 1)
	if !ok {
		return exitUsage
	}
	app := rest[0]

	switch verb {
	case "set":
		req, err := opts.request(time.Now())
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %s: %v\n", prog, cmd, err)
			return exitUsage
		}
		return appMaintenanceSet(stdout, stderr, jsonOut, of, apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv), app, req)
	case "clear":
		return appMaintenanceClear(stdout, stderr, jsonOut, of, apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv), app)
	default:
		return appMaintenanceGet(stdout, stderr, jsonOut, of, apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv), app)
	}
}
