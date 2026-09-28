package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
)

// runNodesProvision implements "nodes provision": POST
// /api/v1/nodes/provision. Creates a server at a cloud provider and
// starts it enrolling; poll "nodes provisions show <id>" for progress.
func runNodesProvision(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes provision", "print the new provision as JSON to stdout and nothing else", stderr)
	var provider, region, size, name, role, controlPlaneAddr string
	var allowSSHInbound bool
	fs.StringVar(&provider, "provider", "", "hetzner, digitalocean, aws, azure or gcp (required)")
	fs.StringVar(&region, "region", "", "provider region/location id, from \"nodes providers list\"'s regions (required)")
	fs.StringVar(&size, "size", "", "provider server size/plan id (required)")
	fs.StringVar(&name, "name", "", "name for the new node: lowercase letters, digits, hyphens (required)")
	fs.StringVar(&role, "role", "general", "general or build")
	fs.StringVar(&controlPlaneAddr, "control-plane-addr", "", "host:port the new server dials to reach this control plane's agent listener (default: this command's --api-url host, port 9443)")
	fs.BoolVar(&allowSSHInbound, "allow-ssh-inbound", false, "aws only: open a dedicated security group's TCP 22 to this instance, off by default")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesProvisionUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if provider == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--provider is required"))
	}
	if region == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--region is required"))
	}
	if size == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--size is required"))
	}
	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name is required"))
	}
	if controlPlaneAddr == "" {
		controlPlaneAddr = defaultControlPlaneAddr(apiURLFlag, lookupEnv, prog, profileFlag)
	}
	if controlPlaneAddr == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--control-plane-addr could not be derived from --api-url, pass it explicitly"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	provisioned, err := client.CreateNodeProvision(context.Background(), createNodeProvisionRequest{
		Provider: provider, Region: region, Size: size, Name: name, Role: role, ControlPlaneAddr: controlPlaneAddr,
		AllowSSHInbound: allowSSHInbound,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create node provision: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, provisioned, func() {
		_, _ = fmt.Fprintf(stdout, "provision %s created, status: %s\n", provisioned.ID, provisioned.Status)
		_, _ = fmt.Fprintf(stdout, "poll with: %s nodes provisions show %s\n", prog, provisioned.ID)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// defaultControlPlaneAddr derives a best-effort control plane agent
// address from the resolved --api-url's own hostname, port 9443 (the
// fixed agent gRPC listener port, cmd/levelrail/main.go's
// defaultAgentAddr), the same fallback AddNodeDialog.tsx's own manual
// join-token flow pre-fills and lets the operator override. Empty if the
// URL can't be parsed, so the caller can require an explicit flag
// instead.
func defaultControlPlaneAddr(apiURLFlag string, lookupEnv func(string) (string, bool), prog, profileFlag string) string {
	profile := resolveProfile(profileFlag, lookupEnv)
	resolved := resolveAPIURL(apiURLFlag, lookupEnv, prog, profile)
	u, err := url.Parse(resolved)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	return u.Hostname() + ":9443"
}

func nodesProvisionUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes provision --provider NAME --region ID --size ID --name NAME [flags]

Creates a server at a cloud provider (Hetzner, DigitalOcean, AWS, Azure
or GCP) and starts it enrolling as a new node: a fresh join token is
minted, the server boots with a cloud-init script that installs Docker
if missing and runs the node agent, and this command returns immediately
with a provision id to poll.

A credential must already be stored for the chosen provider ("nodes
providers set-credential").

Flags:
  --provider string             hetzner, digitalocean, aws, azure or gcp (required)
  --region string                provider region/location id, from "nodes providers list"'s regions (required)
  --size string                  provider server size/plan id (required)
  --name string                  name for the new node: lowercase letters, digits, hyphens (required)
  --role string                  general or build (default general)
  --control-plane-addr string   host:port the new server dials to reach this control plane's agent listener (default: --api-url's host, port 9443)
  --allow-ssh-inbound             aws only: open a dedicated security group's TCP 22 to this instance, off by default
  --token string                 API token (default: %[2]s env var, then the credentials file)
  --api-url string              control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string              named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                           print the new provision as JSON to stdout, nothing else
  --output string                 output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string                  JMESPath expression to filter the result before printing
  -h, --help                      show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
