package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/iac"
)

const rollbackWarning = "Rollback restores configuration only. Secret values and database contents are NOT rolled back."

func stackUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s stack <command> [flags]

Stacks keep a named set of resources in git, plan each change, apply it as a
numbered revision and roll back to any earlier one. Needs APP_EXPERIMENTAL=iac.

Commands:
  list                                  list stacks
  show <name>                           one stack with its owned resources
  create <name> --project P             create an empty stack
  plan <name> -f PATH [flags]           show what an apply would change
  apply <name> -f PATH [flags]          apply the files as a new revision
  history <name> [--limit N]            revisions, newest first
  show-revision <name> <n>              documents, plan and result of one revision
  diff <name> <n> [--against M|live]    compare a revision with another or with live state
  rollback <name> <n> [--yes]           apply the documents of revision n as a new revision
  pin <name> <n>                        refuse applies except a rollback to revision n
  unpin <name>                          remove the pin
  unlock <name>                         release a stuck apply lease (root only)
  delete <name> [--mode forget|destroy] stop managing a stack, or also delete its resources
  settings <name> [flags]               show or change --allow-prune, --drift, --require-approval (root only)

Run '%[1]s stack <command> --help' for the flags of a command.
`, prog)
}

func runStack(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprint(stderr, stackUsage(prog))
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	rest := args[1:]
	switch args[0] {
	case "list":
		return stackList(prog, rest, stdout, stderr, lookupEnv)
	case "show":
		return stackShow(prog, rest, stdout, stderr, lookupEnv)
	case "create":
		return stackCreate(prog, rest, stdout, stderr, lookupEnv)
	case "plan":
		return stackPlan(prog, rest, stdout, stderr, lookupEnv)
	case "apply":
		return stackApply(prog, rest, stdout, stderr, lookupEnv)
	case "history":
		return stackHistory(prog, rest, stdout, stderr, lookupEnv)
	case "show-revision":
		return stackShowRevision(prog, rest, stdout, stderr, lookupEnv)
	case "diff":
		return stackDiff(prog, rest, stdout, stderr, lookupEnv)
	case "rollback":
		return stackRollback(prog, rest, stdout, stderr, lookupEnv)
	case "pin":
		return stackPin(prog, rest, stdout, stderr, lookupEnv)
	case "unpin":
		return stackSimple(prog, "unpin", rest, stdout, stderr, lookupEnv)
	case "unlock":
		return stackSimple(prog, "unlock", rest, stdout, stderr, lookupEnv)
	case "delete":
		return stackDelete(prog, rest, stdout, stderr, lookupEnv)
	case "settings":
		return stackSettings(prog, rest, stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s stack: unknown command %q\n\n%s", prog, args[0], stackUsage(prog))
	return exitUsage
}

func stackCmdUsage(prog, sub, synopsis, flags string) string {
	return fmt.Sprintf("Usage:\n  %s stack %s %s\n%s%s", prog, sub, synopsis, flags, commonFlagsHelp)
}

// stackPositionals checks the positional count after flags were parsed.
func stackPositionals(c cliCall, prog, sub, synopsis string, want int) ([]string, int, bool) {
	pos := c.fs.Args()
	if len(pos) != want {
		code := c.invalid("usage: %s stack %s %s", prog, sub, synopsis)
		return nil, code, false
	}
	return pos, exitOK, true
}

func parseRevision(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, newValidationError("revision must be a positive number, got %q", s)
	}
	return n, nil
}

func stackList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "stack list", stackCmdUsage(prog, "list", "", ""), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	list, raw, err := c.client.ListStacks(context.Background())
	if err != nil {
		return iacFail(c, err)
	}
	return c.render(raw, func() { printStackList(c.stdout, list) })
}

func stackShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "stack show", stackCmdUsage(prog, "show", "<name>", ""), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	pos, code, ok := stackPositionals(c, prog, "show", "<name>", 1)
	if !ok {
		return code
	}
	s, raw, err := c.client.GetStack(context.Background(), pos[0])
	if err != nil {
		return iacFail(c, err)
	}
	return c.render(raw, func() { printStack(c.stdout, s) })
}

func stackSettingsFlags(fs *flag.FlagSet) (allowPrune *bool, drift *string, approval *bool) {
	allowPrune = fs.Bool("allow-prune", false, "let this stack delete resources it created (root only)")
	drift = fs.String("drift", "", "drift policy: warn or lock (root only)")
	approval = fs.Bool("require-approval", false, "hold applies for approval (root only)")
	return
}

func stackCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var project, drift *string
	var allowPrune, approval *bool
	c, code, ok := parseCLICall(prog, "stack create", stackCmdUsage(prog, "create", "<name> --project P [flags]", `
