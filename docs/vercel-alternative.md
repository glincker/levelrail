---
{
  "layout": "landing",
  "title": "Self-hosted Vercel alternative for Next.js and Docker apps",
  "description": "Levelrail is an open source, self-hosted Vercel alternative: git push deploys, automatic TLS, preview environments per pull request, one-click rollback and built-in logs on servers you own.",
  "sidebar": false,
  "aside": false,
  "landing": {
    "eyebrow": "Vercel alternative",
    "headline": "Vercel-style deploys on servers you own",
    "sub": "Push to git and get a running app with HTTPS, preview environments per pull request, logs, metrics and one-click rollback, on your own Linux boxes.",
    "primary": {
      "text": "Get started",
      "link": "/getting-started"
    },
    "secondary": {
      "text": "Migration guide",
      "link": "/migrating-from-vercel"
    },
    "install": "curl -fsSL https://levelrail.com/install.sh | sudo sh",
    "shot": {
      "mock": "deploys",
      "alt": "Levelrail deploy history with one-click rollback"
    },
    "cardsHeading": "The parts of Vercel people miss when they leave",
    "cards": [
      {
        "title": "Preview environments per PR",
        "body": "GitHub, GitLab and Bitbucket webhooks create a preview for each pull request and tear it down automatically when it closes.",
        "visual": [
          {
            "k": "cmd",
            "t": "git push origin feat/pricing-page"
          },
          {
            "k": "ok",
            "t": "preview ready for PR #128"
          }
        ],
        "icon": "pullrequest"
      },
      {
        "title": "One-click rollback",
        "body": "Prior images stay pinned, so going back means switching to an image that is still there, not rebuilding from an old commit.",
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
      },
      {
        "title": "Predictable cost",
        "body": "You pay for the servers you already rent, not per request or per seat. Metrics and logs stay on your own nodes.",
        "visual": [
          {
            "k": "out",
            "t": "1 VPS, any number of apps"
          },
          {
            "k": "ok",
            "t": "no usage meter"
          }
        ],
        "icon": "dollar"
      }
    ],
    "compare": {
      "heading": "Where the trade-offs are",
      "intro": "Vercel is a managed cloud with a global edge network. Levelrail is the other model: your servers, your data, your bill.",
      "left": "Vercel",
      "right": "Levelrail",
      "rows": [
        {
          "label": "Where it runs",
          "left": "A managed cloud you do not control",
          "right": "Linux servers you own, one or many"
        },
        {
          "label": "Deploys",
          "left": "Git integration, previews, instant rollback",
          "right": "Git integration, previews per PR, rollback with pinned images"
        },
        {
          "label": "Observability",
          "left": "See Vercel's own documentation",
          "right": "Metrics and log search built in, node-local"
        },
        {
          "label": "Data",
          "left": "Handled by the vendor",
          "right": "Secrets, metrics and logs stay on your nodes unless you configure an export or drain"
        },
        {
          "label": "Edge network",
          "left": "Global edge network",
          "right": "Not provided: bring your own CDN if you need one"
        }
      ],
      "more": {
        "text": "Read the Vercel migration runbook",
        "href": "/migrating-from-vercel"
      }
    },
    "stepsHeading": "From Vercel to your own server",
    "steps": [
      {
        "title": "Inventory what Vercel holds",
        "body": "Env vars per environment, domains, redirects and cron jobs live in the dashboard, not your repo. The runbook has the checklist."
      },
      {
        "title": "Deploy to Levelrail",
        "body": "Connect the repo, set env vars, and deploy a Dockerfile build. Check it on a temporary domain."
      },
      {
        "title": "Cut DNS over",
        "body": "Lower TTLs first, switch, and keep Vercel as a fallback until you trust the new setup."
      }
    ],
    "faq": [
      {
        "q": "Can Levelrail host Next.js?",
        "a": "Yes, as a container built from your Dockerfile. Build-time variables such as NEXT_PUBLIC_* are inlined at build, which the migration runbook covers."
      },
      {
        "q": "Does it have serverless functions or an edge network?",
        "a": "No. It runs long-lived containers on your servers. If you need a global edge network, put a CDN in front."
      },
      {
        "q": "Is there an importer for Vercel?",
        "a": "No, migration from Vercel is manual. The runbook maps each Vercel concept to its Levelrail equivalent and lists the gaps."
      },
      {
        "q": "What does it cost?",
        "a": "The software is free under Apache 2.0. Your cost is the servers you run it on."
      }
    ],
    "cta": {
      "heading": "Own your deploy pipeline",
      "sub": "Run a side project on it first. Open source, so you can read how every step works."
    }
  }
}
---
