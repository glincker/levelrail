package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strings"

	"golang.org/x/term"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// upResult is the --json shape and the data behind the final summary.
type upResult struct {
	App       string   `json:"app"`
	Created   bool     `json:"created"`
	Databases []string `json:"databases,omitempty"`
	Domain    string   `json:"domain,omitempty"`
	URL       string   `json:"url,omitempty"`
	Plan      []string `json:"plan,omitempty"`
	DryRun    bool     `json:"dry_run,omitempty"`
	DeployID  string   `json:"deploy_id,omitempty"`
	Status    string   `json:"status,omitempty"`
	// Certificate is issued, pending or failed for the primary domain; empty when unknown.
	Certificate string                   `json:"certificate,omitempty"`
	Failure     *apiclient.DeployFailure `json:"failure,omitempty"`
	Error       string                   `json:"error,omitempty"`
}

// runUp implements "up [dir]": infer an app from the directory, then create or update it, connect databases, build and add the domain.
func runUp(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, tokenP, apiURLP, profileP, jsonOutP, outputP, queryP := apiFlagSet(prog, "up", "print the result as JSON to stdout and nothing else", stderr)
	var f upFlags
	var dryRun, yes, allowUnpushed, noWait bool
	var timeoutStr string
	f.secrets = map[string]string{}
	fs.StringVar(&f.name, "name", "", "app name (default: the directory name)")
	fs.IntVar(&f.port, "port", 0, "container port (default: from the app spec, the Dockerfile or the detected framework)")
	fs.StringVar(&f.domain, "domain", "", "domain to add once the app is live (default: none, the app keeps its automatic URL)")
	fs.Func("with", "database to create and connect, repeatable or comma separated: postgres, redis, mysql, or ENGINE:VERSION", func(s string) error {
		f.with = append(f.with, splitCSV(s)...)
		return nil
	})
	fs.IntVar(&f.replicas, "replicas", 0, "number of container replicas (default: the app spec's, then 1)")
	fs.StringVar(&f.strategy, "strategy", "", "deploy strategy: rolling, recreate or blue-green (default: the app spec's, then blue-green)")
	fs.StringVar(&f.memory, "memory", "", "memory limit, e.g. 512Mi or 2Gi (default: the app spec's, then unlimited)")
	fs.Float64Var(&f.cpu, "cpu", 0, "CPU limit in cores, e.g. 0.5 (default: the app spec's, then unlimited)")
	fs.StringVar(&f.repo, "repo", "", "git repository URL to build from (default: the directory's remote \"origin\")")
	fs.StringVar(&f.ref, "ref", "", "git branch to build (default: the current branch, then main)")
	fs.Var(stringMapFlag(f.secrets), "secret", "secret env var as KEY=VALUE, repeatable, for a required secret the app spec declares")
	fs.BoolVar(&dryRun, "dry-run", false, "print the inferred settings and the exact commands, change nothing")
	fs.BoolVar(&allowUnpushed, "allow-unpushed", false, "build even when the remote branch lacks your local commit")
	fs.BoolVar(&noWait, "no-wait", false, "return once the build is started instead of waiting for the deploy to become healthy")
	fs.StringVar(&timeoutStr, "timeout", defaultDeployUploadWait, "stop waiting for the deploy after this long (the deploy itself keeps running)")
	fs.BoolVar(&yes, "yes", false, "do not ask for confirmation on a terminal")
	fs.BoolVar(&yes, "y", false, "shorthand for --yes")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, upUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonOutP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	dir := "."
	switch rest := fs.Args(); len(rest) {
	case 0:
	case 1:
		dir = rest[0]
	default:
		_, _ = fmt.Fprintf(stderr, "%s: up takes at most one directory\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return reportError(stdout, stderr, jsonOut, newValidationError("%q is not a directory", dir))
	}
	if f.strategy != "" && f.strategy != "rolling" && f.strategy != "recreate" && f.strategy != "blue-green" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--strategy must be rolling, recreate or blue-green, got %q", f.strategy))
	}

	timeout, err := parseDurationOrZero(timeoutStr)
	if err != nil || timeout <= 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--timeout must be a valid positive duration, got %q", timeoutStr))
	}

	inf, err := inferUp(dir, f, detectGitAt(dir))
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	existing, exists, err := lookupApp(ctx, client, inf.Flags.name)
	if err != nil && !dryRun {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("look up app %q: %w", inf.Flags.name, err))
	}
	result := upResult{App: inf.Flags.name, Created: !exists, Domain: inf.Domain, DryRun: dryRun, Plan: inf.upCommands(prog, exists)}
	if !jsonOut {
		printUpInference(stderr, prog, inf, exists)
	}
	if dryRun {
		return writeScheduledTaskResult(stdout, stderr, of, result, func() {})
	}
	if !jsonOut && !yes && isTerminal(stdin) && !confirmUp(stdin, stderr) {
		_, _ = fmt.Fprintln(stderr, "cancelled, nothing changed")
		return exitUsage
	}

	chk := checkPushed(ctx, dir, inf, f.ref != "")
	if chk.Warning != "" && !jsonOut {
		_, _ = fmt.Fprintf(stderr, "warning: %s\n", chk.Warning)
	}
	if chk.Problem != "" && !allowUnpushed {
		return reportError(stdout, stderr, jsonOut, newValidationError("%s", chk.Problem))
	}

	if err := executeUp(ctx, client, prog, inf, existing, exists, &result, stderr, jsonOut); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	waited := !noWait
	if waited {
		if err := waitUpDeploy(ctx, client, &result, timeout, stderr, jsonOut); err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
	}
	code := writeScheduledTaskResult(stdout, stderr, of, result, func() {
		if waited {
			if result.Status == apiclient.DeployOutcomeHealthy && result.URL != "" {
				_, _ = fmt.Fprintf(stdout, "%s\n", result.URL)
			}
			printUpWaitVerdict(stderr, &result)
			return
		}
		if result.URL != "" {
			_, _ = fmt.Fprintf(stdout, "%s\n", result.URL)
		}
		_, _ = fmt.Fprintf(stderr, "follow it with \"%s apps deploys wait %s\", logs with \"%s apps logs %s\"\n", prog, result.App, prog, result.App)
	})
	if code != exitOK || !waited {
		return code
	}
	return upWaitExit(result.Status)
}

