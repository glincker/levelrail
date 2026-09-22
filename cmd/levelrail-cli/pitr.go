package main

import (
	"fmt"
	"io"
	"os"
)

// runPITR dispatches "pitr <verb> <database> [flags]": point-in-time
// restore (internal/api/pitr.go), Postgres only today. Deliberately no
// "pitr restores" list subcommand: GET .../pitr-restores exists for
// scripting, but only the dashboard reads it today, the same gap
// docs/managing-databases.md already documents for GET .../clone-restores.
func runPITR(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, pitrUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, pitrUsage(prog))
		return exitOK
	case "enable":
		return runPITREnable(prog, args[1:], stdout, stderr, lookupEnv)
	case "disable":
		return runPITRDisable(prog, args[1:], stdout, stderr, lookupEnv)
	case "status":
		return runPITRStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "base-backups":
		return runPITRBaseBackups(prog, args[1:], stdout, stderr, lookupEnv)
	case "restore":
		return runPITRRestore(prog, args[1:], stdout, stderr, lookupEnv, os.Stdin)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown pitr subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, pitrUsage(prog))
		return exitUsage
	}
}

func pitrUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s pitr enable <database> [flags]                                             turn on continuous WAL archiving going forward
  %[1]s pitr disable <database> [flags]                                            turn WAL archiving back off
  %[1]s pitr status <database> [flags]                                             show whether PITR is enabled and the current recoverable window
  %[1]s pitr base-backups list <database> [flags]                                  list physical base backup attempts
  %[1]s pitr base-backups trigger <database> --target ID [flags]                   trigger a manual physical base backup
  %[1]s pitr restore <database> --base-backup ID --target-time RFC3339 [--confirm NAME] [flags]
                                                                                     restore a database to an exact point in time (destructive)

Point-in-time restore only ever covers the time range from when
"pitr enable" was called, forward. Nothing from before that moment is
recoverable by timestamp; use "backups restore" against an ordinary
backup for anything older.

Run "%[1]s pitr <subcommand> -h" for a subcommand's own flags.
`, prog)
}
