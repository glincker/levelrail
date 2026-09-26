package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"golang.org/x/term"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// importPromptFn asks the operator for one missing env value. It is nil
// when stdin is not a terminal, and swapped out in tests.
var importPromptFn = defaultImportPrompt

func defaultImportPrompt(key string, secret bool, stderr io.Writer) (string, bool, error) {
	fd := int(os.Stdin.Fd()) //nolint:gosec // a file descriptor always fits in int
	if !term.IsTerminal(fd) {
		return "", false, nil
	}
	_, _ = fmt.Fprintf(stderr, "%s: ", key)
	if secret {
		b, err := term.ReadPassword(fd)
		_, _ = fmt.Fprintln(stderr)
		if err != nil {
			return "", true, fmt.Errorf("read %s: %w", key, err)
		}
		return string(b), true, nil
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", true, fmt.Errorf("read %s: %w", key, err)
	}
	return strings.TrimSpace(line), true, nil
}

type importFlags struct {
	file, dockerRun, name, ref string
	port                       int
	deploy                     bool
	env                        map[string]string
}

// runImport implements "import": classify a repo URL, image, docker run
// command or compose file, print the deployment plan, and optionally deploy
// it through the existing create, build and compose endpoints.
func runImport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "import", "print the plan as JSON to stdout and nothing else", stderr)
	f := importFlags{env: map[string]string{}}
	fs.StringVar(&f.file, "f", "", "read a compose file or Dockerfile from this path (- for stdin)")
	fs.StringVar(&f.file, "file", "", "same as -f")
	fs.StringVar(&f.dockerRun, "docker-run", "", "a docker run command to import")
	fs.StringVar(&f.name, "name", "", "app name (default: derived from the source)")
	fs.StringVar(&f.ref, "ref", "", "git branch for a repo import")
	fs.IntVar(&f.port, "port", 0, "container port override")
	fs.BoolVar(&f.deploy, "deploy", false, "create the app and deploy it after printing the plan")
	fs.Var(stringMapFlag(f.env), "env", "env value as KEY=VALUE, repeatable (satisfies required variables)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, importUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	text, err := importInputText(f, fs.Args())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	req := apiclient.ImportPlanRequest{Text: text, Ref: f.ref, Name: f.name, Port: f.port, Env: f.env}
	plan, err := client.PlanImport(context.Background(), req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("plan import: %w", err))
	}

	if !f.deploy {
		if err := renderResult(stdout, of.Format, of.Query, plan, func() { printImportPlan(stdout, plan) }); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitCodeForError(err)
		}
		return exitOK
	}

	if len(plan.MissingRequiredEnv) > 0 {
		filled, perr := promptMissingEnv(plan, f.env, stderr)
		if perr != nil {
			return reportError(stdout, stderr, jsonOut, perr)
		}
		if len(filled) > 0 {
			req.Env = filled
			if plan, err = client.PlanImport(context.Background(), req); err != nil {
				return reportError(stdout, stderr, jsonOut, fmt.Errorf("plan import: %w", err))
			}
		}
	}
	if len(plan.MissingRequiredEnv) > 0 {
		printImportPlan(stderr, plan)
		return reportError(stdout, stderr, jsonOut, newValidationError("required env vars have no value, pass each with --env KEY=VALUE: %s", strings.Join(plan.MissingRequiredEnv, ", ")))
	}
	if err := deployImportPlan(context.Background(), client, plan, req.Env, stdout, stderr, of); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	return exitOK
}

func importInputText(f importFlags, positional []string) (string, error) {
	sources := 0
	for _, s := range []bool{f.file != "", f.dockerRun != "", len(positional) > 0} {
		if s {
			sources++
		}
	}
	if sources != 1 {
		return "", newValidationError("give exactly one of: a URL or image argument, -f FILE, or --docker-run \"...\"")
	}
	switch {
	case f.dockerRun != "":
		return f.dockerRun, nil
	case f.file != "":
		var data []byte
		var err error
		if f.file == "-" {
			data, err = io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
		} else {
			data, err = os.ReadFile(f.file) //nolint:gosec // operator-supplied CLI flag
		}
		if err != nil {
			return "", fmt.Errorf("read %s: %w", f.file, err)
		}
		return string(data), nil
	default:
		return strings.Join(positional, " "), nil
	}
}

