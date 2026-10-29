package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

func runAppsVolumesLs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps volumes ls", "print the listing as JSON to stdout and nothing else", stderr)
	var limit int
	fs.IntVar(&limit, "limit", 0, "max entries (default: the server cap)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps volumes ls <app> <volume> [path] [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) < 2 || len(rest) > 3 {
		_, _ = fmt.Fprintf(stderr, "%s: apps volumes ls requires an app, a volume and an optional path\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	dir := ""
	if len(rest) == 3 {
		dir = rest[2]
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.ListVolumeFiles(context.Background(), rest[0], rest[1], dir, limit)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list files in %s/%s: %w", rest[0], rest[1], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		for _, e := range res.Entries {
			name := e.Name
			if e.Type == "dir" {
				name += "/"
			}
			_, _ = fmt.Fprintf(stdout, "%-11s %10d  %s  %s\n", e.Mode, e.Size, e.ModTime, name)
		}
		if res.Truncated {
			_, _ = fmt.Fprintln(stdout, "(listing truncated, narrow the path or raise --limit)")
		}
	})
}

func runAppsVolumesRm(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps volumes rm", "print {\"deleted\": true} as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps volumes rm <app> <volume> <path> [flags]\n\nDeletes one file, never a directory.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps volumes rm", "an app, a volume and a path", 3)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.DeleteVolumeFile(context.Background(), rest[0], rest[1], rest[2]); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete %s from %s/%s: %w", rest[2], rest[0], rest[1], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "deleted %s\n", rest[2])
	})
}

// runAppsVolumesCp copies one file: the remote side is written ":path", the other is a local file or "-".
func runAppsVolumesCp(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps volumes cp", "print the transfer result as JSON to stdout and nothing else", stderr)
	var backupID string
	fs.StringVar(&backupID, "from-backup", "", "restore the remote path from this backup id instead of uploading a local file")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s apps volumes cp <app> <volume> :remote/path <local|->   download\n  %[1]s apps volumes cp <app> <volume> <local|-> :remote/path   upload\n  %[1]s apps volumes cp <app> <volume> --from-backup ID :remote/path   restore one file from a backup\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	want := 4
	if backupID != "" {
		want = 3
	}
	if len(rest) != want {
		_, _ = fmt.Fprintf(stderr, "%s: apps volumes cp has the wrong number of arguments\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	app, volume := rest[0], rest[1]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	if backupID != "" {
		if !strings.HasPrefix(rest[2], ":") {
			_, _ = fmt.Fprintf(stderr, "%s: the restore target must be a remote path like :conf/app.ini\n", prog)
			return exitUsage
		}
		res, err := client.RestoreVolumeFile(ctx, app, volume, backupID, strings.TrimPrefix(rest[2], ":"))
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("restore %s from backup %s: %w", rest[2], backupID, err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, res, func() { _, _ = fmt.Fprintf(stdout, "restored %s (%d bytes)\n", res.Path, res.Size) })
	}

	src, dst := rest[2], rest[3]
	srcRemote, dstRemote := strings.HasPrefix(src, ":"), strings.HasPrefix(dst, ":")
	if srcRemote == dstRemote {
		_, _ = fmt.Fprintf(stderr, "%s: exactly one side must be a remote path starting with ':'\n", prog)
		return exitUsage
	}
	if srcRemote {
		w, closeFn, err := openLocalDest(dst, stdout)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		n, err := client.DownloadVolumeFile(ctx, app, volume, strings.TrimPrefix(src, ":"), w)
		if cerr := closeFn(); err == nil {
			err = cerr
		}
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("download %s: %w", src, err))
		}
		if dst == "-" {
			return exitOK
		}
		return writeScheduledTaskResult(stdout, stderr, of, map[string]any{"path": strings.TrimPrefix(src, ":"), "size": n}, func() { _, _ = fmt.Fprintf(stdout, "downloaded %d bytes to %s\n", n, dst) })
	}
	in := io.Reader(os.Stdin)
	if src != "-" {
		f, err := os.Open(src) //nolint:gosec // the operator names the local file to upload
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("open %s: %w", src, err))
		}
		defer func() { _ = f.Close() }()
		in = f
	}
	res, err := client.UploadVolumeFile(ctx, app, volume, strings.TrimPrefix(dst, ":"), in)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("upload to %s: %w", dst, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { _, _ = fmt.Fprintf(stdout, "uploaded %d bytes to %s\n", res.Size, res.Path) })
}

func openLocalDest(dst string, stdout io.Writer) (io.Writer, func() error, error) {
	if dst == "-" {
		return stdout, func() error { return nil }, nil
	}
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // the operator names the local destination
	if err != nil {
		return nil, nil, fmt.Errorf("create %s: %w", dst, err)
	}
	return f, f.Close, nil
}
