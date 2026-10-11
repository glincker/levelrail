package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runTemplatesUpdates implements "templates updates": GET
// /api/v1/service-templates/updates. Read-only: it reports newer upstream
// image tags for every app installed from a catalog template and never
// redeploys anything.
func runTemplatesUpdates(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	cmd, code, ok := parseComposeCmd(prog, "templates updates", "templates updates", args, 0, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	report, err := cmd.client.ServiceTemplateUpdates(context.Background())
	if err != nil {
		return reportError(stdout, stderr, cmd.json, fmt.Errorf("check template updates: %w", err))
	}
	return renderCompose(stdout, stderr, cmd, report, func() { printTemplateUpdates(stdout, report) })
}

func printTemplateUpdates(out io.Writer, report apiclient.TemplateUpdateReport) {
	if len(report.Apps) == 0 {
		_, _ = fmt.Fprintln(out, "no apps installed from a template")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "APP\tTEMPLATE\tSERVICE\tIMAGE\tSTATUS\tNEWER TAGS")
	for _, app := range report.Apps {
		for _, s := range app.Services {
			detail := dashJoin(s.NewerTags)
			if s.Note != "" && len(s.NewerTags) == 0 {
				detail = s.Note
			}
			if s.CatalogImage != "" {
				detail += " (catalog now pins " + s.CatalogImage + ")"
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", app.App, app.TemplateID, s.Service, s.Image, s.Status, detail)
		}
	}
	_ = tw.Flush()
}

// printTemplateInstallInfo adds an installable template's services,
// settings, domains and source to "templates get".
func printTemplateInstallInfo(out io.Writer, t serviceTemplateDetail) {
	if t.Manifest == nil && len(t.Stack) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, "services:")
	for _, s := range t.Stack {
		ports := make([]string, len(s.Ports))
		for i, p := range s.Ports {
			ports[i] = fmt.Sprint(p)
		}
		_, _ = fmt.Fprintf(out, "  %s  %s  ports=%s  volumes=%s\n", s.Name, s.Image, dashJoin(ports), dashJoin(s.Volumes))
	}
	if t.Manifest == nil {
		return
	}
	if len(t.Manifest.Env) > 0 {
		_, _ = fmt.Fprintln(out, "settings:")
		for _, e := range t.Manifest.Env {
			_, _ = fmt.Fprintf(out, "  %s  %s\n", e.Key, settingKind(e))
		}
	}
	for _, d := range t.Manifest.Domains {
		_, _ = fmt.Fprintf(out, "domain:  --domain %s=HOST (port %d)\n", d.Service, d.Port)
	}
	if src := t.Manifest.Source; src != nil {
		_, _ = fmt.Fprintf(out, "source:  %s (%s), licence %s\n", src.Name, src.Path, src.License)
	}
}

func settingKind(e apiclient.TemplateSetting) string {
	switch {
	case e.Generate != "":
		return "generated " + e.Generate
	case e.Required:
		return "required"
	case e.Default != "":
		return "optional, default " + strings.TrimSpace(e.Default)
	default:
		return "optional"
	}
}
