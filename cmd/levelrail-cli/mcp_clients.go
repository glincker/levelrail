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

// runMCPClients dispatches "mcp-clients <verb>": pre-registered OAuth clients for remote MCP, root only,
// available when the control plane has the mcp-http experimental feature on.
func runMCPClients(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	subs := map[string]apiCmd{"create": mcpClientsCreateCommand(), "list": mcpClientsListCommand(), "delete": mcpClientsDeleteCommand()}
	return dispatchSub(prog, "mcp-clients", mcpClientsUsage, subs, args, stdout, stderr, lookupEnv)
}

func mcpClientsUsage(prog string) string {
	return fmt.Sprintf(`Usage (experimental, root only, needs the mcp-http feature on the control plane):
  %[1]s mcp-clients create --name N --redirect-uri URI [--redirect-uri URI]... [--redirect-uris URI,URI] [--scope mcp:read|mcp:read:sensitive]... [--json]
  %[1]s mcp-clients list [--json]
  %[1]s mcp-clients delete --id ID [--json]

create registers a public client and prints its generated client_id; there is no client secret.
Redirect URIs must be absolute https (or http on 127.0.0.1 or [::1]) with no fragment. --scope
defaults to mcp:read. delete removes the client and revokes every token it was granted.
`, prog)
}

func mcpClientsCreateCommand() apiCmd {
	var name, csv string
	var uris, scopes headerFlags
	return apiCmd{label: "mcp-clients create", usage: mcpClientsUsage,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&name, "name", "", "client name shown on the consent page (required, at most 64 characters)")
			fs.Var(&uris, "redirect-uri", "allowed redirect URI, repeatable")
			fs.StringVar(&csv, "redirect-uris", "", "allowed redirect URIs, comma separated")
			fs.Var(&scopes, "scope", "scope the client may request: mcp:read or mcp:read:sensitive, repeatable (default mcp:read)")
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			all := append([]string{}, uris...)
			for _, u := range strings.Split(csv, ",") {
				if u = strings.TrimSpace(u); u != "" {
					all = append(all, u)
				}
			}
			if strings.TrimSpace(name) == "" {
				return nil, nil, newValidationError("--name is required")
			}
			if len(all) == 0 {
				return nil, nil, newValidationError("at least one --redirect-uri is required")
			}
			created, err := c.CreateMCPClient(ctx, apiclient.MCPClientCreate{Name: name, RedirectURIs: all, Scopes: scopes})
			return created, func(w io.Writer) {
				_, _ = fmt.Fprintf(w, "client_id: %s\nname: %s\nscopes: %s\n", created.ID, created.Name, strings.Join(created.Scopes, " "))
			}, err
		}}
}

func mcpClientsListCommand() apiCmd {
	return apiCmd{label: "mcp-clients list", usage: mcpClientsUsage,
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			list, err := c.ListMCPClients(ctx)
			return list, func(w io.Writer) {
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				_, _ = fmt.Fprintln(tw, "CLIENT ID\tNAME\tSCOPES\tREDIRECT URIS")
				for _, m := range list {
					_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", m.ID, m.Name, strings.Join(m.Scopes, " "), strings.Join(m.RedirectURIs, ","))
				}
				_ = tw.Flush()
			}, err
		}}
}

func mcpClientsDeleteCommand() apiCmd {
	var id string
	return apiCmd{label: "mcp-clients delete", usage: mcpClientsUsage,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&id, "id", "", "client_id to remove (required)")
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			if id == "" {
				return nil, nil, newValidationError("--id is required")
			}
			err := c.DeleteMCPClient(ctx, id)
			return map[string]string{"deleted": id}, func(w io.Writer) { _, _ = fmt.Fprintf(w, "mcp client %s deleted, its tokens are revoked\n", id) }, err
		}}
}
