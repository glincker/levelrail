---
{
  "layout": "landing",
  "title": "Dokploy alternative: self-hosted deploys without Docker Swarm",
  "description": "Levelrail is an open source Dokploy alternative that skips Docker Swarm: an agent on each node, a reconciler over the Docker Engine API, built-in observability, and a read-only importer.",
  "sidebar": false,
  "aside": false,
  "landing": {
    "eyebrow": "Dokploy alternative",
    "headline": "Self-hosted deploys without betting on Docker Swarm",
    "sub": "Levelrail runs apps on plain Docker with its own reconciler and a small agent per node. No Swarm and no overlay network to debug.",
    "primary": {
      "text": "Get started",
      "link": "/getting-started"
    },
    "secondary": {
      "text": "View on GitHub",
      "link": "https://github.com/glincker/levelrail"
    },
    "install": "curl -fsSL https://levelrail.com/install.sh | sudo sh",
    "shot": {
      "src": "/assets/screenshots/apps-list.png",
      "alt": "Levelrail apps list showing every service, its status and its node"
    },
    "cardsHeading": "What is different under the hood",
    "cards": [
      {
        "title": "A reconciler, not Swarm services",
        "body": "Desired state lives in the database and a level-triggered loop converges each node toward it, recording a status condition with a reason after every pass.",
        "visual": [
          {
            "k": "out",
            "t": "app web, desired: web:42"
          },
          {
            "k": "out",
            "t": "observed: web:42"
          },
          {
            "k": "ok",
            "t": "condition: Ready (probe passed)"
          }
        ],
        "icon": "cube"
      },
      {
        "title": "Agent per node, nothing inbound",
        "body": "Each extra server runs a small agent that dials out to the control plane with mutual TLS. Cordon and drain relocate containers between real Docker daemons.",
        "visual": [
          {
            "k": "cmd",
            "t": "levelrail-cli nodes list"
          },
          {
            "k": "out",
            "t": "node1  online  control plane"
          },
          {
            "k": "out",
            "t": "node2  online  agent"
          }
        ],
        "icon": "network"
      },
      {
        "title": "Observability is core",
        "body": "Metrics, full-text logs, crashloop detection with the last log lines, and alerts to 18 notification channel kinds all ship in the box, with no extra stack to install.",
        "visual": [
          {
            "k": "cmd",
            "t": "levelrail-cli attention"
          },
          {
            "k": "out",
            "t": "1 app failing, 0 nodes offline"
          },
          {
            "k": "ok",
            "t": "certificates: none expiring"
          }
        ],
        "icon": "chart"
      }
    ],
    "terminal": {
      "heading": "Bring your apps with you",
      "intro": "The importer reads your existing instance through its own HTTP API and never changes anything there. Run it with --dry-run first to see exactly what would be created.",
      "title": "install, import, verify",
      "ariaLabel": "Terminal recording: installing Levelrail then importing apps from dokploy with a dry run first",
      "lines": [
        {
          "kind": "command",
          "text": "curl -fsSL https://levelrail.com/install.sh | sudo sh"
        },
        {
          "kind": "success",
          "text": "control plane running as a systemd service"
        },
        {
          "kind": "command",
          "text": "levelrail-cli import platform dokploy --url https://dokploy.example.com --dry-run"
        },
        {
          "kind": "output",
          "text": "dry run: report only, nothing is written here or changed there"
        },
        {
          "kind": "command",
          "text": "levelrail-cli import platform dokploy --url https://dokploy.example.com --only web,api"
        },
        {
          "kind": "success",
          "text": "apps and databases created, ready to deploy"
        }
      ]
    },
    "compare": {
      "heading": "Dokploy and Levelrail, side by side",
      "intro": "Dokploy is a polished project built on a mature mechanism. These are the design differences.",
      "left": "Dokploy",
      "right": "Levelrail",
      "rows": [
        {
          "label": "Multi-node",
          "left": "Docker Swarm",
          "right": "Agent per node over gRPC with mTLS"
        },
        {
          "label": "Orchestration",
          "left": "Swarm services and Docker Compose",
          "right": "Level-triggered reconciler over the Docker Engine API"
        },
        {
          "label": "Ingress",
          "left": "Traefik",
          "right": "Caddy embedded in the control plane"
        },
        {
          "label": "Runs as",
          "left": "A web application with its own database",
          "right": "One Go binary with embedded SQLite"
        },
        {
          "label": "Observability",
          "left": "Monitoring features in the dashboard",
          "right": "Node-local metrics and log search with deploy markers, federated across agents"
        }
      ],
      "more": {
        "text": "Read the full comparison",
        "href": "/comparison"
      }
    },
    "stepsHeading": "Evaluate it safely",
    "steps": [
      {
        "title": "Install on a spare box",
        "body": "Run the install script on any Linux server with outbound access."
      },
      {
        "title": "Import read-only",
        "body": "The importer only reads from Dokploy through its API. Use --dry-run to see the report first."
      },
      {
        "title": "Deploy and compare",
        "body": "Run both for a while. Rollback, readiness gating and the built-in charts are easy to judge side by side."
      }
    ],
    "faq": [
      {
        "q": "Does Levelrail use Docker Swarm?",
        "a": "No. It runs plain Docker containers on each node, driven through the Docker Engine API by a reconciler. Placement across nodes is explicit, and cordon and drain are supported."
      },
      {
        "q": "How do I migrate from Dokploy?",
        "a": "Use the platform importer, levelrail-cli import platform dokploy with your Dokploy URL. It is read-only against the source and supports --dry-run and --only."
      },
      {
        "q": "Is Levelrail free?",
        "a": "Yes, Apache 2.0 with a single edition and no feature gated behind a plan."
      },
      {
        "q": "How mature is it?",
        "a": "It is pre-release and moving fast. Single node is the most tested path, and multi-node enrollment, cordon and drain have been verified against real Docker daemons."
      }
    ],
    "cta": {
      "heading": "See how a Swarm-free setup feels",
      "sub": "Install it on a spare server and import read-only. If it is not for you, nothing was changed."
    },
    "related": [
      {
        "text": "Coolify alternative",
        "link": "/coolify-alternative"
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
        "text": "Pricing",
        "link": "/pricing"
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
