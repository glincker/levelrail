package main

import (
	"fmt"
	"io"
	"os"
)

// runAppVolumeBackups dispatches "app-volume-backups <verb> <app>
// <volume> [flags]" to one of list/trigger/restore/schedule/verify/
// verifications, mirroring runBackups' exact verb set (backups.go) for
// an app service's own named Docker volume instead of a managed
// database. <volume> is the logical name declared in that app's
// app.yaml volumes: field (see "levelrail-cli apps get <app>" to list
// them), not the resolved, platform-prefixed Docker volume name.
func runAppVolumeBackups(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appVolumeBackupsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appVolumeBackupsUsage(prog))
		return exitOK
	case "list":
		return runAppVolumeBackupsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "trigger":
		return runAppVolumeBackupsTrigger(prog, args[1:], stdout, stderr, lookupEnv)
	case "restore":
		return runAppVolumeBackupsRestore(prog, args[1:], stdout, stderr, lookupEnv, os.Stdin)
	case "schedule":
		return runAppVolumeBackupsSchedule(prog, args[1:], stdout, stderr, lookupEnv)
	case "verify":
		return runAppVolumeBackupsVerify(prog, args[1:], stdout, stderr, lookupEnv)
	case "verifications":
		return runAppVolumeBackupsVerifications(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown app-volume-backups subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appVolumeBackupsUsage(prog))
		return exitUsage
	}
}

func appVolumeBackupsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s app-volume-backups list <app> <volume> [flags]                                  list backup history for an app's named volume
  %[1]s app-volume-backups trigger <app> <volume> --target ID [flags]                 trigger a manual backup
  %[1]s app-volume-backups restore <app> <volume> --backup ID --confirm APP/VOLUME [flags]   restore a volume from a backup (destructive)
  %[1]s app-volume-backups schedule set <app> <volume> --target ID --cron EXPR [flags]   configure a recurring backup
  %[1]s app-volume-backups schedule clear <app> <volume> [flags]                       remove a recurring backup
  %[1]s app-volume-backups verify <app> <volume> --backup ID [flags]                   verify a backup is intact (no live restore)
  %[1]s app-volume-backups verifications <app> <volume> --backup ID [flags]            list past verification attempts for a backup

<volume> is a logical volume name declared in <app>'s app.yaml (see
"%[1]s apps get <app>" to list an app's volumes).

Run "%[1]s app-volume-backups <subcommand> -h" for a subcommand's own flags.
`, prog)
}
