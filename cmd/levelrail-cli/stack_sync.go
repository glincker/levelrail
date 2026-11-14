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

func stackSyncUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s stack sync <command> [flags]

Keep a stack in step with a git branch. Apply, prune and deploy are all off
until you turn them on.

Commands:
  status <stack>                         show the sync settings and the latest result
  set <stack> [flags]                    create or change the sync; flags not given keep the current value (root or stack admin)
  refresh <stack>                        fetch the branch and plan it once now
  webhook-secret rotate <stack>          rotate the webhook secret; shown once

Run '%[1]s stack sync <command> --help' for the flags of a command.
`, prog)
}

func runStackSync(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprint(stderr, stackSyncUsage(prog))
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	switch args[0] {
	case "status":
		return stackSyncStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return stackSyncSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "refresh":
		return stackSyncRefresh(prog, args[1:], stdout, stderr, lookupEnv)
	case "webhook-secret":
		if len(args) > 1 && args[1] == "rotate" {
			return stackSyncRotate(prog, args[2:], stdout, stderr, lookupEnv)
		}
		_, _ = fmt.Fprintf(stderr, "usage: %s stack sync webhook-secret rotate <stack>\n", prog)
		return exitUsage
	}
	_, _ = fmt.Fprintf(stderr, "%s stack sync: unknown command %q\n\n%s", prog, args[0], stackSyncUsage(prog))
	return exitUsage
}

// redactSyncInfo hides which app lends its credential: the output only says
// that a reference is set.
func redactSyncInfo(in apiclient.StackSyncInfo) apiclient.StackSyncInfo {
	if in.Config != nil && in.Config.CredentialRef != "" {
		cfg := *in.Config
		cfg.CredentialRef = "set"
		in.Config = &cfg
	}
	return in
}

func syncOnOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func stackSyncStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "stack sync status", stackCmdUsage(prog, "sync status", "<stack>", ""), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	pos, code, ok := stackPositionals(c, prog, "sync status", "<stack>", 1)
	if !ok {
		return code
	}
	info, _, err := c.client.GetStackSync(context.Background(), pos[0])
	if err != nil {
		return iacFail(c, err)
	}
	info = redactSyncInfo(info)
	return c.render(info, func() {
		if info.Config == nil {
			_, _ = fmt.Fprintf(c.stdout, "stack %s has no git sync. Set one with: %s stack sync set %s --repo URL --branch B --path P\n", pos[0], prog, pos[0])
			return
		}
		printSyncConfig(c.stdout, pos[0], *info.Config)
		_, _ = fmt.Fprintf(c.stdout, "webhook secret: %s\n", map[bool]string{true: "set", false: "not set (run: " + prog + " stack sync webhook-secret rotate " + pos[0] + ")"}[info.WebhookSecretSet])
		if st := info.Status; st != nil {
			printSyncStatus(c.stdout, *st)
		} else {
			_, _ = fmt.Fprintln(c.stdout, "no sync has run yet")
		}
	})
}

func printSyncConfig(w io.Writer, stack string, cfg apiclient.StackSyncConfig) {
	state := "disabled"
	if cfg.Enabled {
		state = "enabled"
	}
	_, _ = fmt.Fprintf(w, "stack %s sync: %s\n  repo: %s\n  branch: %s\n  path: %s\n", stack, state, cfg.RepoURL, cfg.Branch, cfg.Path)
	if cfg.CredentialRef != "" {
		_, _ = fmt.Fprintln(w, "  credential: set")
	}
	_, _ = fmt.Fprintf(w, "  interval: %ds  webhook only: %s\n  apply: %s  prune: %s  deploy: %s\n",
		cfg.IntervalSeconds, syncOnOff(cfg.WebhookOnly), syncOnOff(cfg.Apply), syncOnOff(cfg.Prune), syncOnOff(cfg.Deploy))
}

func printSyncStatus(w io.Writer, st apiclient.StackSyncStatus) {
	_, _ = fmt.Fprintf(w, "last check: %s  commit: %s  pending: %t\n", st.CheckedAt, st.Commit, st.Pending)
	if len(st.Counts) > 0 {
		keys := make([]string, 0, len(st.Counts))
		for k := range st.Counts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s %d", k, st.Counts[k]))
		}
		_, _ = fmt.Fprintf(w, "plan: %s\n", strings.Join(parts, ", "))
	}
	if st.Code != "" {
		_, _ = fmt.Fprintf(w, "failed: %s %s\n", st.Code, st.Detail)
	}
	if st.AppliedSHA != "" {
		_, _ = fmt.Fprintf(w, "applied commit: %s (revision %d)\n", st.AppliedSHA, st.Revision)
	}
}

func stackSyncSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var repo, branch, path, cred *string
	var syncFlags *flag.FlagSet
	var clearCred *bool
	var interval *int
	var webhookOnly, apply, prune, deploy, enable, disable *bool
	c, code, ok := parseCLICall(prog, "stack sync set", stackCmdUsage(prog, "sync set", "<stack> --repo URL --branch B --path P [flags]", `
