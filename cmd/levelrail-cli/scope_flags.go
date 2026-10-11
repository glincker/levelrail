package main

import (
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// scopeFlags holds this run's --team and --project, applied by every client.
var scopeFlags apiclient.Scope

// extractScopeFlags strips --team anywhere and --project before the command
// name. --project after the command keeps its per-command meaning.
func extractScopeFlags(args []string) (remaining []string, scope apiclient.Scope) {
	remaining = make([]string, 0, len(args))
	seenCommand := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			remaining = append(remaining, args[i:]...)
			break
		}
		name, value, inline := strings.Cut(a, "=")
		isTeam := name == "--team"
		isProject := name == "--project" && !seenCommand
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
	apiclient.SetScope(apiclient.ResolveScope(scopeFlags, lookupEnv, prog, profile))
}
