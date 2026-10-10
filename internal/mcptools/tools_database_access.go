package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpDatabaseUser struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Preset      string `json:"preset,omitempty"`
	CanLogin    bool   `json:"can_login"`
	Limit       int    `json:"connection_limit"`
	ValidUntil  string `json:"valid_until,omitempty"`
	Connections int    `json:"connections"`
}

type databaseUsersOutput struct {
	Users []mcpDatabaseUser `json:"users"`
}

type mcpPrincipal struct {
	Type      string   `json:"type"`
	Name      string   `json:"name"`
	Effective []string `json:"effective"`
	Via       []string `json:"via"`
}

type mcpTempLogin struct {
	Role      string `json:"role"`
	Preset    string `json:"preset"`
	ExpiresAt string `json:"expires_at"`
}

type databaseAccessOutput struct {
	Principals []mcpPrincipal `json:"principals"`
	Policies   []string       `json:"policies"`
	Temporary  []mcpTempLogin `json:"temporary_logins"`
}

// registerDatabaseAccessTools is read-only on purpose: creating users, issuing
// credentials and changing rules hand out secrets or change exposure.
func registerDatabaseAccessTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_database_users",
		Description: "List a managed Postgres database's roles: login, preset, limit, expiry, live connections. No passwords. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in databaseNameInput) (*mcp.CallToolResult, databaseUsersOutput, error) {
		users, err := client.ListDatabaseUsers(ctx, in.Name)
		if err != nil {
			return nil, databaseUsersOutput{}, fmt.Errorf("list users of database %q: %w", in.Name, err)
		}
		out := databaseUsersOutput{Users: make([]mcpDatabaseUser, 0, len(users))}
		for _, u := range users {
			out.Users = append(out.Users, mcpDatabaseUser{Name: u.Name, Kind: u.Kind, Preset: u.Preset, CanLogin: u.CanLogin, Limit: u.ConnectionLimit, ValidUntil: u.ValidUntil, Connections: u.Connections})
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_database_access",
		Description: "Who can reach a managed database: users and tokens with effective abilities, IAM policies that touch it, active temporary logins (no secrets). Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in databaseNameInput) (*mcp.CallToolResult, databaseAccessOutput, error) {
		who, err := client.GetDatabaseWho(ctx, in.Name)
		if err != nil {
			return nil, databaseAccessOutput{}, fmt.Errorf("get access of database %q: %w", in.Name, err)
		}
		temp, err := client.ListDatabaseTemp(ctx, in.Name)
		if err != nil {
			return nil, databaseAccessOutput{}, fmt.Errorf("list temporary logins of database %q: %w", in.Name, err)
		}
		out := databaseAccessOutput{Principals: []mcpPrincipal{}, Policies: []string{}, Temporary: []mcpTempLogin{}}
		for _, p := range who.Principals {
			out.Principals = append(out.Principals, mcpPrincipal{Type: p.Type, Name: p.Name, Effective: p.Effective, Via: p.Via})
		}
		for _, p := range who.Policies {
			out.Policies = append(out.Policies, p.Name)
		}
		for _, t := range temp.Items {
			out.Temporary = append(out.Temporary, mcpTempLogin{Role: t.Role, Preset: t.Preset, ExpiresAt: t.ExpiresAt})
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_database_reachability",
		Description: "Explain how a managed database can be reached: verdict, networks, connecting apps, published port and exposure, allowed sources with drift, network scope, TLS. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in databaseNameInput) (*mcp.CallToolResult, reachabilityOutput, error) {
		n, err := client.GetDatabaseNetwork(ctx, in.Name)
		if err != nil {
			return nil, reachabilityOutput{}, fmt.Errorf("get reachability of database %q: %w", in.Name, err)
		}
		return nil, summarizeReachability(n), nil
	})
}

type reachabilityOutput struct {
	Verdict        string   `json:"verdict"`
	Level          string   `json:"level"`
	Host           string   `json:"internal_host"`
	Networks       []string `json:"networks"`
	Apps           []string `json:"apps"`
	AppsOutOfScope []string `json:"apps_out_of_scope"`
	PublishedPort  int      `json:"published_port,omitempty"`
	Bind           []string `json:"bind,omitempty"`
	Exposure       string   `json:"exposure,omitempty"`
	AllowedSources []string `json:"allowed_sources"`
	Drift          []string `json:"drift"`
	Scope          string   `json:"scope"`
	TLSRequired    bool     `json:"tls_required"`
	TLSState       string   `json:"tls_state,omitempty"`
}

func summarizeReachability(n apiclient.DatabaseNetwork) reachabilityOutput {
	out := reachabilityOutput{
		Verdict: n.Verdict.Text, Level: n.Verdict.Level, Host: n.Internal.Host,
		Networks: []string{}, Apps: []string{}, AppsOutOfScope: []string{}, AllowedSources: []string{}, Drift: []string{},
		Scope: n.Scope.Current, TLSRequired: n.TLS.Required, TLSState: n.TLS.State,
	}
	for _, net := range n.Networks {
		out.Networks = append(out.Networks, net.Name)
	}
	for _, c := range n.Clients {
		if c.InScope {
			out.Apps = append(out.Apps, c.App)
		} else {
			out.AppsOutOfScope = append(out.AppsOutOfScope, c.App)
		}
	}
	if n.Published != nil {
		out.PublishedPort, out.Bind, out.Exposure = n.Published.HostPort, n.Published.Bind, n.Published.Class
	}
	for _, r := range n.Rules.Allow {
		out.AllowedSources = append(out.AllowedSources, r.Source)
	}
	for _, m := range n.Rules.Missing {
		out.Drift = append(out.Drift, "missing from firewall: "+m)
	}
	for _, e := range n.Rules.Extra {
		out.Drift = append(out.Drift, "extra in firewall: "+e)
	}
	return out
}
