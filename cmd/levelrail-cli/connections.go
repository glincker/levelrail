package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// connectionsNow is a seam so expiry text is deterministic in tests.
var connectionsNow = time.Now

// runConnections dispatches the read-only integrations hub commands.
func runConnections(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, connectionsUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, connectionsUsage(prog))
		return exitOK
	case "list":
		return runConnectionsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "show":
		return runConnectionsShow(prog, args[1:], stdout, stderr, lookupEnv)
	case "catalog":
		return runConnectionsCatalog(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown connections subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, connectionsUsage(prog))
		return exitUsage
	}
}

func connectionsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s connections list [--kind KIND] [--scope SCOPE] [--health HEALTH] [flags]   list every connected integration
  %[1]s connections show <ref> [flags]                                              show one connection
  %[1]s connections catalog [flags]                                                 list the providers that can be connected

Run "%[1]s connections <subcommand> -h" for a subcommand's own flags.
`, prog)
}

const connectionsFlagHelp = `  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`

func runConnectionsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "connections list", "print connections as a JSON array to stdout and nothing else", stderr)
	kind := fs.String("kind", "", "only connections of this kind (git, registry, secrets, dns, cloud, webhook, notify)")
	scope := fs.String("scope", "", "only connections with this scope (instance, org or app)")
	health := fs.String("health", "", "only connections with this health")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, connectionsListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	list, err := client.ListConnections(context.Background(), apiclient.ConnectionFilter{Kind: *kind, Scope: *scope, Health: *health})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list connections: %w", err))
	}
	if list == nil {
		list = []apiclient.Connection{}
	}
	if err := renderResult(stdout, of.Format, of.Query, list, func() { printConnectionsTable(stdout, list) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runConnectionsShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "connections show", "print the connection as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, connectionsShowUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	ref, ok := requireOneArg(fs, stderr, prog, "connections show", "connection ref")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	c, err := client.GetConnection(context.Background(), ref)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("show connection %q: %w", ref, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, c, func() { printConnectionDetail(stdout, c) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runConnectionsCatalog(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "connections catalog", "print the catalog as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, connectionsCatalogUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	cat, err := client.ConnectionCatalog(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("connections catalog: %w", err))
	}
	if cat == nil {
		cat = []apiclient.ConnectionCatalogEntry{}
	}
	if err := renderResult(stdout, of.Format, of.Query, cat, func() { printConnectionCatalog(stdout, cat) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printConnectionsTable(out io.Writer, list []apiclient.Connection) {
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "No connections")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "REF\tKIND\tPROVIDER\tLABEL\tSCOPE\tHEALTH\tEXPIRES\tBOUND")
	for _, c := range list {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\n", c.Ref, c.Kind, c.Provider, c.Label, connectionScope(c), c.Health, connectionExpiry(c.ExpiresAt), c.BoundCount)
	}
	_ = tw.Flush()
}

func printConnectionDetail(out io.Writer, c apiclient.Connection) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	row := func(k, v string) {
		if v != "" {
			_, _ = fmt.Fprintf(tw, "%s:\t%s\n", k, v)
		}
	}
	row("Ref", c.Ref)
	row("Kind", c.Kind)
	row("Provider", c.Provider)
	row("Label", c.Label)
	row("Account", c.Account)
	row("Host", c.Host)
	row("Scope", connectionScope(c))
	row("Auth", c.AuthMethod)
	row("Health", c.Health)
	row("Health detail", c.HealthDetail)
	row("Expires", connectionExpiry(c.ExpiresAt))
	row("Rotate after", connectionTime(c.RotateAfter))
	row("Last used", connectionTime(c.LastUsedAt))
	row("Checked", connectionTime(c.CheckedAt))
	row("Created", connectionTime(c.CreatedAt))
	row("Capabilities", strings.Join(c.Capabilities, ", "))
	row("Bound", fmt.Sprintf("%d", c.BoundCount))
	_ = tw.Flush()
}

func printConnectionCatalog(out io.Writer, cat []apiclient.ConnectionCatalogEntry) {
	if len(cat) == 0 {
		_, _ = fmt.Fprintln(out, "No providers")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tCATEGORY\tNAME\tCONNECT")
	for _, e := range cat {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.ID, e.Category, e.Name, strings.Join(e.ConnectMethods, ", "))
	}
	_ = tw.Flush()
}

func connectionScope(c apiclient.Connection) string {
	if c.ScopeID == "" {
		return c.ScopeType
	}
	return c.ScopeType + ":" + c.ScopeID
}

func connectionTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// connectionExpiry shows days, switching to hours under 48h so a same-day expiry stays visible.
func connectionExpiry(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "-"
	}
	left := t.Sub(connectionsNow())
	if left <= 0 {
		return "expired"
	}
	if left < 48*time.Hour {
		h := int(left.Hours())
		if h < 1 {
			h = 1
		}
		return fmt.Sprintf("in %d hours", h)
	}
	return fmt.Sprintf("in %d days", int(left.Hours()/24))
}

func connectionsListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s connections list [--kind KIND] [--scope SCOPE] [--health HEALTH] [flags]

Lists every connected integration: git hosts, registries, secret stores, DNS,
cloud, webhooks and notification channels, with health, expiry and how many
apps use each.

Flags:
  --kind string           only this kind (git, registry, secrets, dns, cloud, webhook, notify)
  --scope string          only this scope (instance, org or app)
  --health string         only this health
`+connectionsFlagHelp, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}

func connectionsShowUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s connections show <ref> [flags]

Shows one connection. Secrets are never returned.

Flags:
`+connectionsFlagHelp, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}

func connectionsCatalogUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s connections catalog [flags]

Lists the providers that can be connected and how each connects.

Flags:
`+connectionsFlagHelp, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}
