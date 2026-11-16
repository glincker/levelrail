---
page_title: "Levelrail Provider"
subcategory: ""
description: |-
  Manage projects, environments, apps, domains, databases and environment variables on a self-hosted Levelrail control plane.
---

# Levelrail Provider

Manage projects, environments, apps, domains, databases and environment variables on a self-hosted control plane. The provider drives the same REST API as the dashboard and CLI with your own token, so every policy and approval rule still applies.

## Example Usage

```terraform
terraform {
  required_providers {
    levelrail = {
      source = "glincker/levelrail"
    }
  }
}

provider "levelrail" {
  url   = "https://deploy.example.com"
  token = var.api_token
}
```

## Authentication

`url` and `token` fall back to the `APP_API_URL` and `APP_API_TOKEN` environment variables, the same ones the CLI reads. Prefer the variables over writing the token into a file. Both must be set one way or the other.

## Schema

### Optional

- `url` (String) Base URL of the control plane. Falls back to the APP_API_URL environment variable.
- `token` (String, Sensitive) API token. Falls back to the APP_API_TOKEN environment variable.
- `max_retries` (Number) Times a safe request is retried on 429, 5xx or a connection reset (GET, PUT, DELETE, HEAD, and POST only with an Idempotency-Key). Default 4, 0 turns retries off.
- `retry_wait_max_seconds` (Number) Longest pause between two attempts in seconds, which also caps a server Retry-After. Default 30.
