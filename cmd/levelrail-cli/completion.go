package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/clitree"
	"github.com/GLINCKER/levelrail/internal/experimental"
)

// cmdNode is one node of the CLI's command tree: the verbs a command
// accepts, and any further verbs those verbs themselves dispatch to.
type cmdNode = clitree.Node

// cliCommandTree mirrors every "switch args[0]" dispatch layer in this
// package (main.go's run, apps.go, channels.go, ...), by hand: this CLI
// has no cobra-style registry the dispatch switches and completions can
// both read from, so the two are two independent sources of truth. Adding
// or renaming a command means updating both. TestCommandTree_MatchesDispatchSwitches
// in completion_test.go parses every dispatch switch in this package and
// fails if this tree and the real switches disagree, so drift is caught
// at test time rather than silently shipped.
var cliCommandTree = clitree.Tree

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
			if child == nil || len(child.Subs) == 0 {
				continue
			}
			next := name
			if prefix != "" {
				next = prefix + " " + name
			}
			walk(next, child.Subs)
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
