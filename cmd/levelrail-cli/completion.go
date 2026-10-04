package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/experimental"
)

// cmdNode is one node of the CLI's command tree: the verbs a command
// accepts, and any further verbs those verbs themselves dispatch to.
type cmdNode struct {
	subs map[string]*cmdNode
}

// cliCommandTree mirrors every "switch args[0]" dispatch layer in this
// package (main.go's run, apps.go, channels.go, ...), by hand: this CLI
// has no cobra-style registry the dispatch switches and completions can
// both read from, so the two are two independent sources of truth. Adding
// or renaming a command means updating both. TestCommandTree_MatchesDispatchSwitches
// in completion_test.go parses every dispatch switch in this package and
// fails if this tree and the real switches disagree, so drift is caught
// at test time rather than silently shipped.
var cliCommandTree = map[string]*cmdNode{
	"deploy":   nil,
	"rollback": nil,
	"apps": {subs: map[string]*cmdNode{
		"freeze":                  {subs: map[string]*cmdNode{"set": nil, "show": nil, "clear": nil}},
		"sbom":                    nil,
		"scan":                    {subs: map[string]*cmdNode{"enable": nil, "disable": nil, "status": nil, "run": nil, "gate": nil, "override": nil}},
		"create":                  nil,
		"list":                    nil,
		"get":                     nil,
		"deploy":                  nil,
		"wait":                    nil,
		"deploy-compose":          nil,
		"validate":                nil,
		"deploy-spec":             nil,
		"group":                   nil,
		"hook-runs":               nil,
		"rollback":                nil,
		"auto-rollback":           {subs: map[string]*cmdNode{"enable": nil, "disable": nil, "status": nil}},
		"auto-rollback-slo-burn":  {subs: map[string]*cmdNode{"set": nil, "status": nil}},
		"timeline":                nil,
		"apply":                   nil,
		"domains":                 {subs: map[string]*cmdNode{"list": nil, "add": nil, "remove": nil}},
		"streams":                 {subs: map[string]*cmdNode{"list": nil, "create": nil, "delete": nil}},
		"restart":                 nil,
		"stop":                    nil,
		"start":                   nil,
		"delete":                  nil,
		"status":                  nil,
		"diagnose":                nil,
		"preflight":               nil,
		"resource-recommendation": nil,
		"cost":                    nil,
		"deploys":                 {subs: map[string]*cmdNode{"list": nil, "show": nil, "wait": nil, "compare": nil, "logs": nil, "failed": nil, "steps": nil, "cancel": nil, "rollback-to": nil}},
		"cancel-superseded":       {subs: map[string]*cmdNode{"enable": nil, "disable": nil, "status": nil}},
		"promote":                 nil,
		"network":                 nil,
		"logs":                    nil,
		"metrics":                 nil,
		"requests":                nil,
		"resource-usage":          nil,
		"overview":                nil,
		"exec":                    nil,
		"exec-access":             {subs: map[string]*cmdNode{"enable": nil, "disable": nil, "status": nil}},
		"log-drain":               {subs: map[string]*cmdNode{"get": nil, "set": nil, "clear": nil}},
		"scheduled-tasks":         {subs: map[string]*cmdNode{"create": nil, "list": nil, "get": nil, "update": nil, "delete": nil, "run": nil}},
		"alerts":                  {subs: map[string]*cmdNode{"list": nil, "create": nil, "update": nil, "delete": nil, "slo": nil}},
		"deploy-notify-targets":   {subs: map[string]*cmdNode{"list": nil, "create": nil, "delete": nil}},
		"organizations": {subs: map[string]*cmdNode{
			"create": nil, "list": nil, "get": nil, "delete": nil,
			"set-project": nil, "clear-project": nil, "env-get": nil, "env-set": nil,
		}},
		"projects":          {subs: map[string]*cmdNode{"create": nil, "list": nil, "get": nil, "delete": nil, "stop": nil, "start": nil, "restart": nil, "env-get": nil, "env-set": nil}},
		"environments":      {subs: map[string]*cmdNode{"create": nil, "list": nil, "update": nil, "delete": nil, "env-get": nil, "env-set": nil, "env-diff": nil, "clone-preview": nil, "clone": nil}},
		"set-environment":   nil,
		"clear-environment": nil,
		"set-project":       nil,
		"clear-project":     nil,
		"set-node":          nil,
		"clear-node":        nil,
		"previews": {subs: map[string]*cmdNode{
			"list": nil, "teardown": nil, "enable": nil, "disable": nil, "sweep": nil, "limits": nil, "approve": nil,
			"pr-status": {subs: map[string]*cmdNode{"enable": nil, "disable": nil}},
		}},
		"env":                {subs: map[string]*cmdNode{"import": nil, "export": nil}},
		"secrets":            {subs: map[string]*cmdNode{"list": nil, "set": nil, "delete": nil, "lock": nil}},
		"git-source":         {subs: map[string]*cmdNode{"get": nil, "set": nil, "settings": nil, "rotate-secret": nil, "delete": nil}},
		"webhook-deliveries": {subs: map[string]*cmdNode{"list": nil, "replay": nil}},
		"bulk":               nil,
		"clone":              nil,
		"save-as-template":   nil,
		"images":             nil,
		"storage":            {subs: map[string]*cmdNode{"set": nil, "clear": nil}},
		"database":           {subs: map[string]*cmdNode{"set": nil, "clear": nil}},
		"connect":            nil,
		"disconnect":         nil,
		"connections":        {subs: map[string]*cmdNode{"list": nil, "suggest": nil}},
		"builds":             {subs: map[string]*cmdNode{"trigger": nil}},
		"moves":              {subs: map[string]*cmdNode{"list": nil, "get": nil}},
		"vault-env":          {subs: map[string]*cmdNode{"set": nil, "clear": nil}},
		"preview-env":        {subs: map[string]*cmdNode{"set": nil, "clear": nil}},
		"branch-env":         {subs: map[string]*cmdNode{"list": nil, "set": nil, "clear": nil}},
		"tag":                nil,
		"untag":              nil,
		"egress":             {subs: map[string]*cmdNode{"get": nil, "set": nil, "clear": nil}},
		"build-cache":        {subs: map[string]*cmdNode{"show": nil, "set": nil, "clear": nil, "remove": nil}},
		"health":             {subs: map[string]*cmdNode{"get": nil, "set": nil, "clear": nil, "discover": nil}},
		"health-score":       nil,
		"volumes":            {subs: map[string]*cmdNode{"get": nil, "attach": nil, "detach": nil}},
		"integrations":       {subs: map[string]*cmdNode{"catalog": nil, "list": nil, "add": nil, "remove": nil}},
	}},
	"models":    {subs: map[string]*cmdNode{"list": nil, "get": nil, "deploy": nil, "logs": nil, "delete": nil, "restart": nil, "rotate-key": nil, "metrics": nil, "fit": nil, "residency": nil, "swap-group": nil, "wake": nil, "sleep": nil, "gpus": nil, "preflight": nil, "cache": {subs: map[string]*cmdNode{"list": nil, "prune": nil}}, "keys": {subs: map[string]*cmdNode{"list": nil, "create": nil, "revoke": nil, "rotate": nil}}, "usage": nil}},
	"databases": {subs: map[string]*cmdNode{"create": nil, "list": nil, "get": nil, "status": nil, "delete": nil, "stop": nil, "start": nil, "resource-recommendation": nil, "metrics": nil, "logs": nil, "slow-queries": nil, "set-project": nil, "clear-project": nil, "set-node": nil, "clear-node": nil, "set-resources": nil, "public-access": {subs: map[string]*cmdNode{"set": nil, "clear": nil}}}},
	"auth": {subs: map[string]*cmdNode{"login": nil, "whoami": nil, "session-link": nil, "2fa": {subs: map[string]*cmdNode{
		"status": nil, "setup": nil, "enable": nil, "disable": nil, "recovery-codes": nil,
	}}}},
	"profile": {subs: map[string]*cmdNode{"list": nil}},
	"tokens":  {subs: map[string]*cmdNode{"create": nil, "list": nil, "revoke": nil}},
	"domains": {subs: map[string]*cmdNode{
		"list":           nil,
		"cloudflare-dns": {subs: map[string]*cmdNode{"get": nil, "set": nil, "clear": nil}},
		"route53-dns":    {subs: map[string]*cmdNode{"get": nil, "set": nil, "clear": nil}},
		"basic-auth":     {subs: map[string]*cmdNode{"get": nil, "set": nil, "clear": nil}},
		"maintenance":    {subs: map[string]*cmdNode{"get": nil, "set": nil, "clear": nil}},
		"redirect":       {subs: map[string]*cmdNode{"get": nil, "set": nil, "clear": nil}},
		"tls-cert":       {subs: map[string]*cmdNode{"get": nil, "set": nil, "clear": nil, "renew": nil}},
		"check":          nil,
		"certificates":   nil,
		"waf":            {subs: map[string]*cmdNode{"get": nil, "set": nil, "clear": nil}},
		"error-pages":    {subs: map[string]*cmdNode{"get": nil, "set": nil, "clear": nil}},
		"dns":            {subs: map[string]*cmdNode{"list": nil, "add": nil, "remove": nil}},
	}},
	"backups": {subs: map[string]*cmdNode{
		"list": nil, "list-all": nil, "trigger": nil, "delete": nil, "download": nil, "restore": nil, "restore-as-new": nil, "restores": nil, "clone-restores": nil, "verify": nil, "verifications": nil,
		"schedule": {subs: map[string]*cmdNode{"set": nil, "clear": nil}},
	}},
	"pitr": {subs: map[string]*cmdNode{
		"enable": nil, "disable": nil, "status": nil, "restore": nil, "restores": nil,
		"base-backups": {subs: map[string]*cmdNode{"list": nil, "trigger": nil}},
	}},
	"app-volume-backups": {subs: map[string]*cmdNode{
		"list": nil, "trigger": nil, "delete": nil, "download": nil, "restore": nil, "restore-as-new": nil, "restores": nil, "clone-restores": nil, "verify": nil, "verifications": nil,
		"schedule": {subs: map[string]*cmdNode{"set": nil, "clear": nil}},
	}},
	"control-plane-backups": {subs: map[string]*cmdNode{"list": nil, "create": nil, "download": nil, "verify": nil, "delete": nil, "schedule": {subs: map[string]*cmdNode{"show": nil, "set": nil}}, "run-now": nil, "drill": {subs: map[string]*cmdNode{"run": nil, "status": nil}}, "escrow": nil, "keys": {subs: map[string]*cmdNode{"generate": nil}}, "help-dr": nil}},
	"cloudflare-tunnel":     {subs: map[string]*cmdNode{"get": nil, "set": nil, "disconnect": nil}},
	"vault":                 {subs: map[string]*cmdNode{"get": nil, "set": nil, "disconnect": nil}},
	"ai": {subs: map[string]*cmdNode{
		"chat":     nil,
		"sessions": {subs: map[string]*cmdNode{"list": nil, "get": nil, "delete": nil, "resolve": nil}},
	}},
	"alerts": {subs: map[string]*cmdNode{
		"silences":    {subs: map[string]*cmdNode{"list": nil, "create": nil, "delete": nil}},
		"silence":     nil,
		"maintenance": {subs: map[string]*cmdNode{"list": nil, "create": nil, "update": nil, "delete": nil}},
		"history":     nil,
	}},
	"status-page": {subs: map[string]*cmdNode{
		"get": nil, "set": nil, "preview": nil,
		"components": {subs: map[string]*cmdNode{"list": nil, "add": nil, "delete": nil}},
		"incidents":  {subs: map[string]*cmdNode{"list": nil, "create": nil, "update": nil, "delete": nil}},
	}},
	"channels":             {subs: map[string]*cmdNode{"list": nil, "create": nil, "update": nil, "delete": nil, "test": nil, "deliveries": nil}},
	"push-subscriptions":   {subs: map[string]*cmdNode{"list": nil, "revoke": nil}},
	"shared-env":           {subs: map[string]*cmdNode{"list": nil, "set": nil, "delete": nil}},
	"backup-targets":       {subs: map[string]*cmdNode{"list": nil, "get": nil, "create": nil, "update": nil, "delete": nil, "test": nil}},
	"firewall":             {subs: map[string]*cmdNode{"list": nil, "allow": nil, "deny": nil, "delete": nil}},
	"storage":              {subs: map[string]*cmdNode{"providers": nil, "list": nil, "add": nil, "test": nil, "delete": nil}},
	"logs":                 {subs: map[string]*cmdNode{"archive": {subs: map[string]*cmdNode{"set": nil, "status": nil, "remove": nil}}, "dump": nil, "ls": nil, "fetch": nil, "query": nil}},
	"registry-credentials": {subs: map[string]*cmdNode{"list": nil, "get": nil, "create": nil, "update": nil, "delete": nil, "test": nil, "repositories": nil, "tags": nil}},
	"registry":             {subs: map[string]*cmdNode{"status": nil, "enable": nil, "disable": nil, "repositories": nil, "tags": nil}},
	"flags":                {subs: map[string]*cmdNode{"create": nil, "list": nil, "get": nil, "set": nil, "delete": nil}},
	"pipelines":            {subs: map[string]*cmdNode{"list": nil, "validate": nil, "save": nil, "delete": nil, "run": nil, "runs": nil, "logs": nil, "cancel": nil, "approve": nil, "sync": nil, "triggers": nil, "oidc": nil}},
	"preview":              {subs: map[string]*cmdNode{"status": nil, "enable": nil, "disable": nil, "capture": nil, "prune": nil}},
	"deployments":          {subs: map[string]*cmdNode{"list": nil, "watch": nil, "summary": nil}},
	"lb":                   {subs: map[string]*cmdNode{"list": nil, "show": nil, "set": nil, "clear": nil, "status": nil, "check": nil, "history": nil, "upstream": nil, "export": nil, "import": nil}},
	"tags":                 {subs: map[string]*cmdNode{"list": nil, "create": nil, "delete": nil, "apps": nil}},
	"nodes": {subs: map[string]*cmdNode{
		"list": nil, "get": nil, "delete": nil, "join-token": nil,
		"cordon": nil, "uncordon": nil, "drain": nil, "workloads": nil,
		"health": nil, "patch-status": nil, "events": nil, "metrics": nil, "resource-usage": nil,
		"capacity-forecast": nil,
		"mesh":              nil, "rotate-key": nil, "rejoin-mesh": nil, "reenroll-token": nil, "revoke-cert": nil,
		"providers":      {subs: map[string]*cmdNode{"list": nil, "set-credential": nil}},
		"provision":      nil,
		"provisions":     {subs: map[string]*cmdNode{"list": nil, "show": nil}},
		"ssh-provision":  nil,
		"ssh-provisions": {subs: map[string]*cmdNode{"list": nil, "show": nil}},
	}},
	"status":                   nil,
	"version":                  nil,
	"changelog":                nil,
	"upgrade":                  nil,
	"audit-log":                nil,
	"audit-purge":              nil,
	"attention":                nil,
	"doctor":                   nil,
	"api-docs":                 nil,
	"init":                     nil,
	"containers":               nil,
	"system-prune":             nil,
	"volumes-orphaned":         nil,
	"volumes-orphaned-cleanup": nil,
	"users":                    {subs: map[string]*cmdNode{"list": nil, "create": nil, "set-abilities": nil, "delete": nil, "roles": nil}},
	"invites":                  {subs: map[string]*cmdNode{"create": nil, "list": nil, "revoke": nil}},
	"iam": {subs: map[string]*cmdNode{
		"policies": {subs: map[string]*cmdNode{
			"create":      nil,
			"list":        nil,
			"get":         nil,
			"update":      nil,
			"delete":      nil,
			"attach":      nil,
			"detach":      nil,
			"attachments": nil,
		}},
	}},
	"secrets":    {subs: map[string]*cmdNode{"rotate-master-key": nil, "binding-status": nil, "rebind": nil}},
	"migrate":    {subs: map[string]*cmdNode{"coolify": nil, "dokploy": nil, "caprover": nil}},
	"apply":      nil,
	"diff":       nil,
	"import":     {subs: map[string]*cmdNode{"platform": {subs: map[string]*cmdNode{"coolify": nil, "dokploy": nil, "caprover": nil}}}},
	"completion": {subs: map[string]*cmdNode{"bash": nil, "zsh": nil, "fish": nil}},
	"settings": {subs: map[string]*cmdNode{
		"oauth":         {subs: map[string]*cmdNode{"list": nil, "set": nil}},
		"email":         {subs: map[string]*cmdNode{"get": nil, "set": nil}},
		"ingress":       {subs: map[string]*cmdNode{"get": nil, "set": nil}},
		"dashboard-url": {subs: map[string]*cmdNode{"get": nil, "set": nil}},
		"ai-assistant":  {subs: map[string]*cmdNode{"get": nil, "set": nil, "clear": nil}},
		"updates":       {subs: map[string]*cmdNode{"get": nil, "set": nil}},
		"deploy-freeze": {subs: map[string]*cmdNode{"show": nil, "set": nil, "clear": nil}},
	}},
	"git-providers": nil,
	"github-app": {subs: map[string]*cmdNode{"status": nil, "disconnect": nil, "repos": nil, "branches": nil, "use-as-source": nil, "installations": {subs: map[string]*cmdNode{
		"list": nil, "add": nil, "remove": nil,
	}}}},
	"gitlab-app":       {subs: map[string]*cmdNode{"status": nil, "disconnect": nil, "projects": nil, "branches": nil, "use-as-source": nil}},
	"bitbucket-app":    {subs: map[string]*cmdNode{"status": nil, "disconnect": nil, "repos": nil, "branches": nil, "use-as-source": nil}},
	"gitea-app":        {subs: map[string]*cmdNode{"status": nil, "disconnect": nil, "repos": nil, "branches": nil, "use-as-source": nil}},
	"templates":        {subs: map[string]*cmdNode{"list": nil, "get": nil, "deploy": nil, "delete": nil}},
	"static-sites":     {subs: map[string]*cmdNode{"list": nil}},
	"deploy-approvals": {subs: map[string]*cmdNode{"list": nil, "get": nil, "approve": nil, "reject": nil}},
	"build":            {subs: map[string]*cmdNode{"detect": nil, "branches": nil}},
}

