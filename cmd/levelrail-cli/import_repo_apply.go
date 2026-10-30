package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// repoApplyFlags carries the per-run options of "import repo apply".
type repoApplyFlags struct {
	branch, dir, packages, project, environment string
	dryRun, yes, deploy                         bool
	set, secret                                 stringMapFlag
}

func runImportRepoApply(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "import repo apply", "print the result as JSON to stdout and nothing else", stderr)
	f := repoApplyFlags{set: stringMapFlag{}, secret: stringMapFlag{}}
	fs.StringVar(&f.branch, "branch", "", "branch to import (default: the repository's default branch)")
	fs.StringVar(&f.dir, "dir", "", "only import packages under this directory")
	fs.StringVar(&f.packages, "packages", "", "comma separated package names, directories or app names to import (default: all)")
	fs.StringVar(&f.project, "project", "", "project id for every created app")
	fs.StringVar(&f.environment, "environment", "", "environment id for every created app")
	fs.BoolVar(&f.dryRun, "dry-run", false, "validate and print what would happen, create nothing")
	fs.BoolVar(&f.yes, "yes", false, "confirm an import above the large-import threshold")
	fs.BoolVar(&f.deploy, "deploy", false, "start the first build of every created app")
	fs.Var(f.set, "set", "env value as PACKAGE.KEY=VALUE, repeatable")
	fs.Var(f.secret, "secret", "secret env value as PACKAGE.KEY=VALUE, repeatable")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s import repo apply <repo-url> [flags]\n\nPlans the repository, then creates one app per package. Failed apps do not stop\nindependent ones; dependents of a failed app are skipped. Re-running skips apps\nthat already track the same package. Exits non-zero if any app failed.\n\nFlags:\n  --branch B, --dir D, --packages a,b, --project P, --environment E\n  --set PACKAGE.KEY=VALUE     env value, repeatable\n  --secret PACKAGE.KEY=VALUE  secret env value, repeatable\n  --dry-run                   create nothing\n  --yes                       confirm a large import\n  --deploy                    start the first build of each app\n  --token, --api-url, --profile, --json, --output, --query   as for every command\n", prog)
	}
	client, repo, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "import repo apply", "repository URL"}, lookupEnv)
	if !ok {
		return exitCode
	}
	ctx := context.Background()
	plan, err := client.PlanImportRepo(ctx, apiclient.ImportRepoPlanRequest{RepoURL: repo, Branch: f.branch, BaseDirectory: f.dir})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("plan repository import: %w", err))
	}
	pkgs, err := repoApplyPackages(plan, f)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	req := apiclient.ImportRepoApplyRequest{RepoURL: plan.RepoURL, Branch: plan.Branch, BaseDirectory: plan.BaseDirectory, Packages: pkgs, DryRun: true}
	res, err := client.ApplyImportRepo(ctx, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("check repository import: %w", err))
	}
	if !f.dryRun {
		if res.NeedsConfirmation && !f.yes {
			printRepoApply(stderr, res)
			return reportError(stdout, stderr, jsonOut, newValidationError("this import creates more than %d apps; review the list above and re-run with --yes", res.ConfirmAbove))
		}
		req.DryRun, req.Confirm = false, f.yes
		if res, err = client.ApplyImportRepo(ctx, req); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("apply repository import: %w", err))
		}
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() { printRepoApply(stdout, res) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if res.Counts["failed"] > 0 || res.Counts["skipped_dependency"] > 0 {
		return exitValidation
	}
	return exitOK
}

// repoApplyPackages turns the plan into apply rows, applying the flag filters and overrides.
func repoApplyPackages(plan apiclient.RepoPlan, f repoApplyFlags) ([]apiclient.ImportRepoApplyPackage, error) {
	want := map[string]bool{}
	for _, p := range strings.Split(f.packages, ",") {
		if p = strings.TrimSpace(p); p != "" {
			want[p] = true
		}
	}
	var out []apiclient.ImportRepoApplyPackage
	used := map[string]bool{}
	for _, r := range plan.Packages {
		if len(want) > 0 && !want[r.PackageName] && !want[r.Directory] && !want[r.Name] {
			continue
		}
		p := apiclient.ImportRepoApplyPackage{Package: r.PackageName, Name: r.Name, Directory: r.Directory, ProjectID: f.project, EnvironmentID: f.environment, Deploy: f.deploy}
		if r.Port != nil {
			p.Port = *r.Port
		}
		for _, src := range []struct {
			vals   stringMapFlag
			secret bool
		}{{f.set, false}, {f.secret, true}} {
			for k, v := range src.vals {
				pkg, key, ok := strings.Cut(k, ".")
				if !ok || key == "" || (pkg != r.PackageName && pkg != r.Name) {
					continue
				}
				used[k] = true
				if p.Env == nil {
					p.Env = map[string]string{}
				}
				p.Env[key] = v
				if src.secret {
					p.SecretEnvKeys = append(p.SecretEnvKeys, key)
				}
			}
		}
		sort.Strings(p.SecretEnvKeys)
		out = append(out, p)
	}
	for _, m := range []stringMapFlag{f.set, f.secret} {
		for k := range m {
			if !used[k] {
				return nil, newValidationError("%q matches no selected package; use PACKAGE.KEY=VALUE with a package name from the plan", k)
			}
		}
	}
	if len(out) == 0 {
		return nil, newValidationError("no packages selected; run \"import repo plan\" to list them")
	}
	return out, nil
}

func printRepoApply(out io.Writer, r apiclient.ImportRepoApplyResponse) {
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tDIRECTORY\tSTATUS\tDETAIL")
	for _, x := range r.Results {
		detail := strings.TrimSpace(x.Code + " " + x.Message)
		if x.DeployError != "" {
			detail = strings.TrimSpace(detail + " (deploy: " + x.DeployError + ")")
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", x.Name, x.Directory, x.Status, dash(detail))
	}
	_ = tw.Flush()
	keys := make([]string, 0, len(r.Counts))
	for k := range r.Counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d %s", r.Counts[k], k))
	}
	prefix := ""
	if r.DryRun {
		prefix = "dry run: "
	}
	_, _ = fmt.Fprintf(out, "\n%s%s\n", prefix, strings.Join(parts, ", "))
}
