package main

import (
	"fmt"
	"io"
	"os"
)

// runBackups dispatches "backups <verb> <database> [flags]" to one of
// list/trigger/restore. All three routes are requireAbility-gated
// (internal/api/backups.go, internal/api/restore.go), not
// session-only, so unlike "auth"/"tokens" these use the normal bearer
// token --token/APP_API_TOKEN/credentials-file resolution every other
// command in this CLI uses, provided the token actually carries the
// ability the route needs (write:sensitive for trigger, root for
// restore, see backups_restore.go's own doc comment on why restore's
// gate is root specifically).
func runBackups(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, backupsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, backupsUsage(prog))
		return exitOK
	case "list":
		return runBackupsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "trigger":
		return runBackupsTrigger(prog, args[1:], stdout, stderr, lookupEnv)
	case "restore":
		return runBackupsRestore(prog, args[1:], stdout, stderr, lookupEnv, os.Stdin)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown backups subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, backupsUsage(prog))
		return exitUsage
	}
}

func backupsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backups list <database> [flags]                                    list backup history for a database
  %[1]s backups trigger <database> --target ID [flags]                 trigger a manual backup
  %[1]s backups restore <database> --backup ID --confirm NAME [flags]   restore a database from a backup (destructive)

Run "%[1]s backups <subcommand> -h" for a subcommand's own flags.
`, prog)
}
