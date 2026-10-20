package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func archiveUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s archive ls [flags]                       list archive bundles and the live row counts
  %[1]s archive create [--kind K] [--older-than DAYS]   archive cold rows now (no kind runs the whole policy)
  %[1]s archive restore <id> [flags]             put a bundle's rows back into the live tables
  %[1]s archive verify <id> [flags]              check hash and restore into a scratch database (exit 1 if not ok)
  %[1]s archive download <id> [--out FILE]       save a bundle (zstd compressed NDJSON)
  %[1]s archive delete <id> [flags]              delete one bundle
  %[1]s archive policy [--enabled=BOOL] [--audit-days N] [--deploy-days N] [--pipeline-days N] [--keep-days N]

Kinds: audit, deploys, pipelines. Bundles hold no secret values.
All subcommands need a root-scoped token.
`, prog)
}

func archiveFlagsUsage(prog, verb, tail string) string {
	return fmt.Sprintf(`Usage:
  %[1]s archive %[5]s [flags]

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

// runArchive dispatches "archive ls|create|restore|verify|download|delete|policy".
func runArchive(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, archiveUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, archiveUsage(prog))
		return exitOK
	case "ls", "list":
		return runArchiveLs(prog, args[1:], stdout, stderr, lookupEnv)
	case "create":
		return runArchiveCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "restore":
		return runArchiveByID(prog, "restore", args[1:], stdout, stderr, lookupEnv)
	case "verify":
		return runArchiveByID(prog, "verify", args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runArchiveByID(prog, "delete", args[1:], stdout, stderr, lookupEnv)
	case "download":
		return runArchiveDownload(prog, args[1:], stdout, stderr, lookupEnv)
	case "policy":
		return runArchivePolicy(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown archive subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, archiveUsage(prog))
		return exitUsage
	}
}

func finishArchive(stdout, stderr io.Writer, of outputFlags, v any, text func()) int {
	if err := renderResult(stdout, of.Format, of.Query, v, text); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runArchiveLs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "archive ls", "print the list as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, archiveFlagsUsage(prog, "ls", "Lists archive bundles, newest first, with how many rows are still live."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	list, err := client.ListArchives(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list archives: %w", err))
	}
	if list.Bundles == nil {
		list.Bundles = []apiclient.ArchiveBundle{}
	}
	return finishArchive(stdout, stderr, of, list, func() { printArchiveList(stdout, list) })
}

func printArchiveList(out io.Writer, list apiclient.ArchiveList) {
	if len(list.Bundles) == 0 {
		_, _ = fmt.Fprintln(out, "no archive bundles")
	} else {
		tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tKIND\tROWS\tSIZE\tCREATED\tVERIFIED")
		for _, b := range list.Bundles {
			v := "never"
			if b.VerifiedOK != nil {
				v = "failed"
				if *b.VerifiedOK {
					v = "ok"
				}
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%s\t%s\n", b.ID, b.Kind, b.TotalRows, b.SizeBytes, b.CreatedAt, v)
		}
		_ = tw.Flush()
	}
	kinds := make([]string, 0, len(list.HotRows))
	for k := range list.HotRows {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		_, _ = fmt.Fprintf(out, "live %s rows: %d\n", k, list.HotRows[k])
	}
}

func runArchiveCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "archive create", "print the result as JSON to stdout and nothing else", stderr)
	kind := fs.String("kind", "", "audit, deploys or pipelines (default: run the whole policy)")
	days := fs.Int("older-than", 0, "archive rows older than this many days (default: the policy's hot days)")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, archiveFlagsUsage(prog, "create", "Moves cold rows into a compressed bundle and removes them from the live\ntables once the bundle has been re-read and matches.\n\n  --kind string        audit, deploys or pipelines (default: run the whole policy)\n  --older-than int     archive rows older than this many days (default: the policy's hot days)"))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.CreateArchive(context.Background(), *kind, *days)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create archive: %w", err))
	}
	return finishArchive(stdout, stderr, of, res, func() {
		if len(res.Created) == 0 {
			_, _ = fmt.Fprintln(stdout, "nothing old enough to archive")
		}
		for _, b := range res.Created {
			_, _ = fmt.Fprintf(stdout, "archived %d rows into %s (%d bytes)\n", b.TotalRows, b.ID, b.SizeBytes)
		}
		if res.Pruned > 0 {
			_, _ = fmt.Fprintf(stdout, "deleted %d expired bundles\n", res.Pruned)
		}
	})
}

