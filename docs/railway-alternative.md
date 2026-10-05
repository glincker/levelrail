---
{
  "layout": "landing",
  "title": "Self-hosted Railway alternative with built-in observability",
  "description": "Levelrail is an open source Railway alternative you run on your own servers: git push deploys, managed databases, domains with TLS, preview environments and built-in metrics and logs.",
  "sidebar": false,
  "aside": false,
  "landing": {
    "eyebrow": "Railway alternative",
    "headline": "Railway-style simplicity on your own infrastructure",
    "sub": "Connect a repo, get a running service with a domain, TLS, a database and logs. The difference is that it runs on servers you own and the usage meter is yours.",
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
      "alt": "Levelrail apps list across nodes"
    },
    "cardsHeading": "What you get",
    "cards": [
      {
        "title": "Services, domains and TLS",
        "body": "Add a domain and Caddy issues and renews the certificate. Routing is handled in the control plane, with no separate proxy to run.",
        "visual": [
          {
            "k": "out",
            "t": "domain app.example.com added"
          },
          {
            "k": "ok",
            "t": "certificate issued"
          }
        ],
        "icon": "globe"
      },
      {
        "title": "Databases next to your app",
        "body": "Create a managed database, link it to a service, and connect over the internal network. Backups are scheduled and checked.",
        "visual": [
          {
            "k": "out",
            "t": "postgres main created"
          },
          {
            "k": "ok",
            "t": "DATABASE_URL set on web"
          }
        ],
        "icon": "database"
      },
      {
        "title": "Observability without an add-on",
        "body": "Metrics, log search and deploy markers overlaid on charts answer which deploy changed things without installing Grafana.",
        "visual": [
          {
            "k": "out",
            "t": "p95 latency up after deploy 7f3a"
          },
          {
            "k": "ok",
            "t": "deploy marker on chart"
          }
        ],
        "icon": "chart"
      }
    ],
    "compare": {
      "heading": "Railway and Levelrail",
      "intro": "Railway is a managed cloud. Levelrail is software you run.",
      "left": "Railway",
      "right": "Levelrail",
      "rows": [
        {
          "label": "Where it runs",
          "left": "Vendor-managed cloud",
          "right": "Your Linux servers, one to many"
        },
        {
          "label": "Pricing model",
          "left": "Usage-based",
          "right": "Your own server cost, nothing per service"
        },
        {
          "label": "Previews",
          "left": "Environments per PR",
          "right": "Preview environments per pull request with automatic teardown"
        },
        {
          "label": "Data location",
          "left": "With the vendor",
          "right": "On your nodes: secrets, metrics and logs stay local"
        }
      ],
      "more": {
        "text": "Read the full comparison",
        "href": "/comparison"
      }
    },
    "stepsHeading": "Running in three steps",
    "steps": [
      {
        "title": "Install",
        "body": "One script on a Linux server."
      },
      {
        "title": "Connect your repo",
        "body": "GitHub, GitLab or Bitbucket."
      },
      {
        "title": "Add a domain",
        "body": "Point DNS and the certificate follows."
      }
    ],
    "faq": [
      {
        "q": "Can I scale across several servers?",
        "a": "Yes. Extra servers join with a one-time token and a small agent. Placement is explicit, and cordon and drain are supported."
      },
      {
        "q": "Is there a managed cloud version?",
        "a": "No. Levelrail is self-hosted software only today."
      },
      {
        "q": "Can I import from Railway?",
        "a": "There is no Railway importer. You deploy from the same git repos and recreate env vars and domains."
      },
      {
        "q": "Is it open source?",
        "a": "Yes, Apache 2.0."
      }
    ],
    "cta": {
      "heading": "Run your own Railway",
      "sub": "Start with one small server and see how little it asks of it."
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
