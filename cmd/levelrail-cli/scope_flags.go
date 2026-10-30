package main

import (
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// scopeFlags holds this run's --team and --project, applied by every client.
var scopeFlags apiclient.Scope

// extractScopeFlags strips --team anywhere and --project (or -n) before the command
// name. --project after the command keeps its per-command meaning, and so does a
// later -n (several commands use it for a count).
func extractScopeFlags(args []string) (remaining []string, scope apiclient.Scope) {
	remaining = make([]string, 0, len(args))
	seenCommand := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			remaining = append(remaining, args[i:]...)
			break
		}
		if a == "-A" || a == "--all-projects" {
			scope.Project = allProjectsMarker
			continue
		}
		name, value, inline := strings.Cut(a, "=")
		isTeam := name == "--team"
		isProject := (name == "--project" || name == "-n") && !seenCommand
		if isTeam || isProject {
			if !inline && i+1 < len(args) {
				i++
				value = args[i]
			}
			if isTeam {
				scope.Team = value
			} else {
				scope.Project = value
			}
			continue
		}
		if !strings.HasPrefix(a, "-") {
			seenCommand = true
		}
		remaining = append(remaining, a)
	}
	return remaining, scope
}

// applyScope resolves and activates the request scope for a client.
func applyScope(prog, profile string, lookupEnv func(string) (string, bool)) {
	sc := apiclient.ResolveScope(scopeFlags, lookupEnv, prog, profile)
	if scopeFlags.Project == allProjectsMarker {
		sc.Project = ""
	}
	apiclient.SetScope(sc)
}

// Never a valid project name, so it cannot collide with a real --project value.
const allProjectsMarker = "*"

var outputShorthandValues = map[string]bool{"json": true, "yaml": true, "table": true, "wide": true, "text": true, "name": true}

func isOutputShorthandValue(v string) bool {
	return outputShorthandValues[v] || strings.HasPrefix(v, outputColumnsPrefix) || strings.HasPrefix(v, outputPathPrefix)
}

// Rewrites -o <format> to --output <format> only when the value is an output format,
// so commands that use -o for a file path keep working.
func expandOutputShorthand(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			return append(out, args[i:]...)
		}
		if a == "-o" && i+1 < len(args) && isOutputShorthandValue(args[i+1]) {
			out = append(out, "--output", args[i+1])
			i++
			continue
		}
		if v, ok := strings.CutPrefix(a, "-o="); ok && isOutputShorthandValue(v) {
			out = append(out, "--output="+v)
			continue
		}
		out = append(out, a)
	}
	return out
}
