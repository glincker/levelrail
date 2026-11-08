package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runTemplatesInstall implements "templates install <id>": renders a
// manifest template with --env and --domain values and deploys it as a
// compose stack (POST /api/v1/service-templates/{id}/install).
func runTemplatesInstall(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var name string
	var dryRun bool
	env := stringMapFlag{}
	domains := stringMapFlag{}
	cmd, code, ok := parseComposeCmd(prog, "templates install", "templates install <id> [--name NAME] [--env KEY=VALUE]... [--domain SERVICE=HOST]... [--dry-run]", args, 1, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&name, "name", "", "app name (default: the template id plus a short random suffix)")
		fs.Var(env, "env", "template setting KEY=VALUE (repeatable; see templates get <id>)")
		fs.BoolVar(&dryRun, "dry-run", false, "print the rendered plan and missing settings without deploying")
		fs.Var(domains, "domain", "put a service behind a domain, SERVICE=HOST (repeatable)")
	})
	if !ok {
		return code
	}
	if dryRun {
		return runTemplatesInstallPlan(cmd, name, env, domains, stdout, stderr)
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

func runTemplatesInstallPlan(cmd composeCmd, name string, env, domains stringMapFlag, stdout, stderr io.Writer) int {
	plan, err := cmd.client.PlanTemplateInstall(context.Background(), cmd.args[0], apiclient.TemplateInstallRequest{Name: name, Env: env, Domains: domains})
	if err != nil {
		return reportError(stdout, stderr, cmd.json, fmt.Errorf("plan template %q: %w", cmd.args[0], err))
	}
	return renderCompose(stdout, stderr, cmd, plan, func() {
		_, _ = fmt.Fprintf(stdout, "dry run: nothing was stored. app name %s\n", plan.Name)
		for _, e := range plan.Env {
			_, _ = fmt.Fprintf(stdout, "  %s\t%s\t%s\n", e.Key, e.Source, e.Value)
		}
		if len(plan.Missing) > 0 {
			_, _ = fmt.Fprintf(stdout, "missing required settings: %s\n", strings.Join(plan.Missing, ", "))
		}
		_, _ = fmt.Fprintf(stdout, "\n%s", plan.Compose)
	})
}
