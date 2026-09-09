package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerRegistryCredentialTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_registry_credentials",
		Description: "List every private image registry credential connected to the control plane: host, username, and an expiry_status of healthy/expiring_soon/expired when an expiry was set. No password field, ever. Read-only; does not create, edit, delete, or test a credential.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.RegistryCredentialResource, error) {
		creds, err := client.ListRegistryCredentials(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list registry credentials: %w", err)
		}
		return nil, creds, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_registry_credential_repositories",
		Description: "List every repository in the external registry a stored credential authenticates against, so an operator or agent can find an image reference for a direct-image app deploy without leaving the assistant. Read-only; resolves the credential's stored password server-side only to authenticate the upstream query, never returns it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in registryCredentialIDInput) (*mcp.CallToolResult, apiclient.RegistryRepositoriesResource, error) {
		repos, err := client.ListRegistryCredentialRepositories(ctx, in.ID)
		if err != nil {
			return nil, apiclient.RegistryRepositoriesResource{}, fmt.Errorf("list registry credential %q repositories: %w", in.ID, err)
		}
		return nil, repos, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_registry_credential_tags",
		Description: "List every tag pushed for one repository in the external registry a stored credential authenticates against. Read-only, same credential-resolution boundary as list_registry_credential_repositories.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in registryCredentialTagsInput) (*mcp.CallToolResult, apiclient.RegistryTagsResource, error) {
		tags, err := client.ListRegistryCredentialTags(ctx, in.ID, in.Repository)
		if err != nil {
			return nil, apiclient.RegistryTagsResource{}, fmt.Errorf("list registry credential %q tags for %q: %w", in.ID, in.Repository, err)
		}
		return nil, tags, nil
	})
}

type registryCredentialIDInput struct {
	ID string `json:"id" jsonschema:"the registry credential's id"`
}

type registryCredentialTagsInput struct {
	ID         string `json:"id" jsonschema:"the registry credential's id"`
	Repository string `json:"repository" jsonschema:"repository name to list tags for"`
}
