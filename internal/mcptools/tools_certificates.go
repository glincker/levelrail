package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerCertificateTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_certificates",
		Description: "List every TLS certificate the control plane holds: domain, SANs, issuer, validity window and status (healthy, expiring_soon, expired).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.CertificateResource, error) {
		certs, err := client.ListCertificates(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list certificates: %w", err)
		}
		return nil, certs, nil
	})
}
