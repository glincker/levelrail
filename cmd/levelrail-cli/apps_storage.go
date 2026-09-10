package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsStorage dispatches "apps storage <verb> [flags]" to one of
// set/clear: PUT/DELETE /api/v1/apps/{name}/storage
// (internal/api/apps_storage.go), attaching or detaching an
// already-connected backup target (see "backup-targets") as an app's
// object-storage credential source. There is no "get": the currently
// attached target already shows up on "apps get <name>" as
// storage_target_id, the same way node_id/project_id do for their own
// PUT-only sub-resources.
func runAppsStorage(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsStorageUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsStorageUsage(prog))
		return exitOK
	case "set":
		return runAppsStorageSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runAppsStorageClear(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps storage subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsStorageUsage(prog))
		return exitUsage
	}
}

func appsStorageUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps storage set <name> --storage-target-id ID [flags]   attach a connected bucket as object storage
  %[1]s apps storage clear <name> [flags]                        detach an app's object storage

Attaching a target injects its bucket credentials into the app's
container as S3_* env vars at container-create time (no manual AWS SDK
config needed). See "%[1]s backup-targets list" for connected targets.

Run "%[1]s apps storage <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsStorageSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps storage set", "print the resulting app_name/storage_target_id as JSON to stdout and nothing else", stderr)
	var storageTargetID string
	fs.StringVar(&storageTargetID, "storage-target-id", "", "an already-connected backup target's ID (required, see \"backup-targets list\")")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps storage set <name> --storage-target-id ID [flags]\n\nAttaches an already-connected backup target to <name> as its\nobject-storage credential source.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "apps storage set", "app name")
	if !ok {
		return exitUsage
	}
	if storageTargetID == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps storage set requires --storage-target-id\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := client.SetAppStorage(context.Background(), name, storageTargetID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set storage for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printAppStorageHuman(stdout, result) })
}

func runAppsStorageClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps storage clear", "print {\"cleared\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps storage clear <name> [flags]\n\nDetaches whatever backup target <name> currently resolves object-storage\ncredentials from.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps storage clear", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	if err := client.ClearAppStorage(context.Background(), name); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear storage for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"cleared": true}, func() {
		_, _ = fmt.Fprintf(stdout, "storage detached for app %q\n", name)
	})
}

func printAppStorageHuman(out io.Writer, r appStorageResource) {
	_, _ = fmt.Fprintf(out, "app_name:          %s\n", r.AppName)
	_, _ = fmt.Fprintf(out, "storage_target_id: %s\n", r.StorageTargetID)
}
