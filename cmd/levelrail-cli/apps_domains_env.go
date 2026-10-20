package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// matchEnvironmentSet finds want (id, name or kind, case-insensitive) among
// an app's per-environment domain sets.
func matchEnvironmentSet(sets []apiclient.EnvironmentDomainSet, want string) (apiclient.EnvironmentDomainSet, bool) {
	want = strings.TrimSpace(want)
	for _, pass := range []func(apiclient.EnvironmentDomainSet) bool{
		func(s apiclient.EnvironmentDomainSet) bool { return s.EnvironmentID == want },
		func(s apiclient.EnvironmentDomainSet) bool { return strings.EqualFold(s.Name, want) },
		func(s apiclient.EnvironmentDomainSet) bool { return strings.EqualFold(s.Kind, want) },
	} {
		for _, s := range sets {
			if pass(s) {
				return s, true
			}
		}
	}
	return apiclient.EnvironmentDomainSet{}, false
}

func runAppsDomainsListEnvironment(client *Client, name, env string, jsonOut bool, of outputFlags, stdout, stderr io.Writer) int {
	view, err := client.GetAppEnvironmentDomains(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get environment domains for app %q: %w", name, err))
	}
	if strings.EqualFold(env, "all") {
		return writeScheduledTaskResult(stdout, stderr, of, view, func() { printEnvironmentDomainsHuman(stdout, view) })
	}
	set, ok := matchEnvironmentSet(view.Environments, env)
	if !ok {
		return reportError(stdout, stderr, jsonOut, newValidationError("no environment %q for app %q", env, name))
	}
	return writeScheduledTaskResult(stdout, stderr, of, set.Domains, func() {
		if len(set.Domains) == 0 {
			_, _ = fmt.Fprintf(stdout, "app %q has no domains in environment %q (it routes its default set there)\n", name, set.Name)
			return
		}
		for _, d := range set.Domains {
			_, _ = fmt.Fprintln(stdout, d)
		}
	})
}

func printEnvironmentDomainsHuman(w io.Writer, v apiclient.AppEnvironmentDomains) {
	_, _ = fmt.Fprintf(w, "app %s\n", v.App)
	_, _ = fmt.Fprintf(w, "default set: %s\n", joinOrDash(v.DefaultDomains))
	_, _ = fmt.Fprintf(w, "routing now: %s\n", joinOrDash(v.RoutedDomains))
	for _, e := range v.Environments {
		marker := " "
		if e.Active {
			marker = "*"
		}
		_, _ = fmt.Fprintf(w, "%s %-14s %s\n", marker, e.Name, joinOrDash(e.Domains))
	}
}
