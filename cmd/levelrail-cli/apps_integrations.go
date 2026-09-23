package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// runAppsIntegrations dispatches "apps integrations <verb> [flags]" to
// one of catalog/list/add/remove, mirroring runAppsSecrets' own dispatch
// shape for a different app-scoped resource.
func runAppsIntegrations(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsIntegrationsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsIntegrationsUsage(prog))
		return exitOK
	case "catalog":
		return runAppsIntegrationsCatalog(prog, args[1:], stdout, stderr, lookupEnv)
	case "list":
		return runAppsIntegrationsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "add":
		return runAppsIntegrationsAdd(prog, args[1:], stdout, stderr, lookupEnv)
	case "remove":
		return runAppsIntegrationsRemove(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps integrations subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsIntegrationsUsage(prog))
		return exitUsage
	}
}

func appsIntegrationsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps integrations catalog [flags]                                     list every available integration
  %[1]s apps integrations list <name> [flags]                                 list an app's attached integrations
  %[1]s apps integrations add <name> <key> [--field NAME=VALUE ...] [flags]   attach a catalog integration
  %[1]s apps integrations remove <name> <id> [flags]                          detach an attached integration

Field values (API keys, DSNs) are stored through the same envelope
encryption every app secret uses, and are never printed back.

Run "%[1]s apps integrations <subcommand> -h" for a subcommand's own flags.
`, prog)
}

// fieldFlags is a repeatable --field NAME=VALUE flag, accumulated into a
// map for AttachAppIntegration's own Fields parameter.
type fieldFlags map[string]string

func (f fieldFlags) String() string {
	return ""
}

func (f fieldFlags) Set(raw string) error {
	name, value, ok := strings.Cut(raw, "=")
	if !ok || strings.TrimSpace(name) == "" {
		return fmt.Errorf("must be NAME=VALUE, got %q", raw)
	}
	f[strings.TrimSpace(name)] = value
	return nil
}

func runAppsIntegrationsCatalog(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps integrations catalog", "print the catalog as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps integrations catalog [flags]\n\nLists every integration internal/integrations knows how to attach.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, _, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	catalog, err := client.ListIntegrationCatalog(context.Background())
	if err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("list integration catalog: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, catalog, func() { printIntegrationCatalogHuman(stdout, catalog) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runAppsIntegrationsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps integrations list", "print the attached integrations as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps integrations list <name> [flags]\n\nLists an app's attached integrations.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps integrations list", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	attached, err := client.ListAppIntegrations(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list integrations for app %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, attached, func() { printAppIntegrationsHuman(stdout, attached) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runAppsIntegrationsAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps integrations add", "print the attached integration as JSON to stdout and nothing else", stderr)
	fields := fieldFlags{}
	fs.Var(fields, "field", "a NAME=VALUE field the integration needs (repeatable), e.g. --field SENTRY_DSN=https://...")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps integrations add <name> <key> [--field NAME=VALUE ...] [flags]\n\nAttaches a catalog integration (see \"apps integrations catalog\" for\nvalid keys and required fields) to an app.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag := *tokenFlagP, *apiURLFlagP, *profileFlagP
	format, ferr := resolveOutputFormat(*jsonOutP, *outputFlagP)
	if ferr != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, ferr)
		return exitValidation
	}

	rest, ok := requireArgs(fs, stderr, prog, "apps integrations add", "an app name and an integration key", 2)
	if !ok {
		return exitUsage
	}
	name, key := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	attached, err := client.AttachAppIntegration(context.Background(), name, key, fields)
	if err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("attach integration %q to app %q: %w", key, name, err))
	}

	if err := renderResult(stdout, format, *queryFlagP, attached, func() {
		_, _ = fmt.Fprintf(stdout, "integration %q attached to app %q (id %s)\n", attached.IntegrationKey, name, attached.ID)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runAppsIntegrationsRemove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, _, _, _ := apiFlagSet(prog, "apps integrations remove", "unused for this subcommand", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps integrations remove <name> <id> [flags]\n\nDetaches an attached integration and clears its stored field values.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag := *tokenFlagP, *apiURLFlagP, *profileFlagP

	rest, ok := requireArgs(fs, stderr, prog, "apps integrations remove", "an app name and an attachment id", 2)
	if !ok {
		return exitUsage
	}
	name, id := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.DetachAppIntegration(context.Background(), name, id); err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("detach integration %q from app %q: %w", id, name, err))
	}
	_, _ = fmt.Fprintf(stdout, "integration %q detached from app %q\n", id, name)
	return exitOK
}

func printIntegrationCatalogHuman(out io.Writer, catalog []integrationCatalogEntryResource) {
	if len(catalog) == 0 {
		_, _ = fmt.Fprintln(out, "no integrations available")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KEY\tNAME\tENV VARS")
	for _, entry := range catalog {
		names := make([]string, len(entry.EnvVars))
		for i, v := range entry.EnvVars {
			names[i] = v.Name
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", entry.Key, entry.Name, strings.Join(names, ", "))
	}
	_ = tw.Flush()
}

func printAppIntegrationsHuman(out io.Writer, attached []appIntegrationResource) {
	if len(attached) == 0 {
		_, _ = fmt.Fprintln(out, "no integrations attached")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tKEY\tNAME")
	for _, ai := range attached {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", ai.ID, ai.IntegrationKey, ai.Name)
	}
	_ = tw.Flush()
}
