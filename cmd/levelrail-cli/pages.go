package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runPages implements "pages ...", the Levelrail Pages (static sites) front door.
func runPages(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, pagesUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, pagesUsage(prog))
		return exitOK
	case "init":
		return runPagesInit(prog, rest, stdout, stderr)
	case "import":
		return runPagesImport(prog, rest, stdout, stderr)
	case "deploy":
		return runPagesDeploy(prog, rest, stdout, stderr, lookupEnv, stdin)
	case "list":
		return runStaticSitesList(prog, rest, stdout, stderr, lookupEnv)
	case "analytics":
		return runPagesAnalytics(prog, rest, stdout, stderr, lookupEnv)
	case "show":
		return runPagesShow(prog, rest, stdout, stderr, lookupEnv)
	case "delete":
		return runPagesDelete(prog, rest, stdout, stderr, lookupEnv)
	case "rollback":
		return runPagesRollback(prog, rest, stdout, stderr, lookupEnv)
	case "deploys":
		return runPagesDeploys(prog, rest, stdout, stderr, lookupEnv)
	case "previews":
		return runPagesPreviews(prog, rest, stdout, stderr, lookupEnv)
	case "logs":
		return runPagesLogs(prog, rest, stdout, stderr, lookupEnv)
	case "promote":
		return runPagesPromote(prog, rest, stdout, stderr, lookupEnv)
	case "domains":
		return runPagesDomains(prog, rest, stdout, stderr, lookupEnv)
	case "settings":
		return runPagesSettings(prog, rest, stdout, stderr, lookupEnv)
	case "preset":
		return runPagesPreset(prog, rest, stdout, stderr, lookupEnv)
	case "routes":
		return runStaticRoutes(prog, rest, stdout, stderr, lookupEnv)
	case "submissions":
		return runPagesSubmissions(prog, rest, stdout, stderr, lookupEnv)
	case "hook":
		return runPagesHook(prog, rest, stdout, stderr, lookupEnv)
	case "notify":
		return runPagesNotify(prog, rest, stdout, stderr, lookupEnv)
	case "token":
		return runPagesToken(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown pages subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, pagesUsage(prog))
		return exitUsage
	}
}

func pagesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s pages init [dir] [--starter blank|landing|docs|portfolio]
  %[1]s pages import <dir> [--write]
  %[1]s pages deploy <dir|zip> [--name N] [--domain D] [flags]
  %[1]s pages deploy --url <https://host/site.zip|.tar.gz> [--name N] [--domain D]
  %[1]s pages deploy --git <url> [--branch b] [--build-command c] [--output-dir d] [--name N]
  %[1]s pages list [flags]
  %[1]s pages show <name> [flags]
  %[1]s pages analytics <name> [--range 24h|7d|30d] [flags]
  %[1]s pages delete <name> [flags]
  %[1]s pages rollback <name> [version] [flags]
  %[1]s pages token create|list|revoke <site> [flags]   deploy-only tokens for CI (set as APP_API_TOKEN)
  %[1]s pages deploys <name> [flags]
  %[1]s pages previews <name> [flags]
  %[1]s pages promote <name> <deploy> [flags]
  %[1]s pages logs <name> [deploy] [--follow] [flags]
  %[1]s pages domains add <name> <domain> [flags]
  %[1]s pages domains status <name> <domain> [flags]
  %[1]s pages routes show|test|lint <name> [flags]
  %[1]s pages routes list|add|rm <name> ...
  %[1]s pages settings show|set <name> [flags]
  %[1]s pages preset list|show|set <name> [preset|none]   cache and security header presets
  %[1]s pages submissions <site> | rm <site> <form> <id> [flags]
  %[1]s pages hook create|rotate|rm|show <site> [flags]
  %[1]s pages notify show|set|off|reset <site> [flags]

