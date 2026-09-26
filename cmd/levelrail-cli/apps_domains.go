package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
)

func appsDomainsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps domains list <name> [flags]                 list an app's domains
  %[1]s apps domains add <name> <domain>... [flags]      add one or more domains
  %[1]s apps domains remove <name> <domain>... [flags]   remove one or more domains

add and remove read the app, change only its domain list and save it back.
A domain already used by another app is refused with the conflicting app named.

Run "%[1]s apps domains <subcommand> -h" for a subcommand's own flags.
`, prog)
}

// runAppsDomains dispatches "apps domains list|add|remove".
func runAppsDomains(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsDomainsUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsDomainsUsage(prog))
		return exitOK
	case "list":
		return runAppsDomainsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "add":
		return runAppsDomainsChange(prog, "add", args[1:], stdout, stderr, lookupEnv)
	case "remove":
		return runAppsDomainsChange(prog, "remove", args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps domains subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsDomainsUsage(prog))
		return exitUsage
	}
}

func runAppsDomainsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps domains list", "print the domains as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps domains list <name> [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps domains list", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	app, err := client.GetApp(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get app %q: %w", name, err))
	}
	domains := app.Domains
	if domains == nil {
		domains = []string{}
	}
	return writeScheduledTaskResult(stdout, stderr, of, domains, func() {
		if len(domains) == 0 {
			_, _ = fmt.Fprintf(stdout, "app %q has no domains\n", name)
			return
		}
		for _, d := range domains {
			_, _ = fmt.Fprintln(stdout, d)
		}
	})
}

type appsDomainsResult struct {
	App     string   `json:"app"`
	Domains []string `json:"domains"`
	Changed bool     `json:"changed"`
}

func runAppsDomainsChange(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "apps domains " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s %s <name> <domain>... [flags]\n\nFlags:\n", prog, label)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) < 2 {
		_, _ = fmt.Fprintf(stderr, "%s: %s requires an app name and at least one domain\n\n", prog, label)
		fs.Usage()
		return exitUsage
	}
	name, wanted := rest[0], normalizeDomains(rest[1:])

	ctx := context.Background()
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	app, err := client.GetApp(ctx, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get app %q: %w", name, err))
	}

	next, err := applyDomainChange(app.Domains, verb, wanted)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("app %q: %v", name, err))
	}
	result := appsDomainsResult{App: name, Domains: next, Changed: !slices.Equal(app.Domains, next)}
	if result.Changed {
		app.Domains = next
		if _, err := client.UpdateApp(ctx, name, app); err != nil {
			var apiErr *apiError
			if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict {
				return reportError(stdout, stderr, jsonOut, fmt.Errorf("cannot %s domains on app %q, nothing was changed: %w", verb, name, err))
			}
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("update domains for app %q: %w", name, err))
		}
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		if !result.Changed {
			_, _ = fmt.Fprintf(stdout, "no change: app %q domains are already as requested\n", name)
			return
		}
		_, _ = fmt.Fprintf(stdout, "app %q domains: %s\n", name, strings.Join(next, ", "))
	})
}

func normalizeDomains(in []string) []string {
	out := make([]string, 0, len(in))
	for _, d := range in {
		d = strings.ToLower(strings.TrimSpace(d))
		if d != "" && !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	return out
}

// applyDomainChange returns current with wanted added or removed. Removing a
// domain the app does not have is an error rather than a silent no-op.
func applyDomainChange(current []string, verb string, wanted []string) ([]string, error) {
	next := slices.Clone(current)
	for _, d := range wanted {
		has := slices.Contains(next, d)
		switch {
		case verb == "add" && !has:
			next = append(next, d)
		case verb == "remove" && has:
			next = slices.DeleteFunc(next, func(x string) bool { return x == d })
		case verb == "remove":
			return nil, fmt.Errorf("domain %q is not set", d)
		}
	}
	return next, nil
}
