# Terraform and OpenTofu provider

A terraform-plugin-framework provider for the control plane REST API. It is a separate Go module so it never affects the main binary.

User documentation, the example, import IDs and the list of what is not covered: `docs/terraform-provider.md`.

## Layout

- `main.go`: provider server entry point.
- `internal/provider/brand.go`: the only file that names the product (resource prefix, registry address, env vars).
- `internal/provider/client.go`: JSON client and the wire types, checked by hand against the Go handlers because the OpenAPI document does not describe request bodies.
- `internal/provider/resource_*.go`, `datasources.go`: resources and data sources.

## Check

```bash
go build ./... && go vet ./... && go test ./...
```

Acceptance tests are skipped unless `TF_ACC=1`, `APP_API_URL` and `APP_API_TOKEN` are set. See the docs page for the dev mode control plane recipe.

## Adding a resource

Add a `resource_<name>.go`, register it in `provider.go`, build its type name from `req.ProviderTypeName`, and add it to `docs/terraform-provider.md` with its import ID.