Flags:
  --project P          project the stack belongs to (required)
  --allow-prune        let this stack delete resources it created (root only)
  --drift warn|lock    drift policy (root only)
  --require-approval   hold applies for approval (root only)
`), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		project = fs.String("project", "", "project the stack belongs to")
		allowPrune, drift, approval = stackSettingsFlags(fs)
	})
	if !ok {
		return code
	}
	pos, code, ok := stackPositionals(c, prog, "create", "<name> --project P", 1)
	if !ok {
		return code
	}
	if *project == "" {
		return c.invalid("--project is required")
	}
	req := apiclient.StackCreateRequest{Name: pos[0], Project: *project, DriftPolicy: *drift}
	if flagWasSet(c.fs, "allow-prune") {
		req.AllowPrune = allowPrune
	}
	if flagWasSet(c.fs, "require-approval") {
		req.RequireApproval = approval
	}
	s, raw, err := c.client.CreateStack(context.Background(), req)
	if err != nil {
		return iacFail(c, err)
	}
	return c.render(raw, func() { _, _ = fmt.Fprintf(c.stdout, "created stack %s in project %s\n", s.Name, s.Project) })
}

func stackSettings(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var drift *string
	var allowPrune, approval *bool
	c, code, ok := parseCLICall(prog, "stack settings", stackCmdUsage(prog, "settings", "<name> [--allow-prune[=false]] [--drift warn|lock] [--require-approval[=false]]", `
Without flags, prints the current settings. Changing them needs a root token.
`), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) { allowPrune, drift, approval = stackSettingsFlags(fs) })
	if !ok {
		return code
	}
	pos, code, ok := stackPositionals(c, prog, "settings", "<name> [flags]", 1)
	if !ok {
		return code
	}
	var set apiclient.StackSettings
	changed := false
	if flagWasSet(c.fs, "allow-prune") {
		set.AllowPrune, changed = allowPrune, true
	}
	if flagWasSet(c.fs, "drift") {
		if *drift != "warn" && *drift != "lock" {
			return c.invalid("--drift must be warn or lock")
		}
		set.DriftPolicy, changed = drift, true
	}
	if flagWasSet(c.fs, "require-approval") {
		set.RequireApproval, changed = approval, true
	}
	var s apiclient.StackInfo
	var raw json.RawMessage
	var err error
	if changed {
		s, raw, err = c.client.UpdateStackSettings(context.Background(), pos[0], set)
	} else {
		s, raw, err = c.client.GetStack(context.Background(), pos[0])
	}
	if err != nil {
		return iacFail(c, err)
	}
	return c.render(raw, func() { printStackSettings(c.stdout, s) })
}

func stackSimple(prog, sub string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "stack "+sub, stackCmdUsage(prog, sub, "<name>", ""), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	pos, code, ok := stackPositionals(c, prog, sub, "<name>", 1)
	if !ok {
		return code
	}
	var raw json.RawMessage
	var err error
	msg := ""
	if sub == "unpin" {
		_, raw, err = c.client.UnpinStack(context.Background(), pos[0])
		msg = "removed the pin from stack " + pos[0]
	} else {
		_, raw, err = c.client.UnlockStack(context.Background(), pos[0])
		msg = "released the apply lease of stack " + pos[0]
	}
	if err != nil {
		return iacFail(c, err)
	}
	return c.render(raw, func() { _, _ = fmt.Fprintln(c.stdout, msg) })
}

func stackPin(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "stack pin", stackCmdUsage(prog, "pin", "<name> <n>", "\nWhile pinned, applies are refused except a rollback to revision n.\nRemove the pin with: "+prog+" stack unpin <name>\n"), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	pos, code, ok := stackPositionals(c, prog, "pin", "<name> <n>", 2)
	if !ok {
		return code
	}
	n, err := parseRevision(pos[1])
	if err != nil {
		return c.fail(err)
	}
	_, raw, err := c.client.PinStack(context.Background(), pos[0], n)
	if err != nil {
		return iacFail(c, err)
	}
	return c.render(raw, func() {
		_, _ = fmt.Fprintf(c.stdout, "pinned stack %s to revision %d\n", pos[0], n)
	})
}

func stackDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var mode *string
	var yes *bool
	c, code, ok := parseCLICall(prog, "stack delete", stackCmdUsage(prog, "delete", "<name> [--mode forget|destroy] [--yes]", `
  --mode forget    stop managing the stack, keep every resource (default)
  --mode destroy   also delete the resources the stack owns (root only)
  --yes            do not ask for confirmation
`), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		mode = fs.String("mode", "forget", "forget or destroy")
		yes = fs.Bool("yes", false, "do not ask for confirmation")
	})
	if !ok {
		return code
	}
	pos, code, ok := stackPositionals(c, prog, "delete", "<name> [--mode forget|destroy]", 1)
	if !ok {
		return code
	}
	if *mode != "forget" && *mode != "destroy" {
		return c.invalid("--mode must be forget or destroy")
	}
	if !*yes {
		q := fmt.Sprintf("Forget stack %s (resources are kept)", pos[0])
		if *mode == "destroy" {
			q = fmt.Sprintf("DESTROY stack %s and delete every resource it owns", pos[0])
		}
		if err := stackConfirm(stderr, q); err != nil {
			return iacFail(c, err)
		}
	}
	res, raw, err := c.client.DeleteStack(context.Background(), pos[0], *mode)
	if err != nil {
		return iacFail(c, err)
	}
	rc := c.render(raw, func() { printStackDelete(c.stdout, res) })
	if len(res.Failed) > 0 {
		return iacExitError
	}
	return rc
}

func stackHistory(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var limit *int
	c, code, ok := parseCLICall(prog, "stack history", stackCmdUsage(prog, "history", "<name> [--limit N]", ""), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		limit = fs.Int("limit", 0, "show at most this many revisions (default: the server's default)")
	})
	if !ok {
		return code
	}
	pos, code, ok := stackPositionals(c, prog, "history", "<name> [--limit N]", 1)
	if !ok {
		return code
	}
	h, raw, err := c.client.StackHistory(context.Background(), pos[0], *limit)
	if err != nil {
		return iacFail(c, err)
	}
	return c.render(raw, func() { printStackHistory(c.stdout, h) })
}

func stackShowRevision(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "stack show-revision", stackCmdUsage(prog, "show-revision", "<name> <n>", ""), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	pos, code, ok := stackPositionals(c, prog, "show-revision", "<name> <n>", 2)
	if !ok {
		return code
	}
	n, err := parseRevision(pos[1])
	if err != nil {
		return c.fail(err)
	}
	d, raw, err := c.client.StackRevision(context.Background(), pos[0], n)
	if err != nil {
		return iacFail(c, err)
	}
	return c.render(raw, func() { printStackRevision(c.stdout, pos[0], d) })
}

func stackDiff(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var against *string
	c, code, ok := parseCLICall(prog, "stack diff", stackCmdUsage(prog, "diff", "<name> <n> [--against M|live]", "\nWithout --against, revision n is compared with the one before it.\n"), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		against = fs.String("against", "", "revision number or live")
	})
	if !ok {
		return code
	}
	pos, code, ok := stackPositionals(c, prog, "diff", "<name> <n> [--against M|live]", 2)
	if !ok {
		return code
	}
	n, err := parseRevision(pos[1])
	if err != nil {
		return c.fail(err)
	}
	if *against != "" && *against != "live" {
		if _, err := parseRevision(*against); err != nil {
			return c.invalid("--against must be a revision number or live")
		}
	}
	d, raw, err := c.client.StackRevisionDiff(context.Background(), pos[0], n, *against)
	if err != nil {
		return iacFail(c, err)
	}
	return c.render(raw, func() { printStackDiff(c.stdout, d) })
}

// stackManifestFlags are the flags shared by plan, apply and rollback.
type stackManifestFlags struct {
	files    stringListFlag
	project  *string
	prune    *bool
	vars     stringMapFlag
	varFiles stringListFlag
	allowEnv stringListFlag
	secrets  stringMapFlag
}

func bindStackManifestFlags(fs *flag.FlagSet, withFiles bool) *stackManifestFlags {
	f := &stackManifestFlags{vars: stringMapFlag{}, secrets: stringMapFlag{}}
	if withFiles {
		fs.Var(&f.files, "f", "resource file, directory or - for stdin (repeatable)")
		f.project = fs.String("project", "", "project of the stack (default: the Stack document)")
		f.prune = fs.Bool("prune", false, "delete resources the files no longer declare (default: the Stack document)")
		fs.Var(f.vars, "var", "NAME=VALUE for a ${{ env.NAME }} placeholder")
		fs.Var(&f.varFiles, "var-file", "file of KEY=VALUE lines for placeholders (repeatable)")
		fs.Var(&f.allowEnv, "allow-env", "NAME[,NAME] or PREFIX* that placeholders may read from the environment")
	}
	fs.Var(f.secrets, "secret", "NAME=ENVVAR, read the secret value from a local environment variable (also env:VAR or file:PATH)")
	return f
}

var stackDocSplit = regexp.MustCompile(`(?m)^---[ \t]*$`)

type stackDocHead struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec iac.StackSpec `yaml:"spec"`
}

// stripStackDocs removes kind: Stack documents because the resource parser does
// not know them. The first one found supplies the defaults.
func stripStackDocs(files []apiclient.IaCFile) (out []apiclient.IaCFile, stack *stackDocHead, err error) {
	for _, f := range files {
		var kept []string
		found := false
		for _, chunk := range stackDocSplit.Split(f.Content, -1) {
			var head stackDocHead
			if yaml.Unmarshal([]byte(chunk), &head) == nil && head.Kind == string(iac.KindStack) {
				found = true
				if stack != nil {
					return nil, nil, newValidationError("more than one Stack document (%s)", f.Name)
				}
				h := head
				stack = &h
				continue
			}
			if strings.TrimSpace(chunk) != "" {
				kept = append(kept, chunk)
			}
		}
		if !found {
			out = append(out, f)
			continue
		}
		if len(kept) > 0 {
			out = append(out, apiclient.IaCFile{Name: f.Name, Content: strings.Join(kept, "\n---\n")})
		}
	}
	return out, stack, nil
}

// buildStackRequest reads the files, drops the Stack document and resolves
// vars and secrets. Flags win over the Stack document.
func buildStackRequest(name string, f *stackManifestFlags, lookupEnv func(string) (string, bool)) (apiclient.StackRequest, error) {
	if len(f.files) == 0 {
		return apiclient.StackRequest{}, newValidationError("pass at least one -f file, directory or -")
	}
	all, err := readIaCFiles(f.files)
	if err != nil {
		return apiclient.StackRequest{}, err
	}
	files, doc, err := stripStackDocs(all)
	if err != nil {
		return apiclient.StackRequest{}, err
	}
	if len(files) == 0 {
		return apiclient.StackRequest{}, newValidationError("the files hold only a Stack document, add the resources to apply")
	}
	req := apiclient.StackRequest{Files: files, Project: *f.project, Prune: *f.prune}
	if doc != nil {
		if doc.Metadata.Name != "" && doc.Metadata.Name != name {
			return apiclient.StackRequest{}, newValidationError("the Stack document is named %q but the command names %q", doc.Metadata.Name, name)
		}
		if req.Project == "" {
			req.Project = doc.Spec.Project
		}
		req.Prune = req.Prune || doc.Spec.Prune
	}
	if req.Vars, err = resolveVars(files, varSources{vars: f.vars, varFiles: f.varFiles, allowEnv: f.allowEnv}, lookupEnv); err != nil {
		return apiclient.StackRequest{}, err
	}
	return req, nil
}

// resolveStackSecrets accepts NAME=ENVVAR as well as the env: and file: forms.
func resolveStackSecrets(specs map[string]string, lookupEnv func(string) (string, bool)) (map[string]string, error) {
	norm := make(map[string]string, len(specs))
	for k, v := range specs {
		if !strings.HasPrefix(v, "env:") && !strings.HasPrefix(v, "file:") {
			v = "env:" + v
		}
		norm[k] = v
	}
	return resolveSecretFlags(norm, lookupEnv)
}

// redactWriter hides secret values from anything the command prints, such as
// a server message that echoes a request.
type redactWriter struct {
	w      io.Writer
	values []string
}

func (r redactWriter) Write(p []byte) (int, error) {
	out := string(p)
	for _, v := range r.values {
		out = strings.ReplaceAll(out, v, "***")
	}
	if _, err := io.WriteString(r.w, out); err != nil {
		return 0, err
	}
	return len(p), nil
}

func redactCall(c cliCall, secrets map[string]string) cliCall {
	var vals []string
	for _, v := range secrets {
		if len(v) >= 4 {
			vals = append(vals, v)
		}
	}
	if len(vals) == 0 {
		return c
	}
	c.stdout = redactWriter{c.stdout, vals}
	c.stderr = redactWriter{c.stderr, vals}
	return c
}

func stackPlan(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var mf *stackManifestFlags
	var exitCode *bool
	c, code, ok := parseCLICall(prog, "stack plan", stackCmdUsage(prog, "plan", "<name> -f file|dir [flags]", `
