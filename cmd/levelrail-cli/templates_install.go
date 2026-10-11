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
	var name string
	env := stringMapFlag{}
	domains := stringMapFlag{}
	cmd, code, ok := parseComposeCmd(prog, "templates install", "templates install <id> [--name NAME] [--env KEY=VALUE]... [--domain SERVICE=HOST]...", args, 1, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&name, "name", "", "app name (default: the template id plus a short random suffix)")
		fs.Var(env, "env", "template setting KEY=VALUE (repeatable; see templates get <id>)")
		fs.Var(domains, "domain", "put a service behind a domain, SERVICE=HOST (repeatable)")
	})
	if !ok {
		return code
	}
	res, err := cmd.client.InstallTemplate(context.Background(), cmd.args[0], apiclient.TemplateInstallRequest{Name: name, Env: env, Domains: domains})
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