Publishes a folder or .zip as a static site: no build, no source control.
The upload limit comes from %[2]s. "deploy" accepts every flag of "%[1]s deploy".
`, prog, envUploadMaxBytes)
}

// pagesValueFlags are the "deploy" flags that consume the next argument.
var pagesValueFlags = map[string]bool{
	"app": true, "name": true, "port": true, "env": true, "build-env": true, "include": true,
	"upload-hash": true, "timeout": true, "token": true, "api-url": true, "profile": true,
	"output": true, "query": true, "domain": true,
	"git": true, "url": true, "starter": true, "branch": true, "build-command": true, "output-dir": true,
}

// pagesArgs splits deploy args into the --domain value, the rest, and the positional path.
func pagesArgs(args []string) (domain string, rest []string, dir string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") || a == "-" {
			dir = a
			rest = append(rest, a)
			continue
		}
		name, val, hasVal := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if name == "domain" {
			if !hasVal && i+1 < len(args) {
				i++
				val = args[i]
			}
			domain = val
			continue
		}
		rest = append(rest, a)
		if pagesValueFlags[name] && !hasVal && i+1 < len(args) {
			i++
			rest = append(rest, args[i])
		}
	}
	return domain, rest, dir
}

// pagesFlagValue reads one flag's value from args without a flag set.
func pagesFlagValue(args []string, names ...string) string {
	for i, a := range args {
		name, val, hasVal := strings.Cut(strings.TrimLeft(a, "-"), "=")
		for _, n := range names {
			if strings.HasPrefix(a, "-") && name == n {
				if hasVal {
					return val
				}
				if i+1 < len(args) {
					return args[i+1]
				}
			}
		}
	}
	return ""
}

func runPagesDeploy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		_, _ = fmt.Fprint(stderr, pagesUsage(prog))
		return exitOK
	}
	if repo := pagesFlagValue(args, "git"); repo != "" {
		return runPagesDeployGit(prog, repo, args, stdout, stderr, lookupEnv)
	}
	if src := pagesFlagValue(args, "url"); src != "" {
		return runPagesDeployURL(prog, src, args, stdout, stderr, lookupEnv)
	}
	domain, rest, dir := pagesArgs(args)
	if dir == "" {
		dir = "."
	}
	code := runDeployUpload(prog, append([]string{"--static"}, rest...), stdout, stderr, lookupEnv, stdin)
	if code != exitOK || domain == "" || pagesHasFlag(rest, "dry-run") {
		return code
	}
	name := pagesFlagValue(rest, "app", "name")
	if name == "" {
		name = staticAppName(dir)
	}
	client := apiClientFromFlags(prog, pagesFlagValue(rest, "api-url"), pagesFlagValue(rest, "token"), pagesFlagValue(rest, "profile"), lookupEnv)
	res, err := client.AddStaticSiteDomain(context.Background(), name, domain)
	if err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("site %q is live but adding domain %q failed: %w", name, domain, err))
	}
	printPagesDomain(stderr, res)
	return exitOK
}

// runPagesDeployGit creates a site from a repository and starts its first build in one call.
func runPagesDeployGit(prog, repo string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	name := pagesFlagValue(args, "app", "name")
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(strings.TrimRight(repo, "/")), ".git")
	}
	git := apiclient.StaticSiteGit{RepoURL: repo, Branch: pagesFlagValue(args, "branch"), BuildCommand: pagesFlagValue(args, "build-command"), OutputDir: pagesFlagValue(args, "output-dir")}
	client := apiClientFromFlags(prog, pagesFlagValue(args, "api-url"), pagesFlagValue(args, "token"), pagesFlagValue(args, "profile"), lookupEnv)
	res, err := client.CreateStaticSiteFromGit(context.Background(), name, git, true)
	if err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("create site %q from %s: %w", name, repo, err))
	}
	_, _ = fmt.Fprintf(stdout, "site %s created from %s, first build started", res.Name, repo)
	if res.DeployID != "" {
		_, _ = fmt.Fprintf(stdout, " (deploy %s)", res.DeployID)
	}
	_, _ = fmt.Fprintln(stdout)
	return exitOK
}

func runPagesShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "pages show", "print the site as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "pages show", "site name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	site, err := client.GetStaticSite(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("show site %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, site, func() { printPagesSite(stdout, site) })
}

func printPagesSite(out io.Writer, s staticSiteDetail) {
	_, _ = fmt.Fprintf(out, "name:       %s\npublished:  %t\nfiles:      %d (%s)\ndomains:    %s\n", s.Name, s.Published, s.FileCount, humanBytes(s.SizeBytes), strings.Join(s.Domains, ", "))
	if d := s.LatestDeploy; d != nil {
		_, _ = fmt.Fprintf(out, "latest:     %s %s (%s)\n", d.ID, d.Status, d.Source)
		if d.Error != "" {
			_, _ = fmt.Fprintf(out, "error:      %s\n", d.Error)
		}
	}
}

// previousPagesDeploy follows the production history, not the list order: it returns
// the content that was live before the current deploy, resolving promote and rollback
// copies to the deploy they re-published, so a rollback after a promote undoes the promote.
func previousPagesDeploy(attempts []deployAttemptResource) string {
	var done []deployAttemptResource
	for _, a := range attempts {
		if a.Status == "succeeded" {
			done = append(done, a)
		}
	}
	sort.Slice(done, func(i, j int) bool { return done[i].StartedAt.After(done[j].StartedAt) })
	cur := -1
	for i, a := range done {
		if a.Live {
			cur = i
			break
		}
	}
	if cur < 0 {
		if len(done) == 0 {
			return ""
		}
		cur = 0
	}
	current := pagesDeployContent(done[cur])
	for _, a := range done[cur+1:] {
		if c := pagesDeployContent(a); c != current {
			return c
		}
	}
	return ""
}

// pagesDeployContent is the deploy whose files a row serves: a promote or rollback copy
// names its source in Reason, every other row serves its own files.
func pagesDeployContent(a deployAttemptResource) string {
	if src, ok := strings.CutPrefix(strings.TrimSpace(a.Reason), "RollbackTo:"); ok && strings.TrimSpace(src) != "" {
		return strings.TrimSpace(src)
	}
	return a.ID
}

func runPagesRollback(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "pages rollback", "print the result as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) < 1 || len(rest) > 2 {
		_, _ = fmt.Fprintf(stderr, "%s: pages rollback requires a site name and an optional deploy id\n", prog)
		return exitUsage
	}
	name := rest[0]
	ctx := context.Background()
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	version := ""
	if len(rest) == 2 {
		version = rest[1]
	} else {
		attempts, err := client.ListDeployAttempts(ctx, name)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list deploys of %q: %w", name, err))
		}
		if version = previousPagesDeploy(attempts); version == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("site %q has no earlier successful deploy to roll back to", name))
		}
	}
	res, err := client.RollbackToDeploy(ctx, name, version, apiclient.RollbackToRequest{})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("roll back site %q to %q: %w", name, version, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]any{"site": name, "rolled_back_to": version, "deploy_id": res.DeployID}, func() {
		_, _ = fmt.Fprintf(stdout, "site %q rolled back to deploy %s\n", name, version)
	})
}

func runPagesDomains(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || (args[0] != "add" && args[0] != "status") {
		_, _ = fmt.Fprint(stderr, pagesUsage(prog))
		return exitUsage
	}
	verb := args[0]
	label := "pages domains " + verb
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, label, "print the domain result as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, label, "a site name and a domain", 2)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	var res staticSiteDomain
	var err error
	if verb == "add" {
		res, err = client.AddStaticSiteDomain(context.Background(), rest[0], rest[1])
	} else {
		res, err = client.StaticSiteDomainStatus(context.Background(), rest[0], rest[1])
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s %q on site %q: %w", verb, rest[1], rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printPagesDomain(stdout, res) })
}

func printPagesDomain(out io.Writer, d staticSiteDomain) {
	_, _ = fmt.Fprintf(out, "domain: %s\nmode:   %s\nDNS:    verified=%t\ncert:   %s\n", d.Domain, d.Mode, d.Verified, d.Cert)
	if d.Mode == "manual" && len(d.Records) > 0 {
		_, _ = fmt.Fprintln(out, "create these DNS records at your DNS provider:")
	}
	if len(d.Records) > 0 {
		tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "TYPE\tNAME\tVALUE")
		for _, r := range d.Records {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Type, r.Name, r.Value)
		}
		_ = tw.Flush()
	}
	if d.Detail != "" {
		_, _ = fmt.Fprintln(out, d.Detail)
	}
	printPagesDiagnosis(out, d.Diagnosis)
}

func pagesHasFlag(args []string, name string) bool {
	for _, a := range args {
		if n, _, _ := strings.Cut(strings.TrimLeft(a, "-"), "="); strings.HasPrefix(a, "-") && n == name {
			return true
		}
	}
	return false
}

// looksLikeStaticSite is true for a folder with index.html and no container or compose files.
func looksLikeStaticSite(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		return false
	}
	for _, f := range []string{"Dockerfile", "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml", "package.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return false
		}
	}
	return true
}
