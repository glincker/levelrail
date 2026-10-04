package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

func appsStreamsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps streams list <name> [flags]                                     list an app's TCP streams
  %[1]s apps streams create <name> --container-port N --host-port N [flags]  forward a host port to one container port
  %[1]s apps streams delete <name> <id> [flags]                              remove a TCP stream

A stream forwards host-port on this control plane's own host, as raw
TCP, to container-port on the app's container. Only tcp is supported
today. Creating or deleting a stream takes effect on the app's next
container recreation (an image change, or "%[1]s apps restart <name>"),
not on an already-running container: Docker has no way to add a
published port to a running container.

Run "%[1]s apps streams <subcommand> -h" for a subcommand's own flags.
`, prog)
}

// runAppsStreams dispatches "apps streams list|create|delete" to
// POST/GET/DELETE /api/v1/apps/{name}/streams(/{id}), the same per-app
// sub-resource shape apps_domains.go's own dispatcher establishes.
func runAppsStreams(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsStreamsUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsStreamsUsage(prog))
		return exitOK
	case "list":
		return runAppsStreamsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "create":
		return runAppsStreamsCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runAppsStreamsDelete(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps streams subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsStreamsUsage(prog))
		return exitUsage
	}
}

func runAppsStreamsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps streams list", "print the streams as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps streams list <name> [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps streams list", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	streams, err := client.ListAppStreams(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list streams for app %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, streams, func() { printAppStreamsTable(stdout, streams) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printAppStreamsTable(out io.Writer, streams []appStreamResource) {
	if len(streams) == 0 {
		_, _ = fmt.Fprintln(out, "no streams")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tHOST PORT\tCONTAINER PORT\tPROTOCOL\tCREATED")
	for _, s := range streams {
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\t%s\t%s\n", s.ID, s.HostPort, s.ContainerPort, s.Protocol, s.CreatedAt)
	}
	_ = tw.Flush()
}

func runAppsStreamsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps streams create", "print the created stream as JSON to stdout and nothing else", stderr)
	var containerPort, hostPort int
	var protocol string
	fs.IntVar(&containerPort, "container-port", 0, "port the container listens on (required)")
	fs.IntVar(&hostPort, "host-port", 0, "port on this control plane's own host to forward, 1-65535 (required)")
	fs.StringVar(&protocol, "protocol", "tcp", "only \"tcp\" is supported today")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsStreamsCreateUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "apps streams create", "app name")
	if !ok {
		return exitUsage
	}
	if containerPort < 1 || containerPort > 65535 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--container-port is required and must be between 1 and 65535"))
	}
	if hostPort < 1 || hostPort > 65535 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--host-port is required and must be between 1 and 65535"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	created, err := client.CreateAppStream(context.Background(), name, createAppStreamRequest{
		ContainerPort: containerPort, HostPort: hostPort, Protocol: protocol,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create stream on app %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, created, func() {
		_, _ = fmt.Fprintf(stdout, "stream %q created: host port %d -> app %q container port %d\n", created.ID, created.HostPort, name, created.ContainerPort)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func appsStreamsCreateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps streams create <name> --container-port N --host-port N [flags]

Forwards host-port on this control plane's own host, as raw TCP, to
container-port on app <name>'s container. Takes effect on the app's
next container recreation, not immediately on an already-running one.

Flags:
  --container-port int     port the container listens on (required)
  --host-port int          port on this host to forward, 1-65535 (required)
  --protocol string        only "tcp" is supported today (default "tcp")
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the created stream as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runAppsStreamsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps streams delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsStreamsDeleteUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: apps streams delete requires an app name and a stream id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name, id := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.DeleteAppStream(context.Background(), name, id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete stream %q on app %q: %w", id, name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "stream %q removed from app %q\n", id, name)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func appsStreamsDeleteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps streams delete <name> <id> [flags]

Removes a TCP stream. Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print {"deleted": true} as JSON to stdout on success, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
