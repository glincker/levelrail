package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// composeRestoreIsTTY decides whether a prompt can be answered.
var composeRestoreIsTTY = func(r io.Reader) bool {
	f, ok := r.(*os.File)
	return ok && term.IsTerminal(int(f.Fd())) //nolint:gosec // a terminal file descriptor always fits an int
}

func appsBackupsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps backups services <app> [flags]
  %[1]s apps backups schedule set <app> <service> --target ID --cron EXPR [--retain N] [--retain-days N] [--disable] [flags]
  %[1]s apps backups schedule clear <app> <service> [flags]
  %[1]s apps backups runs <app> <service> [flags]
  %[1]s apps backups run <app> <service> --target ID [flags]
  %[1]s apps backups restore <app> <service> <run> [--yes] [flags]

services lists detected database services and their schedules. schedule
clear keeps existing backup objects.

Replaces a compose database service's live data from one succeeded backup
run (postgres, mysql, mariadb). A safety dump of the current data is taken
first and the restore is abandoned if that dump fails. The command prints
a preview and asks you to type the confirm phrase; without --yes and
without a terminal it sends no restore request.

Flags:
  --yes     confirm without prompting (for scripts)
%[2]s`, prog, commonFlagsHelp)
}

func runAppsBackups(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsBackupsUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsBackupsUsage(prog))
		return exitOK
	case "services":
		return runAppsBackupsServices(prog, args[1:], stdout, stderr, lookupEnv)
	case "schedule":
		return runAppsBackupsSchedule(prog, args[1:], stdout, stderr, lookupEnv)
	case "runs":
		return runAppsBackupsRuns(prog, args[1:], stdout, stderr, lookupEnv)
	case "run":
		return runAppsBackupsRun(prog, args[1:], stdout, stderr, lookupEnv)
	case "restore":
		return runAppsBackupsRestore(prog, args[1:], stdout, stderr, lookupEnv, stdin)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown apps backups subcommand %q\n\n%s", prog, args[0], appsBackupsUsage(prog))
	return exitUsage
}

func runAppsBackupsRestore(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps backups restore", "print the started restore as JSON to stdout and nothing else", stderr)
	yes := fs.Bool("yes", false, "confirm without prompting")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsBackupsUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps backups restore", "an app name, a service name and a run id", 3)
	if !ok {
		return exitUsage
	}
	app, service, run := rest[0], rest[1], rest[2]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	// The preview changes nothing server side, so it is safe before any confirmation.
	pv, err := client.RestoreComposeDBBackup(ctx, app, service, run, apiclient.ComposeDBRestoreRequest{Preview: true})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("preview restore of %s/%s: %w", app, service, err))
	}
	_, _ = fmt.Fprintf(stderr, "Restore %s/%s (%s) from run %q, object %q, %d bytes.\nThis replaces the live database. A safety dump is taken first.\n", app, service, pv.Engine, pv.RunID, pv.ObjectKey, pv.SizeBytes)

	if !*yes {
		if !composeRestoreIsTTY(stdin) {
			return reportError(stdout, stderr, jsonOut, newValidationError("not a terminal: pass --yes to restore, nothing was sent"))
		}
		_, _ = fmt.Fprintf(stderr, "Type %q to confirm: ", pv.ConfirmPhrase)
		line, rerr := bufio.NewReader(stdin).ReadString('\n')
		if rerr != nil && line == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("read confirmation: %v", rerr))
		}
		if strings.TrimSpace(line) != pv.ConfirmPhrase {
			return reportError(stdout, stderr, jsonOut, newValidationError("confirmation does not match %q, refusing to restore", pv.ConfirmPhrase))
		}
	}

	started, err := client.RestoreComposeDBBackup(ctx, app, service, run, apiclient.ComposeDBRestoreRequest{Confirm: pv.ConfirmPhrase})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("restore %s/%s: %w", app, service, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, started, func() {
		_, _ = fmt.Fprintf(stdout, "restore of %s/%s from run %q started; safety dump run is %q\n", app, service, run, started.SafetyRunID)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
