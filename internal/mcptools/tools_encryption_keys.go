package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type encryptionKeyStatusInput struct{}

type encryptionKeyStatusOutput struct {
	MasterKeyAgeDays int      `json:"master_key_age_days"`
	MaxKeyAgeDays    int      `json:"max_key_age_days"`
	Apps             []string `json:"apps"`
	LastRun          string   `json:"last_run"`
	Hints            []string `json:"hints"`
}

func registerEncryptionKeyTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_encryption_key_status",
		Description: "Key ages (master and per app), max age policy, last rotate-all run and fixes. No key material.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ encryptionKeyStatusInput) (*mcp.CallToolResult, encryptionKeyStatusOutput, error) {
		st, err := client.GetEncryptionKeys(ctx)
		if err != nil {
			return nil, encryptionKeyStatusOutput{}, fmt.Errorf("get encryption key status: %w", err)
		}
		out := encryptionKeyStatusOutput{MaxKeyAgeDays: st.Policy.MaxKeyAgeDays, MasterKeyAgeDays: -1, Apps: []string{}, Hints: []string{}}
		if st.MasterKey.AgeDays != nil {
			out.MasterKeyAgeDays = *st.MasterKey.AgeDays
		}
		for _, a := range st.Apps {
			out.Apps = append(out.Apps, fmt.Sprintf("%s: %d days, %d values, overdue %t", a.App, a.AgeDays, a.ValueCount, a.Overdue))
		}
		if r := st.LastRun; r != nil {
			out.LastRun = fmt.Sprintf("%d/%d apps, %d failed, finished %t", r.Done, r.Total, r.Failed, r.FinishedAt != nil)
		}
		for _, h := range st.Hints {
			out.Hints = append(out.Hints, h.Message)
		}
		return nil, out, nil
	})
}
