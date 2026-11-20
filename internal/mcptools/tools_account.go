package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerAccountTools registers read-only views of the calling account. No
// tool here changes the account, and none returns a token secret.
func registerAccountTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "whoami",
		Description: "Show which identity this connection authenticates as: kind, name, abilities and expiry.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, apiclient.WhoamiResource, error) {
		who, err := client.Whoami(ctx)
		if err != nil {
			return nil, apiclient.WhoamiResource{}, fmt.Errorf("whoami: %w", err)
		}
		return nil, who, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_my_access",
		Description: "What this account can manage: tier, teams, IAM policies, sign-in providers, per-area capabilities with the missing ability.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, apiclient.MyAccessResource, error) {
		access, err := client.MyAccess(ctx)
		if err != nil {
			return nil, apiclient.MyAccessResource{}, fmt.Errorf("get my access: %w", err)
		}
		return nil, access, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_my_sessions",
		Description: "List this account's live browser sessions: browser, network, created, last seen, expiry. Needs a user token with read:sensitive.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.Session, error) {
		creds, err := client.MyCredentials(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list my sessions: %w", err)
		}
		return nil, creds.Sessions, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_my_tokens",
		Description: "List this account's active API tokens: name, abilities, expiry, last use; no secrets. Needs a user token with read:sensitive.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.SessionToken, error) {
		creds, err := client.MyCredentials(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list my tokens: %w", err)
		}
		return nil, creds.Tokens, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_my_activity",
		Description: "This account's sign-ins, token use and security changes, newest first. Filter with kind; all includes non-security rows.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in myActivityInput) (*mcp.CallToolResult, apiclient.MyActivity, error) {
		kind := in.Kind
		if in.All && kind == "" {
			kind = "all"
		}
		feed, err := client.MyActivity(ctx, apiclient.MyActivityOptions{Limit: in.Limit, Kind: kind, Cursor: in.Cursor})
		if err != nil {
			return nil, apiclient.MyActivity{}, fmt.Errorf("list my activity: %w", err)
		}
		return nil, feed, nil
	})
}

type myActivityInput struct {
	Limit  int    `json:"limit,omitempty" jsonschema:"rows to return, 1 to 100, default 25"`
	Kind   string `json:"kind,omitempty" jsonschema:"one category: sign_in, security, sessions, tokens, token_use, account or failed"`
	Cursor string `json:"cursor,omitempty" jsonschema:"the next_cursor of a previous page"`
	All    bool   `json:"all,omitempty" jsonschema:"include raw API requests, default is security events only"`
}