func promptMissingEnv(plan apiclient.ImportPlan, have map[string]string, stderr io.Writer) (map[string]string, error) {
	secret := map[string]bool{}
	for _, s := range plan.Services {
		for _, e := range s.Env {
			secret[e.Key] = e.Secret
		}
	}
	out := map[string]string{}
	for k, v := range have {
		out[k] = v
	}
	asked := false
	for _, key := range plan.MissingRequiredEnv {
		v, prompted, err := importPromptFn(key, secret[key], stderr)
		if err != nil {
			return nil, err
		}
		if !prompted {
			return nil, nil
		}
		asked = true
		out[key] = v
	}
	if !asked {
		return nil, nil
	}
	return out, nil
}

func printImportPlan(out io.Writer, p apiclient.ImportPlan) {
	_, _ = fmt.Fprintf(out, "source:  %s\n", p.Source)
	_, _ = fmt.Fprintf(out, "name:    %s\n", p.SuggestedName)
	_, _ = fmt.Fprintf(out, "deploy:  %s\n", p.Deploy)
	if p.RepoURL != "" {
		_, _ = fmt.Fprintf(out, "repo:    %s (%s)\n", p.RepoURL, p.Ref)
	}
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "\nSERVICE\tBUILD\tIMAGE\tPORT\tWHY")
	for _, s := range p.Services {
		port := "-"
		if s.Port > 0 {
			port = fmt.Sprint(s.Port)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", s.Name, s.Build, dash(s.Image), port, s.BuildReason)
	}
	_ = tw.Flush()
	for _, s := range p.Services {
		if len(s.Env) == 0 && len(s.Volumes) == 0 {
			continue
		}
		_, _ = fmt.Fprintf(out, "\n%s:\n", s.Name)
		for _, e := range s.Env {
			_, _ = fmt.Fprintf(out, "  env  %-28s %s\n", e.Key, envStatus(e))
		}
		for _, v := range s.Volumes {
			_, _ = fmt.Fprintf(out, "  vol  %s\n", volumeText(v))
		}
	}
	if len(p.Warnings) > 0 {
		_, _ = fmt.Fprintln(out, "\nwarnings:")
		for _, w := range p.Warnings {
			_, _ = fmt.Fprintf(out, "  - %s\n", w.Message)
		}
	}
	if len(p.MissingRequiredEnv) > 0 {
		_, _ = fmt.Fprintf(out, "\nrequired env with no value: %s\n", strings.Join(p.MissingRequiredEnv, ", "))
	}
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func envStatus(e apiclient.ImportEnvVar) string {
	parts := []string{}
	switch {
	case e.Required:
		parts = append(parts, "required")
	case e.HasDefault:
		parts = append(parts, "default")
	}
	if e.Secret {
		parts = append(parts, "secret")
	} else if e.Value != "" {
		parts = append(parts, "="+e.Value)
	}
	return strings.Join(parts, ", ")
}

func volumeText(v apiclient.ImportVolume) string {
	src := v.Name
	if v.HostPath != "" {
		src = v.HostPath + " (bind mount, needs root)"
	}
	if src == "" {
		src = "(anonymous)"
	}
	return src + " -> " + v.ContainerPath
}

func deployImportPlan(ctx context.Context, c *Client, p apiclient.ImportPlan, supplied map[string]string, stdout, stderr io.Writer, of outputFlags) error {
	name := p.SuggestedName
	switch p.Deploy {
	case "compose":
		res, err := c.DeployCompose(ctx, name, []byte(p.ComposeYAML))
		if err != nil {
			return fmt.Errorf("deploy compose as app %q: %w", name, err)
		}
		return renderImportResult(stdout, stderr, of, res, func() { printComposeDeployResultHuman(stdout, res) })
	case "app", "build":
		if len(p.Services) == 0 {
			return errors.New("plan has no services")
		}
		body := importCreateBody(p, p.Services[0], supplied)
		if _, err := c.CreateApp(ctx, body); err != nil {
			return fmt.Errorf("create app %q: %w", name, err)
		}
		if p.Deploy == "app" {
			return renderImportResult(stdout, stderr, of, body, func() {
				_, _ = fmt.Fprintf(stdout, "app %q created from image %s; follow it with \"apps status %s\"\n", name, body.Image, name)
			})
		}
		s := p.Services[0]
		res, err := c.TriggerBuild(ctx, name, importBuildRequest(p, s))
		if err != nil {
			return fmt.Errorf("app %q was created but the build failed to start: %w", name, err)
		}
		return renderImportResult(stdout, stderr, of, res, func() {
			_, _ = fmt.Fprintf(stdout, "app %q created, build started (deploy attempt %q); follow it with \"apps deploys logs %s %s\"\n", name, res.ID, name, res.ID)
		})
	default:
		return newValidationError("this input cannot be deployed on its own (deploy kind %q); see the warnings above", p.Deploy)
	}
}

func renderImportResult(stdout, stderr io.Writer, of outputFlags, v any, human func()) error {
	if err := renderResult(stdout, of.Format, of.Query, v, human); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return err
	}
	return nil
}

