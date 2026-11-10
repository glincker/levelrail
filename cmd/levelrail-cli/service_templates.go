package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func serviceTemplatesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s templates service list [flags]                     list the service template catalog
  %[1]s templates service show <id> [flags]                show one template and what installing it does
  %[1]s templates service validate -f <template.yaml | dir> [flags]   check a submission on the server
  %[1]s templates service install <id> [--name N] [--env K=V] [--domain SVC=HOST] [--yes]
  %[1]s templates service deploy <id> [--yes]              one-click trial deploy under a generated name

"install" and "deploy" print a plan and change nothing until you pass --yes.
`, prog)
}

func runTemplatesService(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, serviceTemplatesUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, serviceTemplatesUsage(prog))
		return exitOK
	case "list":
		return runServiceTemplatesList(prog, rest, stdout, stderr, lookupEnv)
	case "show":
		return runServiceTemplatesShow(prog, rest, stdout, stderr, lookupEnv)
	case "validate":
		return runServiceTemplatesValidate(prog, rest, stdout, stderr, lookupEnv)
	case "install":
		return runServiceTemplatesInstall(prog, rest, stdout, stderr, lookupEnv)
	case "deploy":
		return runServiceTemplatesDeploy(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown templates service subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, serviceTemplatesUsage(prog))
		return exitUsage
	}
}

func runServiceTemplatesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	cmd, code, ok := parseComposeCmd(prog, "templates service list", "templates service list [flags]", args, 0, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	items, err := cmd.client.ListServiceTemplates(context.Background())
	if err != nil {
		return reportError(stdout, stderr, cmd.json, fmt.Errorf("list service templates: %w", err))
	}
	return renderCompose(stdout, stderr, cmd, items, func() {
		if len(items) == 0 {
			_, _ = fmt.Fprintln(stdout, "no templates")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tNAME\tCATEGORY\tSLOGAN")
		for _, t := range items {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", t.ID, t.Name, t.Category, t.Slogan)
		}
		_ = tw.Flush()
	})
}

func runServiceTemplatesShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	cmd, code, ok := parseComposeCmd(prog, "templates service show", "templates service show <id> [flags]", args, 1, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	id := cmd.args[0]
	t, err := cmd.client.GetServiceTemplate(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, cmd.json, fmt.Errorf("get service template %q: %w", id, err))
	}
	return renderCompose(stdout, stderr, cmd, t, func() {
		_, _ = fmt.Fprintf(stdout, "id:        %s\nname:      %s\ncategory:  %s\nslogan:    %s\n", t.ID, t.Name, t.Category, t.Slogan)
		_, _ = fmt.Fprintf(stdout, "needs settings: %t (use install with --env when true)\n", t.RequiresConfiguration)
	})
}

// readSubmissionFiles loads template.yaml and its sibling compose file,
// the same layout the offline "templates validate" reads.
func readSubmissionFiles(path string) (tpl, compose []byte, err error) {
	if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
		path = filepath.Join(path, "template.yaml")
	}
	tpl, err = os.ReadFile(path) //nolint:gosec // operator-chosen submission path
	if err != nil {
		return nil, nil, fmt.Errorf("read template: %w", err)
	}
	var head struct {
		ComposeFile string `yaml:"compose_file"`
	}
	if err = yaml.Unmarshal(tpl, &head); err != nil {
		return nil, nil, fmt.Errorf("template.yaml: %w", err)
	}
	name := head.ComposeFile
	if name == "" {
		name = "compose.yaml"
	}
	compose, err = os.ReadFile(filepath.Join(filepath.Dir(path), filepath.Base(name))) //nolint:gosec // sibling of the submission file
	if err != nil {
		return nil, nil, fmt.Errorf("read compose file: %w", err)
	}
	return tpl, compose, nil
}

func runServiceTemplatesValidate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var file string
	cmd, code, ok := parseComposeCmd(prog, "templates service validate", "templates service validate -f <template.yaml | dir> [flags]", args, 0, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&file, "f", "", "template.yaml or a directory holding it (the compose file is read beside it)")
		fs.StringVar(&file, "file", "", "same as -f")
	})
	if !ok {
		return code
	}
	if file == "" {
		_, _ = fmt.Fprintf(stderr, "%s: templates service validate requires -f\n", prog)
		return exitUsage
	}
	tpl, compose, err := readSubmissionFiles(file)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
		return exitUsage
	}
	res, err := cmd.client.ValidateServiceTemplate(context.Background(), apiclient.ValidateTemplateRequest{TemplateYAML: string(tpl), ComposeYAML: string(compose)})
	if err != nil {
		return reportError(stdout, stderr, cmd.json, fmt.Errorf("validate template: %w", err))
	}
	if rc := renderCompose(stdout, stderr, cmd, res, func() {
		if res.Valid {
			_, _ = fmt.Fprintf(stdout, "ok: %s (%s)\n", res.ID, res.Name)
			return
		}
		_, _ = fmt.Fprintf(stdout, "invalid: %s\n", res.ID)
		for _, e := range res.Errors {
			_, _ = fmt.Fprintf(stdout, "  %s\n", e)
		}
	}); rc != exitOK {
		return rc
	}
	if !res.Valid {
		return exitValidation
	}
	return exitOK
}

func runServiceTemplatesInstall(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var name string
	var yes bool
	env := stringMapFlag{}
	domains := stringMapFlag{}
	cmd, code, ok := parseComposeCmd(prog, "templates service install", "templates service install <id> [--name N] [--env K=V]... [--domain SVC=HOST]... [--yes]", args, 1, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&name, "name", "", "app name (default: the template id plus a short random suffix)")
		fs.Var(env, "env", "template setting KEY=VALUE (repeatable)")
		fs.Var(domains, "domain", "put a service behind a domain, SERVICE=HOST (repeatable)")
		fs.BoolVar(&yes, "yes", false, "apply; without it only the plan is printed")
	})
	if !ok {
		return code
	}
	id := cmd.args[0]
	if !yes {
		return printServiceTemplatePlan(prog, cmd, stdout, stderr, id, "install", name, env, domains)
	}
	res, err := cmd.client.InstallTemplate(context.Background(), id, apiclient.TemplateInstallRequest{Name: name, Env: env, Domains: domains})
	if err != nil {
		return reportError(stdout, stderr, cmd.json, fmt.Errorf("install template %q: %w", id, err))
	}
	return renderServiceTemplateDeploy(stdout, stderr, cmd, res, id)
}

func runServiceTemplatesDeploy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var yes bool
	cmd, code, ok := parseComposeCmd(prog, "templates service deploy", "templates service deploy <id> [--yes]", args, 1, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.BoolVar(&yes, "yes", false, "apply; without it only the plan is printed")
	})
	if !ok {
		return code
	}
	id := cmd.args[0]
	if !yes {
		return printServiceTemplatePlan(prog, cmd, stdout, stderr, id, "deploy", "", nil, nil)
	}
	res, err := cmd.client.DeployServiceTemplateNow(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, cmd.json, fmt.Errorf("deploy template %q: %w", id, err))
	}
	return renderServiceTemplateDeploy(stdout, stderr, cmd, res, id)
}

type serviceTemplatePlan struct {
	Template string            `json:"template"`
	Action   string            `json:"action"`
	Name     string            `json:"name,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	Domains  map[string]string `json:"domains,omitempty"`
	Applied  bool              `json:"applied"`
}

