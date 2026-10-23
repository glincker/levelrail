package cliconsole

import (
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/clitree"
	"github.com/GLINCKER/levelrail/internal/hostdiag"
)

// deniedCommands are command paths that only make sense on an operator's own
// machine: they read or write local files, local config, or need a TTY.
var deniedCommands = []string{
	"auth", "login", "up", "profile", "completion", "init", "tui", "proxy", "migrate",
	"import", "apply", "diff", "export", "update",
	"tokens create",
	"pipelines validate", "pipelines save",
	"apps exec",
	"env pull", "env push",
	"logs fetch",
	"backups download", "app-volume-backups download",
	"control-plane-backups download", "control-plane-backups keys", "control-plane-backups escrow",
	"secrets generate-master-key", "secrets rotate-master-key",
}

// allowedUnderDenied re-opens a read-only leaf under a denied group.
var allowedUnderDenied = map[string]bool{"auth whoami": true}

// deniedFlags read or write local paths, stdin or local env, or redirect the
// CLI's own credentials and target; the console supplies those itself.
var deniedFlags = map[string]bool{
	"file": true, "f": true, "out": true, "o": true, "out-dir": true, "output-file": true,
	"env-file": true, "var-file": true, "key-file": true, "new-key-file": true, "cert-file": true,
	"body-file": true, "ssh-key": true, "extract": true, "dir": true, "dynamic-dir": true,
	"upload": true, "mcp-binary": true, "password-stdin": true, "token-stdin": true,
	"hf-token-from-env": true, "tok-env": true, "interactive": true, "i": true,
	"token": true, "api-url": true, "profile": true, "profile-name": true,
	"target-api-url": true, "target-token": true, "target-profile": true, "debug": true,
}

// DeniedFlags returns the refused flag names, sorted, for docs and tests.
func DeniedFlags() []string {
	out := make([]string, 0, len(deniedFlags))
	for f := range deniedFlags {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

func commandDenied(path string) bool {
	if allowedUnderDenied[path] {
		return false
	}
	for _, d := range deniedCommands {
		if path == d || strings.HasPrefix(path, d+" ") {
			return true
		}
	}
	return false
}

// Check vets argv against the CLI command tree and the console's denylists.
// It returns the matched command path, for audit.
func Check(argv []string) (string, error) {
	if len(argv) == 0 {
		return "", reject("empty command")
	}
	if argv[0] == "help" || argv[0] == "-h" || argv[0] == "--help" {
		return "help", checkArgs(argv[1:])
	}
	if err := checkArgs(argv); err != nil {
		return "", err
	}
	// Walk the argv the CLI itself dispatches on, after it strips scope flags.
	walk := stripScopeFlags(argv)
	if len(walk) == 0 {
		return "", reject("empty command")
	}
	// Host lines are answered in process and must never reach the CLI binary.
	if IsHost(argv) || walk[0] == hostdiag.Command {
		return "", reject("%q is only available as the first word of a line", hostdiag.Command)
	}
	node, ok := clitree.Tree[walk[0]]
	if !ok {
		return "", reject("unknown command %q; type help for the list", walk[0])
	}
	words := []string{canonical(walk[0])}
	i := 1
	for node != nil && len(node.Subs) > 0 && i < len(walk) {
		next, ok := node.Subs[walk[i]]
		if !ok {
			break
		}
		words = append(words, walk[i])
		node = next
		i++
	}
	path := strings.Join(words, " ")
	if commandDenied(path) {
		return path, reject("%q is not available in the web console; run it from a local CLI", path)
	}
	if node != nil && len(node.Subs) > 0 {
		for _, a := range walk[i:] {
			if _, sub := node.Subs[a]; sub {
				return path, reject("put the subcommand %q right after %q, before any flags or arguments", a, path)
			}
		}
	}
	return path, nil
}

// stripScopeFlags mirrors the CLI's extractScopeFlags: --team anywhere,
// --project only before the first positional word.
func stripScopeFlags(argv []string) []string {
	out := make([]string, 0, len(argv))
	seenCommand := false
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		name, _, inline := strings.Cut(a, "=")
		if name == "--team" || (name == "--project" && !seenCommand) {
			if !inline {
				i++
			}
			continue
		}
		if !strings.HasPrefix(a, "-") {
			seenCommand = true
		}
		out = append(out, a)
	}
	return out
}

func canonical(top string) string {
	if top == "db" {
		return "databases"
	}
	return top
}

func checkArgs(args []string) error {
	for _, a := range args {
		if strings.Contains(strings.ToLower(a), "file:") {
			return reject("local file references are not available in the web console")
		}
		if a == "--" {
			return reject("\"--\" pass-through arguments are not available in the web console")
		}
		name, isFlag := flagName(a)
		if isFlag && deniedFlags[name] {
			return reject("flag --%s reads or writes local files or credentials and is not available in the web console", name)
		}
	}
	return nil
}

// flagName mirrors Go's flag package: one or two dashes, optional =value.
func flagName(a string) (string, bool) {
	if len(a) < 2 || a[0] != '-' {
		return "", false
	}
	name := strings.TrimPrefix(strings.TrimPrefix(a, "-"), "-")
	if k, _, ok := strings.Cut(name, "="); ok {
		name = k
	}
	if name == "" {
		return "", false
	}
	return name, true
}

// Command is one top-level CLI command and its direct subcommands.
type Command struct {
	Name        string   `json:"name"`
	Subcommands []string `json:"subcommands"`
}

// Commands lists what the console accepts, for client-side completion.
func Commands() []Command {
	out := []Command{}
	for name, node := range clitree.Tree {
		if commandDenied(canonical(name)) && !hasAllowedChild(canonical(name)) {
			continue
		}
		c := Command{Name: name, Subcommands: []string{}}
		if node != nil {
			for sub := range node.Subs {
				if !commandDenied(canonical(name) + " " + sub) {
					c.Subcommands = append(c.Subcommands, sub)
				}
			}
		}
		sort.Strings(c.Subcommands)
		out = append(out, c)
	}
	if hostdiag.Enabled() {
		subs := hostdiag.Subcommands()
		sort.Strings(subs)
		out = append(out, Command{Name: hostdiag.Command, Subcommands: subs})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// IsHost reports whether a parsed line belongs to the in-process host diagnostics.
func IsHost(argv []string) bool { return len(argv) > 0 && argv[0] == hostdiag.Command }

func hasAllowedChild(top string) bool {
	for p := range allowedUnderDenied {
		if strings.HasPrefix(p, top+" ") {
			return true
		}
	}
	return false
}
