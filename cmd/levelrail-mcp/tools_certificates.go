package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerCertificateTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_certificates",
		Description: "List every TLS certificate this control plane currently holds: domain, SANs, issuer, validity window, and status (healthy, expiring_soon, or expired). A cert renewal failing silently is this project's central operational risk, so this is the surface for catching one before it becomes an outage. Read-only; a control plane that has never issued a certificate returns an empty list, not an error.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.CertificateResource, error) {
		certs, err := client.ListCertificates(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list certificates: %w", err)
		}
		return nil, certs, nil
	})
}
