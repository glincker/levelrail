package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/spendcap"
)

const domainsLimitsJSONUsage = "print the domain limits as JSON to stdout and nothing else"

// runDomainsLimits dispatches "domains limits <verb> [flags]".
func runDomainsLimits(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, domainsLimitsUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, domainsLimitsUsage(prog))
		return exitOK
	case "get":
		return runDomainsLimitsVerb(prog, "get", args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runDomainsLimitsVerb(prog, "set", args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runDomainsLimitsVerb(prog, "clear", args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains limits subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, domainsLimitsUsage(prog))
		return exitUsage
	}
}

func domainsLimitsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains limits get <app> <domain> [flags]     show a domain's request limits
  %[1]s domains limits set <app> <domain> [--max-body SIZE] [--read-timeout SECONDS] [--write-timeout SECONDS]
  %[1]s domains limits clear <app> <domain> [flags]   go back to the app and instance limits

A request body cap (such as 100MiB) and upstream read and write timeouts for
one domain. 0 or unset keeps the app and instance behavior. Every set call
replaces all three values. Takes effect on the next ingress reconcile pass.
`, prog)
}

func runDomainsLimitsVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "domains limits " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, domainsLimitsJSONUsage, stderr)
	var maxBody string
	var readT, writeT int
	if verb == "set" {
		fs.StringVar(&maxBody, "max-body", "0", "request body cap such as 100MiB, 0 for no domain cap")
		fs.IntVar(&readT, "read-timeout", 0, "seconds to wait reading from the app, 0 for the default")
		fs.IntVar(&writeT, "write-timeout", 0, "seconds to wait writing to the app, 0 for the default")
	}
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s %s <app> <domain> [flags]\n\nFlags:\n", prog, label)
		fs.PrintDefaults()
	}
	client, appName, domain, jsonOut, of, exitCode, ok := domainWAFResolveArgs(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, label, stderr, lookupEnv)
	if !ok {
		return exitCode
	}
	ctx := context.Background()
	var res domainLimitsResource
	var err error
	switch verb {
	case "get":
		res, err = client.GetDomainLimits(ctx, appName, domain)
	case "clear":
		res, err = client.ClearDomainLimits(ctx, appName, domain)
	default:
		var body int64
		if maxBody != "0" {
			if body, err = spendcap.ParseBytes(maxBody); err != nil {
				_, _ = fmt.Fprintf(stderr, "%s: --max-body: %v\n", prog, err)
				return exitUsage
			}
		}
		res, err = client.SetDomainLimits(ctx, appName, domain, setDomainLimitsRequest{MaxBodyBytes: body, ReadTimeoutSeconds: readT, WriteTimeoutSeconds: writeT})
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s for domain %q: %w", label, domain, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		if !res.Configured {
			_, _ = fmt.Fprintf(stdout, "%s has no own limits\n", res.Domain)
			return
		}
		_, _ = fmt.Fprintf(stdout, "max_body_bytes:        %d\nread_timeout_seconds:  %d\nwrite_timeout_seconds: %d\n", res.MaxBodyBytes, res.ReadTimeoutSeconds, res.WriteTimeoutSeconds)
	})
}
