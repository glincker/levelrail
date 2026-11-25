package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runTemplatesInstall implements "templates install <id>": renders a
// manifest template with --env and --domain values and deploys it as a
// compose stack (POST /api/v1/service-templates/{id}/install).
func runTemplatesInstall(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var name, project string
	env := stringMapFlag{}
	domains := stringMapFlag{}
	cmd, code, ok := parseComposeCmd(prog, "templates install", "templates install <id> [--name NAME] [--project ID] [--env KEY=VALUE]... [--domain SERVICE=HOST]...", args, 1, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&name, "name", "", "app name (default: the template id plus a short random suffix)")
		fs.StringVar(&project, "project", "", "group the installed services under this project id")
		fs.Var(env, "env", "template setting KEY=VALUE (repeatable; see templates get <id>)")
		fs.Var(domains, "domain", "put a service behind a domain, SERVICE=HOST (repeatable)")
	})
	if !ok {
		return code
	}
	warnTemplateNeedsDomain(context.Background(), cmd.client, stderr, prog, cmd.args[0], len(domains) > 0)
	res, err := cmd.client.InstallTemplate(context.Background(), cmd.args[0], apiclient.TemplateInstallRequest{Name: name, Env: env, Domains: domains, ProjectID: project})
	if err != nil {
		return reportError(stdout, stderr, cmd.json, fmt.Errorf("install template %q: %w", cmd.args[0], err))
	}
	return renderCompose(stdout, stderr, cmd, res, func() {
		_, _ = fmt.Fprintf(stdout, "%s: installed from %s, %d service(s)\n", res.AppID, cmd.args[0], len(res.Services))
		for _, s := range res.Services {
			_, _ = fmt.Fprintf(stdout, "  %s\t%s\n", s.Name, s.Image)
		}
		_, _ = fmt.Fprintf(stdout, "watch it converge: %s compose status %s\n", prog, res.AppID)
	})
}

// warnTemplateNeedsDomain warns, never refuses: a lookup failure or a bare-IP test install must still go through.
func warnTemplateNeedsDomain(ctx context.Context, client *Client, stderr io.Writer, prog, id string, haveDomain bool) {
	if haveDomain {
		return
	}
	detail, err := client.GetServiceTemplate(ctx, id)
	if err != nil || !detail.NeedsDomain {
		return
	}
	_, _ = fmt.Fprintf(stderr, "warning: template %q needs a domain but none was given; it installs anyway, add one with --domain SERVICE=HOST (see %s templates get %s)\n", id, prog, id)
}
