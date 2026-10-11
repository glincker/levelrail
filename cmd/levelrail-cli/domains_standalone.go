package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const standaloneJSONUsage = "print the standalone domain as JSON to stdout and nothing else"

// runDomainsStandalone dispatches "domains standalone <verb>": domains not
// owned by any app that proxy to a port, address, URL, app or container.
func runDomainsStandalone(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, domainsStandaloneUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, domainsStandaloneUsage(prog))
		return exitOK
	case "list":
		return runDomainsStandaloneList(prog, args[1:], stdout, stderr, lookupEnv)
	case "add":
		return runDomainsStandaloneWrite(prog, "add", args[1:], stdout, stderr, lookupEnv)
	case "update":
		return runDomainsStandaloneWrite(prog, "update", args[1:], stdout, stderr, lookupEnv)
	case "remove":
		return runDomainsStandaloneRemove(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains standalone subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, domainsStandaloneUsage(prog))
		return exitUsage
	}
}

func domainsStandaloneUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains standalone list [flags]
  %[1]s domains standalone add <domain> --target TARGET [flags]
  %[1]s domains standalone update <domain> [flags]
  %[1]s domains standalone remove <domain> [flags]

A standalone domain is not linked to any app: the ingress proxies it to
TARGET, which is one of
  3000 or 127.0.0.1:3000          a port on this server
  10.0.0.5:8080, [fd12::5]:80     an address on a private network or the mesh
  https://backend.example.org     an external URL (TLS and Host follow the URL)
  app:NAME[:PORT]                 another app's port (main port by default)
  container:NAME:PORT             a container on this server