// globalFlags lists the flags apiFlagSet registers on nearly every
// subcommand (flagutil.go), plus the handful main.go's run() itself
// strips out before dispatch (--debug, see extractDebugFlag), offered as
// completions at every command depth.
var globalFlags = []string{"--json", "--output", "--query", "--token", "--api-url", "--debug", "-h", "--help"}

// treeEntry is cliCommandTree flattened to one entry per node that has
// children: path is the space-joined verb sequence leading to that node
// ("" for the root, "apps" for apps' own verbs, "apps log-drain" for its
// nested verbs), children is that node's own verb names, sorted.
type treeEntry struct {
	path     string
	children []string
}

// walkCommandTree flattens cliCommandTree so every completion script
// generator (bash/zsh/fish) renders from one traversal instead of three
// hand-written copies that could drift from each other.
func walkCommandTree() []treeEntry { return walkTree(true) }

// walkFullCommandTree flattens every command, gated or not, for drift tests.
func walkFullCommandTree() []treeEntry { return walkTree(false) }

func walkTree(hideDisabled bool) []treeEntry {
	var entries []treeEntry
	var walk func(prefix string, node map[string]*cmdNode)
	walk = func(prefix string, node map[string]*cmdNode) {
		names := make([]string, 0, len(node))
		for name := range node {
			if hideDisabled && completionHidden(prefix, name) {
				continue
			}
			names = append(names, name)
		}
		sort.Strings(names)
		entries = append(entries, treeEntry{path: prefix, children: names})
		for _, name := range names {
			child := node[name]
			if child == nil || len(child.subs) == 0 {
				continue
			}
			next := name
			if prefix != "" {
				next = prefix + " " + name
			}
			walk(next, child.subs)
		}
	}
	walk("", cliCommandTree)
	return entries
}

