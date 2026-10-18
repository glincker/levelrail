package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const dashboardsHelpFlags = `
Flags:
  --token string     API token (default: %[1]s env var, then the credentials file)
  --api-url string   control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string   named credentials profile to read
  --json             print the result as JSON to stdout, nothing else
  --output string    output format: json, table, or text
  --query string     JMESPath expression to filter the result before printing
  -h, --help         show this help
`

func dashboardsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s dashboards list [--project ID] [flags]     list the dashboards you can see (yours plus shared)
  %[1]s dashboards show <id|name> [flags]          print one dashboard with its widgets
  %[1]s dashboards create --name NAME [--template KEY --target NAME] [--scope user|shared] [--project ID] [--default] [flags]
  %[1]s dashboards export <id|name> [flags]        print the portable JSON document, ready to commit to git
  %[1]s dashboards import <file|-> [--scope user|shared] [--project ID] [flags]   create a dashboard from an exported document
  %[1]s dashboards delete <id|name> [flags]        delete a dashboard

Templates: app-health, fleet-overview, cost. App templates need --target APP.
`+dashboardsHelpFlags, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runDashboards(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, dashboardsUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, dashboardsUsage(prog))
		return exitOK
	case "list":
		return runDashboardsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "show":
		return runDashboardsShow(prog, args[1:], stdout, stderr, lookupEnv)
	case "create":
		return runDashboardsCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "export":
		return runDashboardsExport(prog, args[1:], stdout, stderr, lookupEnv)
	case "import":
		return runDashboardsImport(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runDashboardsDelete(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown dashboards subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, dashboardsUsage(prog))
		return exitUsage
	}
}

// resolveDashboardID accepts an id or a name so operators never copy ids by hand.
func resolveDashboardID(ctx context.Context, client *Client, ref string) (string, error) {
	rows, err := client.ListDashboards(ctx, "")
	if err != nil {
		return "", err
	}
	for _, d := range rows {
		if d.ID == ref || d.Name == ref {
			return d.ID, nil
		}
	}
	return "", fmt.Errorf("dashboard %q not found", ref)
}

type dashboardsCmd struct {
	client  *Client
	jsonOut bool
	of      outputFlags
}

func (c dashboardsCmd) render(stdout, stderr io.Writer, data any, table func()) int {
	if err := renderResult(stdout, c.of.Format, c.of.Query, data, table); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func (c dashboardsCmd) fail(stdout, stderr io.Writer, err error) int {
	return reportError(stdout, stderr, c.jsonOut, err)
}

func runDashboardsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profP, jsonP, outP, qP := apiFlagSet(prog, "dashboards list", "print dashboards as JSON", stderr)
	project := fs.String("project", "", "only dashboards of this project id")
	token, api, prof, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profP, jsonP, outP, qP}, prog, stderr)
	if !ok {
		return code
	}
	c := dashboardsCmd{client: apiClientFromFlags(prog, api, token, prof, lookupEnv), jsonOut: jsonOut, of: of}
	rows, err := c.client.ListDashboards(context.Background(), *project)
	if err != nil {
		return c.fail(stdout, stderr, fmt.Errorf("list dashboards: %w", err))
	}
	return c.render(stdout, stderr, rows, func() { printDashboardsTable(stdout, rows) })
}

func printDashboardsTable(out io.Writer, rows []apiclient.DashboardResource) {
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(out, "no dashboards")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tSCOPE\tPROJECT\tDEFAULT\tUPDATED")
	for _, d := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%t\t%s\n", d.ID, d.Name, d.Scope, d.ProjectID, d.IsDefault, d.UpdatedAt.Format("2006-01-02 15:04"))
	}
	_ = tw.Flush()
}

func runDashboardsShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profP, jsonP, outP, qP := apiFlagSet(prog, "dashboards show", "print the dashboard as JSON", stderr)
	token, api, prof, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profP, jsonP, outP, qP}, prog, stderr)
	if !ok {
		return code
	}
	ref, ok := requireOneArg(fs, stderr, prog, "dashboards show", "dashboard id or name")
	if !ok {
		return exitUsage
	}
	c := dashboardsCmd{client: apiClientFromFlags(prog, api, token, prof, lookupEnv), jsonOut: jsonOut, of: of}
	id, err := resolveDashboardID(context.Background(), c.client, ref)
	if err != nil {
		return c.fail(stdout, stderr, fmt.Errorf("show dashboard: %w", err))
	}
	d, err := c.client.GetDashboard(context.Background(), id)
	if err != nil {
		return c.fail(stdout, stderr, fmt.Errorf("show dashboard %q: %w", ref, err))
	}
	return c.render(stdout, stderr, d, func() {
		printDashboardsTable(stdout, []apiclient.DashboardResource{d})
		_, _ = fmt.Fprintln(stdout)
		_, _ = stdout.Write(prettyJSON(d.Doc))
	})
}

func prettyJSON(raw json.RawMessage) []byte {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return append([]byte(raw), '\n')
	}
	buf.WriteByte('\n')
	return buf.Bytes()
}

func runDashboardsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profP, jsonP, outP, qP := apiFlagSet(prog, "dashboards create", "print the new dashboard as JSON", stderr)
	name := fs.String("name", "", "dashboard name (defaults to the template's name)")
	tpl := fs.String("template", "", "start from a template: app-health, fleet-overview or cost")
	target := fs.String("target", "", "app the template's widgets read from")
	scope := fs.String("scope", "user", "user (private) or shared (whole organization)")
	project := fs.String("project", "", "project id the dashboard belongs to")
	def := fs.Bool("default", false, "make this the project's default dashboard (shared, needs --project)")
	token, api, prof, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profP, jsonP, outP, qP}, prog, stderr)
	if !ok {
		return code
	}
	c := dashboardsCmd{client: apiClientFromFlags(prog, api, token, prof, lookupEnv), jsonOut: jsonOut, of: of}
	ctx := context.Background()
	doc, err := dashboardDocFromTemplate(ctx, c.client, *tpl, *target, *name)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
		return exitValidation
	}
	d, err := c.client.CreateDashboard(ctx, apiclient.DashboardWrite{Scope: *scope, ProjectID: *project, IsDefault: *def, Doc: doc})
	if err != nil {
		return c.fail(stdout, stderr, fmt.Errorf("create dashboard: %w", err))
	}
	return c.render(stdout, stderr, d, func() { _, _ = fmt.Fprintf(stdout, "dashboard %q created (%s)\n", d.Name, d.ID) })
}

func dashboardDocFromTemplate(ctx context.Context, client *Client, key, target, name string) (json.RawMessage, error) {
	if key == "" {
		if name == "" {
			return nil, fmt.Errorf("--name or --template is required")
		}
		return json.Marshal(map[string]any{"version": 1, "name": name, "widgets": []any{}})
	}
	tpls, err := client.DashboardTemplates(ctx)
	if err != nil {
		return nil, fmt.Errorf("load templates: %w", err)
	}
	for _, t := range tpls {
		if t.Key != key {
			continue
		}
		if t.NeedsTarget != "" && target == "" {
			return nil, fmt.Errorf("template %q needs --target %s", key, t.NeedsTarget)
		}
		var doc map[string]json.RawMessage
		if err := json.Unmarshal(t.Doc, &doc); err != nil {
			return nil, fmt.Errorf("decode template: %w", err)
		}
		if name != "" {
			doc["name"], _ = json.Marshal(name)
		}
		var widgets []map[string]json.RawMessage
		if err := json.Unmarshal(doc["widgets"], &widgets); err != nil {
			return nil, fmt.Errorf("decode template widgets: %w", err)
		}
		for _, w := range widgets {
			if raw, has := w["target"]; has {
				var tg map[string]string
				if json.Unmarshal(raw, &tg) == nil {
					tg["name"] = target
					w["target"], _ = json.Marshal(tg)
				}
			}
		}
		doc["widgets"], _ = json.Marshal(widgets)
		return json.Marshal(doc)
	}
	return nil, fmt.Errorf("unknown template %q", key)
}

func runDashboardsExport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profP, jsonP, outP, qP := apiFlagSet(prog, "dashboards export", "accepted for symmetry, export always prints JSON", stderr)
	token, api, prof, jsonOut, _, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profP, jsonP, outP, qP}, prog, stderr)
	if !ok {
		return code
	}
	ref, ok := requireOneArg(fs, stderr, prog, "dashboards export", "dashboard id or name")
	if !ok {
		return exitUsage
	}
	c := dashboardsCmd{client: apiClientFromFlags(prog, api, token, prof, lookupEnv), jsonOut: jsonOut}
	id, err := resolveDashboardID(context.Background(), c.client, ref)
	if err != nil {
		return c.fail(stdout, stderr, fmt.Errorf("export dashboard: %w", err))
	}
	doc, err := c.client.ExportDashboard(context.Background(), id)
	if err != nil {
		return c.fail(stdout, stderr, fmt.Errorf("export dashboard %q: %w", ref, err))
	}
	_, _ = stdout.Write(prettyJSON(doc))
	return exitOK
}

func runDashboardsImport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profP, jsonP, outP, qP := apiFlagSet(prog, "dashboards import", "print the new dashboard as JSON", stderr)
	scope := fs.String("scope", "user", "user (private) or shared (whole organization)")
	project := fs.String("project", "", "project id the dashboard belongs to")
	token, api, prof, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profP, jsonP, outP, qP}, prog, stderr)
	if !ok {
		return code
	}
	path, ok := requireOneArg(fs, stderr, prog, "dashboards import", "file path or -")
	if !ok {
		return exitUsage
	}
	var raw []byte
	var err error
	if path == "-" {
		raw, err = io.ReadAll(os.Stdin)
	} else {
		raw, err = os.ReadFile(path) //nolint:gosec // the operator names the file to import
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: read %s: %v\n", prog, path, err)
		return exitValidation
	}
	c := dashboardsCmd{client: apiClientFromFlags(prog, api, token, prof, lookupEnv), jsonOut: jsonOut, of: of}
	d, err := c.client.ImportDashboard(context.Background(), raw, *scope, *project)
	if err != nil {
		return c.fail(stdout, stderr, fmt.Errorf("import dashboard: %w", err))
	}
	return c.render(stdout, stderr, d, func() { _, _ = fmt.Fprintf(stdout, "dashboard %q imported (%s)\n", d.Name, d.ID) })
}

func runDashboardsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profP, jsonP, outP, qP := apiFlagSet(prog, "dashboards delete", "print {\"deleted\": true} as JSON", stderr)
	token, api, prof, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profP, jsonP, outP, qP}, prog, stderr)
	if !ok {
		return code
	}
	ref, ok := requireOneArg(fs, stderr, prog, "dashboards delete", "dashboard id or name")
	if !ok {
		return exitUsage
	}
	c := dashboardsCmd{client: apiClientFromFlags(prog, api, token, prof, lookupEnv), jsonOut: jsonOut, of: of}
	id, err := resolveDashboardID(context.Background(), c.client, ref)
	if err != nil {
		return c.fail(stdout, stderr, fmt.Errorf("delete dashboard: %w", err))
	}
	if err := c.client.DeleteDashboard(context.Background(), id); err != nil {
		return c.fail(stdout, stderr, fmt.Errorf("delete dashboard %q: %w", ref, err))
	}
	return c.render(stdout, stderr, map[string]bool{"deleted": true}, func() { _, _ = fmt.Fprintf(stdout, "dashboard %q deleted\n", ref) })
}
