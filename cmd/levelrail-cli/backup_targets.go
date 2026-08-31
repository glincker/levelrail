package main

import (
	"fmt"
	"io"
)

// runBackupTargets dispatches "backup-targets <verb> [flags]" to one of
// list/get/create/update/delete: managing the connected S3-compatible
// buckets internal/api/backup_targets.go exposes (Settings -> Backup
// targets in the web UI), the same destinations "backups schedule set
// --target ID" references by ID.
func runBackupTargets(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, backupTargetsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, backupTargetsUsage(prog))
		return exitOK
	case "list":
		return runBackupTargetsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "get":
		return runBackupTargetsGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "create":
		return runBackupTargetsCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "update":
		return runBackupTargetsUpdate(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runBackupTargetsDelete(prog, args[1:], stdout, stderr, lookupEnv)
	case "test":
		return runBackupTargetsTest(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown backup-targets subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, backupTargetsUsage(prog))
		return exitUsage
	}
}

func backupTargetsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backup-targets list [flags]                                                              list connected backup targets
  %[1]s backup-targets get <id> [flags]                                                           show one backup target
  %[1]s backup-targets create --name NAME --provider PROVIDER --bucket BUCKET --access-key-id ID --secret-access-key KEY [flags]   connect a new backup target
  %[1]s backup-targets update <id> --name NAME --provider PROVIDER --bucket BUCKET [flags]        update a backup target, optionally rotating its credentials
  %[1]s backup-targets delete <id> [flags]                                                        disconnect a backup target
  %[1]s backup-targets test <id> [flags]                                                          probe a target's bucket over its stored credentials, without uploading or deleting anything

Valid --provider values: aws, r2, custom. --endpoint is required for r2 and custom (aws resolves its own default endpoint per region).

Run "%[1]s backup-targets <subcommand> -h" for a subcommand's own flags.
`, prog)
}