// printServiceTemplatePlan fetches the template so a typo fails here, then
// prints what --yes would do. Env values are shown by key only.
func printServiceTemplatePlan(prog string, cmd composeCmd, stdout, stderr io.Writer, id, action, name string, env, domains map[string]string) int {
	t, err := cmd.client.GetServiceTemplate(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, cmd.json, fmt.Errorf("get service template %q: %w", id, err))
	}
	masked := map[string]string{}
	for k := range env {
		masked[k] = "***"
	}
	plan := serviceTemplatePlan{Template: t.ID, Action: action, Name: name, Env: masked, Domains: domains}
	return renderCompose(stdout, stderr, cmd, plan, func() {
		_, _ = fmt.Fprintf(stdout, "plan: %s %s (%s)\n", action, t.ID, t.Name)
		if name != "" {
			_, _ = fmt.Fprintf(stdout, "  app name: %s\n", name)
		}
		for k := range masked {
			_, _ = fmt.Fprintf(stdout, "  setting: %s\n", k)
		}
		for svc, host := range domains {
			_, _ = fmt.Fprintf(stdout, "  domain: %s -> %s\n", svc, host)
		}
		if t.RequiresConfiguration && action == "deploy" {
			_, _ = fmt.Fprintf(stdout, "  note: this template needs settings, use install with --env\n")
		}
		_, _ = fmt.Fprintf(stdout, "preview only, nothing changed. Re-run with --yes to apply: %s templates service %s %s --yes\n", prog, action, strings.TrimSpace(id))
	})
}

func renderServiceTemplateDeploy(stdout, stderr io.Writer, cmd composeCmd, res apiclient.ComposeStackDeploy, id string) int {
	return renderCompose(stdout, stderr, cmd, res, func() {
		_, _ = fmt.Fprintf(stdout, "%s: deployed from %s, %d service(s)\n", res.AppID, id, len(res.Services))
		for _, s := range res.Services {
			_, _ = fmt.Fprintf(stdout, "  %s\t%s\n", s.Name, s.Image)
		}
	})
}
