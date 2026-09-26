package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const envImportSourceToken = "APP_IMPORT_SOURCE_TOKEN" //nolint:gosec // env var name, not a credential

type importPlatformFlags struct {
	url, token, only, collision, apiURL, profile string
	tokenStdin, dryRun, jsonOut                  bool
	insecure, allowPrivate, allowLoopback        bool
}

func runImportPlatform(prog string, args []string, stdin io.Reader, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprint(stdout, importPlatformUsage(prog))
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	platform := args[0]
	if platform != "coolify" && platform != "dokploy" && platform != "caprover" {
		_, _ = fmt.Fprintf(stderr, "%s: unknown platform %q, want coolify, dokploy or caprover\n", prog, platform)
		return exitUsage
	}
	var f importPlatformFlags
	fs := flag.NewFlagSet(prog+" import platform", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, importPlatformUsage(prog)) }
	fs.StringVar(&f.url, "url", "", "source platform base URL")
	fs.StringVar(&f.token, "token", "", "source platform token (prefer "+envImportSourceToken+" or --token-stdin)")
	fs.BoolVar(&f.tokenStdin, "token-stdin", false, "read the source token from the first line of stdin")
	fs.BoolVar(&f.dryRun, "dry-run", false, "report what would be created without creating anything")
	fs.StringVar(&f.only, "only", "", "comma-separated source ids or names to import")
	fs.StringVar(&f.collision, "collision", "suffix", "name collision handling: suffix or skip")
	fs.BoolVar(&f.insecure, "insecure-tls", false, "skip TLS verification of the source")
	fs.BoolVar(&f.allowPrivate, "allow-private", false, "allow a private-network source (also needs the control plane env opt-in)")
	fs.BoolVar(&f.allowLoopback, "allow-loopback", false, "allow a loopback source (also needs the control plane env opt-in)")
	fs.StringVar(&f.apiURL, "api-url", "", "control plane API base URL")
	fs.StringVar(&f.profile, "profile", "", "named credentials profile for the control plane")
	fs.BoolVar(&f.jsonOut, "json", false, "print the report as JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return exitUsage
	}
	if f.url == "" {
		_, _ = fmt.Fprintf(stderr, "%s: --url is required\n", prog)
		return exitUsage
	}
	token, code, ok := resolveSourceToken(prog, f, stdin, stderr, lookupEnv)
	if !ok {
		return code
	}
	req := apiclient.PlatformImportRequest{Platform: platform, URL: f.url, Token: token, InsecureTLS: f.insecure,
		AllowPrivate: f.allowPrivate, AllowLoopback: f.allowLoopback, Collision: f.collision}
	for _, o := range strings.Split(f.only, ",") {
		if o = strings.TrimSpace(o); o != "" {
			req.Only = append(req.Only, o)
		}
	}
	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	ctx := context.Background()
	var rep apiclient.PlatformImportReport
	var err error
	if f.dryRun {
		rep, err = client.DiscoverPlatformImport(ctx, req)
	} else {
		rep, err = client.ApplyPlatformImport(ctx, req)
	}
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("import from %s: %w", platform, err))
	}
	if f.jsonOut {
		if err := writeJSONValue(stdout, rep); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
	} else {
		printPlatformImportReport(stdout, rep, f.dryRun)
	}
	if rep.Counts["failed"] > 0 {
		return exitCheckFailed
	}
	return exitOK
}

func resolveSourceToken(prog string, f importPlatformFlags, stdin io.Reader, stderr io.Writer, lookupEnv func(string) (string, bool)) (string, int, bool) {
	switch {
	case f.tokenStdin:
		line, err := bufio.NewReader(stdin).ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "%s: read token from stdin: %v\n", prog, err)
			} else {
				_, _ = fmt.Fprintf(stderr, "%s: the token on stdin is empty\n", prog)
			}
			return "", exitUsage, false
		}
		return line, exitOK, true
	case f.token != "":
		_, _ = fmt.Fprintf(stderr, "%s: warning: --token is visible in shell history and the process list, prefer %s or --token-stdin\n", prog, envImportSourceToken)
		return f.token, exitOK, true
	}
	if v, ok := lookupEnv(envImportSourceToken); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v), exitOK, true
	}
	_, _ = fmt.Fprintf(stderr, "%s: a source token is required: set %s, use --token-stdin, or pass --token\n", prog, envImportSourceToken)
	return "", exitUsage, false
}

func printPlatformImportReport(w io.Writer, rep apiclient.PlatformImportReport, dryRun bool) {
	mode := "Import result"
	if dryRun {
		mode = "Dry run (nothing was created)"
	}
	_, _ = fmt.Fprintf(w, "%s from %s\n\n", mode, rep.Platform)
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KIND\tSOURCE\tTARGET\tSTATUS")
	for _, it := range rep.Items {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", it.Kind, it.SourceName, it.Target, it.Status)
	}
	_ = tw.Flush()
	for _, it := range rep.Items {
		if len(it.Reasons) == 0 && len(it.Manual) == 0 {
			continue
		}
		_, _ = fmt.Fprintf(w, "\n%s %q (%s)\n", it.Kind, it.SourceName, it.Status)
		for _, r := range it.Reasons {
			_, _ = fmt.Fprintf(w, "  - %s\n", r)
		}
		for _, m := range it.Manual {
			_, _ = fmt.Fprintf(w, "  next: %s\n", m)
		}
	}
	for _, n := range rep.Notes {
		_, _ = fmt.Fprintf(w, "\nNote: %s\n", n)
	}
}

func importPlatformUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s import platform coolify|dokploy|caprover --url URL [flags]

Reads apps, databases and settings from another platform (read-only) and
creates them here. Use --dry-run first. Databases and volumes are created
empty, data is not migrated. Re-running skips what was already imported.

The source token (Coolify API token, Dokploy API key, CapRover password) is
read from %[2]s, or --token-stdin, or --token (warns: it lands in shell
history). It is sent to this control plane in the request body only and is
never stored.

Flags:
  --url string          source platform base URL (required)
  --dry-run             report only, create nothing
  --only string         comma-separated source ids or names to import
  --collision string    suffix (default) or skip when a name is taken
  --insecure-tls        skip TLS verification of the source
  --allow-private       allow a private-network source (control plane also needs APP_IMPORT_ALLOW_PRIVATE_NETWORKS=true)
  --allow-loopback      allow a loopback source (control plane also needs APP_IMPORT_ALLOW_LOOPBACK=true)
  --token-stdin         read the source token from stdin
  --api-url string      control plane API base URL
  --profile string      named credentials profile for the control plane
  --json                print the report as JSON

Delete imported resources by label: apps carry import/source-id.
`, prog, envImportSourceToken)
}
