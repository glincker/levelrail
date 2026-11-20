package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"golang.org/x/term"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/projectarchive"
)

const envArchivePassphrase = "LEVELRAIL_ARCHIVE_PASSPHRASE" //nolint:gosec // an env var name, not a credential

// archivePassphrase never takes the secret as a flag value, which would land
// in shell history: a file, the environment, or a hidden prompt.
func archivePassphrase(passFile string, lookupEnv func(string) (string, bool), stderr io.Writer, confirm bool) (string, error) {
	if passFile != "" {
		b, err := os.ReadFile(passFile) //nolint:gosec // the operator names their own passphrase file
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	}
	if v, ok := lookupEnv(envArchivePassphrase); ok && v != "" {
		return v, nil
	}
	fd := int(os.Stdin.Fd()) //nolint:gosec // a terminal file descriptor always fits an int
	if !term.IsTerminal(fd) {
		return "", fmt.Errorf("a passphrase is needed: use --passphrase-file, set %s, or run in a terminal", envArchivePassphrase)
	}
	_, _ = fmt.Fprint(stderr, "Archive passphrase: ")
	b, err := term.ReadPassword(fd)
	_, _ = fmt.Fprintln(stderr)
	if err != nil {
		return "", err
	}
	if confirm {
		_, _ = fmt.Fprint(stderr, "Repeat passphrase: ")
		again, err := term.ReadPassword(fd)
		_, _ = fmt.Fprintln(stderr)
		if err != nil || string(again) != string(b) {
			return "", errors.New("passphrases do not match")
		}
	}
	return string(b), nil
}

func runProjectArchiveExport(client *Client, name, outFile, passFile string, includeSecrets, includeData, jsonOut bool, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	req := apiclient.ProjectArchiveExportRequest{IncludeSecrets: includeSecrets, IncludeData: includeData}
	if includeSecrets {
		pass, err := archivePassphrase(passFile, lookupEnv, stderr, true)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		req.Passphrase = pass
	}
	if outFile == "" {
		outFile = name + ".lrx"
	}
	f, err := os.OpenFile(outFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // the operator names the output file
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create %s: %w", outFile, err))
	}
	n, err := client.ExportProjectArchiveTo(context.Background(), resolveProjectID(client, name), req, f)
	cerr := f.Close()
	if err == nil {
		err = cerr
	}
	if err == nil {
		err = verifyArchiveFile(outFile)
	}
	if err != nil {
		_ = os.Remove(outFile)
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("export project %q: %w", name, err))
	}
	if jsonOut {
		b, _ := json.Marshal(map[string]any{"file": outFile, "bytes": n})
		_, _ = fmt.Fprintln(stdout, string(b))
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "wrote %s (%d bytes)\n", outFile, n)
	return exitOK
}

func verifyArchiveFile(path string) error {
	f, err := os.Open(path) //nolint:gosec // the file this command just wrote
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = projectarchive.Read(f)
	return err
}

// runAppsProjectsImport implements "projects import FILE": a dry run with
// --plan, otherwise plan, stop on conflicts, then apply.
func runAppsProjectsImport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "projects import", "print the plan and result as JSON", stderr)
	var rename, passFile string
	var planOnly bool
	fs.BoolVar(&planOnly, "plan", false, "print what would be created, the conflicts and the manual steps, change nothing")
	fs.StringVar(&rename, "rename", "", "import under this project name instead of the archive's")
	fs.StringVar(&passFile, "passphrase-file", "", "apply sealed secrets with the passphrase in this file (else LEVELRAIL_ARCHIVE_PASSPHRASE)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s projects import FILE.lrx [--plan] [--rename NAME] [--passphrase-file FILE] [flags]\n\nThe archive is verified first. Hostnames, image repos and names that already\nexist are conflicts and stop the import. DNS, git integrations and node\nplacement stay manual steps.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: projects import requires exactly one archive file\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	raw, err := os.ReadFile(fs.Arg(0)) //nolint:gosec // the operator names their own archive
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	arc, err := projectarchive.Read(bytes.NewReader(raw))
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s: %w", fs.Arg(0), err))
	}
	req := apiclient.ProjectImportRequest{Archive: raw, Rename: rename}
	if arc.Manifest.SecretsIncluded {
		if pass, perr := archivePassphrase(passFile, lookupEnv, stderr, false); perr == nil {
			req.Passphrase = pass
		}
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	planResp, err := client.PlanProjectImport(context.Background(), req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("plan import: %w", err))
	}
	if jsonOut && planOnly {
		b, _ := json.MarshalIndent(planResp, "", "  ")
		_, _ = fmt.Fprintln(stdout, string(b))
		return exitOK
	}
	if !jsonOut {
		printProjectImportPlan(stdout, planResp.Plan)
	}
	if planOnly {
		return exitOK
	}
	if len(planResp.Plan.Conflicts) > 0 {
		return reportError(stdout, stderr, jsonOut, errors.New("import stopped: resolve the conflicts above or use --rename"))
	}
	req.ExpectedPlanHash = planResp.Plan.Hash
	res, err := client.ImportProject(context.Background(), req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("import: %w", err))
	}
	if jsonOut {
		b, _ := json.MarshalIndent(res, "", "  ")
		_, _ = fmt.Fprintln(stdout, string(b))
	} else if res.Result != nil {
		_, _ = fmt.Fprintf(stdout, "imported %s: %d applied, %d failed\n", res.Plan.Project, res.Result.Applied, res.Result.Failed)
	}
	if res.Result != nil && res.Result.Failed > 0 {
		return exitAPIError
	}
	return exitOK
}

func printProjectImportPlan(w io.Writer, p apiclient.ProjectImportPlan) {
	_, _ = fmt.Fprintf(w, "project %s (from %s, source version %s)\n", cleanArchiveText(p.Project), cleanArchiveText(p.SourceProject), cleanArchiveText(p.SourceVersion))
	_, _ = fmt.Fprintf(w, "will create %d:\n", len(p.Creates))
	for _, c := range p.Creates {
		_, _ = fmt.Fprintf(w, "  + %s %s\n", cleanArchiveText(c.Kind), cleanArchiveText(c.Name))
	}
	if len(p.Conflicts) > 0 {
		_, _ = fmt.Fprintln(w, "conflicts:")
		for _, c := range p.Conflicts {
			_, _ = fmt.Fprintf(w, "  ! %s %s: %s\n", cleanArchiveText(c.Kind), cleanArchiveText(c.Name), cleanArchiveText(c.Detail))
		}
	}
	for _, wn := range p.Warnings {
		_, _ = fmt.Fprintf(w, "warning: %s\n", cleanArchiveText(wn))
	}
	if len(p.Manual) > 0 {
		_, _ = fmt.Fprintln(w, "manual steps:")
		for _, m := range p.Manual {
			_, _ = fmt.Fprintf(w, "  - %s\n", cleanArchiveText(m))
		}
	}
}

// cleanArchiveText drops control characters so archive text cannot move the
// cursor or rewrite earlier terminal output.
func cleanArchiveText(in string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, in)
}