Prints what an apply would change and changes nothing. Any kind: Stack
document in the files is removed before sending; it supplies the default
project and prune. Exit codes: 0 ok, 1 error, 2 changes pending (--exit-code).

Flags:
  -f PATH              resource file, directory or "-" for stdin (repeatable)
  --project P          project of the stack (needed when the stack does not exist yet)
  --prune              plan deletions of resources the files no longer declare
  --var NAME=VALUE     value for a ${{ env.NAME }} placeholder (repeatable)
  --var-file PATH      KEY=VALUE lines for placeholders (repeatable)
  --allow-env NAMES    names (or PREFIX*) placeholders may read from your environment
  --exit-code          exit 2 when changes are pending
`), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		mf = bindStackManifestFlags(fs, true)
		exitCode = fs.Bool("exit-code", false, "exit 2 when changes are pending")
	})
	if !ok {
		return code
	}
	pos, code, ok := stackPositionals(c, prog, "plan", "<name> -f file|dir", 1)
	if !ok {
		return code
	}
	req, err := buildStackRequest(pos[0], mf, lookupEnv)
	if err != nil {
		return iacFail(c, err)
	}
	res, raw, err := c.client.PlanStack(context.Background(), pos[0], req)
	if err != nil {
		return iacFail(c, err)
	}
	if rc := c.render(raw, func() { printStackPlan(c.stdout, res) }); rc != exitOK {
		return rc
	}
	switch {
	case res.Plan.Summary.Error > 0:
		return iacExitError
	case res.Plan.Pending() && *exitCode:
		return iacExitPending
	}
	return exitOK
}

// stackRunFlags are the flags of the calls that write a revision.
type stackRunFlags struct {
	*stackManifestFlags
	expectedHash    *string
	noDeploy        *bool
	continueOnError *bool
	idemKey         *string
	yes             *bool
}

func bindStackRunFlags(fs *flag.FlagSet, withFiles bool) *stackRunFlags {
	f := &stackRunFlags{stackManifestFlags: bindStackManifestFlags(fs, withFiles)}
	f.expectedHash = fs.String("expected-plan-hash", "", "refuse to apply when the plan no longer has this hash")
	f.noDeploy = fs.Bool("no-deploy", false, "do not restart running apps to apply env changes")
	f.continueOnError = fs.Bool("continue-on-error", false, "keep applying after an item fails")
	f.idemKey = fs.String("idempotency-key", "", "make a retry return the stored revision (default: derived from the CI run and the files)")
	f.yes = fs.Bool("yes", false, "do not ask for confirmation")
	return f
}

func (f *stackRunFlags) fill(req *apiclient.StackRequest, secrets map[string]string, lookupEnv func(string) (string, bool)) {
	req.Secrets = secrets
	req.NoDeploy = *f.noDeploy
	req.ContinueOnError = *f.continueOnError
	req.ExpectedPlanHash = *f.expectedHash
	meta := detectStackGit(lookupEnv)
	req.GitRepo, req.GitRef, req.GitSHA, req.CIRunURL = meta.repo, meta.ref, meta.sha, meta.runURL
}

const stackRunFlagsHelp = `
  --expected-plan-hash H   refuse (409) when the plan no longer has this hash
  --no-deploy              do not restart running apps to apply env changes
  --continue-on-error      keep applying after an item fails
  --secret NAME=ENVVAR     secret value read from a local environment variable; never printed or stored
  --idempotency-key K      a retry with the same key returns the stored revision
  --yes                    do not ask for confirmation
