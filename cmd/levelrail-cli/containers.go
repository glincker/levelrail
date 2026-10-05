package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// runContainers dispatches "containers <verb> [flags]". With no verb, or
// a verb that looks like a flag (e.g. "--json"), it falls back to "list"
// for backward compatibility with "containers [flags]" from before
// stop/remove/claim existed. "list" is read-only; stop/remove/claim only
// ever act on an orphaned container (internal/api/containers_orphaned.go
// re-confirms this server-side on every call), never a Levelrail-managed
// one, use "apps stop"/"apps start"/"apps restart"/"apps delete" for
// those instead, which update desired state correctly.
func runContainers(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	sub := "list"
	rest := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub = args[0]
		rest = args[1:]
	}

	switch sub {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, containersUsage(prog))
		return exitOK
	case "list":
		return runContainersList(prog, rest, stdout, stderr, lookupEnv)
	case "stop":
		return runContainersStop(prog, rest, stdout, stderr, lookupEnv)
	case "remove":
		return runContainersRemove(prog, rest, stdout, stderr, lookupEnv)
	case "orphans":
		return runContainersOrphans(prog, rest, stdout, stderr, lookupEnv)
	case "reap":
		return runContainersReap(prog, rest, stdout, stderr, lookupEnv)
	case "claim":
		return runContainersClaim(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown containers subcommand %q\n\n", prog, sub)
		_, _ = fmt.Fprint(stderr, containersUsage(prog))
		return exitUsage
	}
}

// runContainersList implements "containers list": GET
// /api/v1/system/containers, every container on this node whether or
// not Levelrail manages it.
func runContainersList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "containers list", "print containers as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, containersUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	containers, err := client.ListContainers(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list containers: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, containers, func() { printContainersHuman(stdout, containers) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// runContainersStop implements "containers stop <name>": POST
// /api/v1/system/containers/{name}/stop. 409 if the container is
// Levelrail-managed; use "apps stop" instead for those.
func runContainersStop(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "containers stop", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s containers stop <name> [flags]\n\nStops an orphaned (not Levelrail-managed) container by its Docker name.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "containers stop", "container name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	if err := client.StopOrphanedContainer(context.Background(), name); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("stop container %q: %w", name, err))
	}

	result := map[string]string{"status": "stopped", "container": name}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintf(stdout, "container %q stopped\n", name)
	})
}

// runContainersRemove implements "containers remove <name>": POST
// /api/v1/system/containers/{name}/remove. Force-removes, including a
// still-running container; there is no separate confirmation step here,
// the orphaned-container safety check (409 on a managed container) is
// the only guard.
func runContainersRemove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "containers remove", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s containers remove <name> [flags]\n\nRemoves an orphaned (not Levelrail-managed) container by its Docker\nname, stopping it first if still running. Irreversible.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "containers remove", "container name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	if err := client.RemoveOrphanedContainer(context.Background(), name); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("remove container %q: %w", name, err))
	}

	result := map[string]string{"status": "removed", "container": name}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintf(stdout, "container %q removed\n", name)
	})
}

// runContainersClaim implements "containers claim <name> [--as
// app-name]": POST /api/v1/system/containers/{name}/claim, creating a
// real Levelrail app (build.type: image) from the container's own
// image. --as overrides the app name the server would otherwise derive
// from the container's own name.
func runContainersClaim(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "containers claim", "print the created app as JSON to stdout and nothing else", stderr)
	var asName string
	fs.StringVar(&asName, "as", "", "app name to create (default: derived from the container's own name)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s containers claim <name> [--as app-name] [flags]\n\nAdopts an orphaned (not Levelrail-managed) container into a new\nLevelrail app, built from the container's own image. Does not adopt\nthe container's live state: the new app starts its own container.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "containers claim", "container name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	app, err := client.ClaimOrphanedContainer(context.Background(), name, claimOrphanedContainerRequest{Name: asName})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("claim container %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, app, func() {
		_, _ = fmt.Fprintf(stdout, "container %q claimed as app %q\n", name, app.Name)
	})
}

func printContainersHuman(out io.Writer, containers []containerResource) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "STATUS\tNAME\tIMAGE\tPORTS")
	for _, c := range containers {
		status := "stopped"
		if c.Running {
			status = "running"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", status, c.Name, c.Image, formatContainerPorts(c.Ports))
	}
	_ = tw.Flush()
}

func formatContainerPorts(ports []containerPortResource) string {
	if len(ports) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		parts = append(parts, fmt.Sprintf("%d->%d/%s", p.HostPort, p.ContainerPort, p.Protocol))
	}
	return strings.Join(parts, ", ")
}

func containersUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s containers [list] [flags]                       list every container on this node, managed or not
  %[1]s containers stop <name> [flags]                   stop an orphaned container
  %[1]s containers remove <name> [flags]                 remove an orphaned container
  %[1]s containers claim <name> [--as app-name] [flags]  adopt an orphaned container into a new app
  %[1]s containers orphans [flags]                       leftover containers and volumes, with grace status
  %[1]s containers reap [--dry-run] [flags]              remove due leftovers now (the reaper also runs on a schedule)

stop/remove/claim only ever act on a container Levelrail does not
already manage (409 otherwise); for a %[1]s-managed app, use
"%[1]s apps stop/start/restart/delete <name>" instead, which update
desired state correctly rather than fighting the reconciler.

Run "%[1]s containers <subcommand> -h" for a subcommand's own flags.

Flags:
  --token string       API token (default: %[2]s env var, then the credentials file)
  --api-url string    control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string    named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                  print the result as JSON to stdout, nothing else
  --output string        output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string         JMESPath expression to filter the result before printing
  -h, --help            show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
