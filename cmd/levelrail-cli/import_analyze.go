package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// importAnalyzeFlags are the extra flags "import --analyze|--apply" accepts.
type importAnalyzeFlags struct {
	analyze, apply, noDatabase bool
	baseDir, dockerfile        string
	buildType                  string
}

const mibBytes = 1 << 20

// runImportAnalyze prints what the wizard would suggest for a repository, or
// with --apply creates the databases, app, connections and first build.
func runImportAnalyze(client *Client, f importFlags, af importAnalyzeFlags, positional []string, stdout, stderr io.Writer, jsonOut bool, of outputFlags) int {
	if len(positional) != 1 || f.file != "" || f.dockerRun != "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--analyze and --apply take exactly one repository URL"))
	}
	ctx := context.Background()
	analysis, err := client.AnalyzeImport(ctx, apiclient.ImportAnalyzeRequest{
		RepoURL: positional[0], Ref: f.ref, BaseDirectory: af.baseDir, DockerfilePath: af.dockerfile, BuildType: af.buildType, Name: f.name,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("analyze repository: %w", err))
	}
	if !af.apply {
		if err := renderResult(stdout, of.Format, of.Query, analysis, func() { printImportAnalysis(stdout, analysis) }); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitCodeForError(err)
		}
		return exitOK
	}
	if missing := missingAnalysisEnv(analysis, f.env); len(missing) > 0 {
		printImportAnalysis(stderr, analysis)
		return reportError(stdout, stderr, jsonOut, newValidationError("required env vars have no value, pass each with --env KEY=VALUE: %s", strings.Join(missing, ", ")))
	}
	res, err := applyImportAnalysis(ctx, client, analysis, f, af)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() { printImportApplyResult(stdout, res) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// importApplyResult lists what --apply created, in order.
type importApplyResult struct {
	App       string   `json:"app"`
	Databases []string `json:"databases"`
	Domain    string   `json:"domain,omitempty"`
	DeployID  string   `json:"deploy_attempt_id,omitempty"`
}

func missingAnalysisEnv(a apiclient.ImportAnalysis, supplied map[string]string) []string {
	var out []string
	for _, e := range a.Env {
		if e.Required && e.ProvidedBy == "" && supplied[e.Key] == "" {
			out = append(out, e.Key)
		}
	}
	sort.Strings(out)
	return out
}

func applyImportAnalysis(ctx context.Context, c *Client, a apiclient.ImportAnalysis, f importFlags, af importAnalyzeFlags) (importApplyResult, error) {
	res := importApplyResult{App: a.Name.Value, Databases: []string{}}
	port := a.Port.Value
	if f.port > 0 {
		port = f.port
	}
	if port <= 0 {
		return res, newValidationError("no port was detected, pass --port")
	}
	var dbs []apiclient.AnalyzeDatabase
	if !af.noDatabase {
		dbs = a.Databases
	}
	for _, d := range dbs {
		if _, err := c.GetDatabase(ctx, d.SuggestedName); err != nil {
			if _, cerr := c.CreateDatabase(ctx, apiclient.DatabaseResource{Name: d.SuggestedName, Engine: d.Engine, Version: d.Version}); cerr != nil {
				return res, fmt.Errorf("create database %q: %w", d.SuggestedName, cerr)
			}
		}
		res.Databases = append(res.Databases, d.SuggestedName)
	}

	body := appResource{Name: a.Name.Value, Image: a.Name.Value + spec.PendingImageTag, Port: port}
	if a.Health.Value != "" {
		body.Health = &apiclient.ServiceHealth{Readiness: &apiclient.ServiceProbe{Path: a.Health.Value}}
	}
	if a.Memory.Bytes > 0 {
		body.Resources = &apiclient.ServiceResources{MemoryBytes: a.Memory.Bytes}
	}
	env, secrets := splitAnalysisEnv(a, f.env)
	if len(env) > 0 {
		body.Env = env
	}
	if len(secrets) > 0 {
		keys := make([]string, 0, len(secrets))
		for k := range secrets {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		body.Secrets, body.SecretEnv = secrets, keys
	}
	created, err := c.CreateApp(ctx, body)
	if err != nil {
		return res, fmt.Errorf("create app %q (databases created so far: %s): %w", body.Name, strings.Join(res.Databases, ", "), err)
	}
	if len(created.Domains) > 0 {
		res.Domain = created.Domains[0]
	}
	for _, d := range dbs {
		if _, err := c.CreateAppConnection(ctx, a.Name.Value, apiclient.CreateAppConnectionRequest{Database: d.SuggestedName, Field: d.Field, EnvVar: d.EnvVar}); err != nil {
			return res, fmt.Errorf("app %q was created but attaching database %q failed: %w", a.Name.Value, d.SuggestedName, err)
		}
	}
	if _, err := c.SetGitSource(ctx, a.Name.Value, apiclient.SetGitSourceRequest{
		RepoURL: a.RepoURL, Branch: a.Branch, BuildType: a.Build.Type, BuildPath: a.Build.DockerfilePath, BaseDirectory: a.Build.BaseDirectory,
	}); err != nil {
		return res, fmt.Errorf("app %q was created but connecting the repository failed: %w", a.Name.Value, err)
	}
	build, err := c.TriggerBuild(ctx, a.Name.Value, apiclient.BuildTriggerRequest{
		RepoURL: a.RepoURL, Ref: a.Branch, DetectedFramework: a.Stack.Framework,
		Build: apiclient.BuildTriggerRequestBuild{Type: a.Build.Type, Path: a.Build.DockerfilePath, BaseDirectory: a.Build.BaseDirectory},
	})
	if err != nil {
		return res, fmt.Errorf("app %q was created but the build failed to start: %w", a.Name.Value, err)
	}
	res.DeployID = build.ID
	return res, nil
}

// splitAnalysisEnv maps supplied values onto the keys the analysis found.
// Only supplied values are sent; nothing is ever read from the repository.
func splitAnalysisEnv(a apiclient.ImportAnalysis, supplied map[string]string) (env, secrets map[string]string) {
	env, secrets = map[string]string{}, map[string]string{}
	secretKeys := map[string]bool{}
	for _, e := range a.Env {
		secretKeys[e.Key] = e.Secret
	}
	for k, v := range supplied {
		if v == "" {
			continue
		}
		if secretKeys[k] {
			secrets[k] = v
		} else {
			env[k] = v
		}
	}
	return env, secrets
}

func printImportAnalysis(out io.Writer, a apiclient.ImportAnalysis) {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	row := func(label, value, evidence string) {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", label, dash(value), evidence)
	}
	framework := a.Stack.Framework
	if framework == "" {
		framework = a.Stack.Runtime
	}
	row("Name", a.Name.Value, a.Name.Evidence)
	row("Stack", framework, a.Stack.Evidence)
	build := a.Build.Type
	if a.Build.DockerfilePath != "" {
		build += " " + a.Build.DockerfilePath
	}
	if a.Build.BaseDirectory != "" {
		build += " (context " + a.Build.BaseDirectory + ")"
	}
	row("Build", build, a.Build.Evidence)
	row("Port", fmt.Sprint(a.Port.Value), a.Port.Evidence)
	row("Health", a.Health.Value, a.Health.Evidence)
	row("Memory", fmt.Sprintf("%d MiB", a.Memory.Bytes/mibBytes), a.Memory.Evidence)
	row("Domain", a.Domain.Value, a.Domain.Evidence)
	for _, d := range a.Databases {
		row("Database", fmt.Sprintf("%s %s as %s -> %s", d.Engine, d.Version, d.SuggestedName, d.EnvVar), d.Evidence)
	}
	for _, e := range a.Env {
		state := "optional"
		switch {
		case e.ProvidedBy != "":
			state = "set by " + e.ProvidedBy
		case e.Required:
			state = "required"
		}
		if e.Secret {
			state += ", secret"
		}
		row("Env", e.Key, state+" ("+e.Source+")")
	}
	_ = w.Flush()
	for _, wn := range a.Warnings {
		_, _ = fmt.Fprintf(out, "warning: %s\n", wn.Message)
	}
}

func printImportApplyResult(out io.Writer, r importApplyResult) {
	_, _ = fmt.Fprintf(out, "app %q created", r.App)
	if len(r.Databases) > 0 {
		_, _ = fmt.Fprintf(out, " with database(s) %s", strings.Join(r.Databases, ", "))
	}
	if r.Domain != "" {
		_, _ = fmt.Fprintf(out, " at %s", r.Domain)
	}
	_, _ = fmt.Fprintf(out, "; build started (deploy attempt %q), follow it with \"apps deploys logs %s %s\"\n", r.DeployID, r.App, r.DeployID)
}
