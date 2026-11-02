package main

import (
	"context"
	"fmt"
	"io"
	"os"
)

// runVolumes handles the top-level "volumes" verb, which carries the transfer pair.
func runVolumes(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) > 0 {
		switch args[0] {
		case "export":
			return runAppsVolumesExport(prog, args[1:], stdout, stderr, lookupEnv)
		case "import":
			return runAppsVolumesImport(prog, args[1:], stdout, stderr, lookupEnv)
		}
	}
	_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s volumes export <app> <volume> --out FILE.tar.gz [flags]\n  %[1]s volumes import <app> <volume> --in FILE.tar.gz [--force] [flags]\n", prog)
	return exitUsage
}

// runAppsVolumesExport writes an app volume to a local tar.gz.
func runAppsVolumesExport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps volumes export", "print the export result as JSON to stdout and nothing else", stderr)
	var out string
	fs.StringVar(&out, "out", "", "file to write the tar.gz to, or - for stdout (required)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps volumes export <app> <volume> --out FILE.tar.gz [flags]\n\nStreams the volume through a read-only helper container. For a consistent copy of a busy app, stop it first.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps volumes export", "an app and a volume", 2)
	if !ok {
		return exitUsage
	}
	if out == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps volumes export requires --out\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	w, finish, err := openLocalDest(out, stdout)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	n, err := client.ExportVolume(context.Background(), rest[0], rest[1], w)
	if ferr := finish(err); err == nil {
		err = ferr
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("export %s/%s: %w", rest[0], rest[1], err))
	}
	if out == "-" {
		return exitOK
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]any{"app": rest[0], "volume": rest[1], "file": out, "bytes": n}, func() {
		_, _ = fmt.Fprintf(stdout, "exported %s/%s to %s (%d bytes)\n", rest[0], rest[1], out, n)
	})
}

// runAppsVolumesImport unpacks a local tar.gz into an app volume.
func runAppsVolumesImport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps volumes import", "print the import result as JSON to stdout and nothing else", stderr)
	var in string
	var force bool
	fs.StringVar(&in, "in", "", "tar.gz file to import, or - for stdin (required)")
	fs.BoolVar(&force, "force", false, "replace files already in the volume (needs the volumes.import_overwrite setting)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps volumes import <app> <volume> --in FILE.tar.gz [--force] [flags]\n\nThe app must be stopped. Entries with absolute paths, \"..\" or links leaving the volume are refused.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps volumes import", "an app and a volume", 2)
	if !ok {
		return exitUsage
	}
	if in == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps volumes import requires --in\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	src := io.Reader(os.Stdin)
	if in != "-" {
		f, err := os.Open(in) //nolint:gosec // the operator names the local archive to import
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("open %s: %w", in, err))
		}
		defer func() { _ = f.Close() }()
		src = f
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.ImportVolume(context.Background(), rest[0], rest[1], src, force)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("import into %s/%s: %w", rest[0], rest[1], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "imported %d entries (%d bytes) into %s/%s\n", res.Entries, res.Bytes, rest[0], rest[1])
	})
}
