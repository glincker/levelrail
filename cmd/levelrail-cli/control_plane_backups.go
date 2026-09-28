package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type controlPlaneBackup = apiclient.ControlPlaneBackup

// runControlPlaneBackups dispatches "control-plane-backups
// list|create|download|delete", the CLI face of /api/v1/system/backups.
func runControlPlaneBackups(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, controlPlaneBackupsUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "help-dr":
		_, _ = fmt.Fprint(stdout, controlPlaneDRUsage(prog))
		return exitOK
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, controlPlaneBackupsUsage(prog))
		return exitOK
	case "list":
		rest := args[1:]
		for _, a := range rest {
			if a == "--offbox" || a == "-offbox" {
				return listOffbox(prog, rest, stdout, stderr, lookupEnv)
			}
		}
		return runControlPlaneBackupsList(prog, rest, stdout, stderr, lookupEnv)
	case "schedule", "run-now", "drill", "escrow", "keys":
		return runControlPlaneDR(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	case "create":
		return runControlPlaneBackupsCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "download":
		return runControlPlaneBackupsDownload(prog, args[1:], stdout, stderr, lookupEnv)
	case "verify":
		return runControlPlaneBackupsVerify(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runControlPlaneBackupsDelete(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown control-plane-backups subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, controlPlaneBackupsUsage(prog))
		return exitUsage
	}
}

func controlPlaneBackupsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s control-plane-backups list [flags]                    list control plane database snapshots, newest first
  %[1]s control-plane-backups create [flags]                  take a snapshot now (manual snapshots are never auto-deleted)
  %[1]s control-plane-backups download <name> [--out FILE]    save a snapshot (default: raw bytes to stdout)
  %[1]s control-plane-backups verify <name> [flags]           check a snapshot's checksum, integrity and schema (exit 1 if not ok)
  %[1]s control-plane-backups delete <name> [flags]           delete one snapshot
  %[1]s control-plane-backups list --offbox                   list encrypted off-box backups (see "control-plane-backups help-dr")

Snapshots hold the control plane database only, never the master key.
Restore offline with "levelrail restore-db <file>" on the server, or restore an
encrypted off-box backup with "levelrail restore --from s3://... --identity FILE".

Disaster recovery subcommands: schedule show|set, run-now, drill run|status, escrow, keys generate.
Run "%[1]s control-plane-backups help-dr" for details.
All subcommands need a root-scoped token.

Run "%[1]s control-plane-backups <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func controlPlaneBackupFlagsUsage(prog, verb, tail string) string {
	return fmt.Sprintf(`Usage:
  %[1]s control-plane-backups %[5]s [flags]

%[6]s

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL, verb, tail)
}

func runControlPlaneBackupsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "control-plane-backups list", "print snapshots as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, controlPlaneBackupFlagsUsage(prog, "list", "Lists control plane database snapshots, newest first."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	list, err := client.ListControlPlaneBackups(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list control plane backups: %w", err))
	}
	if list == nil {
		list = []controlPlaneBackup{}
	}
	if err := renderResult(stdout, of.Format, of.Query, list, func() { printControlPlaneBackupsTable(stdout, list) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printControlPlaneBackupsTable(out io.Writer, list []controlPlaneBackup) {
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "no control plane backups")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tSIZE\tCREATED\tSHA256")
	for _, b := range list {
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\t%s\n", b.Name, b.SizeBytes, b.CreatedAt, b.SHA256)
	}
	_ = tw.Flush()
}

func runControlPlaneBackupsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "control-plane-backups create", "print the new snapshot as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, controlPlaneBackupFlagsUsage(prog, "create", "Takes a consistent snapshot of the control plane database now. It never\nincludes the master key."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	b, err := client.CreateControlPlaneBackup(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create control plane backup: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, b, func() {
		_, _ = fmt.Fprintf(stdout, "created %s (%d bytes, sha256 %s)\n", b.Name, b.SizeBytes, b.SHA256)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runControlPlaneBackupsDownload(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "control-plane-backups download", "not applicable, the response body is always the raw snapshot file", stderr)
	var outFile string
	fs.StringVar(&outFile, "out", "", "write the snapshot to this file instead of stdout")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, controlPlaneBackupFlagsUsage(prog, "download <name>", "Saves one snapshot. With --out FILE it is written there (mode 0600);\notherwise the raw SQLite bytes go to stdout."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "control-plane-backups download", "a backup name", 1)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	data, err := client.DownloadControlPlaneBackup(context.Background(), rest[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("download control plane backup %q: %w", rest[0], err))
	}
	if outFile == "" {
		_, _ = stdout.Write(data)
		return exitOK
	}
	if err := os.WriteFile(outFile, data, 0o600); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("write %s: %w", outFile, err))
	}
	_, _ = fmt.Fprintf(stderr, "saved %s (%d bytes) to %s\n", rest[0], len(data), outFile)
	return exitOK
}

func runControlPlaneBackupsVerify(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "control-plane-backups verify", "print the verification result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, controlPlaneBackupFlagsUsage(prog, "verify <name>", "Re-checks a snapshot's sha256 against the recorded one, runs SQLite's\nintegrity_check and confirms its schema is not newer than this binary.\nNothing is restored. Exits 1 when any check fails."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "control-plane-backups verify", "a backup name", 1)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.VerifyControlPlaneBackup(context.Background(), rest[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("verify control plane backup %q: %w", rest[0], err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() { printControlPlaneBackupVerification(stdout, res) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if !res.OK {
		return exitCheckFailed
	}
	return exitOK
}

func printControlPlaneBackupVerification(out io.Writer, res apiclient.ControlPlaneBackupVerification) {
	for _, c := range res.Checks {
		status := "ok"
		if !c.OK {
			status = "FAIL"
		}
		_, _ = fmt.Fprintf(out, "%-4s  %s: %s\n", status, c.Name, c.Detail)
	}
	if res.OK {
		_, _ = fmt.Fprintf(out, "backup %s verified\n", res.Name)
	} else {
		_, _ = fmt.Fprintf(out, "backup %s FAILED verification\n", res.Name)
	}
}

func runControlPlaneBackupsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "control-plane-backups delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, controlPlaneBackupFlagsUsage(prog, "delete <name>", "Permanently deletes one snapshot file. Cannot be undone."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "control-plane-backups delete", "a backup name", 1)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.DeleteControlPlaneBackup(context.Background(), rest[0]); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete control plane backup %q: %w", rest[0], err))
	}
	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "backup %q deleted\n", rest[0])
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
