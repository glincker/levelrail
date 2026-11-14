package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/iac"
)

// adoptTarget is a referencing App document and the file its revision goes to.
type adoptTarget struct {
	App, Path string
}

type adoptWritten struct {
	App      string   `json:"app"`
	Path     string   `json:"path"`
	Revision int      `json:"revision"`
	Notes    []string `json:"notes,omitempty"`
}

type adoptResult struct {
	Stack   string         `json:"stack"`
	Written []adoptWritten `json:"written"`
	Noop    bool           `json:"noop"`
	Summary iac.Summary    `json:"summary"`
	Next    []string       `json:"next"`
}

// adoptTargets reads the App documents that reference a compose file.
func adoptTargets(files []apiclient.IaCFile, only string) ([]adoptTarget, error) {
	var out []adoptTarget
	for _, f := range files {
		dec := yaml.NewDecoder(strings.NewReader(f.Content))
		for {
			var d struct {
				Kind     string `yaml:"kind"`
				Metadata struct {
					Name string `yaml:"name"`
				} `yaml:"metadata"`
				Spec struct {
					Compose struct {
						Path string `yaml:"path"`
					} `yaml:"compose"`
				} `yaml:"spec"`
			}
			if err := dec.Decode(&d); err != nil {
				break
			}
			if d.Kind != "App" || d.Spec.Compose.Path == "" || (only != "" && d.Metadata.Name != only) {
				continue
			}
			clean, err := iac.CleanComposePath(d.Spec.Compose.Path)
			if err != nil {
				return nil, newValidationError("App %s: %v", d.Metadata.Name, err)
			}
			out = append(out, adoptTarget{App: d.Metadata.Name, Path: clean})
		}
	}
	if len(out) == 0 {
		return nil, newValidationError("no App document with spec.compose.path found in the -f files (write the referencing App first, see import compose-ref --reference)")
	}
	return out, nil
}

// adoptRoot is the folder compose paths resolve from: the -f directory, or the folder of a -f file.
func adoptRoot(p string) (string, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if fi.IsDir() {
		return p, nil
	}
	return filepath.Dir(p), nil
}

func stackAdoptFromExport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var mf *stackManifestFlags
	var appFlag *string
	var force *bool
	c, code, ok := parseCLICall(prog, "stack adopt-from-export", stackCmdUsage(prog, "adopt-from-export", "<stack> -f DIR [--app NAME] [--force]", `
For an already running compose App, writes its current deployed compose
revision next to the referencing App document (the file spec.compose.path
names), then plans and shows that the first plan is empty. Existing files are
kept unless --force. Needs the referencing App document to exist already.
Environment values are never read: the revision body is the compose file as
it was deployed, before interpolation.

Flags:
  -f PATH        folder (or file) with the referencing App document (repeatable)
  --app NAME     adopt only this App
  --force        overwrite an existing compose file
  --var, --secret, --values  placeholder and secret values for the verifying plan
`), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		mf = bindStackManifestFlags(fs, true)
		appFlag = fs.String("app", "", "adopt only this App")
		force = fs.Bool("force", false, "overwrite an existing compose file")
	})
	if !ok {
		return code
	}
	pos, code, ok := stackPositionals(c, prog, "adopt-from-export", "<stack> -f DIR", 1)
	if !ok {
		return code
	}
	if len(mf.files) != 1 || mf.files[0] == "-" {
		return c.invalid("pass exactly one -f folder that holds the referencing App document")
	}
	root, err := adoptRoot(mf.files[0])
	if err != nil {
		return c.fail(err)
	}
	files, err := readIaCFilesNamed(mf.files, true)
	if err != nil {
		return c.fail(err)
	}
	targets, err := adoptTargets(files, *appFlag)
	if err != nil {
		return c.fail(err)
	}
	ctx := context.Background()
	var written []adoptWritten
	for _, t := range targets {
		w, err := adoptOne(ctx, c.client, root, t, *force)
		if err != nil {
			return c.fail(err)
		}
		written = append(written, w)
	}
	res := adoptResult{Stack: pos[0], Written: written}
	plan, err := adoptVerifyPlan(ctx, c, mf, lookupEnv)
	if err != nil {
		return iacFail(c, err)
	}
	res.Summary, res.Noop = plan.Summary, !plan.Pending() && plan.Summary.Error == 0
	res.Next = []string{
		fmt.Sprintf("%s apply -f %s --dry-run   (the plan should show only noop)", prog, mf.files[0]),
		fmt.Sprintf("%s apply -f %s --yes   (from now on the file is the source of truth)", prog, mf.files[0]),
	}
	if rc := c.render(res, func() { printAdopt(c.stdout, res) }); rc != exitOK {
		return rc
	}
	switch {
	case plan.Summary.Error > 0:
		return iacExitError
	case plan.Pending():
		return iacExitPending
	}
	return exitOK
}

