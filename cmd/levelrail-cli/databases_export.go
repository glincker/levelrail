package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/term"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	exportWaitEnv = "LEVELRAIL_EXPORT_WAIT"
	exportPollEnv = "LEVELRAIL_EXPORT_POLL"
	// Fallbacks used only when neither the flag nor the env var is set.
	exportWaitFallback = 15 * time.Minute
	exportPollFallback = 2 * time.Second
	exportHistoryLimit = 50
)

// Tests replace these; production reads the real terminal state.
var (
	exportStdoutIsTerminal = func(w io.Writer) bool {
		f, ok := w.(*os.File)
		return ok && term.IsTerminal(int(f.Fd())) //nolint:gosec // a file descriptor always fits an int
	}
	exportPollInterval = func(lookupEnv func(string) (string, bool)) time.Duration {
		return durationFromEnv(lookupEnv, exportPollEnv, exportPollFallback)
	}
)

func durationFromEnv(lookupEnv func(string) (string, bool), key string, fallback time.Duration) time.Duration {
	if v, ok := lookupEnv(key); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return fallback
}

func runDatabasesExport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases export", "not applicable, the dump is the output", stderr)
	var outPath, targetID string
	var fresh, latest, force bool
	var wait time.Duration
	fs.StringVar(&outPath, "out", "", "file to write the dump to, or - for stdout (required when stdout is a terminal)")
	fs.StringVar(&targetID, "target", "", "backup target id for the fresh backup (default: the only configured target)")
	fs.BoolVar(&fresh, "fresh", false, "take a new backup first and export it (default)")
	fs.BoolVar(&latest, "latest", false, "export the newest succeeded backup without taking a new one")
	fs.BoolVar(&force, "force", false, "overwrite --out if it already exists")
	fs.DurationVar(&wait, "wait", 0, "how long to wait for the fresh backup (default: "+exportWaitEnv+" env var, then 15m)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesExportUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: databases export requires exactly one database name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]
	if fresh && latest {
		return reportError(stdout, stderr, jsonOut, newValidationError("--fresh and --latest are mutually exclusive"))
	}
	toStdout := outPath == "" || outPath == "-"
	if toStdout && exportStdoutIsTerminal(stdout) {
		return reportError(stdout, stderr, jsonOut, newValidationError("refusing to write a database dump to a terminal; pass --out FILE or redirect stdout"))
	}
	if !toStdout && !force {
		if _, err := os.Lstat(outPath); err == nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("%s already exists; pass --force to overwrite it", outPath))
		}
	}
	if wait <= 0 {
		wait = durationFromEnv(lookupEnv, exportWaitEnv, exportWaitFallback)
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	historyID, err := pickExportBackup(ctx, client, prog, name, targetID, latest, wait, exportPollInterval(lookupEnv), stderr)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	_, _ = fmt.Fprintf(stderr, "downloading backup %s of database %q\n", historyID, name)
	data, err := client.DownloadBackup(ctx, name, historyID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("download backup %q of database %q: %w", historyID, name, err))
	}
	if toStdout {
		if _, err := stdout.Write(data); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("write dump: %w", err))
		}
		return exitOK
	}
	if err := writeDumpFile(outPath, data, force); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	_, _ = fmt.Fprintf(stderr, "wrote %d bytes to %s\n", len(data), outPath)
	return exitOK
}

// pickExportBackup returns the id of a succeeded backup: the newest one with
// latest, otherwise a new one triggered here and awaited. Fails closed on
// a failed backup or when wait runs out.
func pickExportBackup(ctx context.Context, client *Client, prog, name, targetID string, latest bool, wait, poll time.Duration, stderr io.Writer) (string, error) {
	if latest {
		history, err := client.ListBackups(ctx, name, apiclient.ListBackupsOptions{Limit: exportHistoryLimit})
		if err != nil {
			return "", fmt.Errorf("list backups for database %q: %w", name, err)
		}
		best := -1
		for i, h := range history {
			if h.Status == "succeeded" && (best < 0 || h.StartedAt > history[best].StartedAt) {
				best = i
			}
		}
		if best < 0 {
			return "", newValidationError("database %q has no succeeded backup yet; run without --latest to take one", name)
		}
		return history[best].ID, nil
	}

	if targetID == "" {
		id, err := soleBackupTarget(ctx, client, prog)
		if err != nil {
			return "", err
		}
		targetID = id
	}
	started, err := client.TriggerBackup(ctx, name, targetID)
	if err != nil {
		return "", fmt.Errorf("trigger backup for database %q: %w", name, err)
	}
	_, _ = fmt.Fprintf(stderr, "backup %s started, waiting up to %s\n", started.ID, wait)

	waitCtx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	tick := time.NewTicker(poll)
	defer tick.Stop()
	for {
		history, err := client.ListBackups(waitCtx, name, apiclient.ListBackupsOptions{Limit: exportHistoryLimit})
		if err != nil {
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				return "", fmt.Errorf("timed out after %s waiting for backup %s; it may still finish, check \"%s backups list %s\"", wait, started.ID, prog, name)
			}
			return "", fmt.Errorf("list backups for database %q: %w", name, err)
		}
		for _, h := range history {
			if h.ID != started.ID {
				continue
			}
			switch h.Status {
			case "succeeded":
				return h.ID, nil
			case "failed":
				return "", fmt.Errorf("backup %s failed: %s", h.ID, h.Error)
			}
		}
		select {
		case <-waitCtx.Done():
			return "", fmt.Errorf("timed out after %s waiting for backup %s; it may still finish, check \"%s backups list %s\"", wait, started.ID, prog, name)
		case <-tick.C:
		}
	}
}

// writeDumpFile writes via a private temp file in the same directory, then
// publishes it by link (no overwrite) or rename (--force), so a partial dump never appears.
func writeDumpFile(path string, data []byte, force bool) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dump-*")
	if err != nil {
		return fmt.Errorf("create temp file next to %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("restrict dump file mode: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write dump: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write dump: %w", err)
	}
	if force {
		if err := os.Rename(tmpName, path); err != nil {
			return fmt.Errorf("move dump into place: %w", err)
		}
		return nil
	}
	if err := os.Link(tmpName, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return newValidationError("%s already exists; pass --force to overwrite it", path)
		}
		return fmt.Errorf("move dump into place: %w", err)
	}
	return nil
}

func databasesExportUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases export <name> [--out FILE|-] [--fresh | --latest] [--wait DURATION] [flags]

Writes a dump of a managed database. By default a new backup is taken
first and its dump is downloaded once it succeeds (--fresh). With --latest
the newest succeeded backup is downloaded instead and nothing new is taken.
The bytes are the backup object as stored (an encrypted backup stays
encrypted), in whatever format that database engine's backup runner emits.

The dump is never printed to a terminal: pass --out FILE or redirect stdout.
Files are written with mode 0600 and an existing file is only replaced
with --force. Progress goes to stderr, stdout carries only the dump.

Flags:
  --out string       file to write, or - for stdout
  --fresh            take a new backup first (default)
  --latest           use the newest succeeded backup
  --target string    backup target id for the fresh backup (default: the only configured target)
  --wait duration    how long to wait for the fresh backup (default: %[5]s env var, then 15m)
  --force            overwrite --out if it exists
  --token string     API token (default: %[2]s env var, then the credentials file)
  --api-url string   control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string   named credentials profile to read (overrides APP_PROFILE, default "default")
  -h, --help         show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL, exportWaitEnv)
}
