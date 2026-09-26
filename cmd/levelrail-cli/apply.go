package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/iac"
	"golang.org/x/term"
)

// Exit codes of apply, diff and export. They follow terraform's
// -detailed-exitcode: any failure is 1, pending changes are 2.
const (
	iacExitError   = 1
	iacExitPending = 2
)

type applyFlags struct {
	files           stringListFlag
	dryRun          *bool
	prune           *bool
	yes             *bool
	project         *string
	source          *string
	noDeploy        *bool
	continueOnError *bool
	exitCode        *bool
	secrets         stringMapFlag
	vars            stringMapFlag
}

func applyUsage(prog, name string) string {
	return fmt.Sprintf(`Usage:
  %[1]s %[2]s -f file|dir|- [flags]

Validates the resource files, prints the plan, and (apply only) applies it
through the API with your own permissions. Exit codes: 0 no changes or
applied, 1 error, 2 changes pending (with --exit-code, or always for diff).

Flags:
  -f PATH              resource file, directory or "-" for stdin (repeatable)
  --dry-run            print the plan, change nothing
  --exit-code          with --dry-run, exit 2 when changes are pending
  --prune              delete what this --source created and the files no longer declare
  --source NAME        tag apps this run manages; required by --prune
  --project P          only the documents belonging to project P
  --yes                do not ask for confirmation
  --secret K=env:VAR   store a secret value read from an env var (or K=file:PATH); K or app/K
  --var NAME=VALUE     value for a ${{ env.NAME }} placeholder (default: the environment)
  --no-deploy          do not restart running apps to apply env changes
  --continue-on-error  keep applying after an item fails
%[3]s`, prog, name, commonFlagsHelp)
}

func bindApplyFlags(fs *flag.FlagSet, diff bool) *applyFlags {
	f := &applyFlags{secrets: stringMapFlag{}, vars: stringMapFlag{}}
	fs.Var(&f.files, "f", "resource file, directory or - for stdin (repeatable)")
	f.dryRun = fs.Bool("dry-run", diff, "print the plan and change nothing")
	f.prune = fs.Bool("prune", false, "delete managed resources absent from the files")
	f.yes = fs.Bool("yes", false, "do not ask for confirmation")
	f.project = fs.String("project", "", "only the documents belonging to this project")
	f.source = fs.String("source", "", "name of this apply source, tags the apps it manages")
	f.noDeploy = fs.Bool("no-deploy", false, "do not restart running apps to apply env changes")
	f.continueOnError = fs.Bool("continue-on-error", false, "keep applying after an item fails")
	f.exitCode = fs.Bool("exit-code", diff, "with --dry-run, exit 2 when changes are pending")
	fs.Var(f.secrets, "secret", "K=env:VAR or K=file:PATH, store a secret value at apply time")
	fs.Var(f.vars, "var", "NAME=VALUE for a ${{ env.NAME }} placeholder")
	return f
}

func runApply(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runApplyKind(prog, "apply", false, args, stdout, stderr, lookupEnv)
}

func runDiff(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runApplyKind(prog, "diff", true, args, stdout, stderr, lookupEnv)
}

func runApplyKind(prog, name string, diff bool, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var af *applyFlags
	c, code, ok := parseCLICall(prog, name, applyUsage(prog, name), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) { af = bindApplyFlags(fs, diff) })
	if !ok {
		return code
	}
	req, err := buildIaCRequest(af, lookupEnv)
	if err != nil {
		return iacFail(c, err)
	}
	ctx := context.Background()
	plan, err := c.client.PlanIaC(ctx, req)
	if err != nil {
		return iacFail(c, err)
	}
	stopAfterPlan := *af.dryRun || !plan.Pending() || (plan.Summary.Error > 0 && !*af.continueOnError)
	if stopAfterPlan {
		if c.jsonOut {
			_ = c.render(plan, func() {})
		} else {
			printPlan(stdout, plan)
		}
	} else if !c.jsonOut {
		printPlan(stdout, plan)
	}
	if !*af.dryRun && plan.Summary.Error > 0 && !*af.continueOnError {
		_, _ = fmt.Fprintln(stderr, "the plan has errors; nothing was applied (use --continue-on-error to apply the rest)")
		return iacExitError
	}
	if stopAfterPlan {
		return planExit(plan, af)
	}
	if !*af.yes {
		if err := confirmApply(stderr, af, plan); err != nil {
			return iacFail(c, err)
		}
	}
	req.ExpectedPlanHash = plan.Hash
	res, err := c.client.ApplyIaC(ctx, req)
	if err != nil {
		return iacFail(c, err)
	}
	if c.jsonOut {
		_ = c.render(res, func() {})
	} else {
		printApplyResult(stdout, res)
	}
	if !res.OK() {
		return iacExitError
	}
	return exitOK
}

