---
{
  "layout": "landing",
  "title": "Self-hosted Heroku alternative: git push to your own servers",
  "description": "Levelrail is an open source Heroku alternative for your own Linux servers: git push deploys, managed Postgres and Redis, TLS, logs, rollback and alerts, with no per-dyno pricing.",
  "sidebar": false,
  "aside": false,
  "landing": {
    "eyebrow": "Heroku alternative",
    "headline": "The Heroku workflow on a server you control",
    "sub": "Git push to deploy, managed databases, automatic HTTPS, logs and rollback, running on Linux boxes you own instead of per-dyno billing.",
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
      "src": "/assets/screenshots/app-overview.png",
      "alt": "Levelrail app overview with live metrics and deploy history"
    },
    "cardsHeading": "What carries over from Heroku",
    "cards": [
      {
        "title": "Git push to deploy",
        "body": "Connect GitHub, GitLab or Bitbucket and every push to a branch builds and deploys with a readiness-gated cutover.",
        "visual": [
          {
            "k": "cmd",
            "t": "git push origin feat/billing"
          },
          {
            "k": "ok",
            "t": "deployed, readiness probe passed"
          }
        ],
        "icon": "branch"
      },
      {
        "title": "Managed databases",
        "body": "Postgres, Redis, MySQL, MongoDB, MariaDB, KeyDB, Dragonfly and ClickHouse as first-class resources, with scheduled backups and automatic verification.",
        "visual": [
          {
            "k": "out",
            "t": "postgres main: backup 03:00"
          },
          {
            "k": "ok",
            "t": "backup verified after each run"
          }
        ],
        "icon": "database"
      },
      {
        "title": "Logs, metrics and alerts",
        "body": "Full-text log search, node-local metrics and alerts to Slack, Discord, email, PagerDuty and more, with no add-on subscriptions.",
        "visual": [
          {
            "k": "out",
            "t": "crashloop detected: web"
          },
          {
            "k": "ok",
            "t": "last 200 log lines attached"
          }
        ],
        "icon": "bell"
      }
    ],
    "compare": {
      "heading": "Heroku and Levelrail",
      "intro": "Heroku is a managed platform. Levelrail is the same workflow on infrastructure you operate.",
      "left": "Heroku",
      "right": "Levelrail",
      "rows": [
        {
          "label": "Billing",
          "left": "Per dyno and per add-on",
          "right": "Your own servers, no per-app fee"
        },
        {
          "label": "Deploys",
          "left": "git push, buildpacks, release phase",
          "right": "git push, Dockerfile builds, readiness-gated cutover"
        },
        {
          "label": "Databases",
          "left": "Add-ons",
          "right": "Eight engines managed in the platform, scheduled and verified backups"
        },
        {
          "label": "Operations",
          "left": "Handled by the vendor",
          "right": "You operate the servers, with alerts and a status page to help"
        }
      ],
      "more": {
        "text": "Read the full comparison",
        "href": "/comparison"
      }
    },
    "stepsHeading": "A first deploy",
    "steps": [
      {
        "title": "Pick a server",
        "body": "Any Linux box with Docker works, or provision one from Hetzner, DigitalOcean, AWS, Azure or GCP."
      },
      {
        "title": "Install and connect git",
        "body": "Run the install script, open the dashboard, and connect a repository."
      },
      {
        "title": "Add a database and deploy",
        "body": "Create Postgres or Redis, link it to the app, and push."
      }
    ],
    "faq": [
      {
        "q": "Do I need to learn Docker?",
        "a": "Not deeply. If your repo has a Dockerfile, it deploys. Levelrail handles the runtime, TLS and routing."
      },
      {
        "q": "What replaces Heroku add-ons?",
        "a": "Managed databases, built-in metrics and logs, alerting channels and scheduled backups cover the common ones. Third-party add-ons are not replicated."
      },
      {
        "q": "Who is on call?",
        "a": "You are, since you run the servers. Alerts, crashloop detection and a status page are built in to make that manageable."
      },
      {
        "q": "Is it free?",
        "a": "Yes, Apache 2.0, one edition."
      }
    ],
    "cta": {
      "heading": "Stop paying per dyno",
      "sub": "Try it on one small server and move a project across when it feels right."
    }
  }
}
---
