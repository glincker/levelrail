---
{
  "layout": "landing",
  "title": "Levelrail demo: install, deploy and roll back in minutes",
  "description": "See Levelrail in action: install script, git push deploys, live logs, metrics with deploy markers, nodes, databases and one-click rollback, with real screenshots from the dashboard.",
  "sidebar": false,
  "aside": false,
  "landing": {
    "eyebrow": "Demo",
    "headline": "See a deploy from install to rollback",
    "sub": "A walkthrough of the dashboard and the CLI, using real screenshots. No sign-up, nothing to host on our side: try the same steps on your own server.",
    "primary": {
      "text": "Run it yourself",
      "link": "/getting-started"
    },
    "secondary": {
      "text": "View on GitHub",
      "link": "https://github.com/glincker/levelrail"
    },
    "install": "curl -fsSL https://levelrail.com/install.sh | sudo sh",
    "shot": {
      "src": "/assets/screenshots/dashboard-home.png",
      "alt": "Levelrail dashboard home with apps, nodes and recent deploys"
    },
    "terminal": {
      "heading": "Install and first deploy",
      "intro": "The install script checks the host, installs Docker if it is missing and starts the control plane as a systemd service. Then you deploy.",
      "title": "install, deploy",
      "ariaLabel": "Terminal recording of installing Levelrail and deploying an app that goes live over HTTPS",
      "lines": [
        {
          "kind": "command",
          "text": "curl -fsSL https://levelrail.com/install.sh | sudo sh"
        },
        {
          "kind": "output",
          "text": "checking host: docker, systemd, ports 80/443/8080 free"
        },
        {
          "kind": "output",
          "text": "installing control plane as a systemd service"
        },
        {
          "kind": "success",
          "text": "dashboard ready, setup token printed above"
        },
        {
          "kind": "command",
          "text": "levelrail-cli deploy myapp --image registry.example.com/acme/myapp:latest"
        },
        {
          "kind": "output",
          "text": "readiness probe passed, cutting traffic"
        },
        {
          "kind": "success",
          "text": "myapp is live over HTTPS"
        }
      ]
    },
    "galleryHeading": "A tour of the dashboard",
    "gallery": [
      {
        "src": "/assets/screenshots/apps-list.png",
        "alt": "Levelrail apps list",
        "caption": "Every app across every node, with status at a glance."
      },
      {
        "src": "/assets/screenshots/deploy-history.png",
        "alt": "Levelrail deploy history",
        "caption": "Deploy history with one-click rollback to a pinned image."
      },
      {
        "src": "/assets/screenshots/logs.png",
        "alt": "Levelrail live log viewer",
        "caption": "Live log tail with full-text search."
      },
      {
        "src": "/assets/screenshots/app-overview.png",
        "alt": "Levelrail app overview with metrics",
        "caption": "Metrics with deploy markers overlaid on the charts."
      },
      {
        "src": "/assets/screenshots/nodes.png",
        "alt": "Levelrail nodes list",
        "caption": "Nodes, their health and what is placed on them."
      },
      {
        "src": "/assets/screenshots/database-overview.png",
        "alt": "Levelrail database overview",
        "caption": "Managed databases with backups and verification."
      }
    ],
    "stepsHeading": "What to try first",
    "steps": [
      {
        "title": "Install and open the dashboard",
        "body": "Run the install script, then sign in with the setup token it prints."
      },
      {
        "title": "Deploy something small",
        "body": "Connect a repo or point at an image and watch the build and the readiness gate."
      },
      {
        "title": "Break it on purpose",
        "body": "Deploy a failing version, see the crashloop detection with the last log lines, then roll back."
      }
    ],
    "faq": [
      {
        "q": "Is there a hosted demo?",
        "a": "No. Levelrail is self-hosted, so the quickest demo is the install script on any Linux box. The screenshots here are from a real dashboard."
      },
      {
        "q": "How long does the install take?",
        "a": "The install script handles the host check, Docker and the systemd service in one run. Most of the time is Docker pulling images on a fresh server."
      },
      {
        "q": "Can I try it without touching my current setup?",
        "a": "Yes. Install on a spare server. If you use the importer, it is read-only against your existing platform."
      }
    ],
    "cta": {
      "heading": "Run the demo on your own server",
      "sub": "Ten minutes, one small VPS, nothing to sign up for."
    }
  }
}
---