func adoptOne(ctx context.Context, cl *apiclient.Client, root string, t adoptTarget, force bool) (adoptWritten, error) {
	w := adoptWritten{App: t.App, Path: t.Path}
	target := filepath.Join(root, filepath.FromSlash(t.Path))
	if _, err := os.Lstat(target); err == nil && !force {
		return w, newValidationError("%s already exists, pass --force to overwrite it", target)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return w, err
	}
	stack, err := cl.GetComposeStack(ctx, t.App)
	if err != nil {
		return w, fmt.Errorf("app %s: %w", t.App, err)
	}
	if stack.Revision == 0 {
		return w, newValidationError("app %s has no deployed compose revision to adopt", t.App)
	}
	rev, err := cl.GetComposeRevision(ctx, t.App, stack.Revision)
	if err != nil {
		return w, fmt.Errorf("app %s: %w", t.App, err)
	}
	if rev.Compose == "" {
		return w, newValidationError("app %s revision %d has no stored compose body (a git sourced app keeps its file in the repository)", t.App, stack.Revision)
	}
	if len(rev.Compose) > compose.LoadStrictLimits().MaxBytes {
		return w, newValidationError("app %s revision %d is larger than the compose size cap", t.App, stack.Revision)
	}
	w.Revision = stack.Revision
	if _, err := iac.StrictComposeCheck([]byte(rev.Compose), t.Path, requiredEnvNames(rev.Compose)); err != nil {
		for _, is := range iac.ComposeStrictIssues(t.Path, err) {
			w.Notes = append(w.Notes, fmt.Sprintf("%s: %s", is.Code, is.Message))
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil { //nolint:gosec // reviewable project folder
		return w, err
	}
	if err := os.WriteFile(target, []byte(rev.Compose), 0o644); err != nil { //nolint:gosec // a compose file meant to be committed
		return w, err
	}
	return w, nil
}

func requiredEnvNames(body string) map[string]string {
	m := map[string]string{}
	for _, n := range requiredComposeVars([]byte(body)) {
		m[n] = "set"
	}
	return m
}

func adoptVerifyPlan(ctx context.Context, c cliCall, mf *stackManifestFlags, lookupEnv func(string) (string, bool)) (iac.Plan, error) {
	files, att, err := readIaCInput(mf.files)
	if err != nil {
		return iac.Plan{}, err
	}
	vars, err := resolveVars(files, varSources{vars: mf.vars, varFiles: mf.varFiles, values: mf.values, allowEnv: mf.allowEnv}, lookupEnv)
	if err != nil {
		return iac.Plan{}, err
	}
	secrets, err := resolveSecretFlags(mf.secrets, lookupEnv)
	if err != nil {
		return iac.Plan{}, err
	}
	return c.client.PlanIaC(ctx, apiclient.IaCRequest{Files: files, Project: *mf.project, Vars: vars, Secrets: secrets, Attachments: att})
}

func printAdopt(w io.Writer, r adoptResult) {
	for _, a := range r.Written {
		_, _ = fmt.Fprintf(w, "wrote %s from revision %d of %s\n", a.Path, a.Revision, a.App)
		for _, n := range a.Notes {
			_, _ = fmt.Fprintf(w, "  NOTE %s\n", n)
		}
	}
	if r.Noop {
		_, _ = fmt.Fprintf(w, "plan: nothing to change, %s is adopted\n", r.Stack)
	} else {
		_, _ = fmt.Fprintf(w, "plan: %d create, %d update, %d delete, %d error: the first plan is not empty, review it before applying\n", r.Summary.Create, r.Summary.Update, r.Summary.Delete, r.Summary.Error)
	}
	_, _ = fmt.Fprintln(w, "\nNEXT STEPS")
	for _, n := range r.Next {
		_, _ = fmt.Fprintf(w, "  %s\n", n)
	}
}
