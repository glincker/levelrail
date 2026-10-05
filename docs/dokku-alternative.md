---
{
  "layout": "landing",
  "title": "Dokku alternative: git push deploys with a dashboard and multi-server",
  "description": "Levelrail is an open source Dokku alternative that adds a dashboard, deploy history with rollback, built-in metrics and logs, and extra servers through an agent.",
  "sidebar": false,
  "aside": false,
  "landing": {
    "eyebrow": "Dokku alternative",
    "headline": "When one server and git push stops being enough",
    "sub": "Dokku is a classic for a single host. Levelrail keeps the git push workflow and adds a dashboard, deploy history with one-click rollback, built-in observability and a way to add more servers.",
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
      "mock": "deploys",
      "alt": "Illustrative rendering of the Levelrail deploy history with a rollback button"
    },
    "cardsHeading": "What you gain on top of git push",
    "cards": [
      {
        "title": "A dashboard and history",
        "body": "See every deploy, its logs and its status in one place, and roll back to a pinned previous image with one click.",
        "visual": [
          {
            "k": "cmd",
            "t": "levelrail-cli rollback web --image web:41"
          },
          {
            "k": "ok",
            "t": "web is live on web:41"
          }
        ]
      },
      {
        "title": "Metrics and logs built in",
        "body": "Node-local metrics at 15 second resolution and full-text log search, with deploy markers on the charts.",
        "visual": [
          {
            "k": "cmd",
            "t": "levelrail-cli apps logs web"
          },
          {
            "k": "out",
            "t": "12:04:11 GET /healthz 200"
          }
        ]
      },
      {
        "title": "Room to grow",
        "body": "Add servers with a one-time join token and a small agent that dials out. Cordon, drain and explicit placement are supported.",
        "visual": [
          {
            "k": "cmd",
            "t": "levelrail-cli nodes list"
          },
          {
            "k": "out",
            "t": "node1  online"
          },
          {
            "k": "ok",
            "t": "node2  online"
          }
        ]
      }
    ],
    "compare": {
      "heading": "Dokku and Levelrail, side by side",
      "intro": "Dokku has almost no idle footprint and is hard to beat for simplicity on one server. These are the design differences.",
      "left": "Dokku",
      "right": "Levelrail",
      "rows": [
        {
          "label": "How it manages servers",
          "left": "Commands run on the host itself, driven over SSH or git push",
          "right": "A control plane with an agent per node that dials out over mTLS"
        },
        {
          "label": "Orchestration",
          "left": "A plugin-based scheduler over Docker on one host",
          "right": "Level-triggered reconciler with recorded status conditions"
        },
        {
          "label": "Ingress",
          "left": "nginx by default, other proxies via plugins",
          "right": "Caddy embedded in the control plane"
        },
        {
          "label": "Runs as",
          "left": "A set of scripts on the host, no separate web service",
          "right": "One Go binary with an embedded dashboard"
        },
        {
          "label": "Observability",
          "left": "Add your own stack",
          "right": "Metrics and log search in the core"
        }
      ],
      "more": {
        "text": "Read the full comparison",
        "href": "/comparison"
      }
    },
    "steps": [
      {
        "title": "Install on a Linux box",
        "body": "The install script handles Docker and the systemd service."
      },
      {
        "title": "Connect a git repo",
        "body": "GitHub, GitLab, Bitbucket and Gitea webhooks trigger deploys."
      },
      {
        "title": "Deploy and roll back",
        "body": "Traffic cuts over only after the readiness probe passes, and previous images stay pinned."
      }
    ],
    "stepsHeading": "Getting started",
    "faq": [
      {
        "q": "Is there an importer for Dokku?",
        "a": "No. You deploy the same repositories and recreate environment variables and domains. The importer covers Coolify, Dokploy and CapRover."
      },
      {
        "q": "Does Levelrail use buildpacks?",
        "a": "Levelrail builds Dockerfiles with BuildKit. Check the docs for the current state of automatic build detection."
      },
      {
        "q": "Do I need more than one server?",
        "a": "No. Single node is the best tested path, and multi-server is there when you need it."
      },
      {
        "q": "Is it free?",
        "a": "Yes. Apache 2.0, one edition."
      }
    ],
    "cta": {
      "heading": "Add a dashboard to your deploys",
      "sub": "Install it beside your current setup and compare."
    }
  }
}
---
