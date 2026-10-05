---
{
  "layout": "landing",
  "title": "Coolify alternative: a self-hosted deploy platform that stays light",
  "description": "Levelrail is an open source Coolify alternative: Docker Engine API instead of SSH and CLI shelling, built-in metrics and logs, one-click rollback, and a read-only importer for your Coolify apps.",
  "sidebar": false,
  "aside": false,
  "landing": {
    "eyebrow": "Coolify alternative",
    "headline": "A lighter way to self-host what you deploy with Coolify",
    "sub": "Levelrail is an open source deployment platform for your own servers. Its agent talks to Docker's Engine API directly instead of SSHing in and shelling out, so the platform stays out of your apps' way.",
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
    "cardsHeading": "Why people look for a Coolify alternative",
    "cards": [
      {
        "title": "No SSH, no CLI parsing",
        "body": "A node agent dials out to the control plane over mutual TLS and uses the Docker Engine API. Nothing shells out to the docker CLI, and managed servers need no inbound port.",
        "visual": [
          {
            "k": "cmd",
            "t": "levelrail-cli nodes join-token"
          },
          {
            "k": "out",
            "t": "agent dials out with the token (mTLS)"
          },
          {
            "k": "ok",
            "t": "node online, 0 inbound ports"
          }
        ],
        "icon": "plugs"
      },
      {
        "title": "Built-in metrics and logs",
        "body": "Node-local metrics at 15 second resolution and full-text log search ship in the core. Deploy markers sit on the charts, so which deploy caused a spike is a visual answer.",
        "visual": [
          {
            "k": "cmd",
            "t": "levelrail-cli apps logs web"
          },
          {
            "k": "out",
            "t": "12:04:11 GET /healthz 200"
          },
          {
            "k": "out",
            "t": "12:04:16 GET /healthz 200"
          }
        ],
        "icon": "chart"
      },
      {
        "title": "Rollback that survives cleanup",
        "body": "Previous images stay pinned so garbage collection can never remove your rollback target. Traffic only cuts over once the new container passes a real readiness probe.",
        "visual": [
          {
            "k": "cmd",
            "t": "levelrail-cli rollback web --image web:41"
          },
          {
            "k": "ok",
            "t": "web is live on web:41"
          }
        ],
        "icon": "history"
      }
    ],
    "terminal": {
      "heading": "Bring your apps with you",
      "intro": "The importer reads your existing instance through its own HTTP API and never changes anything there. Run it with --dry-run first to see exactly what would be created.",
      "title": "install, import, verify",
      "ariaLabel": "Terminal recording: installing Levelrail then importing apps from coolify with a dry run first",
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
          "text": "levelrail-cli import platform coolify --url https://coolify.example.com --dry-run"
        },
        {
          "kind": "output",
          "text": "dry run: report only, nothing is written here or changed there"
        },
        {
          "kind": "command",
          "text": "levelrail-cli import platform coolify --url https://coolify.example.com --only web,api"
        },
        {
          "kind": "success",
          "text": "apps and databases created, ready to deploy"
        }
      ]
    },
    "compare": {
      "heading": "Coolify and Levelrail, side by side",
      "intro": "Architecture and design choices, not a ranking. Coolify is a mature project with a very large template catalog.",
      "left": "Coolify",
      "right": "Levelrail",
      "rows": [
        {
          "label": "Server management",
          "left": "SSH from the control plane to each server",
          "right": "Agent dials out over gRPC with mTLS, talks to the Docker Engine API"
        },
        {
          "label": "Orchestration",
          "left": "Docker and Docker Compose per resource",
          "right": "Level-triggered reconciler: desired state in the database, converged continuously"
        },
        {
          "label": "Ingress",
          "left": "Traefik (Caddy also offered)",
          "right": "Caddy embedded in the control plane"
        },
        {
          "label": "Runs as",
          "left": "A web application with its own database",
          "right": "One control plane binary, SQLite in WAL mode, one agent binary per extra node"
        },
        {
          "label": "License",
          "left": "Open source",
          "right": "Apache 2.0, one edition, no paid tier"
        }
      ],
      "more": {
        "text": "Read the full comparison",
        "href": "/comparison"
      }
    },
    "stepsHeading": "Move over in four steps",
    "steps": [
      {
        "title": "Install on a Linux box",
        "body": "One script checks the host, installs Docker if missing, and starts the control plane as a systemd service."
      },
      {
        "title": "Dry-run the importer",
        "body": "Point it at your Coolify URL with --dry-run. You get a report of what would be created. Nothing changes on either side."
      },
      {
        "title": "Import and deploy",
        "body": "Import all apps and databases or a subset with --only, then deploy and watch the readiness probe gate the cutover."
      },
      {
        "title": "Cut DNS when you are ready",
        "body": "Keep Coolify running until the new setup has earned your trust, then switch your domains."
      }
    ],
    "faq": [
      {
        "q": "Is Levelrail a drop-in replacement for Coolify?",
        "a": "It covers the same job, deploying apps and databases to your own servers from a git push, but it is a different design, not a fork. A read-only importer brings apps and databases across, and the migration guide lists what maps and what does not."
      },
      {
        "q": "Does Levelrail have Coolify's one-click service catalog?",
        "a": "Levelrail ships a curated template catalog and is expanding it. Coolify has a larger catalog today, so check whether the services you need are covered before moving."
      },
      {
        "q": "Can I run Levelrail on more than one server?",
        "a": "Yes. Extra servers join with a one-time token and run a small agent that dials out to the control plane, so managed servers never open an inbound port. Enrollment, cordon and drain are verified against real Docker daemons."
      },
      {
        "q": "What does it cost?",
        "a": "Nothing. Levelrail is Apache 2.0 with one edition: SSO, audit logging, IAM and scheduled backups all ship in the same free binary."
      },
      {
        "q": "Is it production ready?",
        "a": "Levelrail is in active pre-release development, and single node is the best tested path. Run it next to your current setup first, which the importer's read-only design makes safe."
      }
    ],
    "cta": {
      "heading": "Try it next to your current setup",
      "sub": "Import read-only, compare for a week, and keep what works. Open source, so you can read every line."
    },
    "related": [
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
