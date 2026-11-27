package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const templatesSourcesUsage = `Usage:
  %[1]s templates sources add [--ref REF] [--deploy-key-app APP] <name> <git-url>
  %[1]s templates sources ls [<name>]
  %[1]s templates sources sync <name>
  %[1]s templates sources rm <name>

Private template sources are git repos of templates. Root only, and off until
the templates.sources_enabled setting is on. Only https and ssh URLs to public
hosts are accepted. An ssh source reuses the stored deploy key of the app named
by --deploy-key-app; the key is never shown. Templates appear as <name>/<id> and
never replace a built-in one. Put flags before the name.
`

// runTemplatesSources dispatches "templates sources <add|ls|sync|rm>".
func runTemplatesSources(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, templatesSourcesUsage, prog)
		return exitUsage
	}
	switch args[0] {
	case "add":
		return runTemplatesSourcesAdd(prog, args[1:], stdout, stderr, lookupEnv)
	case "ls", "list":
		return runTemplatesSourcesList(prog, args[1:], stdout, stderr, lookupEnv)
	case "sync":
		return runTemplatesSourcesSync(prog, args[1:], stdout, stderr, lookupEnv)
	case "rm", "remove":
		return runTemplatesSourcesRemove(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, templatesSourcesUsage, prog)
		return exitUsage
	}
}

func runTemplatesSourcesAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "templates sources add", "print the source as JSON to stdout and nothing else", stderr)
	var ref, keyApp string
	fs.StringVar(&ref, "ref", "", "branch or tag to sync (default: the repo's default branch)")
	fs.StringVar(&keyApp, "deploy-key-app", "", "app whose stored ssh deploy key an ssh source uses")
	fs.Usage = func() { _, _ = fmt.Fprintf(stderr, templatesSourcesUsage, prog) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	src, err := client.CreateTemplateSource(context.Background(), apiclient.TemplateSourceRequest{
		Name: fs.Arg(0), GitURL: fs.Arg(1), Ref: ref, DeployKeyApp: keyApp,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("add template source: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, src, func() {
		_, _ = fmt.Fprintf(stdout, "added template source %s (run \"templates sources sync %s\" to fetch it)\n", src.Name, src.Name)
	})
}

func runTemplatesSourcesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "templates sources ls", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprintf(stderr, templatesSourcesUsage, prog) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if fs.NArg() == 1 {
		entries, err := client.ListTemplateSourceEntries(context.Background(), fs.Arg(0))
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list source templates: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, entries, func() {
			for _, e := range entries {
				_, _ = fmt.Fprintf(stdout, "%s\t%s\n", e.ID, e.Name)
			}
			if len(entries) == 0 {
				_, _ = fmt.Fprintln(stdout, "no templates synced from this source")
			}
		})
	}
	rows, err := client.ListTemplateSources(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list template sources: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, rows, func() { printTemplateSourcesTable(stdout, rows) })
}

func printTemplateSourcesTable(out io.Writer, rows []apiclient.TemplateSource) {
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(out, "no template sources")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tURL\tREF\tSTATUS\tTEMPLATES\tLAST SYNC")
	for _, r := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n", r.Name, r.GitURL, r.Ref, r.Status, r.Templates, r.LastSync)
	}
	_ = tw.Flush()
}

func runTemplatesSourcesSync(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "templates sources sync", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprintf(stderr, templatesSourcesUsage, prog) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.SyncTemplateSource(context.Background(), fs.Arg(0))
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("sync template source: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "synced %s: %d templates, status %s\n", res.Source.Name, res.Source.Templates, res.Source.Status)
		for _, s := range res.Skipped {
			_, _ = fmt.Fprintf(stdout, "  refused: %s\n", s)
		}
	})
}

func runTemplatesSourcesRemove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "templates sources rm", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprintf(stderr, templatesSourcesUsage, prog) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.DeleteTemplateSource(context.Background(), fs.Arg(0)); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("remove template source: %w", err))
	}
	_, _ = fmt.Fprintf(stdout, "removed template source %s\n", fs.Arg(0))
	return exitOK
}
