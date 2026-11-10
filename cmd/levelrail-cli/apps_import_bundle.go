package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// maxBundleReadBytes bounds the local read; the server applies its own live cap.
const maxBundleReadBytes = 64 << 20

// runAppsImportBundle implements "apps import-bundle <file>": the reverse of apps export.
func runAppsImportBundle(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps import-bundle", "print the plan or result as JSON", stderr)
	var opts apiclient.AppBundleImportOptions
	fs.StringVar(&opts.Name, "name", "", "import under this app name instead of the one in the bundle")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "print what would be created without writing anything")
	fs.BoolVar(&opts.Deploy, "deploy", false, "start the services after import (needs the deploy ability)")
	fs.BoolVar(&opts.Replace, "replace", false, "replace an existing app of the same name (needs write:sensitive and --confirm)")
	fs.StringVar(&opts.Confirm, "confirm", "", "the app name again, required with --replace")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps import-bundle <file|-> [--name NAME] [--dry-run] [--deploy] [--replace --confirm NAME] [flags]\n\nImports a tar written by apps export --out -. Secret values are never imported;\nthe names to set are listed. Services are saved stopped unless --deploy is set.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps import-bundle requires exactly one bundle file\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	if opts.Replace && opts.Confirm == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--replace requires --confirm with the app name"))
	}
	data, err := readBundleFile(rest[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.ImportAppBundle(context.Background(), data, opts)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("import bundle: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() { printBundleImport(stdout, res) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func readBundleFile(path string) ([]byte, error) {
	var r io.Reader = os.Stdin
	if path != "-" {
		f, err := os.Open(path) //nolint:gosec // the operator names the file to upload
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		r = f
	}
	data, err := io.ReadAll(io.LimitReader(r, maxBundleReadBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBundleReadBytes {
		return nil, errors.New("bundle file is too large")
	}
	return data, nil
}

func printBundleImport(w io.Writer, res apiclient.AppBundleImport) {
	verb := "imported"
	if res.DryRun {
		verb = "would import"
	}
	_, _ = fmt.Fprintf(w, "%s app %s (bundle sha256 %s)\n", verb, res.App, res.SHA256)
	if res.Replaces {
		_, _ = fmt.Fprintln(w, "replaces the existing app of this name")
	}
	for _, s := range res.Services {
		_, _ = fmt.Fprintf(w, "  service %s  %s\n", s.Name, s.Image)
	}
	for _, n := range res.SecretsToSet {
		_, _ = fmt.Fprintf(w, "  secret to set: %s\n", n)
	}
	for _, m := range res.Warnings {
		_, _ = fmt.Fprintf(w, "  warning: %s\n", m)
	}
	if !res.DryRun && !res.Deployed {
		_, _ = fmt.Fprintf(w, "next: set the secrets, then %s apps start %s\n", "levelrail-cli", res.App)
	}
}
