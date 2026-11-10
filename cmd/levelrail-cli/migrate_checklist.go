package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/migrationplan"
)

// runMigrateChecklist implements "migrate checklist list|show|step", the
// per-app cutover checklists with the same output flags as other API verbs.
func runMigrateChecklist(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, migrateChecklistUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, migrateChecklistUsage(prog))
		return exitOK
	case "list", "show":
		return runChecklistShow(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	case "step":
		return runChecklistStep(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown migrate checklist subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, migrateChecklistUsage(prog))
		return exitUsage
	}
}

func migrateChecklistUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s migrate checklist list [flags]               every imported app's cutover progress
  %[1]s migrate checklist show <app> [flags]         one app's steps
  %[1]s migrate checklist step <app> <step> --done|--reset [--note TEXT] [--confirm]
                                                    [--database NAME] [--volume NAME=SF:SB:TF:TB] [flags]

Steps: %[2]s. --reset reopens the step and every step after it.
Completing a step needs write:sensitive.
`, prog, strings.Join(migrationplan.Steps, ", "))
}

func runChecklistShow(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "migrate checklist "+verb, "print the checklists as JSON", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, migrateChecklistUsage(prog)+"\nFlags:\n")
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	want, label := 0, "no arguments"
	if verb == "show" {
		want, label = 1, "exactly one app name"
	}
	rest, ok := requireArgs(fs, stderr, prog, "migrate checklist "+verb, label, want)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	if verb == "show" {
		c, err := client.GetMigrationChecklist(ctx, rest[0])
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("checklist of %q: %w", rest[0], err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, c, func() { printMigrationChecklist(stdout, c, true) })
	}
	list, err := client.ListMigrationChecklists(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list checklists: %w", err))
	}
	if list == nil {
		list = []apiclient.MigrationChecklist{}
	}
	return writeScheduledTaskResult(stdout, stderr, of, list, func() {
		if len(list) == 0 {
			_, _ = fmt.Fprintln(stdout, "No imported app to track.")
		}
		for _, c := range list {
			printMigrationChecklist(stdout, c, false)
		}
	})
}

func printMigrationChecklist(w io.Writer, c apiclient.MigrationChecklist, steps bool) {
	next := c.Current
	if next == "" {
		next = "finished"
	}
	_, _ = fmt.Fprintf(w, "%s: %d/%d, next %s\n", c.App, c.Progress, c.Total, next)
	if !steps {
		return
	}
	for _, s := range c.Steps {
		mark := " "
		if s.Status == migrationplan.StatusDone {
			mark = "x"
		}
		_, _ = fmt.Fprintf(w, "  [%s] %s\n", mark, s.ID)
	}
}

func runChecklistStep(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "migrate checklist step", "print the updated checklist as JSON", stderr)
	var req apiclient.MigrationStepRequest
	var dbs, vols listFlag
	done := fs.Bool("done", false, "mark the step complete")
	reset := fs.Bool("reset", false, "reopen the step and every step after it")
	fs.StringVar(&req.Note, "note", "", "note stored with the step")
	fs.BoolVar(&req.Confirm, "confirm", false, "confirm a step the control plane cannot observe (old_stopped), or accept a failed DNS check")
	fs.Var(&dbs, "database", "data_verified: a migrated database to check, repeatable")
	fs.Var(&vols, "volume", "data_verified: NAME=SRC_FILES:SRC_BYTES:DST_FILES:DST_BYTES, repeatable")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, migrateChecklistUsage(prog)+"\nFlags:\n")
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "migrate checklist step", "an app name and a step", 2)
	if !ok {
		return exitUsage
	}
	if *done == *reset {
		return reportError(stdout, stderr, jsonOut, newValidationError("pass exactly one of --done or --reset"))
	}
	req.Action = "complete"
	if *reset {
		req.Action = "reset"
	}
	req.Databases = dbs
	for _, v := range vols {
		st, err := parseVolumeStat(v)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("%v", err))
		}
		req.Volumes = append(req.Volumes, st)
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	c, err := client.StepMigrationChecklist(context.Background(), rest[0], rest[1], req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("step %s of %q: %w", rest[1], rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, c, func() { printMigrationChecklist(stdout, c, false) })
}
