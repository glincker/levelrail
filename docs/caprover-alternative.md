---
{
  "layout": "landing",
  "title": "CapRover alternative: self-hosted deploys without Docker Swarm",
  "description": "Levelrail is an open source CapRover alternative: plain Docker instead of Swarm, an agent per node, built-in metrics and logs, and a read-only importer for your CapRover apps.",
  "sidebar": false,
  "aside": false,
  "landing": {
    "eyebrow": "CapRover alternative",
    "headline": "Keep the simple CapRover workflow, drop the Swarm dependency",
    "sub": "Levelrail runs apps on plain Docker containers with its own reconciler and a small agent per extra server. Git push to deploy, automatic HTTPS, rollback and an importer for your CapRover apps.",
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
      "mock": "apps",
      "alt": "Illustrative rendering of the Levelrail apps list with status, domains and deploy times"
    },
    "cardsHeading": "Where Levelrail differs",
    "cards": [
      {
        "title": "No Swarm, plain Docker",
        "body": "Apps run as ordinary containers. A level-triggered reconciler compares desired and observed state and converges, recording a reason after every pass.",
        "visual": [
          {
            "k": "out",
            "t": "desired: web:42"
          },
          {
            "k": "out",
            "t": "observed: web:42"
          },
          {
            "k": "ok",
            "t": "condition: Ready"
          }
        ]
      },
      {
        "title": "Agent per node",
        "body": "Extra servers run a small agent that dials out to the control plane with mutual TLS, so managed servers need no inbound port.",
        "visual": [
          {
            "k": "cmd",
            "t": "levelrail-cli nodes join-token"
          },
          {
            "k": "out",
            "t": "agent dials out with the token"
          },
          {
            "k": "ok",
            "t": "node online"
          }
        ]
      },
      {
        "title": "Observability included",
        "body": "Node-local metrics, full-text log search, crashloop detection and alerts to eighteen channels ship in the core, with deploy markers on the charts.",
        "visual": [
          {
            "k": "out",
            "t": "crashloop detected: web"
          },
          {
            "k": "ok",
            "t": "last 200 log lines attached"
          }
        ]
      }
    ],
    "terminal": {
      "heading": "Bring your apps with you",
      "intro": "The importer reads your existing instance through its own HTTP API and never changes anything there. Run it with --dry-run first.",
      "title": "install, import, verify",
      "ariaLabel": "Terminal recording: installing Levelrail then importing apps from caprover with a dry run first",
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
          "text": "levelrail-cli import platform caprover --url https://captain.example.com --dry-run"
        },
        {
          "kind": "output",
          "text": "dry run: report only, nothing is written here or changed there"
        },
        {
          "kind": "command",
          "text": "levelrail-cli import platform caprover --url https://captain.example.com --only web,api"
        },
        {
          "kind": "success",
          "text": "apps and databases created, ready to deploy"
        }
      ]
    },
    "compare": {
      "heading": "CapRover and Levelrail, side by side",
      "intro": "CapRover is a long-standing project with a large install base and a one-click app store. These are the design differences.",
      "left": "CapRover",
      "right": "Levelrail",
      "rows": [
        {
          "label": "Server management",
          "left": "Docker Swarm",
          "right": "Agent per node over gRPC with mutual TLS, Docker Engine API"
        },
        {
          "label": "Orchestration",
          "left": "Swarm services",
          "right": "Level-triggered reconciler over plain Docker"
        },
        {
          "label": "Ingress",
          "left": "nginx",
          "right": "Caddy embedded in the control plane"
        },
        {
          "label": "Runs as",
          "left": "A web application running as a Swarm service",
          "right": "One Go binary with embedded SQLite"
        },
        {
          "label": "License",
          "left": "Open source",
          "right": "Apache 2.0, one edition"
        }
      ],
      "more": {
        "text": "Read the full comparison",
        "href": "/comparison"
      }
    },
    "stepsHeading": "Move over safely",
    "steps": [
      {
        "title": "Install on a spare server",
        "body": "One script checks the host, installs Docker if missing and starts the control plane."
      },
      {
        "title": "Dry-run the importer",
        "body": "Point it at your CapRover URL with --dry-run. The importer is read-only against the source."
      },
      {
        "title": "Import and compare",
        "body": "Import a subset with --only, deploy it, and run both for a while before switching DNS."
      }
    ],
    "faq": [
      {
        "q": "Does Levelrail need Docker Swarm?",
        "a": "No. It runs plain Docker containers on each node. Placement across servers is explicit, and cordon and drain are supported."
      },
      {
        "q": "How do I migrate from CapRover?",
        "a": "Use levelrail-cli import platform caprover with your CapRover URL. It is read-only against the source and supports --dry-run and --only."
      },
      {
        "q": "Does it have a one-click app store?",
        "a": "Levelrail ships a curated template catalog and is expanding it, but CapRover's app store is larger today. Check that the services you need are covered."
      },
      {
        "q": "Is it free?",
        "a": "Yes. Apache 2.0, one edition, nothing behind a paywall."
      }
    ],
    "cta": {
      "heading": "Try it beside your CapRover setup",
      "sub": "Import read-only, run both, and keep what works."
    }
  }
}
---