func runArchiveByID(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "archive "+verb, "print the result as JSON to stdout and nothing else", stderr)
	tails := map[string]string{
		"restore": "Puts a bundle's rows back into the live tables. Rows already present are\nleft alone. Restored rows age out again on the next archive run.",
		"verify":  "Checks the bundle's sha256 and row counts, then restores it into a scratch\ndatabase. Nothing in the live database changes. Exits 1 when a check fails.",
		"delete":  "Deletes one bundle. The rows it holds are gone for good.",
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, archiveFlagsUsage(prog, verb+" <id>", tails[verb])) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "archive "+verb, "an archive id", 1)
	if !ok {
		return exitUsage
	}
	id := rest[0]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	switch verb {
	case "restore":
		res, err := client.RestoreArchive(ctx, id)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("restore archive %q: %w", id, err))
		}
		return finishArchive(stdout, stderr, of, res, func() {
			n := 0
			for _, c := range res.Restored {
				n += c
			}
			_, _ = fmt.Fprintf(stdout, "restored %d rows from %s (%d already present, %d skipped without a parent)\n", n, id, res.Existing, res.Skipped)
		})
	case "verify":
		res, err := client.VerifyArchive(ctx, id)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("verify archive %q: %w", id, err))
		}
		if code := finishArchive(stdout, stderr, of, res, func() {
			if res.OK {
				_, _ = fmt.Fprintf(stdout, "archive %s verified by restore\n", id)
			} else {
				_, _ = fmt.Fprintf(stdout, "archive %s FAILED verification: %s\n", id, res.Error)
			}
		}); code != exitOK {
			return code
		}
		if !res.OK {
			return exitCheckFailed
		}
		return exitOK
	default:
		if err := client.DeleteArchive(ctx, id); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete archive %q: %w", id, err))
		}
		return finishArchive(stdout, stderr, of, map[string]string{"deleted": id}, func() { _, _ = fmt.Fprintf(stdout, "deleted %s\n", id) })
	}
}

func runArchiveDownload(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "archive download", "not applicable, the response body is always the raw bundle", stderr)
	var outFile string
	fs.StringVar(&outFile, "out", "", "write the bundle to this file instead of stdout")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, archiveFlagsUsage(prog, "download <id>", "Saves one bundle (zstd compressed NDJSON). With --out FILE it is written\nthere (mode 0600); otherwise the raw bytes go to stdout."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "archive download", "an archive id", 1)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	data, err := client.DownloadArchive(context.Background(), rest[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("download archive %q: %w", rest[0], err))
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

func runArchivePolicy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "archive policy", "print the policy as JSON to stdout and nothing else", stderr)
	enabled := fs.String("enabled", "", "true or false: turn scheduled archiving on or off")
	audit := fs.Int("audit-days", 0, "keep audit rows live this many days")
	deploys := fs.Int("deploy-days", 0, "keep deploy attempts live this many days")
	pipelines := fs.Int("pipeline-days", 0, "keep pipeline runs live this many days")
	keep := fs.Int("keep-days", -1, "delete bundles older than this many days (0 keeps forever)")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, archiveFlagsUsage(prog, "policy", "Shows the archive policy; any of the flags below changes it.\n\n  --enabled string       true or false\n  --audit-days int       keep audit rows live this many days\n  --deploy-days int      keep deploy attempts live this many days\n  --pipeline-days int    keep pipeline runs live this many days\n  --keep-days int        delete bundles older than this (0 keeps forever)"))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	pol, err := client.GetArchivePolicy(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get archive policy: %w", err))
	}
	changed := false
	switch *enabled {
	case "":
	case "true", "false":
		pol.Enabled, changed = *enabled == "true", true
	default:
		return reportError(stdout, stderr, jsonOut, newValidationError("--enabled must be true or false"))
	}
	for _, f := range []struct {
		v   int
		dst *int
		min int
	}{{*audit, &pol.AuditHotDays, 1}, {*deploys, &pol.DeployHotDays, 1}, {*pipelines, &pol.PipelineHotDays, 1}, {*keep, &pol.KeepDays, 0}} {
		if f.v >= f.min {
			*f.dst, changed = f.v, true
		}
	}
	if changed {
		if pol, err = client.SetArchivePolicy(ctx, pol); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("set archive policy: %w", err))
		}
	}
	return finishArchive(stdout, stderr, of, pol, func() {
		_, _ = fmt.Fprintf(stdout, "enabled: %t\naudit hot days: %d\ndeploy hot days: %d\npipeline hot days: %d\nkeep bundles days: %d\n", pol.Enabled, pol.AuditHotDays, pol.DeployHotDays, pol.PipelineHotDays, pol.KeepDays)
		if pol.LastRunAt != "" {
			_, _ = fmt.Fprintf(stdout, "last run: %s %s\n", pol.LastRunAt, pol.LastRunError)
		}
	})
}
