package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runTemplates dispatches "templates <verb> [flags]" to one of
// list/get/deploy: internal/api/service_templates.go's curated service
// catalog (ADR 015), wired into the CLI for the first time. "deploy" is
// new surface, not just a wire-up of an existing route: it is a thin
// wrapper around the same POST /api/v1/apps/{name}/compose call "apps
// deploy-compose" already makes, pre-filled from a catalog entry's own
// compose body the same way BrowseTemplatesFields.tsx pre-fills the
// dashboard's create-app wizard, rather than a second deploy mechanism.
// Deliberately its own command instead of an "apps create --template"
// flag: "apps deploy-compose" already established that a compose-shaped
// deploy is its own command, not a mode of "apps create", and apps
// create's flag surface (name/image/repo/file/interactive) is already
// large enough that adding a template path to it would blur, not
// clarify, its usage.
func runTemplates(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, templatesUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, templatesUsage(prog))
		return exitOK
	case "list":
		return runTemplatesList(prog, args[1:], stdout, stderr, lookupEnv)
	case "get":
		return runTemplatesGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "deploy":
		return runTemplatesDeploy(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runTemplatesDelete(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown templates subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, templatesUsage(prog))
		return exitUsage
	}
}

func templatesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s templates list [--custom] [flags]               browse the curated service catalog, or --custom for your own saved templates
  %[1]s templates get <id> [flags]                       show one entry (built-in or custom), including its full compose.yaml
  %[1]s templates deploy <id> [--name NAME] [flags]     deploy a template's compose.yaml as an app, built-in or custom
  %[1]s templates delete <id> [flags]                    delete a custom template (see "apps save-as-template")

"deploy" defaults the app name to <id>; pass --name to deploy under a
different name (e.g. deploying the same template twice). A custom
template id works with every one of these the same as a built-in one,
except "delete": only a custom template can be deleted, the built-in
catalog is read-only.

Run "%[1]s templates <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runTemplatesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "templates list", "print the result as a JSON array to stdout and nothing else", stderr)
	var custom bool
	fs.BoolVar(&custom, "custom", false, "list your own saved templates (apps save-as-template) instead of the built-in catalog")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s templates list [--custom] [flags]\n\nLists every entry in the curated service catalog, without each entry's\ncompose body (see \"templates get\"). --custom lists your own saved\ntemplates instead.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if custom {
		result, err := client.ListCustomTemplates(context.Background())
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list custom templates: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, result, func() { printCustomTemplatesTable(stdout, result) })
	}

	result, err := client.ListServiceTemplates(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list service templates: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printTemplatesTable(stdout, result) })
}

func printTemplatesTable(out io.Writer, templates []serviceTemplateListItem) {
	if len(templates) == 0 {
		_, _ = fmt.Fprintln(out, "no templates")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tCATEGORY\tRAM\tGPU\tSLOGAN")
	for _, t := range templates {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", t.ID, t.Name, t.Category, formatRecommendedMemory(t.RecommendedMemoryBytes), gpuColumn(t.RequiresGPU), t.Slogan)
	}
	_ = tw.Flush()
}

func gpuColumn(requires bool) string {
	if requires {
		return "nvidia"
	}
	return "-"
}

// formatRecommendedMemory renders a Template.RecommendedMemoryBytes value
// (internal/catalog.Template's own doc comment: a static, pre-deploy
// advisory, not checked against any node's real available memory) as a
// human-readable GiB figure, or "-" when a template has none.
func formatRecommendedMemory(bytes int64) string {
	if bytes <= 0 {
		return "-"
	}
	const gib = 1024 * 1024 * 1024
	return fmt.Sprintf("~%.0f GiB", float64(bytes)/gib)
}

func runTemplatesGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "templates get", "print the template as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s templates get <id> [flags]\n\nShows one catalog entry, including its full compose.yaml body.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, id, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "templates get", "template id"}, lookupEnv)
	if !ok {
		return exitCode
	}

	template, err := client.GetServiceTemplate(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get service template %q: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, template, func() { printTemplateDetailHuman(stdout, template) })
}

func printTemplateDetailHuman(out io.Writer, t serviceTemplateDetail) {
	_, _ = fmt.Fprintf(out, "id:                %s\n", t.ID)
	_, _ = fmt.Fprintf(out, "name:              %s\n", t.Name)
	_, _ = fmt.Fprintf(out, "category:          %s\n", t.Category)
	_, _ = fmt.Fprintf(out, "slogan:            %s\n", t.Slogan)
	_, _ = fmt.Fprintf(out, "documentation_url: %s\n", t.DocumentationURL)
	if t.RecommendedMemoryBytes > 0 {
		_, _ = fmt.Fprintf(out, "recommended_ram:   %s\n", formatRecommendedMemory(t.RecommendedMemoryBytes))
	}
	_, _ = fmt.Fprintln(out, "compose:")
	_, _ = fmt.Fprintln(out, t.Compose)
}

func runTemplatesDeploy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "templates deploy", "print the full deploy result as JSON to stdout and nothing else", stderr)
	var nameFlag string
	fs.StringVar(&nameFlag, "name", "", "app name to deploy under (default: the template id)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s templates deploy <id> [--name NAME] [flags]\n\nFetches a catalog entry's compose.yaml and deploys it as an app, one\nmember service per compose service (the same call \"apps deploy-compose\"\nmakes).\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "templates deploy", "template id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	template, err := client.GetServiceTemplate(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get service template %q: %w", id, err))
	}

	name := nameFlag
	if name == "" {
		name = template.ID
	}

	result, err := client.DeployCompose(context.Background(), name, []byte(template.Compose))
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("deploy template %q as app %q: %w", id, name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printComposeDeployResultHuman(stdout, result) })
}

func printCustomTemplatesTable(out io.Writer, templates []customTemplateListItem) {
	if len(templates) == 0 {
		_, _ = fmt.Fprintln(out, "no custom templates (see \"apps save-as-template\")")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tSOURCE_APP\tNEEDS_CONFIG\tDESCRIPTION")
	for _, t := range templates {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%t\t%s\n", t.ID, t.Name, t.SourceApp, t.RequiresConfiguration, t.Description)
	}
	_ = tw.Flush()
}

// runTemplatesDelete implements "templates delete <id>": DELETE
// /api/v1/templates/custom/{id}. Only ever removes an operator-defined
// template (service_templates_custom.go); the built-in catalog
// (internal/catalog) is static and has no delete route at all, so an id
// that resolves there instead fails with a 404 the same as an unknown
// id.
func runTemplatesDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "templates delete", "print {\"deleted\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s templates delete <id> [flags]\n\nDeletes a custom template saved via \"apps save-as-template\". The app it\nwas saved from, and any apps already deployed from it, are unaffected.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "templates delete", "template id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.DeleteCustomTemplate(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete custom template %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "template %q deleted\n", id)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