`

func stackApply(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var rf *stackRunFlags
	c, code, ok := parseCLICall(prog, "stack apply", stackCmdUsage(prog, "apply", "<name> -f file|dir [flags]", `
Applies the files as the next revision. Git repository, ref, commit and CI run
are filled from GITHUB_* variables or the local checkout. Exit codes: 0 applied,
1 error, 409/422 or a revision that ended partial or failed.

Flags:
  -f PATH, --project P, --prune, --var, --var-file, --allow-env   as for plan`+stackRunFlagsHelp), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) { rf = bindStackRunFlags(fs, true) })
	if !ok {
		return code
	}
	pos, code, ok := stackPositionals(c, prog, "apply", "<name> -f file|dir", 1)
	if !ok {
		return code
	}
	name := pos[0]
	req, err := buildStackRequest(name, rf.stackManifestFlags, lookupEnv)
	if err != nil {
		return iacFail(c, err)
	}
	secrets, err := resolveStackSecrets(rf.secrets, lookupEnv)
	if err != nil {
		return iacFail(c, err)
	}
	c = redactCall(c, secrets)
	rf.fill(&req, secrets, lookupEnv)
	ctx := context.Background()
	if !*rf.yes {
		if rc, done := stackConfirmPlan(ctx, c, rf, name, &req); done {
			return rc
		}
	}
	req.IdempotencyKey = *rf.idemKey
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = idempotencyKey(lookupEnv, time.Now(), "stack-apply", name, filesDigest(req.Files), req.ExpectedPlanHash)
	}
	res, raw, err := c.client.ApplyStack(ctx, name, req)
	if err != nil {
		return iacFail(c, err)
	}
	return finishStackRun(c, raw, res, "applied")
}

// stackConfirmPlan prints the plan and asks. It pins the apply to the plan the
// operator saw unless --expected-plan-hash was given. done is true when the
// command should stop with rc.
func stackConfirmPlan(ctx context.Context, c cliCall, rf *stackRunFlags, name string, req *apiclient.StackRequest) (rc int, done bool) {
	for _, f := range rf.files {
		if f == "-" {
			return iacFail(c, errors.New("stdin holds the files, so confirmation is not possible: pass --yes")), true
		}
	}
	plan, _, err := c.client.PlanStack(ctx, name, *req)
	if err != nil {
		return iacFail(c, err), true
	}
	if !c.jsonOut {
		printStackPlan(c.stdout, plan)
	}
	if plan.Plan.Summary.Error > 0 && !*rf.continueOnError {
		_, _ = fmt.Fprintln(c.stderr, "the plan has errors; nothing was applied (use --continue-on-error to apply the rest)")
		return iacExitError, true
	}
	if !plan.Plan.Pending() {
		return exitOK, true
	}
	q := fmt.Sprintf("Apply %d change(s) to stack %s", plan.Plan.Summary.Create+plan.Plan.Summary.Update+plan.Plan.Summary.Delete, name)
	if plan.Plan.Summary.Delete > 0 {
		q += fmt.Sprintf(", including %d DELETION(S)", plan.Plan.Summary.Delete)
	}
	if err := stackConfirm(c.stderr, q); err != nil {
		return iacFail(c, err), true
	}
	if req.ExpectedPlanHash == "" {
		req.ExpectedPlanHash = plan.Plan.Hash
	}
	return exitOK, false
}

func finishStackRun(c cliCall, raw json.RawMessage, res apiclient.StackApplyResponse, verb string) int {
	rc := c.render(raw, func() { printStackRun(c.stdout, res, verb) })
	if rc != exitOK {
		return rc
	}
	if res.Revision == nil {
		return iacExitError
	}
	switch iac.RevisionStatus(res.Revision.Status) {
	case iac.RevisionPartial, iac.RevisionFailed:
		_, _ = fmt.Fprintf(c.stderr, "revision %d ended %s\n", res.Revision.Number, res.Revision.Status)
		return iacExitError
	}
	return exitOK
}

func stackRollback(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var rf *stackRunFlags
	c, code, ok := parseCLICall(prog, "stack rollback", stackCmdUsage(prog, "rollback", "<name> <n> [flags]", `
