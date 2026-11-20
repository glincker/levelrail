package main

import (
	"context"
	"fmt"
	"io"
)

func runBackupTargetsDrill(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "backup-targets drill", "print the drill result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupTargetsDrillUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "backup-targets drill", "backup target id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	res, err := client.DrillBackupTarget(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("drill backup target %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		if res.OK {
			_, _ = fmt.Fprintf(stdout, "backup target %q: wrote and restored a %d byte sample (upload %d ms, download %d ms), cleaned up: %t\n", id, res.Bytes, res.UploadMs, res.DownloadMs, res.CleanedUp)
			return
		}
		_, _ = fmt.Fprintf(stdout, "backup target %q: drill failed at %s: %s\n", id, res.Stage, res.Error)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if !res.OK {
		return exitCheckFailed
	}
	return exitOK
}

func backupTargetsDrillUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backup-targets drill <id> [flags]

Writes a small random sample object to the target, downloads it, compares
the bytes and deletes it, proving backups can be written and restored.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the drill result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}