func lookupApp(ctx context.Context, client *Client, name string) (appResource, bool, error) {
	app, err := client.GetApp(ctx, name)
	if err == nil {
		return app, true, nil
	}
	var apiErr *apiError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return appResource{}, false, nil
	}
	return appResource{}, false, err
}

func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	return ok && term.IsTerminal(int(f.Fd())) //nolint:gosec // a file descriptor always fits an int
}

func confirmUp(stdin io.Reader, stderr io.Writer) bool {
	_, _ = fmt.Fprint(stderr, "Proceed? [Y/n] ")
	line, _ := bufio.NewReader(stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "", "y", "yes":
		return true
	}
	return false
}

func printUpInference(w io.Writer, prog string, inf upInference, exists bool) {
	_, _ = fmt.Fprintf(w, "up: %s\n", inf.Dir)
	for _, n := range inf.Notes {
		_, _ = fmt.Fprintf(w, "  note: %s\n", n)
	}
	f := inf.Flags
	rows := [][2]string{
		{"name", f.name}, {"port", portLabel(f.port)}, {"build", buildLabel(inf)},
		{"repo", f.repo}, {"ref", f.ref}, {"image repo", f.imageRepo},
		{"replicas", numLabel(f.replicas, "1")}, {"strategy", orDefault(f.strategy, "blue-green")},
		{"domain", orDefault(inf.Domain, "automatic URL")}, {"resources", resourcesLabel(inf.Resources)},
	}
	if f.baseDirectory != "" {
		rows = append(rows, [2]string{"base directory", f.baseDirectory})
	}
	for _, db := range inf.DBs {
		rows = append(rows, [2]string{"database", db.Name + " (" + db.Engine + ")"})
	}
	for _, r := range rows {
		src := inf.Sources[r[0]]
		if src == "" {
			src = srcDefault
		}
		_, _ = fmt.Fprintf(w, "  %-15s %-40s (%s)\n", r[0], r[1], src)
	}
	state := "new app"
	if exists {
		state = "app exists, it is updated and rebuilt, nothing is duplicated"
	}
	_, _ = fmt.Fprintf(w, "plan (%s):\n", state)
	for _, c := range inf.upCommands(prog, exists) {
		_, _ = fmt.Fprintf(w, "  %s\n", c)
	}
	if len(inf.DBs) > 0 && !exists {
		_, _ = fmt.Fprintln(w, "  (up connects databases before the first build starts, so the first container already has its connection env)")
	}
	_, _ = fmt.Fprintf(w, "change any value with its flag, or commit an app.yaml; see \"%s up -h\"\n", prog)
}

func portLabel(p int) string {
	if p == 0 {
		return "none (static site)"
	}
	return fmt.Sprint(p)
}

func buildLabel(inf upInference) string {
	if inf.Flags.buildType != "" {
		return inf.Flags.buildType
	}
	if inf.FileSpec != nil {
		for _, s := range inf.FileSpec.Services {
			return s.Build.Type
		}
	}
	return ""
}

func numLabel(n int, def string) string {
	if n > 0 {
		return fmt.Sprint(n)
	}
	return def
}

func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

