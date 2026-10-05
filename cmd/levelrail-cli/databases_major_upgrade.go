package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func confirmDatabaseName(name, flag, warning string, stdin io.Reader, stderr io.Writer) error {
	if flag != "" {
		if flag != name {
			return newValidationError("--confirm %q does not match database name %q", flag, name)
		}
		return nil
	}
	_, _ = fmt.Fprintf(stderr, "%s\nType %q to confirm: ", warning, name)
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && line == "" {
		return newValidationError("read confirmation: %v", err)
	}
	if strings.TrimSpace(line) != name {
		return newValidationError("confirmation does not match database name %q", name)
	}
	return nil
}

// runDatabasesMajorUpgrade implements "databases major-upgrade <name> --version V".
func runDatabasesMajorUpgrade(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases major-upgrade", "print the started upgrade as JSON to stdout and nothing else", stderr)
	var version, confirm string
	fs.StringVar(&version, "version", "", "target major version, e.g. 17 (required)")
	fs.StringVar(&confirm, "confirm", "", "must equal the database name to skip the interactive prompt")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, majorUpgradeUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "databases major-upgrade", "database name")
	if !ok {
		return exitUsage
	}
	if version == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--version is required"))
	}
	warn := fmt.Sprintf("Database %q will be OFFLINE while it is dumped and restored on version %s. The old data is kept as a rollback snapshot.", name, version)
	if err := confirmDatabaseName(name, confirm, warn, os.Stdin, stderr); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	started, err := client.StartMajorUpgrade(context.Background(), name, version)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("major upgrade of database %q to %q: %w", name, version, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, started, func() {
		_, _ = fmt.Fprintf(stdout, "major upgrade %q of database %q from %s to %s started; follow it with \"%s databases major-upgrades %s\"\n", started.ID, name, started.FromVersion, started.ToVersion, prog, name)
	})
}

// runDatabasesMajorUpgrades implements "databases major-upgrades <name>".
func runDatabasesMajorUpgrades(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases major-upgrades", "print the upgrades as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, majorUpgradeUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "databases major-upgrades", "database name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	list, err := client.ListMajorUpgrades(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list major upgrades for database %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, list, func() { printMajorUpgrades(stdout, list) })
}

func printMajorUpgrades(out io.Writer, list []apiclient.MajorUpgradeResource) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tFROM\tTO\tSTATUS\tPHASE\tSNAPSHOT\tERROR")
	for _, u := range list {
		snap, errText := u.SnapshotVolume, u.Error
		if snap == "" {
			snap = "-"
		}
		if errText == "" {
			errText = "-"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", u.ID, u.FromVersion, u.ToVersion, u.Status, u.Phase, snap, errText)
	}
	_ = tw.Flush()
}

// runDatabasesMajorUpgradeRollback implements "databases major-upgrade-rollback <name> <id>".
func runDatabasesMajorUpgradeRollback(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases major-upgrade-rollback", "print the upgrade as JSON to stdout and nothing else", stderr)
	var confirm string
	fs.StringVar(&confirm, "confirm", "", "must equal the database name to skip the interactive prompt")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, majorUpgradeUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: databases major-upgrade-rollback requires a database name and an upgrade id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name, id := rest[0], rest[1]
	warn := fmt.Sprintf("Rolling back restores database %q to its pre-upgrade data. Every write since the upgrade is lost.", name)
	if err := confirmDatabaseName(name, confirm, warn, os.Stdin, stderr); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	started, err := client.RollbackMajorUpgrade(context.Background(), name, id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("roll back upgrade %q of database %q: %w", id, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, started, func() {
		_, _ = fmt.Fprintf(stdout, "rollback of upgrade %q started; follow it with \"%s databases major-upgrades %s\"\n", id, prog, name)
	})
}

// runDatabasesMajorUpgradeDiscard implements "databases major-upgrade-discard <name> <id>".
func runDatabasesMajorUpgradeDiscard(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases major-upgrade-discard", "print nothing on success", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, majorUpgradeUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: databases major-upgrade-discard requires a database name and an upgrade id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.DiscardMajorUpgradeSnapshot(context.Background(), rest[0], rest[1]); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("discard snapshot of upgrade %q: %w", rest[1], err))
	}
	if !jsonOut {
		_, _ = fmt.Fprintf(stdout, "rollback snapshot of upgrade %q discarded\n", rest[1])
	}
	return exitOK
}

func majorUpgradeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases major-upgrade <name> --version V [--confirm NAME] [flags]
  %[1]s databases major-upgrades <name> [flags]
  %[1]s databases major-upgrade-rollback <name> <id> [--confirm NAME] [flags]
  %[1]s databases major-upgrade-discard <name> <id> [flags]

Upgrades a Postgres database across a major version by dump and restore. The
database is offline for the duration. The old data is copied to a snapshot
volume first and the dump is taken from that copy; any failure after the live
volume is wiped restores the snapshot and the old version automatically.
After a success the snapshot is kept so major-upgrade-rollback can undo it
(losing writes since the upgrade) until major-upgrade-discard frees the disk.

Refused while point-in-time restore is enabled (its base backups cannot be
replayed on another major) and for downgrades. Needs free disk for the
snapshot (the data volume's size again) plus the dump file in the data dir.
`, prog)
}
