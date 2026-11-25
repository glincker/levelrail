package main

import (
	"context"
	"fmt"
	"io"
)

// pinMark renders the pin state for list tables.
func pinMark(pinned bool) string {
	if pinned {
		return "yes"
	}
	return "-"
}

func runBackupsPin(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), pin bool) int {
	verb := "unpin"
	if pin {
		verb = "pin"
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "backups "+verb, "print {\"pinned\": <bool>} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, backupsPinUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest := fs.Args()
	if len(rest) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: backups %s requires a database name and a backup id\n\n", prog, verb)
		fs.Usage()
		return exitUsage
	}
	name, backupID := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.SetBackupPinned(context.Background(), name, backupID, pin); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s backup %q of database %q: %w", verb, backupID, name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"pinned": pin}, func() {
		if pin {
			_, _ = fmt.Fprintf(stdout, "backup %q of database %q pinned: retention and delete will skip it\n", backupID, name)
			return
		}
		_, _ = fmt.Fprintf(stdout, "backup %q of database %q unpinned\n", backupID, name)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func backupsPinUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s backups pin <database> <backup-id> [flags]
  %[1]s backups unpin <database> <backup-id> [flags]

Pins one backup so retention, pruning and delete never remove it (for
example before a risky migration). Unpin releases it. Refused (409) while
the backup is still running.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print {"pinned": <bool>} as JSON to stdout on success, nothing else
  --output string          output format: json, ndjson, yaml, table, wide, text or name (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}