func resourcesLabel(r *serviceResources) string {
	if r == nil {
		return "unlimited"
	}
	var parts []string
	if r.MemoryBytes > 0 {
		parts = append(parts, formatMemoryMi(r.MemoryBytes))
	}
	if r.NanoCPUs > 0 {
		parts = append(parts, fmt.Sprintf("%.2g cpu", float64(r.NanoCPUs)/1e9))
	}
	if len(parts) == 0 {
		return "unlimited"
	}
	return strings.Join(parts, ", ")
}

func (inf upInference) createPlan() (createPlan, error) {
	f := inf.Flags
	var plan createPlan
	var err error
	if f.file != "" {
		plan, err = planFromFile(f, inf.FileSpec, detectedGit{})
	} else {
		plan, err = planGitBuildFlags(f, detectedGit{})
	}
	if err != nil {
		return plan, err
	}
	if plan.DeploySpec != nil {
		return plan, nil
	}
	if f.replicas > 0 {
		plan.CreateBody.Replicas = f.replicas
	}
	if f.strategy != "" {
		plan.CreateBody.Strategy = f.strategy
	}
	if len(f.secrets) > 0 {
		plan.CreateBody.Secrets = f.secrets
	}
	if inf.Resources != nil {
		plan.CreateBody.Resources = inf.Resources
	}
	return plan, nil
}

func executeUp(ctx context.Context, client *Client, prog string, inf upInference, existing appResource, exists bool, result *upResult, stderr io.Writer, quiet bool) error {
	say := func(format string, a ...any) {
		if !quiet {
			_, _ = fmt.Fprintf(stderr, format+"\n", a...)
		}
	}
	plan, err := inf.createPlan()
	if err != nil {
		return err
	}
	name := inf.Flags.name

	switch {
	case plan.DeploySpec != nil:
	case exists:
		if err := updateExistingApp(ctx, client, existing, plan, inf, say); err != nil {
			return err
		}
	default:
		if _, err := client.CreateApp(ctx, plan.CreateBody); err != nil {
			return fmt.Errorf("create app %q: %w", name, err)
		}
		say("created app %q", name)
		if len(plan.Volumes) > 0 {
			if err := attachPlanVolumes(ctx, client, name, plan.Volumes, stderr, quiet); err != nil {
				return fmt.Errorf("app %q was created but attaching its volumes failed, re-run \"%s up\" to retry: %w", name, prog, err)
			}
		}
	}

	if err := connectDatabases(ctx, client, prog, name, inf.DBs, result, say); err != nil {
		return err
	}

	switch {
	case plan.DeploySpec != nil:
		say("deploying static site %q from %s (ref %s)", name, plan.DeploySpec.Request.RepoURL, plan.DeploySpec.Request.Ref)
		if _, err := client.DeploySpec(ctx, name, plan.DeploySpec.Request); err != nil {
			return fmt.Errorf("deploy static site %q: %w", name, err)
		}
	case plan.Build != nil:
		say("building %q from %s (ref %s)", name, plan.Build.RepoURL, plan.Build.Ref)
		trig, err := client.TriggerBuild(ctx, name, *plan.Build)
		if err != nil {
			return fmt.Errorf("app %q is set up but the build did not start, re-run \"%s up\" to retry: %w", name, prog, err)
		}
		result.DeployID = trig.ID
	}

	if inf.Domain != "" && !hasDomain(existing.Domains, inf.Domain) {
		if _, err := client.EditAppDomains(ctx, name, apiclient.EditDomainsRequest{Add: []string{inf.Domain}}); err != nil {
			return fmt.Errorf("app %q is deployed but adding domain %q failed, retry with \"%s apps domains add %s %s\": %w", name, inf.Domain, prog, name, inf.Domain, err)
		}
		say("added domain %s", inf.Domain)
	}

	if final, err := client.GetApp(ctx, name); err == nil {
		result.URL = appPrimaryURL(final)
	}
	return nil
}

func updateExistingApp(ctx context.Context, client *Client, app appResource, plan createPlan, inf upInference, say func(string, ...any)) error {
	changed := false
	if r := plan.CreateBody.Replicas; r > 0 && r != app.Replicas {
		app.Replicas, changed = r, true
	}
	if s := plan.CreateBody.Strategy; s != "" && s != app.Strategy {
		app.Strategy, changed = s, true
	}
	if res := inf.Resources; res != nil && !reflect.DeepEqual(res, app.Resources) {
		app.Resources, changed = res, true
	}
	if !changed {
		say("app %q already exists, settings unchanged", app.Name)
		return nil
	}
	if _, err := client.UpdateApp(ctx, app.Name, app); err != nil {
		return fmt.Errorf("update app %q: %w", app.Name, err)
	}
	say("updated app %q", app.Name)
	return nil
}

