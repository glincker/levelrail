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
        "icon": "key"
      },
      {
        "title": "Audit log and approvals",
        "body": "Every sensitive request is recorded with actor and caller surface, queryable and exportable as CSV. Protected environments, deploy approvals and freeze windows are built in.",
        "link": {
          "text": "Deploy safety",
          "href": "/deploy-safety"
        },
        "icon": "scroll"
      },
      {
        "title": "Backups that get verified",
        "body": "Scheduled database and volume backups with retention, restore, and automatic verification after each run, plus snapshots of the control plane itself.",
        "link": {
          "text": "Backups and storage",
          "href": "/backups-and-storage"
        },
        "icon": "database"
      },
      {
        "title": "Observability and alerts",
        "body": "Node-local metrics, full-text logs, crashloop detection and alerts across seventeen channels, all in the same binary.",
        "link": {
          "text": "Observability",
          "href": "/observability"
        },
        "icon": "chart"
      },
      {
        "title": "Multi-server",
        "body": "Add servers with a one-time join token and a small agent. Cordon, drain and explicit placement are included.",
        "link": {
          "text": "Multi-node",
          "href": "/multi-node"
        },
        "icon": "network"
      },
      {
        "title": "AI-ready API",
        "body": "An MCP server exposes the same API and permission model to AI tools, so agents can list apps, read logs and diagnose a crashloop.",
        "link": {
          "text": "MCP tool surface",
          "href": "/mcp-tool-surface"
        },
        "icon": "robot"
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
          "Levelrail has one edition by design. Features that other platforms reserve for paid tiers, such as SSO, audit logging and fine-grained roles, ship in the same free binary. The comparison page explains where that differs from other projects, and notes that their plans change over time."
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
    },
    "related": [
      {
        "text": "Coolify alternative",
        "link": "/coolify-alternative"
      },
      {
        "text": "Dokploy alternative",
        "link": "/dokploy-alternative"
      },
      {
        "text": "Vercel alternative",
        "link": "/vercel-alternative"
      },
      {
        "text": "Heroku alternative",
        "link": "/heroku-alternative"
      },
      {
        "text": "Railway alternative",
        "link": "/railway-alternative"
      },
      {
        "text": "Privacy",
        "link": "/privacy"
      },
      {
        "text": "Demo",
        "link": "/demo"
      },
      {
        "text": "Case studies",
        "link": "/case-studies"
      },
      {
        "text": "Full comparison",
        "link": "/comparison"
      }
    ]
  }
}
---
