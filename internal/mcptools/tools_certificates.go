package mcptools

import (
	"context"
	"fmt"

	"github.com/glincker/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerCertificateTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_certificates",
		Description: "List TLS certificates: domain, SANs, issuer, validity window, status (healthy, expiring_soon, expired).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.CertificateResource, error) {
		certs, err := client.ListCertificates(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list certificates: %w", err)
		}
		return nil, certs, nil
	})
}