func connectDatabases(ctx context.Context, client *Client, prog, app string, dbs []upDatabase, result *upResult, say func(string, ...any)) error {
	if len(dbs) == 0 {
		return nil
	}
	existing, err := client.ListDatabases(ctx)
	if err != nil {
		return fmt.Errorf("app %q is set up but listing databases failed, re-run \"%s up\" to retry: %w", app, prog, err)
	}
	have := map[string]bool{}
	for _, d := range existing {
		have[d.Name] = true
	}
	connected := map[string]bool{}
	if conns, err := client.ListAppConnections(ctx, app); err == nil {
		for _, c := range conns {
			connected[c.DatabaseName] = true
		}
	}
	var engines []apiclient.DatabaseEngineResource
	for _, db := range dbs {
		if !have[db.Name] {
			if db.Version == "" {
				if engines == nil {
					engines, _ = client.ListDatabaseEngines(ctx)
				}
				db.Version = defaultEngineVersion(engines, db.Engine)
			}
			if _, err := client.CreateDatabase(ctx, databaseResource{Name: db.Name, Engine: db.Engine, Version: db.Version}); err != nil {
				return fmt.Errorf("app %q is set up but creating database %q failed, re-run \"%s up\" to retry: %w", app, db.Name, prog, err)
			}
			say("created database %q (%s %s)", db.Name, db.Engine, db.Version)
		}
		result.Databases = append(result.Databases, db.Name)
		if connected[db.Name] {
			continue
		}
		req := apiclient.CreateAppConnectionRequest{Database: db.Name, EnvVar: db.EnvVar}
		if _, err := client.CreateAppConnection(ctx, app, req); err != nil {
			return fmt.Errorf("app %q and database %q exist but connecting them failed, no build was started; fix it with \"%s apps connect %s %s\" or re-run \"%s up\": %w", app, db.Name, prog, app, db.Name, prog, err)
		}
		say("connected %q to %q", app, db.Name)
	}
	sort.Strings(result.Databases)
	return nil
}

func defaultEngineVersion(engines []apiclient.DatabaseEngineResource, engine string) string {
	for _, e := range engines {
		if e.ID == engine && e.DefaultVersion != "" {
			return e.DefaultVersion
		}
	}
	return "latest"
}

func hasDomain(domains []string, d string) bool {
	for _, x := range domains {
		if strings.EqualFold(x, d) {
			return true
		}
	}
	return false
}

func appPrimaryURL(app appResource) string {
	if len(app.Domains) > 0 {
		return "https://" + app.Domains[0]
	}
	return app.FallbackURL
}

func upUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s up [dir] [flags]

Deploys the project in dir (default: the current directory) with one
command. It reads app.yaml when there is one, otherwise detects the stack
(Dockerfile, Node, Go, Python, Java or a static site), infers the name,
port and build, then creates or updates the app, connects any databases,
builds from the directory's git remote "origin" and adds the domain.
It then follows the first deploy until the app is healthy and exits like
"deploy": 0 healthy, %[6]d not healthy, %[7]d timed out. --no-wait returns
as soon as the build is started. The branch must be pushed first.
Running it again updates the same app and rebuilds; nothing is duplicated.
Every value is printed with where it came from. Precedence: flag, app
spec, detection, default.

Flags:
  --name string       app name (default: the directory name)
  --port int          container port (default: app spec, Dockerfile EXPOSE or framework)
  --domain string     domain to add once the app is live (default: none)
  --with list         databases to create and connect, e.g. --with postgres,redis or postgres:16
  --replicas int      number of container replicas (default 1)
  --strategy string   rolling, recreate or blue-green (default blue-green)
  --memory string     memory limit, e.g. 512Mi or 2Gi
  --cpu float         CPU limit in cores, e.g. 0.5
  --repo string       git repository URL (default: the remote "origin")
  --ref string        git branch (default: the current branch, then main)
  --secret KEY=VALUE  secret env var, repeatable, for a required secret the spec declares
  --allow-unpushed    build even when the remote branch lacks your local commit
  --no-wait           return once the build is started instead of waiting for a healthy deploy
  --timeout string    stop waiting after this long, the deploy keeps running (default %[5]s)
  --dry-run           print the inferred settings and the exact commands, change nothing
  --yes, -y           do not ask for confirmation on a terminal
  --token string      API token (default: %[2]s env var, then the credentials file)
  --api-url string    control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string    named credentials profile to read (overrides APP_PROFILE, default "default")
  --json              print the result as JSON to stdout, nothing else
  --output string     output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string      JMESPath expression to filter the result before printing
  -h, --help          show this help

Example:
  %[1]s up --with postgres --domain app.example.com
`, prog, envAPIToken, envAPIURL, defaultAPIURL, defaultDeployUploadWait, exitDeployNotHealthy, exitDeployTimeout)
}
