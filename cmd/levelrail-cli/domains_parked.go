package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func domainsParkUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains park <app> <domain> [--note TEXT]
  %[1]s domains move <domain> --from APP --to APP [--confirm]
  %[1]s domains parked list
  %[1]s domains parked attach <domain> --app APP
  %[1]s domains parked delete <domain>

park takes a domain off an app but keeps it registered, with its DNS records
and certificate untouched, so it can be attached to any app later. move hands
a domain to another app with its per-domain settings; without --confirm it
only runs the collision check. parked delete forgets the registration and
never touches DNS.
`, prog)
}

// runDomainsPark handles "domains park <app> <domain>".
func runDomainsPark(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "domains park", "print the result as JSON", stderr)
	note := fs.String("note", "", "why the domain is parked")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsParkUsage(prog)+"\nFlags:\n"); fs.PrintDefaults() }
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	rest, ok := requireArgs(fs, stderr, prog, "domains park", "an app and a domain", 2)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv)
	res, err := client.ParkDomain(context.Background(), apiclient.DomainLifecycleRequest{App: rest[0], Domain: rest[1], Note: *note})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("park domain %q: %w", rest[1], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "parked %s (was on %s); attach it to an app with: %s domains parked attach %s --app APP\n", res.Domain, res.From, prog, res.Domain)
	})
}

// runDomainsMove handles "domains move <domain> --from A --to B".
func runDomainsMove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "domains move", "print the result as JSON", stderr)
	from := fs.String("from", "", "app that serves the domain now")
	to := fs.String("to", "", "app that should serve it")
	confirm := fs.Bool("confirm", false, "apply the move after the collision check")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsParkUsage(prog)+"\nFlags:\n"); fs.PrintDefaults() }
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	domain, ok := requireOneArg(fs, stderr, prog, "domains move", "domain")
	if !ok {
		return exitUsage
	}
	if *from == "" || *to == "" {
		_, _ = fmt.Fprintf(stderr, "%s: --from and --to are required\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv)
	res, err := client.MoveDomain(context.Background(), apiclient.DomainLifecycleRequest{Domain: domain, From: *from, To: *to, Confirm: *confirm})
	if err != nil {
		hint := ""
		if strings.Contains(err.Error(), "confirm") {
			hint = "; rerun with --confirm to move it"
		}
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("move domain %q: %w%s", domain, err, hint))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "moved %s from %s to %s\n", res.Domain, res.From, res.To)
	})
}

// runDomainsParked dispatches "domains parked <verb>".
func runDomainsParked(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, domainsParkUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, domainsParkUsage(prog))
		return exitOK
	case "list":
		return runDomainsParkedList(prog, args[1:], stdout, stderr, lookupEnv)
	case "attach":
		return runDomainsParkedAttach(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runDomainsParkedDelete(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains parked subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, domainsParkUsage(prog))
		return exitUsage
	}
}

func runDomainsParkedList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "domains parked list", "print parked domains as a JSON array", stderr)
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	client := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv)
	list, err := client.ListParkedDomains(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list parked domains: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, list, func() {
		if len(list) == 0 {
			_, _ = fmt.Fprintln(stdout, "no parked domains")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "DOMAIN\tPARKED FROM\tPARKED AT\tNOTE")
		for _, p := range list {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", p.Domain, p.ParkedFrom, p.ParkedAt, p.Note)
		}
		_ = tw.Flush()
	})
}

func runDomainsParkedAttach(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "domains parked attach", "print the result as JSON", stderr)
	app := fs.String("app", "", "app that should serve the domain")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsParkUsage(prog)+"\nFlags:\n"); fs.PrintDefaults() }
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	domain, ok := requireOneArg(fs, stderr, prog, "domains parked attach", "domain")
	if !ok {
		return exitUsage
	}
	if *app == "" {
		_, _ = fmt.Fprintf(stderr, "%s: --app is required\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv)
	res, err := client.AttachParkedDomain(context.Background(), domain, *app)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("attach parked domain %q: %w", domain, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "attached %s to %s\n", res.Domain, res.To)
	})
}

func runDomainsParkedDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "domains parked delete", "print nothing; errors as JSON", stderr)
	token, apiURL, profile, jsonOut, _, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	domain, ok := requireOneArg(fs, stderr, prog, "domains parked delete", "domain")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv)
	if err := client.DeleteParkedDomain(context.Background(), domain); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete parked domain %q: %w", domain, err))
	}
	if !jsonOut {
		_, _ = fmt.Fprintf(stdout, "deleted parked domain %s; its DNS records and certificate were not touched\n", domain)
	}
	return exitOK
}
