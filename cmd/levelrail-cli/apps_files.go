package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsFiles manages an app's inline config files. Secret files are
// write-only: ls shows them redacted.
func runAppsFiles(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsFilesUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsFilesUsage(prog))
		return exitOK
	case "ls":
		return runAppsFilesLs(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runAppsFilesSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "rm":
		return runAppsFilesRm(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps files subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsFilesUsage(prog))
		return exitUsage
	}
}

func appsFilesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps files ls <name>                                          list files (secret ones redacted)
  %[1]s apps files set <name> --path /etc/app.conf --from-file ./app.conf [--mode 0644] [--secret] [--apply]
  %[1]s apps files rm <name> --path /etc/app.conf [--apply]

--apply restarts the app so the change is live now; without it the change is
pending until the next restart or deploy. A secret file may use ${VAR}
placeholders and is never shown again.
`, prog)
}

func runAppsFilesLs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps files ls", "print the files as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps files ls <name> [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps files ls", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	res, err := client.ListAppFiles(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list files for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		if len(res.Files) == 0 {
			_, _ = fmt.Fprintln(stdout, "no files")
			return
		}
		for _, f := range res.Files {
			note := fmt.Sprintf("%d bytes", f.SizeBytes)
			if f.Redacted {
				note = "redacted"
			}
			_, _ = fmt.Fprintf(stdout, "%s  %s  %s\n", f.Path, f.Mode, note)
		}
	})
}

func runAppsFilesSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps files set", "print the resulting files as JSON to stdout and nothing else", stderr)
	filePath := fs.String("path", "", "absolute path inside the container")
	from := fs.String("from-file", "", "local file whose text becomes the content")
	content := fs.String("content", "", "inline content (use --from-file for anything long)")
	mode := fs.String("mode", "", "octal mode, default 0644")
	secret := fs.Bool("secret", false, "store encrypted, allow ${VAR} placeholders, never show again")
	apply := fs.Bool("apply", false, "restart the app now so the file is live")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps files set <name> --path P (--from-file F | --content C) [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "apps files set", "app name")
	if !ok {
		return exitUsage
	}
	if *filePath == "" || (*from == "") == (*content == "") {
		_, _ = fmt.Fprintf(stderr, "%s: apps files set needs --path and exactly one of --from-file or --content\n", prog)
		return exitUsage
	}
	text := *content
	if *from != "" {
		raw, err := os.ReadFile(*from) //nolint:gosec // the operator names their own local file
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("read %s: %w", *from, err))
		}
		text = string(raw)
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.SetAppFile(context.Background(), name, apiclient.SetAppFileRequest{Path: *filePath, Content: text, Mode: *mode, Secret: *secret}, *apply)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set file on app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "file %s set on %s (%d files)\n", *filePath, name, len(res.Files))
	})
}

func runAppsFilesRm(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps files rm", "print {\"removed\": true} as JSON to stdout and nothing else", stderr)
	filePath := fs.String("path", "", "absolute path inside the container")
	apply := fs.Bool("apply", false, "restart the app now")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps files rm <name> --path P [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "apps files rm", "app name")
	if !ok || *filePath == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps files rm needs <name> and --path\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.DeleteAppFile(context.Background(), name, *filePath, *apply); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("remove file from app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"removed": true}, func() {
		_, _ = fmt.Fprintf(stdout, "file %s removed from %s\n", *filePath, name)
	})
}
