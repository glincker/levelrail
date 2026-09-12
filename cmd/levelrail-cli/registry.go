package main

import (
	"context"
	"fmt"
	"io"
)

// registryJSONUsage is every registry subcommand's --json flag
// description: identical across get/enable/disable since each returns
// the same settings shape.
const registryJSONUsage = "print the settings as JSON to stdout and nothing else"

// runRegistry dispatches "registry <verb> [flags]" to one of
// status/enable/disable, mirroring runCloudflareTunnel's own get/set/
// disconnect dispatch shape for the API's other platform-singleton
// settings resource. Named "registry", not "registry-settings" or
// similar: "registry-credentials" (cmd/levelrail-cli/registry_credentials.go)
// is a distinct, already-established command group for credentials to an
// external registry an operator already runs elsewhere, so this word is
// free for Levelrail's own built-in one.
func runRegistry(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, registryUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, registryUsage(prog))
		return exitOK
	case "status":
		return runRegistryStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "enable":
		return runRegistryEnable(prog, args[1:], stdout, stderr, lookupEnv)
	case "disable":
		return runRegistryDisable(prog, args[1:], stdout, stderr, lookupEnv)
	case "repositories":
		return runRegistryRepositories(prog, args[1:], stdout, stderr, lookupEnv)
	case "tags":
		return runRegistryTags(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown registry subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, registryUsage(prog))
		return exitUsage
	}
}

func registryUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s registry status [flags]                    show the current settings and container status
  %[1]s registry enable --host HOST [flags]        provision and enable the built-in container registry
  %[1]s registry disable [flags]                   disable the registry and forget its generated credentials
  %[1]s registry repositories [flags]              list every repository pushed to the built-in registry
  %[1]s registry tags --repository NAME [flags]    list every tag pushed for one repository

Runs Levelrail's own built-in image registry (registry:2), so a
multi-node deployment gets a build cache/distribution backend without
signing up for an external registry first. The first "enable" call
generates a username and password; the password is printed once, in
that call's own response, and never shown again. This is a distinct
resource from "%[1]s registry-credentials", which stores credentials to
an external registry an operator already runs elsewhere.

Run "%[1]s registry <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runRegistryRepositories(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "registry repositories", "print repositories as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s registry repositories [flags]\n\nLists every repository pushed to the built-in registry.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	repos, err := client.ListRegistryRepositories(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list registry repositories: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, repos, func() { printRegistryRepositories(stdout, repos) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printRegistryRepositories(out io.Writer, repos registryRepositoriesResource) {
	if len(repos.Repositories) == 0 {
		_, _ = fmt.Fprintln(out, "no repositories")
		return
	}
	for _, name := range repos.Repositories {
		_, _ = fmt.Fprintln(out, name)
	}
}

func runRegistryTags(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "registry tags", "print tags as a JSON array to stdout and nothing else", stderr)
	var repositoryFlag string
	fs.StringVar(&repositoryFlag, "repository", "", "repository name to list tags for (required)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s registry tags --repository NAME [flags]\n\nLists every tag pushed for one repository in the built-in registry.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if repositoryFlag == "" {
		_, _ = fmt.Fprintf(stderr, "%s: --repository is required\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	tags, err := client.ListRegistryTags(context.Background(), repositoryFlag)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list registry tags: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, tags, func() { printRegistryTags(stdout, tags) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printRegistryTags(out io.Writer, tags registryTagsResource) {
	if len(tags.Tags) == 0 {
		_, _ = fmt.Fprintln(out, "no tags")
		return
	}
	for _, tag := range tags.Tags {
		_, _ = fmt.Fprintln(out, tag)
	}
}

func runRegistryStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "registry status", registryJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s registry status [flags]\n\nShows the built-in registry's current settings and container status.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.GetRegistrySettings(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get registry settings: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, settings, func() { printRegistrySettings(stdout, settings) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runRegistryEnable(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "registry enable", registryJSONUsage, stderr)
	var hostFlag string
	fs.StringVar(&hostFlag, "host", "", "hostname the embedded ingress routes to the registry container (required; for multi-node, a WireGuard mesh-resolvable name)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s registry enable --host HOST [flags]\n\nProvisions and enables the built-in container registry. The first call\ngenerates a username and password and prints the password once.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if hostFlag == "" {
		_, _ = fmt.Fprintf(stderr, "%s: --host is required\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.UpdateRegistrySettings(context.Background(), updateRegistrySettingsRequest{Enabled: true, Host: hostFlag})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("enable registry: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, settings, func() { printRegistrySettings(stdout, settings) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runRegistryDisable(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "registry disable", registryJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s registry disable [flags]\n\nDisables the built-in registry and forgets its generated credentials.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.DisableRegistry(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("disable registry: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, settings, func() { printRegistrySettings(stdout, settings) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printRegistrySettings(out io.Writer, s registrySettingsResource) {
	_, _ = fmt.Fprintf(out, "enabled:         %v\n", s.Enabled)
	_, _ = fmt.Fprintf(out, "host:            %s\n", s.Host)
	_, _ = fmt.Fprintf(out, "username:        %s\n", s.Username)
	_, _ = fmt.Fprintf(out, "has_credentials: %v\n", s.HasCredentials)
	_, _ = fmt.Fprintf(out, "status:          %s\n", s.Status)
	if s.Message != "" {
		_, _ = fmt.Fprintf(out, "message:         %s\n", s.Message)
	}
	if s.Password != "" {
		_, _ = fmt.Fprintf(out, "password:        %s\n", s.Password)
		_, _ = fmt.Fprintln(out, "\nThis password is shown once and never again. Store it now, e.g.:")
		_, _ = fmt.Fprintf(out, "  docker login %s -u %s\n", s.Host, s.Username)
	}
}