Applies the documents of revision n as a new revision. History is not rewritten.
`+rollbackWarning+`

Flags:`+stackRunFlagsHelp), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) { rf = bindStackRunFlags(fs, false) })
	if !ok {
		return code
	}
	pos, code, ok := stackPositionals(c, prog, "rollback", "<name> <n>", 2)
	if !ok {
		return code
	}
	n, err := parseRevision(pos[1])
	if err != nil {
		return c.fail(err)
	}
	secrets, err := resolveStackSecrets(rf.secrets, lookupEnv)
	if err != nil {
		return iacFail(c, err)
	}
	c = redactCall(c, secrets)
	_, _ = fmt.Fprintln(c.stderr, "warning: "+rollbackWarning)
	if !*rf.yes {
		if err := stackConfirm(c.stderr, fmt.Sprintf("Roll back stack %s to revision %d", pos[0], n)); err != nil {
			return iacFail(c, err)
		}
	}
	var req apiclient.StackRequest
	rf.fill(&req, secrets, lookupEnv)
	req.IdempotencyKey = *rf.idemKey
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = idempotencyKey(lookupEnv, time.Now(), "stack-rollback", pos[0], strconv.Itoa(n))
	}
	res, raw, err := c.client.RollbackStack(context.Background(), pos[0], n, req)
	if err != nil {
		return iacFail(c, err)
	}
	return finishStackRun(c, raw, res, "rolled back")
}

func filesDigest(files []apiclient.IaCFile) string {
	h := sha256.New()
	for _, f := range files {
		_, _ = io.WriteString(h, f.Name+"\x00"+f.Content+"\x00")
	}
	return hex.EncodeToString(h.Sum(nil))
}

// stackConfirm asks on the terminal; scripts must pass --yes.
func stackConfirm(stderr io.Writer, question string) error {
	if f, ok := stdinSource.(*os.File); !ok || !term.IsTerminal(int(f.Fd())) { //nolint:gosec // a terminal file descriptor always fits an int
		return errors.New("not a terminal: pass --yes to continue without confirmation")
	}
	_, _ = fmt.Fprintf(stderr, "%s? [y/N] ", question)
	line, _ := bufio.NewReader(stdinSource).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
		return errors.New("cancelled")
	}
	return nil
}

type stackGitMeta struct{ repo, ref, sha, runURL string }

// detectStackGit prefers the CI variables and falls back to the local checkout.
func detectStackGit(lookup func(string) (string, bool)) stackGitMeta {
	get := func(k string) string { v, _ := lookup(k); return strings.TrimSpace(v) }
	m := stackGitMeta{repo: get("GITHUB_REPOSITORY"), ref: get("GITHUB_REF"), sha: get("GITHUB_SHA")}
	if run := get("GITHUB_RUN_ID"); run != "" && m.repo != "" {
		server := get("GITHUB_SERVER_URL")
		if server == "" {
			server = "https://github.com"
		}
		m.runURL = strings.TrimRight(server, "/") + "/" + m.repo + "/actions/runs/" + run
	}
	if m.runURL == "" {
		m.runURL = get("CI_PIPELINE_URL")
	}
	if m.repo != "" && m.ref != "" && m.sha != "" {
		return m
	}
	local := detectGitAt(".")
	if m.repo == "" {
		m.repo = stripURLCredentials(local.RepoURL)
	}
	if m.ref == "" {
		m.ref = local.Ref
	}
	if m.sha == "" {
		m.sha = localGitSHA()
	}
	return m
}

// stripURLCredentials drops user:token@ from a remote URL before it is stored.
func stripURLCredentials(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	u.User = nil
	return u.String()
}

func localGitSHA() string {
	repo, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return ""
	}
	head, err := repo.Head()
	if err != nil {
		return ""
	}
	return head.Hash().String()
}
