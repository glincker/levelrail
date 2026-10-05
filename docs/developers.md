---
{
  "layout": "landing",
  "title": "Developers: API, CLI and MCP server for your Levelrail instance",
  "description": "Build on Levelrail: the REST API at /api/v1 on your own instance, the levelrail-cli command line, the self-hosted MCP server, and scoped API tokens.",
  "sidebar": false,
  "aside": false,
  "landing": {
    "eyebrow": "Developers",
    "headline": "Build on the API every Levelrail surface already uses",
    "sub": "Levelrail is self-hosted, so there is no hosted public API. Your own instance serves a versioned REST API at /api/v1, and the dashboard, CLI and MCP server all run on it.",
    "primary": {
      "text": "API reference",
      "link": "/api-reference"
    },
    "secondary": {
      "text": "CLI reference",
      "link": "/cli-reference"
    },
    "cardsHeading": "Three ways in, one API",
    "cards": [
      {
        "title": "REST API",
        "body": "Versioned routes under /api/v1 on your own control plane, authenticated with an API token. Every route lists the ability it needs.",
        "icon": "plugs",
        "link": {
          "text": "API reference",
          "href": "/api-reference"
        }
      },
      {
        "title": "levelrail-cli",
        "body": "A pure HTTP client for the same API: deploy, roll back, read logs and manage nodes from a terminal or a CI job.",
        "icon": "scroll",
        "link": {
          "text": "CLI reference",
          "href": "/cli-reference"
        }
      },
      {
        "title": "MCP server",
        "body": "levelrail-mcp exposes the API as MCP tools over stdio or a network listener you run yourself, with read-only and tool profile modes.",
        "icon": "network",
        "link": {
          "text": "MCP tool surface",
          "href": "/mcp-tool-surface"
        }
      }
    ],
    "prose": [
      {
        "heading": "Scoped API tokens",
        "paragraphs": [
          "Tokens carry abilities such as read, write and deploy, so an agent or CI job gets only what it needs. IAM policies can further allow or deny access to one resource, such as a single app, and an explicit deny always wins.",
          "Pick the narrowest ability set a client actually needs. Details are in Identity and access."
        ]
      },
      {
        "heading": "Route table and spec",
        "paragraphs": [
          "An OpenAPI 3.1 document listing every route, its method, path parameters and required token ability is published at https://levelrail.com/openapi.json and regenerated with the API reference. Request and response bodies are not described yet, so they are left generic. Each instance also serves a route table at GET /api/v1/openapi.json."
        ]
      }
    ],
    "cta": {
      "heading": "Point a tool at your instance",
      "sub": "Install Levelrail, create an API token, and call /api/v1 from the CLI, MCP server or your own code."
    },
    "related": [
      {
        "text": "Identity and access",
        "link": "/identity-and-access"
      },
      {
        "text": "App spec reference",
        "link": "/app-spec-reference"
      },
      {
        "text": "AI assistant integration",
        "link": "/ai-assistant"
      },
      {
        "text": "Working with AI agents",
        "link": "/agents"
      },
      {
        "text": "Getting started",
        "link": "/getting-started"
      }
    ]
  }
}
---
