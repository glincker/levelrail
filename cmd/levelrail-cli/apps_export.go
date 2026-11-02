package main

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// exportFileNames are the only entries written to disk, so a hostile archive cannot pick a path.
var exportFileNames = map[string]bool{"docker-compose.yml": true, ".env.example": true, "README.md": true}

// runAppsExport implements "apps export <name>": the app as plain compose plus an env template.
func runAppsExport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps export", "print the written file names as JSON", stderr)
	var format, out string
	fs.StringVar(&format, "format", "compose", "export format (only compose)")
	fs.StringVar(&out, "out", "", "directory to write the files into, or - for a tar stream on stdout (default ./<name>-export)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps export <name> [--format compose] [--out DIR|-] [flags]\n\nWrites docker-compose.yml, .env.example and README.md. Secret values are never exported;\nsee docs/leave-anytime.md.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps export requires exactly one app name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]
	if format != "compose" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--format must be compose"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	data, err := client.ExportApp(context.Background(), name, format)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("export app %q: %w", name, err))
	}
	if out == "-" {
		if _, err := stdout.Write(data); err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		return exitOK
	}
	if out == "" {
		out = name + "-export"
	}
	written, err := extractExport(data, out)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("write export: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, written, func() {
		for _, f := range written {
			_, _ = fmt.Fprintln(stdout, f)
		}
		_, _ = fmt.Fprintf(stdout, "next: cd %s && cp .env.example .env && docker compose up -d\n", out)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// maxExportFileBytes bounds one extracted file so a bad archive cannot fill memory.
const maxExportFileBytes = 8 << 20

// extractExport writes the known files of a tar into dir and returns their paths.
func extractExport(data []byte, dir string) ([]string, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	tr := tar.NewReader(bytes.NewReader(data))
	var written []string
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return written, nil
		}
		if err != nil {
			return nil, err
		}
		if !exportFileNames[h.Name] {
			return nil, fmt.Errorf("unexpected entry %q in the export", h.Name)
		}
		p := filepath.Join(dir, h.Name) //nolint:gosec // h.Name is checked against the fixed allowlist above
		body, err := io.ReadAll(io.LimitReader(tr, maxExportFileBytes+1))
		if err != nil {
			return nil, err
		}
		if len(body) > maxExportFileBytes {
			return nil, fmt.Errorf("entry %q exceeds %d bytes", h.Name, maxExportFileBytes)
		}
		if err := os.WriteFile(p, body, 0o600); err != nil {
			return nil, err
		}
		written = append(written, p)
	}
}
