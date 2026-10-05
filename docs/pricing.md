---
{
  "layout": "landing",
  "title": "Pricing: Levelrail is free and open source (Apache 2.0)",
  "description": "Levelrail costs nothing: Apache 2.0, one edition, no license key and no feature gated behind a plan. SSO, audit logging, IAM and scheduled backups are included for everyone.",
  "sidebar": false,
  "aside": false,
  "landing": {
    "eyebrow": "Pricing",
    "headline": "Free. One edition. No feature behind a paywall.",
    "sub": "Levelrail is Apache 2.0 software you run on your own servers. There is no license key, no enterprise tier and no usage meter. Your cost is the servers you already rent.",
    "primary": {
      "text": "Get started",
      "link": "/getting-started"
    },
    "secondary": {
      "text": "View on GitHub",
      "link": "https://github.com/glincker/levelrail"
    },
    "install": "curl -fsSL https://levelrail.com/install.sh | sudo sh",
    "stats": [
      {
        "value": "$0",
        "label": "software cost"
      },
      {
        "value": "1",
        "label": "edition for everyone"
      },
      {
        "value": "0",
        "label": "features gated by plan"
      },
      {
        "value": "Apache 2.0",
        "label": "license"
      }
    ],
    "cardsHeading": "Included for everyone",
    "cards": [
      {
        "title": "Sign-in and access control",
        "body": "Email and password, TOTP two-factor, OAuth with Google, GitHub or any OpenID Connect provider, and AWS-style Allow and Deny IAM policies scoped to a single app or database.",
        "link": {
          "text": "Identity and access",
          "href": "/identity-and-access"
        },
        "icon": "key",
        "visual": [
          {
            "k": "out",
            "t": "sign-in  email, TOTP, OAuth, OIDC"
          },
          {
            "k": "out",
            "t": "policy   Allow  app:web"
          },
          {
            "k": "ok",
            "t": "policy   Deny   database:main"
          }
        ]
      },
      {
        "title": "Audit log and approvals",
        "body": "Every sensitive request is recorded with actor and caller surface, queryable and exportable as CSV. Protected environments, deploy approvals and freeze windows are built in.",
        "link": {
          "text": "Deploy safety",
          "href": "/deploy-safety"
        },
        "icon": "scroll",
        "visual": [
          {
            "k": "out",
            "t": "actor ada  POST /api/v1/apps  200"
          },
          {
            "k": "out",
            "t": "surface   cli"
          },
          {
            "k": "ok",
            "t": "export    audit.csv"
          }
        ]
      },
      {
        "title": "Backups that get verified",
        "body": "Scheduled database and volume backups with retention, restore, and automatic verification after each run, plus snapshots of the control plane itself.",
        "link": {
          "text": "Backups and storage",
          "href": "/backups-and-storage"
        },
        "icon": "database",
        "visual": [
          {
            "k": "out",
            "t": "postgres main  backup 03:00"
          },
          {
            "k": "out",
            "t": "re-download and hash"
          },
          {
            "k": "ok",
            "t": "verified, retention 7"
          }
        ]
      },
      {
        "title": "Observability and alerts",
        "body": "Node-local metrics, full-text logs, crashloop detection and alerts across 18 notification channel kinds, all in the same binary.",
        "link": {
          "text": "Observability",
          "href": "/observability"
        },
        "icon": "chart",
        "visual": [
          {
            "k": "out",
            "t": "crashloop detected: web"
          },
          {
            "k": "out",
            "t": "last 200 log lines attached"
          },
          {
            "k": "ok",
            "t": "alert sent to Slack"
          }
        ]
      },
      {
        "title": "Multi-server",
        "body": "Add servers with a one-time join token and a small agent. Cordon, drain and explicit placement are included.",
        "link": {
          "text": "Multi-node",
          "href": "/multi-node"
        },
        "icon": "network",
        "visual": [
          {
            "k": "cmd",
            "t": "levelrail-cli nodes join-token"
          },
          {
            "k": "out",
            "t": "node2 enrolled, pending"
          },
          {
            "k": "ok",
            "t": "node2 online"
          }
        ]
      },
      {
        "title": "AI-ready API",
        "body": "An MCP server exposes the same API and permission model to AI tools, so agents can list apps, read logs and diagnose a crashloop.",
        "link": {
          "text": "MCP tool surface",
          "href": "/mcp-tool-surface"
        },
        "icon": "robot",
        "routes": [
          {
            "method": "GET",
            "path": "/api/v1/apps"
          },
          {
            "method": "GET",
            "path": "/api/v1/system/status"
          },
          {
            "method": "GET",
            "path": "/api/v1/deploys/failed"
          },
          {
            "method": "POST",
            "path": "/api/v1/nodes/join-tokens"
          },
          {
            "method": "POST",
            "path": "/api/v1/imports/platform/discover"
          },
          {
            "method": "GET",
            "path": "/api/v1/certificates"
          },
          {
            "method": "GET",
            "path": "/api/v1/system/doctor"
          }
        ]
      }
    ],
    "prose": [
      {
        "heading": "What you actually pay for",
        "paragraphs": [
          "The only cost is infrastructure: the Linux servers you run Levelrail on and any cloud provider, object storage or registry you choose to connect. A single small VPS is enough to start.",
          "There is no hosted plan today. Levelrail is self-hosted software only, so there is nothing to subscribe to."
        ]
      },
      {
        "heading": "Why it stays this way",
        "paragraphs": [
          "Levelrail has one edition by design. Features such as SSO, audit logging and fine-grained roles ship in the same free binary. The comparison page covers how Levelrail differs from other projects and notes that their plans change over time."
        ]
      }
    ],
    "faq": [
      {
        "q": "Is Levelrail really free?",
        "a": "Yes. It is Apache 2.0 licensed with a single edition. There is no license key, no enterprise tier and no feature gated behind a plan."
      },
      {
        "q": "Is there paid support?",
        "a": "Not today. Help is through GitHub Discussions, issues and the community Discord."
      },
      {
        "q": "Can I use it commercially?",
        "a": "Yes. The Apache 2.0 license permits commercial use, modification and redistribution, subject to its terms."
      },
      {
        "q": "Will features move behind a paywall later?",
        "a": "The project's stated position is one edition for everyone. Releases that are already published remain under Apache 2.0."
      }
    ],
    "cta": {
      "heading": "Start for free",
      "sub": "Install it on one server in a few minutes and see whether it fits."
    }
  }
}
---
