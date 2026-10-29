package main

import (
	"fmt"
	"io"
	"os"
)

// Each verb needs a token ability: write:sensitive for trigger and restore-as-new, root for restore.
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
	case "health":
		return runBackupsHealth(prog, args[1:], stdout, stderr, lookupEnv)
	case "protection":
		return runBackupsProtection(prog, args[1:], stdout, stderr, lookupEnv)
	case "coverage":
		return runBackupsCoverage(prog, args[1:], stdout, stderr, lookupEnv)
	case "protect-all":
		return runBackupsProtectAll(prog, args[1:], stdout, stderr, lookupEnv)
	case "orphans":
		return runBackupsOrphans(prog, args[1:], stdout, stderr, lookupEnv)
	case "drill":
		return runBackupsDrill(prog, args[1:], stdout, stderr, lookupEnv)
	case "volumes":
		return runBackupsVolumes(prog, args[1:], stdout, stderr, lookupEnv)
	case "list-all":
		return runBackupsListAll(prog, args[1:], stdout, stderr, lookupEnv)
	case "trigger":
		return runBackupsTrigger(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runBackupsDelete(prog, args[1:], stdout, stderr, lookupEnv)
	case "download":
		return runBackupsDownload(prog, args[1:], stdout, stderr, lookupEnv)
	case "restore":
		return runBackupsRestore(prog, args[1:], stdout, stderr, lookupEnv, os.Stdin)
	case "restore-as-new":
		return runBackupsRestoreAsNew(prog, args[1:], stdout, stderr, lookupEnv)
	case "restores":
		return runBackupsRestores(prog, args[1:], stdout, stderr, lookupEnv)
	case "clone-restores":
		return runBackupsCloneRestores(prog, args[1:], stdout, stderr, lookupEnv)
	case "schedule":
		return runBackupsSchedule(prog, args[1:], stdout, stderr, lookupEnv)
	case "encryption":
		return runBackupsEncryption(prog, args[1:], stdout, stderr, lookupEnv)
	case "destinations":
		return runBackupsDestinations(prog, args[1:], stdout, stderr, lookupEnv)
	case "copies":
		return runBackupsCopies(prog, args[1:], stdout, stderr, lookupEnv)
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
  %[1]s backups health [flags]                                             every resource with last backup and last verified restore
  %[1]s backups protection [--refresh] [flags]                             bucket object lock and versioning status
  %[1]s backups coverage [--unprotected] [flags]                           which databases and volumes are protected, verified or not backed up
  %[1]s backups protect-all --target ID [--apply] [flags]                  schedule a backup for everything that has none (plan unless --apply)
  %[1]s backups orphans list|reclaim <volume> [flags]                      data kept after its database or app was deleted
  %[1]s backups drill run|list|show [flags]                                prove a restore works (scratch restore, validate, destroy)
  %[1]s backups volumes list|policy|restore <app> <volume> [flags]         app volume backups, retention and hooks, restore to a new volume or node
  %[1]s backups list-all [flags]                                           list backup history across every database and app volume
  %[1]s backups trigger <database> --target ID [flags]                 trigger a manual backup
  %[1]s backups delete <database> <backup-id> [flags]                  delete one archived backup on demand (destructive)
  %[1]s backups download <database> <backup-id> [flags]                stream a succeeded backup's own object to stdout
  %[1]s backups restore <database> --backup ID --confirm NAME [flags]   restore a database from a backup (destructive)
  %[1]s backups restore-as-new <database> --backup ID --new-name NAME [flags] restore a backup into a brand-new database (non-destructive)
  %[1]s backups restores <database> [flags]                                list restore attempt history for a database
  %[1]s backups clone-restores <database> [flags]                          list restore-as-new attempt history for a database
  %[1]s backups schedule set<database> --target ID --cron EXPR [flags] configure a recurring backup
  %[1]s backups schedule clear <database> [flags]                       remove a recurring backup
  %[1]s backups encryption get|set <destination> [on|off] [flags]       show or change database dump encryption for a destination
  %[1]s backups destinations copy show|set <destination|off> [flags]   show or change the second destination backups are copied to
  %[1]s backups copies list [--limit N] [flags]                         list each backup's copy status and lag
  %[1]s backups verify <database> --backup ID [flags]                   verify a backup is intact (no live restore)
  %[1]s backups verifications <database> --backup ID [flags]            list past verification attempts for a backup

Run "%[1]s backups <subcommand> -h" for a subcommand's own flags.
`, prog)
}
