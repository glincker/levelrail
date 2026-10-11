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
	list := fs.Bool("list", false, "list the last 5 releases with a schema compatibility verdict")
	channel := fs.String("channel", "", "with --list: stable, beta or all (default: the configured channel)")
	planVersion := fs.String("rollback-plan", "", "preview a rollback to `version` (read-only; applying happens on the host)")
	history := fs.Bool("history", false, "list recorded control plane upgrades, rollbacks and installs (add --json for JSON)")
	ack := fs.String("ack", "", "acknowledge the recorded upgrade `id`")
	apply := fs.Bool("apply", false, "download, verify, back up, swap and health-check the new release, rolling back automatically on failure")
	applyTo := fs.String("version", "", "with --apply or --plan: release `tag` to install (default: latest on the channel)")
	applyAck := fs.String("ack-breaking", "", "with --apply: comma-separated breaking-change `ids` you acknowledge")
	planOnly := fs.Bool("plan", false, "show the breaking changes between the running version and the target, change nothing")
	attempts := fs.Bool("attempts", false, "list recorded self-upgrade attempts with their step timelines")
	noWatch := fs.Bool("no-wait", false, "with --apply: return once the upgrade has started instead of following it")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s upgrade [flags]\n\nChecks the running version against the latest release, runs the preflight\n(release signature, Docker Engine, free disk, backup), takes a control plane\nbackup, and prints the command that upgrades. It never upgrades by itself.\n\n--history shows every recorded upgrade and --ack <id> acknowledges one;\n--plan shows breaking changes and --apply upgrades with automatic rollback;\n--attempts lists self-upgrade runs; --list shows recent releases; --rollback-plan <version> previews returning to\none. Rolling back is applied on the host with `sudo <control plane binary> rollback`.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	if *apply || *planOnly {
		return runUpgradeApply(ctx, client, upgradeApplyOptions{target: *applyTo, ack: *applyAck, plan: *planOnly && !*apply, watch: !*noWatch}, jsonOut, of, stdout, stderr)
	}
	if *attempts {
		return runUpgradeAttempts(ctx, client, jsonOut, of, stdout, stderr)
	}
	if *history {
		return runUpgradeHistory(ctx, client, jsonOut, of, stdout, stderr)
	}
	if *ack != "" {
		return runUpgradeAck(ctx, client, *ack, jsonOut, of, stdout, stderr)
	}
	if *list {
		return runUpgradeList(ctx, client, *channel, jsonOut, of, stdout, stderr)
	}
	if *planVersion != "" {
		return runUpgradeRollbackPlan(ctx, client, *planVersion, jsonOut, of, stdout, stderr)
	}

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
	for _, c := range p.Checks {
		if c.Code == "release_verifier" && c.Status == "warn" && p.CosignCommand != "" {
			_, _ = fmt.Fprintf(out, "\nInstall cosign to verify release signatures too:\n  %s\n", p.CosignCommand)
		}
	}
	switch {
	case p.Blocked:
		_, _ = fmt.Fprintln(out, "\nUpgrade blocked: fix the failing checks above first.")
	case !p.UpdateAvailable:
		_, _ = fmt.Fprintln(out, "\nAlready up to date.")
	default:
		_, _ = fmt.Fprintf(out, "\nUpgrade with:\n  %s\n\nRoll back with:\n  %s\n", p.UpgradeCommand, p.RollbackCommand)
	}
}