Loopback, private and container targets, and this server's own dashboard
or agent port, need --confirm. Metadata and link-local addresses are always
refused. Every per-domain command (basic-auth, waf, headers, go-live, doctor)
works on a standalone domain with %[2]s as the app name.
`, prog, apiclient.StandaloneScope)
}

type standaloneFlags struct {
	target, serverName, hostHeader, stripPrefix, description string
	upstreamTLS, insecure, noWebSocket, confirm              bool
	connectTimeout, readTimeout                              int
}

func bindStandaloneFlags(fs *flag.FlagSet, f *standaloneFlags) {
	fs.StringVar(&f.target, "target", "", "where to proxy: PORT, HOST:PORT, http(s)://URL, app:NAME[:PORT] or container:NAME:PORT")
	fs.BoolVar(&f.upstreamTLS, "upstream-tls", false, "speak TLS to the target (implied by an https:// URL)")
	fs.BoolVar(&f.insecure, "insecure-skip-verify", false, "do not verify the target's TLS certificate (self-signed upstreams)")
	fs.StringVar(&f.serverName, "tls-server-name", "", "SNI and verification name for upstream TLS")
	fs.StringVar(&f.hostHeader, "host-header", "", "Host sent upstream: preserve, upstream, or a hostname (default preserve, upstream for URLs)")
	fs.BoolVar(&f.noWebSocket, "no-websocket", false, "refuse websocket upgrades (on by default)")
	fs.IntVar(&f.connectTimeout, "connect-timeout", 0, "seconds to wait for a connection (0 keeps the default)")
	fs.IntVar(&f.readTimeout, "read-timeout", 0, "seconds to wait for response headers (0 keeps the default)")
	fs.StringVar(&f.stripPrefix, "strip-prefix", "", "path prefix removed before proxying, e.g. /app")
	fs.StringVar(&f.description, "description", "", "a note shown on the domains page")
	fs.BoolVar(&f.confirm, "confirm", false, "acknowledge a loopback, private, container or control-port target")
}

// request sends only flags set on the command line, so update keeps the rest.
func (f *standaloneFlags) request(fs *flag.FlagSet) apiclient.StandaloneDomainRequest {
	req := apiclient.StandaloneDomainRequest{Confirm: f.confirm}
	fs.Visit(func(fl *flag.Flag) {
		switch fl.Name {
		case "target":
			req.Target = &f.target
		case "upstream-tls":
			req.UpstreamTLS = &f.upstreamTLS
		case "insecure-skip-verify":
			req.TLSSkipVerify = &f.insecure
		case "tls-server-name":
			req.TLSServerName = &f.serverName
		case "host-header":
			req.HostHeader = &f.hostHeader
		case "no-websocket":
			on := !f.noWebSocket
			req.WebSocket = &on
		case "connect-timeout":
			req.DialTimeoutSeconds = &f.connectTimeout
		case "read-timeout":
			req.ReadTimeoutSeconds = &f.readTimeout
		case "strip-prefix":
			req.StripPrefix = &f.stripPrefix
		case "description":
			req.Description = &f.description
		}
	})
	return req
}

func runDomainsStandaloneWrite(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "domains standalone " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, standaloneJSONUsage, stderr)
	var f standaloneFlags
	bindStandaloneFlags(fs, &f)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, domainsStandaloneUsage(prog)+"\nFlags:\n")
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	domain, ok := requireOneArg(fs, stderr, prog, label, "domain")
	if !ok {
		return exitUsage
	}
	req := f.request(fs)
	if verb == "add" && req.Target == nil {
		_, _ = fmt.Fprintf(stderr, "%s: --target is required\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var d apiclient.StandaloneDomain
	var err error
	if verb == "add" {
		req.Domain = domain
		d, err = client.CreateStandaloneDomain(ctx, req)
	} else {
		d, err = client.UpdateStandaloneDomain(ctx, domain, req)
	}
	if err != nil {
		hint := ""
		if strings.Contains(err.Error(), "needs confirmation") {
			hint = "; check the target, then rerun with --confirm"
		}
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s standalone domain %q: %w%s", verb, domain, err, hint))
	}
	return writeScheduledTaskResult(stdout, stderr, of, d, func() {
		_, _ = fmt.Fprintf(stdout, "%s -> %s\n", d.Domain, d.TargetURL)
		if verb != "add" {
			return
		}
		if addr, err := client.GetServerAddress(ctx, d.Domain); err == nil {
			printDNSRecords(stdout, addr)
		}
	})
}

func printDNSRecords(out io.Writer, addr apiclient.ServerAddress) {
	if len(addr.Records) == 0 {
		_, _ = fmt.Fprintln(out, "no public address detected; set one with \"system address --ipv4\" to get the DNS record to create")
		return
	}
	_, _ = fmt.Fprintln(out, "create this DNS record at your DNS provider:")
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	for _, r := range addr.Records {
		note := ""
		if r.Alternative {
			note = "(or, instead of the records above)"
		}
		_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", r.Type, r.Name, r.Value, note)
	}
	_ = tw.Flush()
	if addr.BehindNAT {
		_, _ = fmt.Fprintln(out, "this server is behind NAT: forward ports 80 and 443 to it")
	}
}

func runDomainsStandaloneList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains standalone list", "print standalone domains as a JSON array", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	list, err := client.ListStandaloneDomains(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list standalone domains: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, list, func() {
		if len(list) == 0 {
			_, _ = fmt.Fprintln(stdout, "no standalone domains")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "DOMAIN\tTARGET\tHOST\tWEBSOCKET")
		for _, d := range list {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%t\n", d.Domain, d.TargetURL, d.HostHeader, d.WebSocket)
		}
		_ = tw.Flush()
	})
}

func runDomainsStandaloneRemove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains standalone remove", "print nothing; errors as JSON", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	domain, ok := requireOneArg(fs, stderr, prog, "domains standalone remove", "domain")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.DeleteStandaloneDomain(context.Background(), domain); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("remove standalone domain %q: %w", domain, err))
	}
	if !jsonOut {
		_, _ = fmt.Fprintf(stdout, "removed standalone domain %s; its route is gone on the next ingress pass\n", domain)
	}
	return exitOK
}
