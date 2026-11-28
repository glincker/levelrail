package main

import (
	"fmt"
	"io"

	"github.com/glincker/levelrail/internal/apiclient"
)

// runPrompt prints "lr:<context>[/<team>]" for PS1, starship and tmux; local files only, no network.
func runPrompt(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) > 0 {
		if args[0] == "-h" || args[0] == "--help" {
			_, _ = fmt.Fprintf(stdout, "Usage:\n  %s prompt\n\nPrints the active context and team as a short string for a shell prompt, without any network call.\n", prog)
			return exitOK
		}
		_, _ = fmt.Fprintf(stderr, "%s: prompt takes no arguments\n", prog)
		return exitUsage
	}
	profile := resolveProfile("", lookupEnv)
	out := "lr:" + profile
	if sc, _ := apiclient.ReadContext(prog, profile); sc.Team != "" {
		out += "/" + sc.Team
	}
	_, _ = fmt.Fprintln(stdout, out)
	return exitOK
}