// importCreateBody maps a service plan onto POST /api/v1/apps. Secret
// variables go through Secrets, everything else through Env.
func importCreateBody(p apiclient.ImportPlan, s apiclient.ImportService, supplied map[string]string) appResource {
	body := appResource{Name: p.SuggestedName, Image: s.Image, Port: s.Port}
	if p.Deploy == "build" {
		body.Image = p.SuggestedName + spec.PendingImageTag
	}
	if s.HealthPath != "" {
		body.Health = &apiclient.ServiceHealth{Readiness: &apiclient.ServiceProbe{Path: s.HealthPath}}
	}
	env := map[string]string{}
	secrets := map[string]string{}
	seen := map[string]bool{}
	for _, e := range s.Env {
		seen[e.Key] = true
		v, ok := supplied[e.Key]
		if !ok {
			v = e.Value
		}
		switch {
		case v == "":
		case e.Secret:
			secrets[e.Key] = v
		default:
			env[e.Key] = v
		}
	}
	for k, v := range supplied {
		if !seen[k] && v != "" {
			env[k] = v
		}
	}
	if len(env) > 0 {
		body.Env = env
	}
	if len(secrets) > 0 {
		body.Secrets = secrets
		keys := make([]string, 0, len(secrets))
		for k := range secrets {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		body.SecretEnv = keys
	}
	return body
}

func importBuildRequest(p apiclient.ImportPlan, s apiclient.ImportService) apiclient.BuildTriggerRequest {
	req := apiclient.BuildTriggerRequest{RepoURL: p.RepoURL, Ref: p.Ref}
	switch s.Build {
	case "railpack", "static", "dockerfile":
		req.Build.Type = s.Build
	}
	return req
}

func importUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s import <repo-url|image> [flags]
  %[1]s import -f compose.yaml|Dockerfile [flags]
  %[1]s import --docker-run "docker run -p 80:80 nginx:1" [flags]

Classifies the input (GitHub, GitLab, Gitea or Bitbucket repo URL, docker
run command, image reference, docker-compose.yml or Dockerfile) and prints
a deployment plan. Nothing is created unless --deploy is given.

Flags:
  -f, --file string     read a compose file or Dockerfile (- for stdin)
  --docker-run string   a docker run command to import
  --deploy              create the app and deploy it
  --name string         app name (default derived from the source)
  --ref string          git branch for a repo import
  --port int            container port override
  --env KEY=VALUE       env value, repeatable; required variables with no value block --deploy
  --token, --api-url, --profile, --json, --output, --query   as for every command
  -h, --help            show this help
`, prog)
}