func planExit(plan iac.Plan, af *applyFlags) int {
	switch {
	case plan.Summary.Error > 0:
		return iacExitError
	case plan.Pending() && *af.exitCode:
		return iacExitPending
	}
	return exitOK
}

func iacFail(c cliCall, err error) int {
	_ = c.fail(err)
	return iacExitError
}

func buildIaCRequest(af *applyFlags, lookupEnv func(string) (string, bool)) (apiclient.IaCRequest, error) {
	if len(af.files) == 0 {
		return apiclient.IaCRequest{}, newValidationError("pass at least one -f file, directory or -")
	}
	if *af.prune && *af.source == "" {
		return apiclient.IaCRequest{}, newValidationError("--prune needs --source: only resources that source created are ever deleted")
	}
	files, err := readIaCFiles(af.files)
	if err != nil {
		return apiclient.IaCRequest{}, err
	}
	secrets, err := resolveSecretFlags(af.secrets, lookupEnv)
	if err != nil {
		return apiclient.IaCRequest{}, err
	}
	return apiclient.IaCRequest{
		Files: files, Source: *af.source, Project: *af.project, Prune: *af.prune, Secrets: secrets,
		Vars: collectVars(files, af.vars, lookupEnv), NoDeploy: *af.noDeploy, ContinueOnError: *af.continueOnError,
	}, nil
}

func confirmApply(stderr io.Writer, af *applyFlags, plan iac.Plan) error {
	for _, f := range af.files {
		if f == "-" {
			return errors.New("stdin holds the files, so confirmation is not possible: pass --yes")
		}
	}
	if f, ok := stdinSource.(*os.File); !ok || !term.IsTerminal(int(f.Fd())) { //nolint:gosec // a terminal file descriptor always fits an int
		return errors.New("not a terminal: pass --yes to apply without confirmation")
	}
	q := fmt.Sprintf("Apply %d change(s)", plan.Summary.Create+plan.Summary.Update+plan.Summary.Delete)
	if plan.Summary.Delete > 0 {
		q += fmt.Sprintf(", including %d DELETION(S)", plan.Summary.Delete)
	}
	_, _ = fmt.Fprintf(stderr, "%s? [y/N] ", q)
	line, _ := bufio.NewReader(stdinSource).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
		return errors.New("cancelled")
	}
	return nil
}

func exportUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s export [--project P] [--app A] [-o DIR|-] [--include-env-values=false] [flags]

Writes the live state as resource documents. Output is stable, so exporting
twice gives identical files. Secret values are never written: secret env vars
become secretRef, and secret looking values become ${{ env.NAME }} placeholders.

Flags:
  --project P                  only this project and what it holds
  --app A                      only this app and its load balancer, pipelines and alert rules
  -o DIR|-                     directory to write one file per document, or - for stdout (default -)
  --include-env-values=false   write every plain env value as a ${{ env.NAME }} placeholder
%[2]s`, prog, commonFlagsHelp)
}

func runExport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var project, app, outDir *string
	var includeEnv *bool
	c, code, ok := parseCLICall(prog, "export", exportUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		project = fs.String("project", "", "only this project")
		app = fs.String("app", "", "only this app")
		outDir = fs.String("o", "-", "output directory, or - for stdout")
		includeEnv = fs.Bool("include-env-values", true, "write plain env values (false writes placeholders)")
	})
	if !ok {
		return code
	}
	res, err := c.client.ExportIaC(context.Background(), *project, *app, *includeEnv)
	if err != nil {
		return iacFail(c, err)
	}
	for _, w := range res.Warnings {
		_, _ = fmt.Fprintln(stderr, "warning: "+w)
	}
	if c.jsonOut {
		return c.render(res, func() {})
	}
	if *outDir == "-" {
		_, _ = fmt.Fprint(stdout, res.Join())
		return exitOK
	}
	if err := writeExport(*outDir, res); err != nil {
		return iacFail(c, err)
	}
	_, _ = fmt.Fprintf(stderr, "wrote %d file(s) to %s\n", len(res.Files), *outDir)
	return exitOK
}

func writeExport(dir string, res iac.ExportResult) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	for _, f := range res.Files {
		name := filepath.Base(f.Name)
		if name != f.Name || strings.HasPrefix(name, ".") {
			return fmt.Errorf("refusing to write unexpected file name %q", f.Name)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(f.Content), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}
