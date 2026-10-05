---
{
  "layout": "landing",
  "title": "Self-host Next.js on your own server with git push, TLS and rollback",
  "description": "How to self-host a Next.js app with Levelrail: Dockerfile build, automatic HTTPS, preview environments per pull request, env vars, a database and one-click rollback.",
  "sidebar": false,
  "aside": false,
  "landing": {
    "eyebrow": "Self-host Next.js",
    "headline": "Run Next.js on a server you own, without the glue scripts",
    "sub": "Build from your Dockerfile, get HTTPS, preview environments per pull request, a managed database and one-click rollback, all from a git push.",
    "primary": {
      "text": "Get started",
      "link": "/getting-started"
    },
    "secondary": {
      "text": "Vercel migration runbook",
      "link": "/migrating-from-vercel"
    },
    "install": "curl -fsSL https://levelrail.com/install.sh | sudo sh",
    "shot": {
      "mock": "overview",
      "alt": "Illustrative rendering of a Levelrail app overview with metrics"
    },
    "cardsHeading": "What you get for a Next.js app",
    "cards": [
      {
        "title": "Build and deploy",
        "body": "Connect GitHub, GitLab, Bitbucket or Gitea. Pushes build a Dockerfile with BuildKit and cut traffic over only when the readiness probe passes.",
        "visual": [
          {
            "k": "cmd",
            "t": "git push origin feat/checkout"
          },
          {
            "k": "ok",
            "t": "deployed, readiness probe passed"
          }
        ]
      },
      {
        "title": "Environment and build-time vars",
        "body": "Env vars are encrypted per app. NEXT_PUBLIC_* values are inlined at build time, so set them before the build, which the migration runbook covers.",
        "visual": [
          {
            "k": "out",
            "t": "NEXT_PUBLIC_API_URL set at build"
          },
          {
            "k": "ok",
            "t": "DATABASE_URL injected at run"
          }
        ]
      },
      {
        "title": "Database next to the app",
        "body": "Create Postgres or Redis as a managed resource with scheduled, verified backups and link it to the app.",
        "visual": [
          {
            "k": "out",
            "t": "postgres main created"
          },
          {
            "k": "ok",
            "t": "backup verified"
          }
        ]
      }
    ],
    "stepsHeading": "From repo to HTTPS",
    "steps": [
      {
        "title": "Add a Dockerfile",
        "body": "A standard multi-stage Next.js Dockerfile with output set to standalone keeps the image small."
      },
      {
        "title": "Connect the repo and set env vars",
        "body": "Set build-time variables first, then runtime secrets."
      },
      {
        "title": "Add your domain",
        "body": "Point DNS at the server and Caddy issues and renews the certificate."
      },
      {
        "title": "Open a pull request",
        "body": "A preview environment deploys and is torn down when the pull request closes."
      }
    ],
    "compare": {
      "heading": "Self-hosting Next.js, honestly",
      "intro": "What changes when you leave a managed platform.",
      "left": "Managed platform",
      "right": "Levelrail on your server",
      "rows": [
        {
          "label": "Cost model",
          "left": "Usage and seat based",
          "right": "Your server cost"
        },
        {
          "label": "Edge network",
          "left": "Included",
          "right": "Not provided: put a CDN in front"
        },
        {
          "label": "Serverless functions",
          "left": "Included",
          "right": "Not provided: long-lived containers"
        },
        {
          "label": "Previews and rollback",
          "left": "Included",
          "right": "Included"
        }
      ]
    },
    "faq": [
      {
        "q": "Does it support the App Router and server components?",
        "a": "It runs your container, so anything that runs in a Node container works. Levelrail does not run a separate Next.js-specific runtime."
      },
      {
        "q": "How do preview environments work?",
        "a": "A pull request opened or updated deploys a preview. Closing the pull request tears it down, whether or not it merged."
      },
      {
        "q": "Can I run it on one small VPS?",
        "a": "Yes. A single node is the best tested path."
      },
      {
        "q": "Is there a migration guide?",
        "a": "Yes, a runbook for moving a Vercel-hosted Next.js app, with an inventory, a cutover plan and the gaps."
      }
    ],
    "cta": {
      "heading": "Deploy your Next.js app to your own server",
      "sub": "Start with a side project and move the rest when you trust it."
    }
  }
}
---
