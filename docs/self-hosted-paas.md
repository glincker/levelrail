---
{
  "layout": "landing",
  "title": "Self-hosted PaaS guide: what to look for and how Levelrail fits",
  "description": "A practical guide to choosing a self-hosted PaaS: how it manages servers, how it deploys, rollback, observability, security and cost, and how Levelrail answers each.",
  "sidebar": false,
  "aside": false,
  "landing": {
    "eyebrow": "Self-hosted PaaS guide",
    "headline": "What to look for in a self-hosted PaaS",
    "sub": "Running your own Heroku-style platform is a good trade when the platform stays out of the way. These are the questions that decide whether it does, and how Levelrail answers them.",
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
      "alt": "Illustrative rendering of the Levelrail apps list"
    },
    "cardsHeading": "The six questions",
    "cards": [
      {
        "title": "How does it reach my servers?",
        "body": "SSH and CLI shelling work, but they poll and parse text. Levelrail's agent dials out over mutual TLS and uses the Docker Engine API, with no inbound port.",
        "visual": [
          {
            "k": "out",
            "t": "agent dials out (mTLS)"
          },
          {
            "k": "ok",
            "t": "0 inbound ports"
          }
        ]
      },
      {
        "title": "What happens when a deploy fails?",
        "body": "Traffic should cut over only after a real readiness probe passes, and the previous release should stay available. Levelrail pins prior images so cleanup cannot remove a rollback target.",
        "visual": [
          {
            "k": "out",
            "t": "readiness probe failed"
          },
          {
            "k": "ok",
            "t": "traffic stayed on web:41"
          }
        ]
      },
      {
        "title": "Can I see what is happening?",
        "body": "Metrics, logs and alerts should not need a second stack. Levelrail keeps node-local metrics and log search in the core.",
        "visual": [
          {
            "k": "out",
            "t": "p95 up after deploy 7f3a"
          },
          {
            "k": "ok",
            "t": "deploy marker on chart"
          }
        ]
      },
      {
        "title": "Where do secrets and data live?",
        "body": "Per-app keys wrapped by a master key held only by the control plane, and metrics and logs stay on your nodes.",
        "link": {
          "text": "Privacy and data handling",
          "href": "/privacy"
        },
        "visual": [
          {
            "k": "out",
            "t": "secret: encrypted per app"
          },
          {
            "k": "ok",
            "t": "agent does not persist it"
          }
        ]
      },
      {
        "title": "What is the real cost?",
        "body": "Your servers plus your time. Levelrail is Apache 2.0 with one edition and no paywalled features.",
        "link": {
          "text": "Pricing",
          "href": "/pricing"
        },
        "visual": [
          {
            "k": "out",
            "t": "license  Apache 2.0"
          },
          {
            "k": "ok",
            "t": "no feature behind a plan"
          }
        ]
      },
      {
        "title": "How do I leave?",
        "body": "Importers and plain Docker underneath keep exit costs low. Levelrail can import from Coolify, Dokploy and CapRover read-only.",
        "link": {
          "text": "Migration guide",
          "href": "/migrating-from-coolify-dokploy-and-caprover"
        },
        "visual": [
          {
            "k": "cmd",
            "t": "levelrail-cli import platform coolify --dry-run"
          }
        ]
      }
    ],
    "compare": {
      "heading": "Where Levelrail sits",
      "intro": "Positioning, not a ranking. Every project below is useful, and the full comparison page is candid about where each is strong.",
      "left": "Typical SSH-driven platform",
      "right": "Levelrail",
      "rows": [
        {
          "label": "Server management",
          "left": "SSH and docker CLI commands",
          "right": "Agent over mTLS, Docker Engine API"
        },
        {
          "label": "State",
          "left": "Imperative scripts and polling",
          "right": "Desired state converged by a reconciler"
        },
        {
          "label": "Observability",
          "left": "Install Grafana or similar",
          "right": "Metrics and logs built in"
        },
        {
          "label": "Footprint",
          "left": "A web app plus its own database",
          "right": "One Go binary with embedded SQLite"
        }
      ],
      "more": {
        "text": "Read the full comparison",
        "href": "/comparison"
      }
    },
    "faq": [
      {
        "q": "What is a self-hosted PaaS?",
        "a": "Software you run on your own servers that gives you a platform-as-a-service workflow: push code, get a running app with TLS, logs and rollback."
      },
      {
        "q": "Who is Levelrail for?",
        "a": "Operators running roughly 3 to 50 services on 1 to 10 Linux machines who do not want to run Kubernetes."
      },
      {
        "q": "Is it a Kubernetes alternative?",
        "a": "No. There is no bin-packing scheduler, autoscaling or CRD ecosystem. It borrows the reconciler pattern without the runtime."
      },
      {
        "q": "How mature is it?",
        "a": "Pre-release and moving fast. The case studies page lists what has been measured and what has not."
      }
    ],
    "cta": {
      "heading": "Evaluate it on one server",
      "sub": "Install, import read-only, and judge it on your own workload."
    }
  }
}
---
