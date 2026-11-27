package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runTemplatesInstall implements "templates install <id>": renders a
// manifest template with --env and --domain values and deploys it as a
// compose stack (POST /api/v1/service-templates/{id}/install).
func runTemplatesInstall(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var name, project string
	var allowPlatform bool
	env := stringMapFlag{}
	domains := stringMapFlag{}
	var grants stringListFlag
	cmd, code, ok := parseComposeCmd(prog, "templates install", "templates install <id> [--name NAME] [--project ID] [--env KEY=VALUE]... [--domain SERVICE=HOST]... [--grant CLASS]... [--allow-platform-mismatch]", args, 1, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&name, "name", "", "app name (default: the template id plus a short random suffix)")
		fs.StringVar(&project, "project", "", "group the installed services under this project id")
		fs.Var(env, "env", "template setting KEY=VALUE (repeatable; see templates get <id>)")
		fs.Var(domains, "domain", "put a service behind a domain, SERVICE=HOST (repeatable)")
		fs.BoolVar(&allowPlatform, "allow-platform-mismatch", false, "install even when the template has no verified build for this host's architecture")
		fs.Var(&grants, "grant", "acknowledge a capability class the template declares, e.g. docker_api (repeatable)")
	})
	if !ok {
		return code
	}
	// One lookup serves the domain hint and the grant check; a failed lookup never blocks an install.
	detail, derr := cmd.client.GetServiceTemplate(context.Background(), cmd.args[0])
	if derr == nil {
		if missing := unacknowledgedGrants(detail, grants); len(missing) > 0 {
			_, _ = fmt.Fprintf(stderr, "template %q declares elevated access (%s); re-run with %s to confirm\n", cmd.args[0], strings.Join(missing, ", "), grantFlags(missing))
			return exitUsage
		}
		warnTemplateNeedsDomain(stderr, prog, cmd.args[0], detail, len(domains) > 0)
	}
	res, err := cmd.client.InstallTemplate(context.Background(), cmd.args[0], apiclient.TemplateInstallRequest{Name: name, Env: env, Domains: domains, ProjectID: project, AllowPlatformMismatch: allowPlatform})
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

// unacknowledgedGrants lists declared capability classes the caller did not confirm with --grant.
func unacknowledgedGrants(d apiclient.ServiceTemplateDetail, given []string) []string {
	if d.Manifest == nil {
		return nil
	}
	have := map[string]bool{}
	for _, g := range given {
		have[g] = true
	}
	seen := map[string]bool{}
	var missing []string
	for _, c := range d.Manifest.Capabilities {
		if !have[c.Class] && !seen[c.Class] {
			seen[c.Class] = true
			missing = append(missing, c.Class)
		}
	}
	sort.Strings(missing)
	return missing
}

func grantFlags(classes []string) string {
	parts := make([]string, len(classes))
	for i, c := range classes {
		parts[i] = "--grant " + c
	}
	return strings.Join(parts, " ")
}

// warnTemplateNeedsDomain warns, never refuses: a bare-IP test install must still go through.
func warnTemplateNeedsDomain(stderr io.Writer, prog, id string, detail apiclient.ServiceTemplateDetail, haveDomain bool) {
	if haveDomain || !detail.NeedsDomain {
		return
	}
	_, _ = fmt.Fprintf(stderr, "warning: template %q needs a domain but none was given; it installs anyway, add one with --domain SERVICE=HOST (see %s templates get %s)\n", id, prog, id)
}
