package main

import (
	"context"
	"fmt"
	"io"
)

// runUpgrade implements "upgrade": run the read-only preflight, take a control
// plane backup, and print the command that performs the upgrade. It never
// upgrades by itself.
func runUpgrade(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "upgrade", "print the preflight as JSON to stdout and nothing else", stderr)
	noBackup := fs.Bool("no-backup", false, "skip the automatic control plane backup")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s upgrade [flags]\n\nChecks the running version against the latest release, runs the preflight\n(release signature, Docker Engine, free disk, backup), takes a control plane\nbackup, and prints the command that upgrades. It never upgrades by itself.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	pre, err := client.GetUpdatePreflight(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("upgrade preflight: %w", err))
	}
	backupNote := "skipped (--no-backup)"
	if !*noBackup && !pre.Blocked {
		if b, err := client.CreateControlPlaneBackup(ctx); err != nil {
			backupNote = "failed: " + err.Error()
		} else {
			backupNote = "taken: " + b.Name
		}
	} else if pre.Blocked {
		backupNote = "skipped (preflight blocked)"
	}
	if err := renderResult(stdout, of.Format, of.Query, pre, func() { printUpgradeHuman(stdout, pre, backupNote) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if pre.Blocked {
		return exitCheckFailed
	}
	return exitOK
}

func printUpgradeHuman(out io.Writer, p updatePreflight, backupNote string) {
	_, _ = fmt.Fprintf(out, "running version:  %s\n", p.CurrentVersion)
	if p.LatestVersion == nil {
		_, _ = fmt.Fprintln(out, "latest release:   unknown")
	} else {
		_, _ = fmt.Fprintf(out, "latest release:   %s\n", *p.LatestVersion)
	}
	for _, c := range p.Checks {
		_, _ = fmt.Fprintf(out, "[%s] %s: %s\n", c.Status, c.Name, c.Message)
	}
	_, _ = fmt.Fprintf(out, "control plane backup: %s\n", backupNote)
	switch {
	case p.Blocked:
		_, _ = fmt.Fprintln(out, "\nUpgrade blocked: fix the failing checks above first.")
	case !p.UpdateAvailable:
		_, _ = fmt.Fprintln(out, "\nAlready up to date.")
	default:
		_, _ = fmt.Fprintf(out, "\nUpgrade with:\n  %s\n\nRoll back with:\n  %s\n", p.UpgradeCommand, p.RollbackCommand)
	}
}
