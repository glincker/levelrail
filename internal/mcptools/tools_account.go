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
		Description: "Show what this account can and cannot manage: tier, teams, attached IAM policies, linked sign-in providers and a per-area capability list with the missing ability.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, apiclient.MyAccessResource, error) {
		access, err := client.MyAccess(ctx)
		if err != nil {
			return nil, apiclient.MyAccessResource{}, fmt.Errorf("get my access: %w", err)
		}
		return nil, access, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_my_sessions",
		Description: "List this account's live browser sessions: browser, network, created, last seen and expiry. Needs a token that holds read:sensitive and belongs to a user.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.Session, error) {
		creds, err := client.MyCredentials(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list my sessions: %w", err)
		}
		return nil, creds.Sessions, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_my_tokens",
		Description: "List this account's active API tokens: name, abilities, expiry and last use. A token's secret is never returned. Needs a token that holds read:sensitive and belongs to a user.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.SessionToken, error) {
		creds, err := client.MyCredentials(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list my tokens: %w", err)
		}
		return nil, creds.Tokens, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_my_activity",
		Description: "List what this account's own sessions and tokens did, newest first: sign-ins, token use, session changes and passkey or two-factor changes. Only this account's rows. Filter with kind (sign_in, security, sessions, tokens, token_use, account, failed) and page with cursor.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in myActivityInput) (*mcp.CallToolResult, apiclient.MyActivity, error) {
		feed, err := client.MyActivity(ctx, apiclient.MyActivityOptions{Limit: in.Limit, Kind: in.Kind, Cursor: in.Cursor})
		if err != nil {
			return nil, apiclient.MyActivity{}, fmt.Errorf("list my activity: %w", err)
		}
		return nil, feed, nil
	})
}

type myActivityInput struct {
	Limit  int    `json:"limit,omitempty" jsonschema:"rows to return, 1 to 100, default 25"`
	Kind   string `json:"kind,omitempty" jsonschema:"only one category: sign_in, security, sessions, tokens, token_use, account or failed"`
	Cursor string `json:"cursor,omitempty" jsonschema:"the next_cursor of a previous page"`
}
