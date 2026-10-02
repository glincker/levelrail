package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsVolumes dispatches "apps volumes <verb> [flags]" to one of
// get/attach/detach, the CLI counterpart of
// internal/api/apps_volumes_attach.go's own PUT /api/v1/apps/{name}/volumes
// route: attach or remove a named Docker volume on an app that already
// exists, without hand-editing app.yaml and redeploying. The mount only
// takes effect at the service's next container create (a redeploy or
// restart), the same "baked in at create time" contract env vars already
// have.
func runAppsVolumes(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsVolumesUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsVolumesUsage(prog))
		return exitOK
	case "get":
		return runAppsVolumesGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "attach":
		return runAppsVolumesAttach(prog, args[1:], stdout, stderr, lookupEnv)
	case "detach":
		return runAppsVolumesDetach(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps volumes subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsVolumesUsage(prog))
		return exitUsage
	}
}

func appsVolumesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps volumes get <name> [flags]                                          list an app's named Docker volumes
  %[1]s apps volumes attach <name> --name NAME --path PATH [flags]   attach a new named volume
  %[1]s apps volumes detach <name> --name NAME [flags]                remove an attached volume

NAME is the volume's logical name (lowercase letters, numbers, and
hyphens, starting with a letter), scoped to this service; PATH is the
absolute path it mounts at inside the container. The same volume can
also be declared in app.yaml's own volumes: block, which takes over on
the next redeploy.

Run "%[1]s apps volumes <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsVolumesGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps volumes get", "print the app's volumes as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps volumes get <name> [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps volumes get", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	app, err := client.GetApp(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get volumes for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, app.Volumes, func() { printAppVolumesHuman(stdout, app.Volumes) })
}

func runAppsVolumesAttach(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps volumes attach", "print the resulting volume list as JSON to stdout and nothing else", stderr)
	var volName, path string
	fs.StringVar(&volName, "name", "", "logical name for the new volume (required)")
	fs.StringVar(&path, "path", "", "absolute mount path inside the container (required)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps volumes attach <name> --name NAME --path PATH [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "apps volumes attach", "app name")
	if !ok {
		return exitUsage
	}
	if volName == "" || path == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps volumes attach requires --name and --path\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	app, err := client.GetApp(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get volumes for app %q: %w", name, err))
	}
	for _, v := range app.Volumes {
		if v.Name == volName {
			return reportError(stdout, stderr, jsonOut, newValidationError("volume %q already exists on app %q", volName, name))
		}
	}
	volumes := append(app.Volumes, appVolumeResource{Name: volName, ContainerPath: path}) //nolint:gocritic // append onto app.Volumes is intentional: this request's own copy, not reused afterward

	result, err := client.SetAppVolumes(context.Background(), name, setAppVolumesRequest{Volumes: volumes})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("attach volume %q to app %q: %w", volName, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printAppVolumesHuman(stdout, result.Volumes) })
}

func runAppsVolumesDetach(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps volumes detach", "print the resulting volume list as JSON to stdout and nothing else", stderr)
	var volName string
	fs.StringVar(&volName, "name", "", "logical name of the volume to detach (required)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps volumes detach <name> --name NAME [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "apps volumes detach", "app name")
	if !ok {
		return exitUsage
	}
	if volName == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps volumes detach requires --name\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	app, err := client.GetApp(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get volumes for app %q: %w", name, err))
	}
	volumes := make([]appVolumeResource, 0, len(app.Volumes))
	found := false
	for _, v := range app.Volumes {
		if v.Name == volName {
			found = true
			continue
		}
		volumes = append(volumes, v)
	}
	if !found {
		return reportError(stdout, stderr, jsonOut, newValidationError("volume %q not found on app %q", volName, name))
	}

	result, err := client.SetAppVolumes(context.Background(), name, setAppVolumesRequest{Volumes: volumes})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("detach volume %q from app %q: %w", volName, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printAppVolumesHuman(stdout, result.Volumes) })
}

func printAppVolumesHuman(out io.Writer, volumes []appVolumeResource) {
	if len(volumes) == 0 {
		_, _ = fmt.Fprintln(out, "volumes: none")
		return
	}
	_, _ = fmt.Fprintln(out, "volumes:")
	for _, v := range volumes {
		_, _ = fmt.Fprintf(out, "  - %s -> %s\n", v.Name, v.ContainerPath)
	}
}
