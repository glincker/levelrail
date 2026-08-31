package main

import (
	"fmt"
	"io"
	"os"
)

// runBackups dispatches "backups <verb> <database> [flags]" to one of
// list/trigger/restore/restore-as-new. All routes are requireAbility-gated
// (internal/api/backups.go, internal/api/restore.go, internal/api/
// database_clone_restore.go), not session-only, so unlike "auth"/"tokens"
// these use the normal bearer token --token/APP_API_TOKEN/credentials-file
// resolution every other command in this CLI uses, provided the token
// actually carries the ability the route needs (write:sensitive for
// trigger and restore-as-new, root for restore, see backups_restore.go's
// own doc comment on why restore's gate is root specifically and
// backups_restore_as_new.go's own doc comment on why restore-as-new isn't).
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
	case "restore-as-new":
		return runBackupsRestoreAsNew(prog, args[1:], stdout, stderr, lookupEnv)
	case "schedule":
		return runBackupsSchedule(prog, args[1:], stdout, stderr, lookupEnv)
	case "verify":
		return runBackupsVerify(prog, args[1:], stdout, stderr, lookupEnv)
	case "verifications":
		return runBackupsVerifications(prog, args[1:], stdout, stderr, lookupEnv)
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
  %[1]s backups restore-as-new <database> --backup ID --new-name NAME [flags] restore a backup into a brand-new database (non-destructive)
  %[1]s backups schedule set <database> --target ID --cron EXPR [flags] configure a recurring backup
  %[1]s backups schedule clear <database> [flags]                       remove a recurring backup
  %[1]s backups verify <database> --backup ID [flags]                   verify a backup is intact (no live restore)
  %[1]s backups verifications <database> --backup ID [flags]            list past verification attempts for a backup

Run "%[1]s backups <subcommand> -h" for a subcommand's own flags.
`, prog)
}