// completionHidden reports whether name under prefix is a gated command that is switched off.
func completionHidden(prefix, name string) bool {
	args := []string{name}
	if prefix != "" {
		args = append(strings.Fields(prefix), name)
	}
	f, gated := experimentalFeatureFor(args)
	return gated && !experimental.Enabled(f)
}

// renderChildrenFunc renders a shell function named funcName that maps a
// space-joined command path (its one argument) to that node's children,
// space-joined, or an empty line for an unknown path. The "case ... esac"
// syntax used here is valid in both bash and zsh, so bashCompletionScript
// and zshCompletionScript share this instead of each hand-rendering their
// own copy of the tree.
func renderChildrenFunc(funcName string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s() {\n  case \"$1\" in\n", funcName)
	for _, e := range walkCommandTree() {
		fmt.Fprintf(&b, "    %q) echo %q ;;\n", e.path, strings.Join(e.children, " "))
	}
	b.WriteString("    *) echo \"\" ;;\n  esac\n}\n")
	return b.String()
}

// sanitizeIdent turns prog into a valid shell function-name fragment, so
// a renamed binary (this project's brand-indirection rule means the
// binary name is never a fixed literal, see os.Args[0]) still gets
// completion functions that don't collide with another program's.
func sanitizeIdent(prog string) string {
	var b strings.Builder
	for _, r := range prog {
		switch {
		case r == '_', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" || (out[0] >= '0' && out[0] <= '9') {
		out = "_" + out
	}
	return out
}

// runCompletion implements "completion <bash|zsh|fish>": prints a shell
// completion script for prog to stdout.
func runCompletion(prog string, args []string, stdout, stderr io.Writer, _ func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, completionUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, completionUsage(prog))
		return exitOK
	case "bash":
		_, _ = fmt.Fprint(stdout, bashCompletionScript(prog))
		return exitOK
	case "zsh":
		_, _ = fmt.Fprint(stdout, zshCompletionScript(prog))
		return exitOK
	case "fish":
		_, _ = fmt.Fprint(stdout, fishCompletionScript(prog))
		return exitOK
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown completion shell %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, completionUsage(prog))
		return exitUsage
	}
}

func completionUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s completion bash    print a bash completion script
  %[1]s completion zsh     print a zsh completion script
  %[1]s completion fish    print a fish completion script

Completes command and subcommand names for the CLI's full command tree
(apps, databases, channels, and so on), plus the global flags (%[2]s).
It does not complete flag values or positional arguments like app names.

Install:
  bash   source <(%[1]s completion bash)
         or: %[1]s completion bash | sudo tee /etc/bash_completion.d/%[1]s > /dev/null

  zsh    source <(%[1]s completion zsh)
         or: %[1]s completion zsh > "${fpath[1]}/_%[1]s"

  fish   %[1]s completion fish | source
         or: %[1]s completion fish > ~/.config/fish/completions/%[1]s.fish
`, prog, strings.Join(globalFlags, " "))
}
