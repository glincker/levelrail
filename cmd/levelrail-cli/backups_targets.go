package main

import (
	"fmt"
	"io"
)

// runBackupsTargets exposes backup-targets under backups: add (create) and list.
func runBackupsTargets(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s backups targets add --type local --name NAME --path DIR [flags]\n  %[1]s backups targets list [flags]\n", prog)
		return exitUsage
	}
	switch args[0] {
	case "add":
		return runBackupTargetsCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "list":
		return runBackupTargetsList(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown backups targets subcommand %q\n", prog, args[0])
		return exitUsage
	}
}
