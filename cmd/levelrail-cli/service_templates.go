package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runServiceTemplates dispatches "service-templates <verb> [flags]":
// "list" and "get <id>" against GET /api/v1/service-templates[/{id}]
// (internal/api/service_templates.go), the curated one-click template
// catalog.
func runServiceTemplates(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, serviceTemplatesUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, serviceTemplatesUsage(prog))
		return exitOK
	case "list":
		return runServiceTemplatesList(prog, args[1:], stdout, stderr, lookupEnv)
	case "get":
		return runServiceTemplatesGet(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown service-templates subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, serviceTemplatesUsage(prog))
		return exitUsage
	}
}

func serviceTemplatesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s service-templates list [flags]        list the curated one-click template catalog
  %[1]s service-templates get <id> [flags]    show one template, including its full compose body

Run "%[1]s service-templates <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runServiceTemplatesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "service-templates list", "print templates as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, serviceTemplatesListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	templates, err := client.ListServiceTemplates(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list service templates: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, templates, func() { printServiceTemplatesTable(stdout, templates) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printServiceTemplatesTable(out io.Writer, templates []serviceTemplateListItem) {
	if len(templates) == 0 {
		_, _ = fmt.Fprintln(out, "no service templates")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tCATEGORY")
	for _, t := range templates {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", t.ID, t.Name, t.Category)
	}
	_ = tw.Flush()
}

func serviceTemplatesListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s service-templates list [flags]

Lists the curated one-click template catalog, without each entry's compose body.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print templates as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runServiceTemplatesGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "service-templates get", "print the template as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, serviceTemplatesGetUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "service-templates get", "template id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	tpl, err := client.GetServiceTemplate(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get service template %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, tpl, func() { printServiceTemplateHuman(stdout, tpl) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printServiceTemplateHuman(out io.Writer, t serviceTemplateDetail) {
	_, _ = fmt.Fprintf(out, "id:                %s\n", t.ID)
	_, _ = fmt.Fprintf(out, "name:              %s\n", t.Name)
	_, _ = fmt.Fprintf(out, "slogan:            %s\n", t.Slogan)
	_, _ = fmt.Fprintf(out, "category:          %s\n", t.Category)
	_, _ = fmt.Fprintf(out, "documentation_url: %s\n", t.DocumentationURL)
	_, _ = fmt.Fprintln(out, "compose:")
	_, _ = fmt.Fprintln(out, t.Compose)
}

func serviceTemplatesGetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s service-templates get <id> [flags]

Shows one service template, including its full compose body.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the template as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
