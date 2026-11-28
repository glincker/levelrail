package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/glincker/levelrail/internal/apiclient"
)

// runNodesProvidersServers implements "nodes providers servers": the machines
// already in the provider account. Read-only.
func runNodesProvidersServers(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes providers servers", "print the servers as JSON to stdout and nothing else", stderr)
	var provider, region string
	var refresh bool
	fs.StringVar(&provider, "provider", "", "provider to list servers for (required)")
	fs.StringVar(&region, "region", "", "only servers in this region (aws lists that region, others filter the connected list)")
	fs.BoolVar(&refresh, "refresh", false, "bypass the cached list")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if provider == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--provider is required"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	list, err := client.ListNodeProviderServers(context.Background(), provider, region, refresh)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list servers for provider %q: %w", provider, err))
	}
	list = filterServersByRegion(list, region)
	if err := renderResult(stdout, of.Format, of.Query, list, func() { printProviderServers(stdout, list) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func filterServersByRegion(in []apiclient.NodeProviderServer, region string) []apiclient.NodeProviderServer {
	if region == "" {
		return in
	}
	out := []apiclient.NodeProviderServer{}
	for _, r := range in {
		if r.Region == region {
			out = append(out, r)
		}
	}
	return out
}

func printProviderServers(out io.Writer, list []apiclient.NodeProviderServer) {
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "no servers found")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tREGION\tSIZE\tSTATE\tPUBLIC IP\tNODE")
	for _, r := range list {
		node := "-"
		if r.EnrolledNode != nil {
			node = r.EnrolledNode.Name
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.ID, r.Name, r.Region, r.Size, r.Status, r.PublicIPv4, node)
	}
	_ = tw.Flush()
}

// runNodesImport implements "nodes import": it mints a join token for an
// instance that already exists. It never touches the instance, the operator
// runs the printed command on it.
func runNodesImport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes import", "print the join token response as JSON to stdout and nothing else", stderr)
	var provider, instanceID, name, region string
	var mesh bool
	fs.StringVar(&provider, "provider", "", "provider that owns the instance (required)")
	fs.StringVar(&instanceID, "instance-id", "", "existing instance id, for example i-0abc (required)")
	fs.StringVar(&region, "region", "", "region to search (aws queries it directly, default the connected region)")
	fs.StringVar(&name, "name", "", "node name (default: the instance name tag)")
	fs.BoolVar(&mesh, "mesh", false, "join the WireGuard mesh")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if provider == "" || instanceID == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--provider and --instance-id are required"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	list, err := client.ListNodeProviderServers(context.Background(), provider, region, true)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list servers for provider %q: %w", provider, err))
	}
	srv, found := findServerByInstanceID(list, instanceID)
	if !found {
		return reportError(stdout, stderr, jsonOut, newValidationError("instance %q not found in provider %q (try nodes providers servers)", instanceID, provider))
	}
	if srv.EnrolledNode != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("instance %q is already node %q", instanceID, srv.EnrolledNode.Name))
	}
	if name == "" {
		name = nodeNameFromTag(srv.Name)
	}
	req := apiclient.CreateNodeJoinTokenRequest{NodeName: name, Mesh: mesh, Provider: provider, Region: srv.Region}
	created, err := client.CreateNodeJoinTokenFor(context.Background(), req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create node join token: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, created, func() {
		_, _ = fmt.Fprintf(stdout, "join token for %s (shown once, not recoverable again): %s\n", srv.ID, created.Token)
		if created.CAFingerprint != "" {
			_, _ = fmt.Fprintf(stdout, "control plane CA fingerprint (set APP_CA_FINGERPRINT on the agent): %s\n", created.CAFingerprint)
		}
		if created.JoinCommands != nil {
			_, _ = fmt.Fprintln(stdout)
			printJoinCommands(stdout, created, installAgentOpts{Name: name, Mesh: mesh})
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// findServerByInstanceID matches the bare id or the region/id form.
func findServerByInstanceID(list []apiclient.NodeProviderServer, id string) (apiclient.NodeProviderServer, bool) {
	for _, r := range list {
		if r.ID == id || strings.HasSuffix(r.ID, "/"+id) {
			return r, true
		}
	}
	return apiclient.NodeProviderServer{}, false
}

// nodeNameFromTag lowercases a Name tag into the dash form node names allow.
func nodeNameFromTag(tag string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(tag) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteRune(c)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