Flags:
  --repo URL            https or ssh git URL (required for a new sync)
  --branch B            the only branch that triggers a sync (required for a new sync)
  --path P              folder in the repository that holds the documents ("" or "." is the root; required for a new sync)
  --credential-ref REF  app:NAME, a connected app whose git credential reads a private repo (administrator)
  --clear-credential    drop the credential reference
  --interval SECONDS    poll interval, 0 uses the control plane default
  --webhook-only        no polling, only the webhook and a manual refresh
  --apply               apply a pending plan (default off: plan only)
  --prune               let the apply remove resources that left the documents (needs --apply)
  --deploy              let the apply roll out changed apps (needs --apply)
  --enable, --disable   turn the sync on or off; omitted keeps the current state, new syncs start off
`), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		syncFlags = fs
		clearCred = fs.Bool("clear-credential", false, "drop the credential reference")
		repo = fs.String("repo", "", "git repository URL")
		branch = fs.String("branch", "", "branch to follow")
		path = fs.String("path", "", "folder in the repository")
		cred = fs.String("credential-ref", "", "app:NAME")
		interval = fs.Int("interval", 0, "poll interval in seconds")
		webhookOnly = fs.Bool("webhook-only", false, "no polling")
		apply = fs.Bool("apply", false, "apply pending plans")
		prune = fs.Bool("prune", false, "allow prune on apply")
		deploy = fs.Bool("deploy", false, "allow deploys on apply")
		enable = fs.Bool("enable", false, "turn the sync on")
		disable = fs.Bool("disable", false, "turn the sync off")
	})
	if !ok {
		return code
	}
	pos, code, ok := stackPositionals(c, prog, "sync set", "<stack> --repo URL --branch B --path P", 1)
	if !ok {
		return code
	}
	given := map[string]bool{}
	syncFlags.Visit(func(f *flag.Flag) { given[f.Name] = true })
	if *enable && *disable {
		return c.invalid("--enable and --disable cannot be used together")
	}
	if (given["prune"] && *prune || given["deploy"] && *deploy) && given["apply"] && !*apply {
		return c.invalid("prune and deploy need apply: a sync that does not apply never prunes or deploys")
	}
	ctx := context.Background()
	// A failed read only loses the "changed" summary; the PUT reports real errors.
	prev, _, _ := c.client.GetStackSync(ctx, pos[0])
	switch {
	case *clearCred && given["credential-ref"]:
		return c.invalid("--clear-credential and --credential-ref cannot be used together")
	case prev.Config == nil && (!given["repo"] || !given["branch"] || !given["path"]):
		return c.invalid("a new sync needs --repo, --branch and --path")
	}
	// Absent flags keep the current settings; only the flags given override them.
	var cfg apiclient.StackSyncConfig
	if prev.Config != nil {
		cfg = *prev.Config
	}
	if given["repo"] {
		cfg.RepoURL = *repo
	}
	if given["branch"] {
		cfg.Branch = *branch
	}
	if given["path"] {
		cfg.Path = *path
	}
	if given["credential-ref"] {
		cfg.CredentialRef = *cred
	}
	if *clearCred {
		cfg.CredentialRef = ""
	}
	if given["interval"] {
		cfg.IntervalSeconds = *interval
	}
	for name, dst := range map[string]*bool{"webhook-only": &cfg.WebhookOnly, "apply": &cfg.Apply, "prune": &cfg.Prune, "deploy": &cfg.Deploy} {
		if given[name] {
			*dst = map[string]bool{"webhook-only": *webhookOnly, "apply": *apply, "prune": *prune, "deploy": *deploy}[name]
		}
	}
	switch {
	case *enable:
		cfg.Enabled = true
	case *disable:
		cfg.Enabled = false
	}
	switch {
	case cfg.RepoURL == "" || cfg.Branch == "":
		return c.invalid("--repo and --branch are required")
	case cfg.Prune && !cfg.Apply:
		return c.invalid("prune needs apply: a sync that does not apply never prunes")
	case cfg.Deploy && !cfg.Apply:
		return c.invalid("deploy needs apply: a sync that does not apply never deploys")
	}
	info, _, err := c.client.PutStackSync(ctx, pos[0], cfg)
	if err != nil {
		return iacFail(c, err)
	}
	info = redactSyncInfo(info)
	return c.render(info, func() {
		if info.Config == nil {
			return
		}
		printSyncConfig(c.stdout, pos[0], *info.Config)
		if prev.Config == nil {
			_, _ = fmt.Fprintln(c.stdout, "created a new sync")
		} else if changed := syncChangedFields(*prev.Config, *info.Config, prev.Config.CredentialRef != cfg.CredentialRef); len(changed) > 0 {
			_, _ = fmt.Fprintf(c.stdout, "changed: %s\n", strings.Join(changed, ", "))
		} else {
			_, _ = fmt.Fprintln(c.stdout, "nothing changed")
		}
		if !info.Config.Apply {
			_, _ = fmt.Fprintln(c.stdout, "reminder: apply is off, a sync only plans and reports pending changes")
		}
		if !info.Config.Prune {
			_, _ = fmt.Fprintln(c.stdout, "reminder: prune is off, nothing is ever removed by a sync")
		}
		if !info.Config.Deploy {
			_, _ = fmt.Fprintln(c.stdout, "reminder: deploy is off, a sync never rolls out apps")
		}
	})
}

// syncChangedFields names the settings that differ; credentialChanged is passed
// separately because the redacted copy cannot be compared.
func syncChangedFields(prev, now apiclient.StackSyncConfig, credentialChanged bool) []string {
	var out []string
	add := func(diff bool, name string) {
		if diff {
			out = append(out, name)
		}
	}
	add(prev.RepoURL != now.RepoURL, "repo")
	add(prev.Branch != now.Branch, "branch")
	add(prev.Path != now.Path, "path")
	add(credentialChanged, "credential-ref")
	add(prev.IntervalSeconds != now.IntervalSeconds, "interval")
	add(prev.WebhookOnly != now.WebhookOnly, "webhook-only")
	add(prev.Enabled != now.Enabled, "enabled")
	add(prev.Apply != now.Apply, "apply")
	add(prev.Prune != now.Prune, "prune")
	add(prev.Deploy != now.Deploy, "deploy")
	return out
}

func stackSyncRefresh(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "stack sync refresh", stackCmdUsage(prog, "sync refresh", "<stack>", "\nFetches the branch and plans it once now. It applies only when the sync has apply on.\n"), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	pos, code, ok := stackPositionals(c, prog, "sync refresh", "<stack>", 1)
	if !ok {
		return code
	}
	st, _, err := c.client.RefreshStackSync(context.Background(), pos[0])
	if err != nil {
		return iacFail(c, err)
	}
	return c.render(st, func() {
		_, _ = fmt.Fprintf(c.stdout, "refreshed stack %s\n", pos[0])
		printSyncStatus(c.stdout, st)
	})
}

func stackSyncRotate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "stack sync webhook-secret rotate", stackCmdUsage(prog, "sync webhook-secret rotate", "<stack>", "\nPrints the new secret once. The old secret stops working at once.\n"), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	pos, code, ok := stackPositionals(c, prog, "sync webhook-secret rotate", "<stack> (after 'rotate')", 1)
	if !ok {
		return code
	}
	sec, _, err := c.client.RotateStackSyncSecret(context.Background(), pos[0])
	if err != nil {
		return iacFail(c, err)
	}
	if !c.jsonOut {
		_, _ = fmt.Fprintln(c.stderr, "warning: this secret will not be shown again; store it in your git host webhook settings now")
	}
	return c.render(sec, func() {
		_, _ = fmt.Fprintf(c.stdout, "secret: %s\nsync url: %s\nrefresh url: %s\n", sec.Secret, sec.SyncURL, sec.RefreshURL)
	})
}
